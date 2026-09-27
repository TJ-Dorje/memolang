// Package natstest runs a real NATS server with JetStream inside the test
// process — nats-server is a Go library — so tests of the JetStream broker
// and job queue need no Docker, like pgtest for Postgres.
package natstest

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Start runs a server with JetStream for the test, on a random port with its
// store in the test's temp dir, and returns a connection to it. Both are
// shut down when the test ends.
func Start(t testing.TB) *nats.Conn {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      server.RANDOM_PORT,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	if err != nil {
		t.Fatalf("natstest: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("natstest: server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("natstest: connect: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})
	return nc
}
