package db

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"memolang/internal/testdb/pgtest"
)

func TestMain(m *testing.M) { pgtest.Main(m) }

func openOrFail(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	database, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func count(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func latest() int { return migrations[len(migrations)-1].version }

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	database := openOrFail(t, pgtest.EmptyDatabase(t))

	if got := count(t, database, "SELECT MAX(version) FROM schema_migrations"); got != latest() {
		t.Errorf("schema version = %d, want %d", got, latest())
	}
	for _, table := range []string{"users", "decks", "cards", "llm_providers", "conversations"} {
		if count(t, database, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = $1", table) != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dsn := pgtest.EmptyDatabase(t)
	openOrFail(t, dsn).Close()
	again := openOrFail(t, dsn)

	if n := count(t, again, "SELECT COUNT(*) FROM schema_migrations"); n != len(migrations) {
		t.Errorf("schema_migrations has %d rows, want %d: a migration ran twice", n, len(migrations))
	}
}

// Several pods starting at once must not apply the same migration twice:
// the advisory lock makes the others wait, then find nothing to do.
func TestConcurrentOpensMigrateOnce(t *testing.T) {
	dsn := pgtest.EmptyDatabase(t)

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			database, err := Open(dsn)
			if err != nil {
				errs <- err
				return
			}
			database.Close()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Open: %v", err)
	}

	database := openOrFail(t, dsn)
	if n := count(t, database, "SELECT COUNT(*) FROM schema_migrations"); n != len(migrations) {
		t.Errorf("schema_migrations has %d rows, want %d", n, len(migrations))
	}
}

// A migration that fails leaves nothing behind — neither its changes nor its
// version — so the next start retries it.
func TestFailedMigrationIsNotRecorded(t *testing.T) {
	dsn := pgtest.EmptyDatabase(t)
	database := openOrFail(t, dsn)

	saved := migrations
	t.Cleanup(func() { migrations = saved })
	// Re-running the baseline fails on its first CREATE TABLE.
	migrations = append(append([]migration{}, saved...), migration{version: latest() + 1, file: saved[0].file})

	if err := migrate(context.Background(), database); err == nil {
		t.Fatal("a failing migration reported success")
	}
	if got := count(t, database, "SELECT MAX(version) FROM schema_migrations"); got != saved[len(saved)-1].version {
		t.Errorf("version after a failed migration = %d, want it unchanged", got)
	}
}
