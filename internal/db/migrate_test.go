package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const latestVersion = 4

// preAuthDB builds a database the way the pre-auth schema.sql left it: no
// user_version, decks without an owner, global settings. withData adds one
// deck with a card, a study session and an LLM setting.
func preAuthDB(t *testing.T, withData bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	body, err := migrationFiles.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, raw, string(body))
	if !withData {
		return path
	}
	mustExec(t, raw, `
		INSERT INTO decks (id, name, mode) VALUES (7, 'Spanish', 'srs');
		INSERT INTO cards (deck_id, front, back) VALUES (7, 'hola', 'hello');
		INSERT INTO study_sessions (deck_id, quiz_mode, card_queue) VALUES (7, 'flashcard', '[1]');
		INSERT INTO settings (key, value) VALUES ('llm.model', 'qwen');
	`)
	return path
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func count(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func userVersion(t *testing.T, database *sql.DB) int {
	t.Helper()
	return count(t, database, "PRAGMA user_version")
}

func openOrFail(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// noOwnerEnv blanks the owner variables so a value in the developer's shell
// cannot make a test pass.
func noOwnerEnv(t *testing.T) {
	t.Setenv("MIGRATE_OWNER_EMAIL", "")
	t.Setenv("MIGRATE_OWNER_PASSWORD", "")
}

func TestOpenFreshDB(t *testing.T) {
	noOwnerEnv(t)
	path := filepath.Join(t.TempDir(), "new.db")
	database := openOrFail(t, path)

	if got := userVersion(t, database); got != latestVersion {
		t.Fatalf("user_version = %d, want %d", got, latestVersion)
	}
	if count(t, database, "SELECT COUNT(*) FROM pragma_table_info('decks') WHERE name = 'user_id'") != 1 {
		t.Fatal("decks.user_id missing")
	}
	database.Close()

	// Reopening an up-to-date database applies nothing.
	again := openOrFail(t, path)
	if got := userVersion(t, again); got != latestVersion {
		t.Fatalf("user_version after reopen = %d, want %d", got, latestVersion)
	}
}

// Every pooled connection must enforce foreign keys, not just the first one.
func TestOpenForeignKeysOnEveryConnection(t *testing.T) {
	noOwnerEnv(t)
	database := openOrFail(t, filepath.Join(t.TempDir(), "fk.db"))
	ctx := context.Background()

	var conns []*sql.Conn
	for range 3 {
		conn, err := database.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conns = append(conns, conn)
	}

	for i, conn := range conns {
		var on int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
			t.Fatal(err)
		}
		if on != 1 {
			t.Errorf("connection %d: foreign_keys = %d, want 1", i, on)
		}
	}
}

func TestOpenMigratesPreAuthDecksToOwner(t *testing.T) {
	path := preAuthDB(t, true)
	t.Setenv("MIGRATE_OWNER_EMAIL", " Owner@Example.com ")
	t.Setenv("MIGRATE_OWNER_PASSWORD", "correct horse")

	database := openOrFail(t, path)

	if got := userVersion(t, database); got != latestVersion {
		t.Fatalf("user_version = %d, want %d", got, latestVersion)
	}

	var ownerID int64
	var hash string
	err := database.QueryRow("SELECT id, password_hash FROM users WHERE email = 'owner@example.com'").Scan(&ownerID, &hash)
	if err != nil {
		t.Fatalf("owner not created: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse")) != nil {
		t.Error("owner password does not verify")
	}

	if count(t, database, "SELECT COUNT(*) FROM decks WHERE id = 7 AND user_id = ?", ownerID) != 1 {
		t.Error("deck 7 not assigned to owner")
	}
	// Rebuilding decks must not cascade-delete what hangs off it.
	if count(t, database, "SELECT COUNT(*) FROM cards WHERE deck_id = 7") != 1 {
		t.Error("card lost in decks rebuild")
	}
	if count(t, database, "SELECT COUNT(*) FROM study_sessions WHERE deck_id = 7") != 1 {
		t.Error("study session lost in decks rebuild")
	}
	if count(t, database, "SELECT COUNT(*) FROM settings WHERE user_id = ? AND key = 'llm.model'", ownerID) != 1 {
		t.Error("setting not assigned to owner")
	}

	// The rebuilt table still cascades.
	mustExec(t, database, "DELETE FROM decks WHERE id = 7")
	if count(t, database, "SELECT COUNT(*) FROM cards") != 0 {
		t.Error("deleting a deck no longer cascades to its cards")
	}
}

func TestOpenPreAuthDecksNeedOwner(t *testing.T) {
	path := preAuthDB(t, true)
	noOwnerEnv(t)

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "MIGRATE_OWNER_EMAIL") {
		t.Fatalf("Open error = %v, want one naming MIGRATE_OWNER_EMAIL", err)
	}

	// The failed migration rolled back alone: earlier ones stay applied and
	// the data is untouched, so a retry with the variables set succeeds.
	t.Setenv("MIGRATE_OWNER_EMAIL", "owner@example.com")
	t.Setenv("MIGRATE_OWNER_PASSWORD", "correct horse")
	database := openOrFail(t, path)
	if got := userVersion(t, database); got != latestVersion {
		t.Fatalf("user_version after retry = %d, want %d", got, latestVersion)
	}
	if count(t, database, "SELECT COUNT(*) FROM cards") != 1 {
		t.Error("card lost across failed then retried migration")
	}
}

