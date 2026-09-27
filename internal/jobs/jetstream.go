// Package jobs queues background LLM work on NATS JetStream for worker
// processes, so it survives web deploys and crashes: a job is acknowledged
// only once it is done, and a worker that dies stops heartbeating, so
// JetStream hands its job to another worker.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"memolang/internal/assistant"
)

const (
	// JobsStream holds queued jobs until a worker acknowledges them.
	JobsStream   = "MEMOLANG_JOBS"
	jobsPrefix   = "jobs."
	consumerName = "workers"
)

// Options tune the queue. The zero value uses the defaults.
type Options struct {
	// AckWait is how long JetStream waits for a heartbeat or an ack before
	// giving the job to another worker: roughly how long a dead worker's
	// job waits to be picked up again.
	AckWait time.Duration
	// Heartbeat is how often a running job tells JetStream it is alive.
	// Must be well under AckWait.
	Heartbeat time.Duration
	// MaxDeliver caps attempts per job, the first included.
	MaxDeliver int
	// Concurrency is how many jobs one worker runs at once.
	Concurrency int
}

func (o Options) withDefaults() Options {
	if o.AckWait == 0 {
		o.AckWait = time.Minute
	}
	if o.Heartbeat == 0 {
		o.Heartbeat = 20 * time.Second
	}
	if o.MaxDeliver == 0 {
		o.MaxDeliver = 3
	}
	if o.Concurrency == 0 {
		o.Concurrency = 4
	}
	return o
}

// Queue is a JetStream work queue of assistant jobs; it implements
// assistant.JobQueue, and Work runs the consuming side.
type Queue struct {
	js   jetstream.JetStream
	opts Options
}

// New connects the queue, creating or updating its stream. Work-queue
// retention removes each job once acknowledged.
func New(ctx context.Context, nc *nats.Conn, opts Options) (*Queue, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      JobsStream,
		Subjects:  []string{jobsPrefix + ">"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs stream: %w", err)
	}
	return &Queue{js: js, opts: opts.withDefaults()}, nil
}

// Enqueue publishes a job for a worker.
func (q *Queue) Enqueue(ctx context.Context, job assistant.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if _, err := q.js.Publish(ctx, jobsPrefix+job.Kind, data); err != nil {
		return fmt.Errorf("enqueue %s job: %w", job.Kind, err)
	}
	return nil
}

// RunFunc does one job; attempt counts deliveries, the first being 1.
type RunFunc func(ctx context.Context, job assistant.Job, attempt int) error

// Work consumes jobs until ctx ends, running up to Concurrency at a time.
// On shutdown it stops taking jobs and naks the ones still running, so
// another worker picks them up at once rather than after AckWait.
func (q *Queue) Work(ctx context.Context, run RunFunc) error {
	consumer, err := q.js.CreateOrUpdateConsumer(ctx, JobsStream, jetstream.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       q.opts.AckWait,
		MaxDeliver:    q.opts.MaxDeliver,
		MaxAckPending: 1000,
	})
	if err != nil {
		return fmt.Errorf("jobs consumer: %w", err)
	}

	slots := make(chan struct{}, q.opts.Concurrency)
	var running sync.WaitGroup
	consuming, err := consumer.Consume(func(msg jetstream.Msg) {
		slots <- struct{}{}
		running.Go(func() {
			defer func() { <-slots }()
			q.handle(ctx, msg, run)
		})
	}, jetstream.PullMaxMessages(q.opts.Concurrency))
	if err != nil {
		return fmt.Errorf("consume jobs: %w", err)
	}

	<-ctx.Done()
	consuming.Stop()
	running.Wait()
	return nil
}

// handle runs one job with heartbeats, then acks it — or naks it, to be
// retried, when it could not record its outcome or the worker is stopping.
func (q *Queue) handle(ctx context.Context, msg jetstream.Msg, run RunFunc) {
	var job assistant.Job
	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		log.Printf("jobs: dropping undecodable job: %v", err)
		msg.Term()
		return
	}
	attempt := 1
	if meta, err := msg.Metadata(); err == nil {
		attempt = int(meta.NumDelivered)
	}

	stopHeartbeat := q.heartbeat(msg)
	err := run(ctx, job, attempt)
	stopHeartbeat()

	switch {
	case ctx.Err() != nil:
		// Shutting down mid-job: hand it straight to another worker.
		msg.Nak()
	case err != nil:
		log.Printf("jobs: %s job (attempt %d): %v", job.Kind, attempt, err)
		msg.NakWithDelay(5 * time.Second)
	default:
		if err := msg.Ack(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			log.Printf("jobs: ack %s job: %v", job.Kind, err)
		}
	}
}

// heartbeat keeps a running job's delivery alive until the returned
// function is called.
func (q *Queue) heartbeat(msg jetstream.Msg) (stop func()) {
	ticker := time.NewTicker(q.opts.Heartbeat)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				msg.InProgress()
			case <-done:
				return
			}
		}
	}()
	return func() {
		ticker.Stop()
		close(done)
	}
}
