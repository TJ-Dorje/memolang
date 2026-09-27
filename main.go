package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"memolang/internal/app"
	"memolang/internal/db"
)

func main() {
	dsn, stopDev := databaseURL()
	defer stopDev()

	database, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	server := &http.Server{Addr: ":8080", Handler: app.NewRouter(database)}

	// Shut down cleanly on Ctrl-C or a pod's SIGTERM, so the embedded dev
	// database is stopped rather than left holding its port.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()

	log.Println("MemoLang server starting on :8080")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// startDevDB starts an embedded dev database. It is set only in dev builds
// (devdb_dev.go, -tags dev); in production it is nil.
var startDevDB func() (dsn string, stop func(), err error)

// databaseURL returns the DSN to use, and a function to call on exit.
// DATABASE_URL wins; without it, a dev build starts an embedded database and
// a production build refuses to start.
func databaseURL() (string, func()) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn, func() {}
	}
	if startDevDB == nil {
		log.Fatal("DATABASE_URL is required (a postgres:// URL). For local development run `task dev`, which builds with -tags dev and starts an embedded database.")
	}
	dsn, stop, err := startDevDB()
	if err != nil {
		log.Fatal(err)
	}
	return dsn, stop
}
