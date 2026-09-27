package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const latestVersion = 8

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
		INSERT INTO settings (key, value) VALUES ('llm.provider', 'lmstudio'), ('llm.model', 'qwen');
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
	// The pre-auth LLM settings went to the owner (0003), then became their
	// active provider (0007).
	if count(t, database, "SELECT COUNT(*) FROM llm_providers WHERE user_id = ? AND preset = 'lmstudio' AND model = 'qwen' AND active = 1", ownerID) != 1 {
		t.Error("pre-auth LLM settings did not become the owner's active provider")
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

// 0006 folds the tutor tables into conversations; existing threads and their
// messages must survive with their ids.
func TestConversationsMigrationKeepsTutorThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(context.Background(), database, 5); err != nil {
		t.Fatalf("migrateTo(5): %v", err)
	}
	mustExec(t, database, `
		INSERT INTO users (id, email) VALUES (1, 'a@example.com');
		INSERT INTO decks (id, user_id, name) VALUES (1, 1, 'D');
		INSERT INTO cards (id, deck_id, front, back) VALUES (9, 1, 'hola', 'hello');
		INSERT INTO tutor_threads (id, user_id, card_id) VALUES (3, 1, 9);
		INSERT INTO tutor_messages (id, thread_id, role, content, status) VALUES
			(7, 3, 'user', 'why?', 'done'),
			(8, 3, 'assistant', 'because', 'done');
	`)
	database.Close()

	noOwnerEnv(t)
	migrated := openOrFail(t, path)

	if count(t, migrated, "SELECT COUNT(*) FROM conversations WHERE id = 3 AND user_id = 1 AND kind = 'tutor' AND card_id = 9") != 1 {
		t.Error("tutor thread not carried over as conversation 3")
	}
	if count(t, migrated, "SELECT COUNT(*) FROM conversation_messages WHERE conversation_id = 3 AND id IN (7, 8)") != 2 {
		t.Error("tutor messages not carried over with their ids")
	}
	if count(t, migrated, "SELECT COUNT(*) FROM sqlite_master WHERE name IN ('tutor_threads', 'tutor_messages')") != 0 {
		t.Error("old tutor tables still present")
	}
	// Deleting the card still cascades through the new tables.
	mustExec(t, migrated, "DELETE FROM cards WHERE id = 9")
	if count(t, migrated, "SELECT COUNT(*) FROM conversation_messages") != 0 {
		t.Error("deleting the card left its conversation messages behind")
	}
}

// A database created by the post-auth schema.sql, before migrations existed,
// reports user_version 0 but is already current.
func TestOpenBaselinesPostAuthDB(t *testing.T) {
	noOwnerEnv(t)
	path := filepath.Join(t.TempDir(), "postauth.db")

	// Build exactly what the post-auth schema.sql produced: the schema as of
	// authVersion, and no recorded version. Migrating only that far (rather
	// than building the latest and undoing later migrations by hand) keeps
	// this fixture correct as migrations are added.
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(context.Background(), database, authVersion); err != nil {
		t.Fatalf("migrateTo(%d): %v", authVersion, err)
	}
	mustExec(t, database, "INSERT INTO users (id, email) VALUES (1, 'a@example.com')")
	mustExec(t, database, "INSERT INTO decks (id, user_id, name) VALUES (1, 1, 'French')")
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

// 0007 turns each user's llm.* settings into their first, active provider.
func TestLLMProvidersMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(context.Background(), database, 6); err != nil {
		t.Fatalf("migrateTo(6): %v", err)
	}
	mustExec(t, database, `
		INSERT INTO users (id, email) VALUES (1, 'a@example.com'), (2, 'b@example.com'), (3, 'c@example.com');
		INSERT INTO settings (user_id, key, value) VALUES
			(1, 'llm.provider', 'gemini'), (1, 'llm.base_url', 'https://g/v1'),
			(1, 'llm.model', 'gemini-x'), (1, 'llm.api_key', 'secret'),
			(2, 'llm.provider', 'custom'), (2, 'llm.model', 'm'),
			(3, 'llm.model', 'orphan');
	`)
	database.Close()

	noOwnerEnv(t)
	migrated := openOrFail(t, path)

	if count(t, migrated, `SELECT COUNT(*) FROM llm_providers WHERE user_id = 1 AND name = 'Google Gemini'
		AND preset = 'gemini' AND base_url = 'https://g/v1' AND model = 'gemini-x' AND api_key = 'secret' AND active = 1`) != 1 {
		t.Error("user 1's full configuration did not carry over")
	}
	if count(t, migrated, "SELECT COUNT(*) FROM llm_providers WHERE user_id = 2 AND name = 'Custom' AND base_url = '' AND active = 1") != 1 {
		t.Error("user 2's partial configuration did not carry over")
	}
	// No provider chosen means nothing to carry over.
	if count(t, migrated, "SELECT COUNT(*) FROM llm_providers WHERE user_id = 3") != 0 {
		t.Error("a provider was invented for a user who never chose one")
	}
	if count(t, migrated, "SELECT COUNT(*) FROM settings WHERE key LIKE 'llm.%'") != 0 {
		t.Error("old llm.* settings left behind")
	}
	// The database refuses a second active provider for a user.
	if _, err := migrated.Exec("INSERT INTO llm_providers (user_id, name, preset, active) VALUES (1, 'Second', 'groq', 1)"); err == nil {
		t.Error("a second active provider was accepted")
	}
}
