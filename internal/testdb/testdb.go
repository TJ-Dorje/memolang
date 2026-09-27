// Package testdb gives each test its own fully migrated PostgreSQL database.
//
// Migrating per test would be slow, so the schema is migrated once, into a
// template database, and each test gets a copy of it with CREATE DATABASE …
// TEMPLATE, which Postgres does in milliseconds. Tests are isolated the way
// the old throwaway SQLite files made them.
//
// The package needs the server from pgtest: its TestMain must call
// pgtest.Main.
package testdb

import (
	"database/sql"
	"sync"
	"testing"

	"memolang/internal/db"
	"memolang/internal/testdb/pgtest"
)

var (
	templateOnce sync.Once
	templateName string
	templateErr  error
)

// New returns a migrated database for the test, closed and dropped when it
// ends.
func New(t testing.TB) *sql.DB {
	t.Helper()
	database, err := db.Open(NewDSN(t))
	if err != nil {
		t.Fatalf("testdb: open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// NewDSN is New for callers that open the database themselves (the e2e
// server): the DSN of a fresh, migrated database for the test.
func NewDSN(t testing.TB) string {
	t.Helper()
	templateOnce.Do(buildTemplate)
	if templateErr != nil {
		t.Fatalf("testdb: template: %v", templateErr)
	}
	return pgtest.CreateDatabase(t, pgtest.UniqueName("test"), templateName)
}

// buildTemplate migrates the template once per process. Its connections are
// closed afterwards: Postgres refuses to copy a database anyone is using.
func buildTemplate() {
	name := pgtest.UniqueName("template")
	admin, err := pgtest.Admin()
	if err != nil {
		templateErr = err
		return
	}
	defer admin.Close()
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		templateErr = err
		return
	}

	migrated, err := db.Open(pgtest.DSN(name))
	if err != nil {
		templateErr = err
		return
	}
	templateErr = migrated.Close()
	templateName = name
}

// OpenForMain is New for a TestMain, where there is no *testing.T: a fresh,
// migrated database on the pgtest server started by the caller. It lives
// until the server stops.
func OpenForMain() (*sql.DB, error) {
	templateOnce.Do(buildTemplate)
	if templateErr != nil {
		return nil, templateErr
	}
	admin, err := pgtest.Admin()
	if err != nil {
		return nil, err
	}
	defer admin.Close()

	name := pgtest.UniqueName("main")
	if _, err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE " + templateName); err != nil {
		return nil, err
	}
	return db.Open(pgtest.DSN(name))
}
