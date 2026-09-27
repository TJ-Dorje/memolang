// Package pgtest runs a real PostgreSQL server for tests, embedded in the
// test process: embedded-postgres downloads the Postgres binaries once
// (cached under ~/.embedded-postgres-go) and starts them on a free port. No
// Docker is needed, so `go test` stays one command.
//
// One server is started per test package, from TestMain:
//
//	func TestMain(m *testing.M) { pgtest.Main(m) }
//
// Tests then create throwaway databases on it. This package does not import
// internal/db, so the db package's own tests can use it.
package pgtest

import (
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
	"memolang/internal/devdb"
)

// Version is the Postgres major version tests run against: the same as
// development and production (devdb.PostgresVersion).
const Version = devdb.PostgresVersion

const (
	user     = "postgres"
	password = "postgres"
)

var (
	port    uint32
	counter atomic.Int64
)

// Main starts the server, runs the package's tests and stops it.
func Main(m *testing.M) {
	stop, err := Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgtest: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	stop()
	os.Exit(code)
}

// Start runs a Postgres server for this process and returns a function that
// stops it. Its data lives in a temporary directory, removed on stop.
func Start() (stop func(), err error) {
	p, err := freePort()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "pgtest-*")
	if err != nil {
		return nil, err
	}

	server := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(Version).
		Port(p).
		Username(user).
		Password(password).
		RuntimePath(filepath.Join(dir, "runtime")).
		DataPath(filepath.Join(dir, "data")).
		StartTimeout(60 * time.Second).
		Logger(io.Discard))
	if err := server.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}
	port = p

	return func() {
		server.Stop()
		os.RemoveAll(dir)
	}, nil
}

func freePort() (uint32, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return uint32(ln.Addr().(*net.TCPAddr).Port), nil
}

// DSN is the connection URL for database name on the running server.
func DSN(name string) string {
	return fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, port, name)
}

// Admin opens a connection to the server's maintenance database, for
// creating and dropping test databases.
func Admin() (*sql.DB, error) {
	if port == 0 {
		return nil, fmt.Errorf("pgtest: no server running — call pgtest.Main from TestMain")
	}
	return sql.Open("pgx", DSN("postgres"))
}

// UniqueName returns a database name not used before in this process.
func UniqueName(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), counter.Add(1))
}

// EmptyDatabase creates a new, empty database for the test and drops it
// afterwards, returning its DSN.
func EmptyDatabase(t testing.TB) string {
	t.Helper()
	return CreateDatabase(t, UniqueName("empty"), "")
}

// CreateDatabase creates database name, optionally copied from template,
// and drops it when the test ends.
func CreateDatabase(t testing.TB, name, template string) string {
	t.Helper()
	admin, err := Admin()
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	create := "CREATE DATABASE " + name
	if template != "" {
		create += " TEMPLATE " + template
	}
	if _, err := admin.Exec(create); err != nil {
		t.Fatalf("pgtest: %s: %v", create, err)
	}
	t.Cleanup(func() {
		admin, err := Admin()
		if err != nil {
			return
		}
		defer admin.Close()
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
	})
	return DSN(name)
}
