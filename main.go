package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	"memolang/internal/app"
	"memolang/internal/assistant"
	"memolang/internal/db"
	"memolang/internal/jobs"
	"memolang/internal/stream"
)

// The same binary runs in two modes:
//
//	memolang          the web app
//	memolang worker   runs queued LLM jobs (needs NATS_URL)
//
// With NATS_URL set, the web app streams replies through JetStream and
// enqueues LLM work for workers; without it, it runs that work in-process
// with an in-memory broker (development, a single pod).
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn, stopDev := databaseURL()
	defer stopDev()
	database, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	if len(os.Args) > 1 && os.Args[1] == "worker" {
		runWorker(ctx, database)
		return
	}
	runWeb(ctx, database)
}

func runWeb(ctx context.Context, database *sql.DB) {
	svc := assistant.New(database, stream.NewMemory())
	if nc := connectNATS("memolang-web", false); nc != nil {
		defer nc.Drain()
		useJetStream(ctx, svc, nc)
		log.Println("Background work: queued on NATS JetStream for workers")
	} else {
		log.Println("Background work: in-process (NATS_URL not set)")
	}

	server := &http.Server{Addr: ":8080", Handler: app.NewRouter(database, svc)}
	serve(ctx, server, "MemoLang server starting on :8080")
}

func runWorker(ctx context.Context, database *sql.DB) {
	nc := connectNATS("memolang-worker", true)
	defer nc.Drain()

	svc := assistant.New(database, stream.NewMemory())
	queue := useJetStream(ctx, svc, nc)

	// A liveness endpoint for Kubernetes: alive while connected to NATS.
	health := http.NewServeMux()
	health.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if !nc.IsConnected() {
			http.Error(w, "not connected to NATS", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	go serve(ctx, &http.Server{Addr: ":8081", Handler: health}, "Worker health on :8081")

	log.Println("Worker: consuming jobs")
	if err := queue.Work(ctx, svc.RunJob); err != nil {
		log.Fatalf("worker: %v", err)
	}
	log.Println("Worker: stopped")
}

// connectNATS connects to NATS_URL, or returns nil when it is unset and not
// required. It keeps reconnecting for ever: NATS restarting must not take
// the app down with it.
func connectNATS(name string, required bool) *nats.Conn {
	url := os.Getenv("NATS_URL")
	if url == "" {
		if required {
			log.Fatal("NATS_URL is required for the worker")
		}
		return nil
	}
	nc, err := nats.Connect(url, nats.Name(name), nats.MaxReconnects(-1))
	if err != nil {
		log.Fatalf("connect to NATS at %s: %v", url, err)
	}
	return nc
}

// useJetStream switches the service to the JetStream broker and job queue.
func useJetStream(ctx context.Context, svc *assistant.Service, nc *nats.Conn) *jobs.Queue {
	broker, err := stream.NewJetStream(ctx, nc)
	if err != nil {
		log.Fatalf("JetStream broker: %v", err)
	}
	queue, err := jobs.New(ctx, nc, jobs.Options{AckWait: durationEnv("JOBS_ACK_WAIT")})
	if err != nil {
		log.Fatalf("JetStream job queue: %v", err)
	}
	svc.Broker, svc.Jobs = broker, queue
	return queue
}

// durationEnv reads an optional duration setting such as "30s"; 0 (the
// package default) when unset. A malformed value stops startup rather than
// being silently ignored.
func durationEnv(name string) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Fatalf("%s=%q: %v", name, v, err)
	}
	return d
}

// serve runs server until ctx ends, then shuts it down gracefully.
func serve(ctx context.Context, server *http.Server, banner string) {
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Println(banner)
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
