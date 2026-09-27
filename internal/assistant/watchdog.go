package assistant

import (
	"context"
	"fmt"
	"time"
)

// Limits for one LLM call made in the background. A thinking model can
// reason for minutes before writing a word of its answer, and a large local
// model can be slow throughout, so a fixed total deadline cuts off replies
// that are still progressing. Instead a call fails when the model goes quiet
// for IdleTimeout — any output counts, reasoning included — with MaxDuration
// as a backstop for a stream that never ends.
const (
	DefaultIdleTimeout = 2 * time.Minute
	DefaultMaxDuration = 15 * time.Minute
)

// ErrIdle is the cause of a call cancelled because the model went quiet.
type ErrIdle struct{ After time.Duration }

func (e ErrIdle) Error() string {
	return fmt.Sprintf("no output from the model for %s", e.After)
}

// watchdog is a context cancelled with ErrIdle when Alive has not been
// called for idle, and with context.DeadlineExceeded after max. time.Timer's
// Reset is safe to call from any goroutine, so Alive needs no lock.
type watchdog struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	stop   context.CancelFunc
	timer  *time.Timer
	idle   time.Duration
}

func newWatchdog(idle, max time.Duration) *watchdog {
	deadlineCtx, stop := context.WithTimeout(context.Background(), max)
	ctx, cancel := context.WithCancelCause(deadlineCtx)
	return &watchdog{
		ctx:    ctx,
		cancel: cancel,
		stop:   stop,
		idle:   idle,
		timer:  time.AfterFunc(idle, func() { cancel(ErrIdle{After: idle}) }),
	}
}

// Alive resets the idle timer; call it on every chunk of output.
func (w *watchdog) Alive() { w.timer.Reset(w.idle) }

// explain turns the error a cancelled call returned into the reason it was
// cancelled: "context canceled" says nothing, "no output from the model for
// 2m0s" says what happened.
func (w *watchdog) explain(err error) error {
	if err == nil {
		return nil
	}
	if cause := context.Cause(w.ctx); cause != nil {
		return cause
	}
	return err
}

// Close releases the timers; call it when the call is done.
func (w *watchdog) Close() {
	w.timer.Stop()
	w.cancel(nil)
	w.stop()
}
