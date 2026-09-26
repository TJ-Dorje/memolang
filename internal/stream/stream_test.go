package stream

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func follow(t *testing.T, b Broker, key string) (string, error) {
	t.Helper()
	var got strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
	b := NewMemory()
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
}

// A reader arriving mid-reply (a page reload) must see everything so far.
func TestLateFollowerGetsReplay(t *testing.T) {
	b := NewMemory()
	b.Start("k")
	b.Publish("k", "already ")
	b.Publish("k", "sent ")

	go func() {
		time.Sleep(10 * time.Millisecond)
		b.Publish("k", "then live")
		b.Finish("k", nil)
	}()

	got, _ := follow(t, b, "k")
	if got != "already sent then live" {
		t.Errorf("Follow = %q", got)
	}
}

func TestFollowAfterFinish(t *testing.T) {
	b := NewMemory()
	b.Start("k")
	b.Publish("k", "complete")
	b.Finish("k", nil)

	got, err := follow(t, b, "k")
	if err != nil || got != "complete" {
		t.Errorf("Follow after finish = %q, %v", got, err)
	}
}

func TestFollowReturnsProducerError(t *testing.T) {
	b := NewMemory()
	b.Start("k")
	b.Publish("k", "part")
	boom := errors.New("provider failed")
	b.Finish("k", boom)

	got, err := follow(t, b, "k")
	if !errors.Is(err, boom) || got != "part" {
		t.Errorf("Follow = %q, %v; want the partial text and the producer's error", got, err)
	}
}

func TestFollowUnknownKey(t *testing.T) {
	found, err := NewMemory().Follow(context.Background(), "nope", func(string) error { return nil })
	if found || err != nil {
		t.Errorf("Follow(unknown) = %v, %v; want false, nil", found, err)
	}
}

func TestManyFollowersSeeEverything(t *testing.T) {
	b := NewMemory()
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
}

// A reader that goes away (closed tab) must not affect the producer.
func TestFollowerCancelDoesNotBlockProducer(t *testing.T) {
	b := NewMemory()
	b.Start("k")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := b.Follow(ctx, "k", func(string) error { return nil })
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Follow after cancel = %v", err)
	}

	b.Publish("k", "still fine")
	b.Finish("k", nil)
	got, _ := follow(t, b, "k")
	if got != "still fine" {
		t.Errorf("producer output after a reader left = %q", got)
	}
}
