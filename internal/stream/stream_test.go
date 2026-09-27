package stream

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"memolang/internal/testdb/natstest"
)

// Every test is a contract both Broker implementations must meet: the
// handlers cannot tell which one they have.
func forEachBroker(t *testing.T, test func(t *testing.T, b Broker)) {
	t.Run("memory", func(t *testing.T) { test(t, NewMemory()) })
	t.Run("jetstream", func(t *testing.T) {
		b, err := NewJetStream(context.Background(), natstest.Start(t))
		if err != nil {
			t.Fatal(err)
		}
		test(t, b)
	})
}

func follow(t *testing.T, b Broker, key string) (string, error) {
	t.Helper()
	var got strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	found, err := b.Follow(ctx, key, func(chunk string) error {
		got.WriteString(chunk)
		return nil
	})
	if !found {
		t.Fatalf("Follow(%q): not found", key)
	}
	return got.String(), err
}

func TestFollowLiveOutput(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")
		go func() {
			for _, c := range []string{"El ", "verbo ", "hablar"} {
				time.Sleep(5 * time.Millisecond)
				b.Publish("k", c)
			}
			b.Finish("k", nil)
		}()

		got, err := follow(t, b, "k")
		if err != nil || got != "El verbo hablar" {
			t.Errorf("Follow = %q, %v", got, err)
		}
	})
}

// A reader arriving mid-reply (a page reload) must see everything so far.
func TestLateFollowerGetsReplay(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")
		b.Publish("k", "already ")
		b.Publish("k", "sent ")
		go func() {
			time.Sleep(20 * time.Millisecond)
			b.Publish("k", "then live")
			b.Finish("k", nil)
		}()

		if got, _ := follow(t, b, "k"); got != "already sent then live" {
			t.Errorf("Follow = %q", got)
		}
	})
}

func TestFollowAfterFinish(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")
		b.Publish("k", "complete")
		b.Finish("k", nil)

		got, err := follow(t, b, "k")
		if err != nil || got != "complete" {
			t.Errorf("Follow after finish = %q, %v", got, err)
		}
	})
}

// The producer's error reaches followers — as the same value in memory, as
// the same text across JetStream.
func TestFollowReturnsProducerError(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")
		b.Publish("k", "part")
		b.Finish("k", errors.New("provider failed"))

		got, err := follow(t, b, "k")
		if err == nil || err.Error() != "provider failed" || got != "part" {
			t.Errorf("Follow = %q, %v; want the partial text and the producer's error", got, err)
		}
	})
}

func TestFollowUnknownKey(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		found, err := b.Follow(context.Background(), "nope", func(string) error { return nil })
		if found || err != nil {
			t.Errorf("Follow(unknown) = %v, %v; want false, nil", found, err)
		}
		if b.Exists("nope") {
			t.Error("Exists(unknown) = true")
		}
	})
}

func TestExistsBeforeFirstChunk(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")
		if !b.Exists("k") {
			t.Error("Exists = false right after Start")
		}
	})
}

func TestManyFollowersSeeEverything(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")

		var wg sync.WaitGroup
		results := make([]string, 5)
		for i := range results {
			wg.Go(func() {
				results[i], _ = follow(t, b, "k")
			})
		}

		var want strings.Builder
		for i := range 200 {
			c := string(rune('a' + i%26))
			want.WriteString(c)
			b.Publish("k", c)
		}
		b.Finish("k", nil)
		wg.Wait()

		for i, got := range results {
			if got != want.String() {
				t.Errorf("follower %d got %d bytes, want %d", i, len(got), want.Len())
			}
		}
	})
}

// A reader that goes away (closed tab) must not affect the producer.
func TestFollowerCancelDoesNotBlockProducer(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("k")

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error)
		go func() {
			_, err := b.Follow(ctx, "k", func(string) error { return nil })
			done <- err
		}()
		time.Sleep(20 * time.Millisecond)
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("Follow after cancel = %v", err)
		}

		b.Publish("k", "still fine")
		b.Finish("k", nil)
		if got, _ := follow(t, b, "k"); got != "still fine" {
			t.Errorf("producer output after a reader left = %q", got)
		}
	})
}

// Keys are independent: one reply's output never leaks into another's.
func TestKeysAreSeparate(t *testing.T) {
	forEachBroker(t, func(t *testing.T, b Broker) {
		b.Start("a")
		b.Start("b")
		b.Publish("a", "for a")
		b.Publish("b", "for b")
		b.Finish("a", nil)
		b.Finish("b", nil)

		if got, _ := follow(t, b, "a"); got != "for a" {
			t.Errorf("key a = %q", got)
		}
	})
}
