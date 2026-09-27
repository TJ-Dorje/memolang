package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migration is one schema version.
type migration struct {
	version int
	file    string
}

// migrations must stay in version order. Never edit one that has shipped:
// add a new file instead.
var migrations = []migration{
	{version: 1, file: "0001_baseline.sql"},
}

// migrationLockKey names the Postgres advisory lock held while migrating.
// Two pods starting together would otherwise both apply the same migration;
// with the lock the second waits, then finds nothing left to do.
const migrationLockKey int64 = 0x6d656d6f6c616e67 // "memolang"

// migrate brings the database up to the latest version.
func migrate(ctx context.Context, database *sql.DB) error {
	return migrateTo(ctx, database, migrations[len(migrations)-1].version)
}

// migrateTo applies migrations up to and including target. Only tests stop
// short of the latest.
//
// Postgres runs DDL inside transactions, so each migration is simply one
// transaction: it applies completely, recorded in schema_migrations, or not
// at all. The advisory lock is per connection, so one connection is pinned
// for the whole run.
func migrateTo(ctx context.Context, database *sql.DB, target int) error {
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("migrate: lock: %w", err)
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	var current int
	if err := conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return fmt.Errorf("migrate: read version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current || m.version > target {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.file, err)
		}
		log.Printf("migration: applied %s", m.file)
	}
	return nil
}

func apply(ctx context.Context, conn *sql.Conn, m migration) error {
	body, err := migrationFiles.ReadFile("migrations/" + m.file)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
		return err
	}
	return tx.Commit()
}
