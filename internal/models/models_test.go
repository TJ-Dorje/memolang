package models_test

import (
	"path/filepath"
	"testing"

	"database/sql"

	"memolang/internal/db"
)

// testDB opens a throwaway database with the real schema applied.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}
