package models_test

import (
	"database/sql"
	"testing"

	"memolang/internal/testdb"
	"memolang/internal/testdb/pgtest"
)

func TestMain(m *testing.M) { pgtest.Main(m) }

// testDB returns a fresh, fully migrated database for the test.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.New(t)
}
