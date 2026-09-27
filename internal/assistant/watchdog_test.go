package assistant

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatchdogIdle(t *testing.T) {
	w := newWatchdog(40*time.Millisecond, time.Minute)
	defer w.Close()

	<-w.ctx.Done()
	var idle ErrIdle
	if err := w.explain(w.ctx.Err()); !errors.As(err, &idle) {
		t.Errorf("explain = %v, want ErrIdle", err)
	}
}

// Output keeps a slow call alive well past the idle limit.
func TestWatchdogAliveKeepsGoing(t *testing.T) {
	w := newWatchdog(60*time.Millisecond, time.Minute)
	defer w.Close()

	for range 6 {
		time.Sleep(30 * time.Millisecond)
		w.Alive()
	}
	if err := w.ctx.Err(); err != nil {
		t.Fatalf("cancelled after 180ms of steady output: %v", err)
	}
}

func TestWatchdogMaxDuration(t *testing.T) {
	w := newWatchdog(time.Minute, 40*time.Millisecond)
	defer w.Close()

	<-w.ctx.Done()
	if err := w.explain(w.ctx.Err()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("explain = %v, want the max-duration deadline", err)
	}
}