func TestOpenRejectsShortOwnerPassword(t *testing.T) {
	path := preAuthDB(t, true)
	t.Setenv("MIGRATE_OWNER_EMAIL", "owner@example.com")
	t.Setenv("MIGRATE_OWNER_PASSWORD", "short")

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("Open error = %v, want password length error", err)
	}
}

func TestOpenPreAuthWithoutDecksNeedsNoOwner(t *testing.T) {
	path := preAuthDB(t, false)
	noOwnerEnv(t)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, raw, "INSERT INTO settings (key, value) VALUES ('llm.model', 'qwen')")
	raw.Close()

	database := openOrFail(t, path)

	if got := userVersion(t, database); got != latestVersion {
		t.Fatalf("user_version = %d, want %d", got, latestVersion)
	}
	if n := count(t, database, "SELECT COUNT(*) FROM users"); n != 0 {
		t.Errorf("users = %d, want 0: no owner should be created without decks", n)
	}
	if n := count(t, database, "SELECT COUNT(*) FROM settings"); n != 0 {
		t.Errorf("settings = %d, want 0: ownerless settings are dropped", n)
	}
}

// A database created by the post-auth schema.sql, before migrations existed,
// reports user_version 0 but is already current.
func TestOpenBaselinesPostAuthDB(t *testing.T) {
	noOwnerEnv(t)
	path := filepath.Join(t.TempDir(), "postauth.db")

	database := openOrFail(t, path)
	mustExec(t, database, "INSERT INTO users (id, email) VALUES (1, 'a@example.com')")
	mustExec(t, database, "INSERT INTO decks (id, user_id, name) VALUES (1, 1, 'French')")
	// Wind back to what the post-auth schema.sql produced: no later columns,
	// no recorded version.
	mustExec(t, database, "ALTER TABLE users DROP COLUMN display_name")
	mustExec(t, database, "PRAGMA user_version = 0")
	database.Close()

	again := openOrFail(t, path)
	if got := userVersion(t, again); got != latestVersion {
		t.Fatalf("user_version = %d, want %d", got, latestVersion)
	}
	if count(t, again, "SELECT COUNT(*) FROM decks WHERE user_id = 1") != 1 {
		t.Error("existing deck changed by baselining")
	}
	// Baselined at 3, so the migrations after it still ran.
	if count(t, again, "SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'display_name'") != 1 {
		t.Error("users.display_name missing: migrations after the baseline did not run")
	}
}
