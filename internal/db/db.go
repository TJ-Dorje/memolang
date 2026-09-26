package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens the database and migrates it to the latest schema.
//
// The pragmas go in the DSN rather than a one-off Exec: database/sql pools
// connections, and a pragma applies only to the connection that ran it, so an
// Exec'd foreign_keys = ON left every other pooled connection without cascades.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := migrate(context.Background(), database); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}
