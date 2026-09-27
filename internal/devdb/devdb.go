// Package devdb runs PostgreSQL for local development, embedded in the app
// process, so `task dev` stays one command with nothing else to install or
// start. The data persists in ./.pgdata between runs.
//
// It is only compiled into dev builds (-tags dev, see devdb_dev.go): a
// production pod with a missing DATABASE_URL must fail, not quietly start a
// throwaway database.
package devdb

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

// PostgresVersion is the Postgres major version for development and tests.
// Keep it the same as production (the CloudNativePG cluster's image).
const PostgresVersion = embeddedpostgres.V18

// Settings for the dev server. The port is fixed, so a DSN printed at
// startup can be used from psql or a GUI client.
const (
	Port     = 54329
	user     = "memolang"
	password = "memolang"
	database = "memolang"
	dataDir  = ".pgdata"
)

// DSN is the connection URL of the dev database.
func DSN() string {
	return fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, Port, database)
}

// Start runs the dev server and returns a function that stops it.
func Start() (stop func(), err error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	server := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(PostgresVersion).
		Port(Port).
		Username(user).
		Password(password).
		Database(database).
		DataPath(filepath.Join(abs, "data")).
		RuntimePath(filepath.Join(abs, "runtime")).
		StartTimeout(60 * time.Second).
		Logger(os.Stderr))
	if err := server.Start(); err != nil {
		return nil, fmt.Errorf("start embedded postgres on port %d (a previous run may still be holding it — stop it, or remove %s/data/postmaster.pid if it crashed): %w", Port, dataDir, err)
	}
	log.Printf("dev database: embedded PostgreSQL in %s — %s", dataDir, DSN())
	return func() {
		if err := server.Stop(); err != nil {
			log.Printf("dev database: stop: %v", err)
		}
	}, nil
}
