package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"memolang/internal/assistant"
	"memolang/internal/testdb/natstest"
)

func newQueue(t *testing.T, opts Options) *Queue {
	t.Helper()
	q, err := New(context.Background(), natstest.Start(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// runs records every run a worker makes.
type runs struct {
	mu       sync.Mutex
	attempts []int
	jobs     []assistant.Job
	done     chan struct{}
}

func (r *runs) record(job assistant.Job, attempt int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = append(r.attempts, attempt)
	r.jobs = append(r.jobs, job)
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestEnqueuedJobReachesAWorker(t *testing.T) {
	q := newQueue(t, Options{})
	r := &runs{done: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Work(ctx, func(_ context.Context, job assistant.Job, attempt int) error {
		r.record(job, attempt)
		close(r.done)
		return nil
	})

	spec := assistant.DeckSpec{Name: "Travel", Language: "Spanish", Prompt: "p", Count: 5, Mode: "srs"}
	want := assistant.Job{Kind: assistant.JobGenerate, UserID: 7, DeckID: 42, Spec: &spec}
	if err := q.Enqueue(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	waitFor(t, r.done, "the job to run")

	got := r.jobs[0]
	if got.Kind != want.Kind || got.DeckID != 42 || got.UserID != 7 || got.Spec == nil || *got.Spec != spec {
		t.Errorf("worker got %+v, want %+v", got, want)
	}
	if r.attempts[0] != 1 {
		t.Errorf("attempt = %d, want 1", r.attempts[0])
	}
}

// The case the queue exists for: a worker dies mid-job (here: it stops
// heartbeating, as a killed process would), and JetStream gives the job to
// another worker, which sees it as a second attempt.
func TestDeadWorkersJobIsRedelivered(t *testing.T) {
	q := newQueue(t, Options{AckWait: 300 * time.Millisecond, Heartbeat: time.Hour})

	stuck := make(chan struct{})
	defer close(stuck)
	firstStarted := make(chan struct{})
	var once sync.Once
	second := make(chan int, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Work(ctx, func(_ context.Context, _ assistant.Job, attempt int) error {
		if attempt == 1 {
			once.Do(func() { close(firstStarted) })
			<-stuck // never finishes, never heartbeats
			return nil
		}
		second <- attempt
		return nil
	})

	if err := q.Enqueue(context.Background(), assistant.Job{Kind: assistant.JobReply, ReplyID: 1}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, firstStarted, "the first attempt")

	select {
	case attempt := <-second:
		if attempt != 2 {
			t.Errorf("redelivery attempt = %d, want 2", attempt)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the dead worker's job was never redelivered")
	}
}

// Heartbeats keep a slow but alive job from being redelivered.
func TestHeartbeatKeepsSlowJob(t *testing.T) {
	q := newQueue(t, Options{AckWait: 300 * time.Millisecond, Heartbeat: 100 * time.Millisecond})

	var mu sync.Mutex
	attempts := 0
	done := make(chan struct{})
	var finished sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Work(ctx, func(_ context.Context, _ assistant.Job, _ int) error {
		mu.Lock()
		attempts++
		mu.Unlock()
		time.Sleep(time.Second) // over three AckWaits, heartbeating throughout
		finished.Do(func() { close(done) })
		return nil
	})

	q.Enqueue(context.Background(), assistant.Job{Kind: assistant.JobReply, ReplyID: 1})
	waitFor(t, done, "the slow job")
	time.Sleep(500 * time.Millisecond) // give a wrongful redelivery time to show

	mu.Lock()
	defer mu.Unlock()
	if attempts != 1 {
		t.Errorf("the job ran %d times, want once: heartbeats should hold it", attempts)
	}
}

// A worker shutting down mid-job hands it back at once rather than letting
// it wait out AckWait.
func TestShutdownHandsJobBack(t *testing.T) {
	q := newQueue(t, Options{AckWait: time.Hour})

	started := make(chan struct{})
	ctxA, stopA := context.WithCancel(context.Background())
	workerA := make(chan struct{})
	go func() {
		q.Work(ctxA, func(ctx context.Context, _ assistant.Job, _ int) error {
			close(started)
			<-ctx.Done() // the job notices the shutdown
			return ctx.Err()
		})
		close(workerA)
	}()

	q.Enqueue(context.Background(), assistant.Job{Kind: assistant.JobReply, ReplyID: 1})
	waitFor(t, started, "worker A to start the job")
	stopA()
	waitFor(t, workerA, "worker A to stop")

	pickedUp := make(chan struct{})
	ctxB, stopB := context.WithCancel(context.Background())
	defer stopB()
	go q.Work(ctxB, func(_ context.Context, _ assistant.Job, _ int) error {
		close(pickedUp)
		return nil
	})
	// AckWait is an hour: only an explicit nak gets it here in time.
	waitFor(t, pickedUp, "worker B to pick the job up")
}
