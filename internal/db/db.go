package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := database.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := database.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Checked before the schema runs: schema.sql is all CREATE TABLE IF NOT
	// EXISTS, so it leaves a pre-auth decks table untouched and then fails on
	// the owner index with an opaque "no such column: user_id".
	if err := checkSchemaCurrent(database); err != nil {
		return nil, err
	}

	if _, err := database.Exec(schema); err != nil {
		return nil, fmt.Errorf("run schema: %w", err)
	}

	return database, nil
}

// checkSchemaCurrent rejects a database created before decks gained an owner
// column. A database with no decks table at all is brand new, which is fine:
// the schema is about to create it.
func checkSchemaCurrent(database *sql.DB) error {
	rows, err := database.Query("PRAGMA table_info(decks)")
	if err != nil {
		return fmt.Errorf("inspect decks schema: %w", err)
	}
	defer rows.Close()

	columns := 0
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notNull    int
			dfltValue  sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &primaryKey); err != nil {
			return fmt.Errorf("inspect decks schema: %w", err)
		}
		columns++
		if name == "user_id" {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect decks schema: %w", err)
	}

	if columns == 0 {
		return nil // no decks table yet: a brand-new database
	}

	return fmt.Errorf("schema out of date: decks.user_id is missing — run `task db:reset` to recreate the database")
}
