package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// Open connects to the PostgreSQL database at dsn (a postgres:// URL) and
// migrates it to the latest schema. Everything above this package still
// sees a plain *sql.DB.
func Open(dsn string) (*sql.DB, error) {
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// A small pool: one app process, and the background LLM goroutines each
	// hold a connection only briefly, per write.
	database.SetMaxOpenConns(10)
	database.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := migrate(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}
