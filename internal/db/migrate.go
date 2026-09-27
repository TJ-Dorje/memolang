package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migration is one schema version. prepare, when set, runs in the same
// transaction just before the SQL file, for the steps SQL cannot express.
type migration struct {
	version int
	file    string
	prepare func(ctx context.Context, tx *sql.Tx) error
}

// migrations must stay in version order. Never edit one that has shipped:
// add a new file instead.
var migrations = []migration{
	{version: 1, file: "0001_initial.sql"},
	{version: 2, file: "0002_users.sql"},
	{version: 3, file: "0003_deck_owners.sql", prepare: claimOrphanDecks},
	{version: 4, file: "0004_display_name.sql"},
	{version: 5, file: "0005_tutor.sql"},
	{version: 6, file: "0006_conversations.sql"},
	{version: 7, file: "0007_llm_providers.sql"},
}

// authVersion is where a database created by the post-auth schema.sql, before
// migrations existed, already stands.
const authVersion = 3

// minOwnerPasswordLen matches the register form's rule.
const minOwnerPasswordLen = 8

// migrate brings the database up to the latest version, recorded in the
// file header's PRAGMA user_version. It pins one connection because the
// foreign_keys pragma toggled around each migration is per-connection.
func migrate(ctx context.Context, database *sql.DB) error {
	return migrateTo(ctx, database, migrations[len(migrations)-1].version)
}

// migrateTo applies migrations up to and including target. Only tests stop
// short of the latest, to build databases as an older release left them.
func migrateTo(ctx context.Context, database *sql.DB, target int) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer conn.Close()

	version, err := schemaVersion(ctx, conn)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	for _, m := range migrations {
		if m.version <= version || m.version > target {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.file, err)
		}
		log.Printf("migration: applied %s", m.file)
	}
	return nil
}

// schemaVersion reads user_version. Databases from before migrations existed
// all report 0: those whose decks already have an owner ran the full
// post-auth schema on every start, so they are at authVersion and get it
// recorded; anything else, empty or pre-auth, starts from 0 (see
// 0001_initial.sql for why that is safe).
func schemaVersion(ctx context.Context, conn *sql.Conn) (int, error) {
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	if version > 0 {
		return version, nil
	}

	var owned int
	err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('decks') WHERE name = 'user_id'",
	).Scan(&owned)
	if err != nil {
		return 0, fmt.Errorf("inspect decks schema: %w", err)
	}
	if owned == 0 {
		return 0, nil
	}

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", authVersion)); err != nil {
		return 0, fmt.Errorf("record baseline version: %w", err)
	}
	log.Printf("migration: existing database recorded at version %d", authVersion)
	return authVersion, nil
}

// apply runs one migration in its own transaction, so a failure leaves the
// database at the previous version and the next start retries it.
//
// Table rebuilds drop a table other tables reference; with foreign keys on,
// that DROP cascades and deletes their rows. The pragma is ignored inside a
// transaction, so it is switched off around it and violations are checked
// explicitly before commit instead.
func apply(ctx context.Context, conn *sql.Conn, m migration) error {
	body, err := migrationFiles.ReadFile("migrations/" + m.file)
	if err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.prepare != nil {
		if err := m.prepare(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if err := checkForeignKeys(ctx, tx); err != nil {
		return err
	}
	// user_version lives in the file header, so this write commits or rolls
	// back together with the migration.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("re-enable foreign keys: %w", err)
	}
	return nil
}

func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return rows.Err()
	}
	var (
		table  string
		rowid  sql.NullInt64
		parent string
		fkid   int
	)
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	return fmt.Errorf("foreign key violation: %s rowid %d references missing %s row", table, rowid.Int64, parent)
}

// claimOrphanDecks decides who owns the decks created before accounts
// existed, and hands the answer to 0003_deck_owners.sql through a temp table.
// With no decks there is nothing to own and the table stays empty: pre-auth
// settings (LLM keys, re-enterable in the UI) are then dropped, not guessed at.
// With decks, the operator names the owner via MIGRATE_OWNER_EMAIL and
// MIGRATE_OWNER_PASSWORD; both are ignored once the migration has run.
func claimOrphanDecks(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE migration_owner (id INTEGER NOT NULL)"); err != nil {
		return err
	}

	var decks, settings int
	err := tx.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM decks), (SELECT COUNT(*) FROM settings)",
	).Scan(&decks, &settings)
	if err != nil {
		return err
	}
	if decks == 0 {
		if settings > 0 {
			log.Printf("migration: no decks to own; dropping %d pre-auth settings", settings)
		}
		return nil
	}

	// Lowercased to match models.NormalizeEmail, which the login lookup uses.
	email := strings.ToLower(strings.TrimSpace(os.Getenv("MIGRATE_OWNER_EMAIL")))
	password := os.Getenv("MIGRATE_OWNER_PASSWORD")
	if email == "" || password == "" {
		return fmt.Errorf("%d existing decks have no owner: set MIGRATE_OWNER_EMAIL and MIGRATE_OWNER_PASSWORD to create the account that will own them", decks)
	}
	if len(password) < minOwnerPasswordLen {
		return fmt.Errorf("MIGRATE_OWNER_PASSWORD must be at least %d characters", minOwnerPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	var ownerID int64
	err = tx.QueryRowContext(ctx,
		"INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id", email, string(hash),
	).Scan(&ownerID)
	if err != nil {
		return fmt.Errorf("create owner %s: %w", email, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO migration_owner (id) VALUES (?)", ownerID); err != nil {
		return err
	}

	log.Printf("migration: %d decks and %d settings assigned to %s", decks, settings, email)
	return nil
}
