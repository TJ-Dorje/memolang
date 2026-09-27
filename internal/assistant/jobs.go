package assistant

import (
	"context"
	"errors"
	"fmt"
	"log"

	"memolang/internal/ai"
	"memolang/internal/models"
)

// Background LLM work — a chat reply, a deck's cards — is described by a Job
// and handed to a JobQueue. The default queue runs it in a goroutine of the
// web process; with NATS, jobs go to JetStream and worker processes run them
// (internal/jobs), so a web deploy or crash no longer kills them. Either way
// RunJob does the work, and the output streams through the Broker.

// Job kinds.
const (
	JobReply    = "reply"
	JobGenerate = "generate"
)

// Job is one unit of background work, serialisable as JSON. It never carries
// an API key: whoever runs it reads the user's provider from the database.
type Job struct {
	Kind   string `json:"kind"`
	UserID int64  `json:"user_id"`

	// A chat reply: the placeholder message to fill, and the conversation to
	// send. (ai.ChatRequest itself holds a callback, which JSON cannot.)
	ReplyID  int64            `json:"reply_id,omitempty"`
	System   string           `json:"system,omitempty"`
	Messages []ai.ChatMessage `json:"messages,omitempty"`

	// A deck's cards: the deck to fill, and the plan to fill it from.
	DeckID int64     `json:"deck_id,omitempty"`
	Spec   *DeckSpec `json:"spec,omitempty"`
}

// JobQueue accepts jobs to run in the background.
type JobQueue interface {
	Enqueue(ctx context.Context, job Job) error
}

// InProcess runs each job in a goroutine of this process — the behaviour
// without NATS (development, tests, a single pod).
type InProcess struct{ Service *Service }

func (q InProcess) Enqueue(_ context.Context, job Job) error {
	go func() {
		if err := q.Service.RunJob(context.Background(), job, 1); err != nil {
			log.Printf("assistant: job %s: %v", job.Kind, err)
		}
	}()
	return nil
}

// ErrInterrupted ends a chat reply whose first run was lost with its worker.
var ErrInterrupted = errors.New("the reply was interrupted")

// RunJob does a job's work. attempt counts deliveries: above 1, an earlier
// run was lost with its process.
//
// A lost chat reply is not regenerated: its text so far has already been
// shown, and a second answer streamed after it would read as nonsense. It is
// marked interrupted, which the page shows as "ask again". A lost deck
// generation, long and worth finishing, resumes: cards already saved are
// skipped by the deck's unique (deck_id, front), new ones are added.
//
// The work records its own outcome, including LLM failures, in the database
// and the broker; RunJob returns an error only when it could not do that
// (the database is down), so a queue can retry.
func (s *Service) RunJob(ctx context.Context, job Job, attempt int) error {
	switch job.Kind {
	case JobReply:
		return s.runReply(job, attempt)
	case JobGenerate:
		return s.runGenerate(job)
	}
	return fmt.Errorf("unknown job kind %q", job.Kind)
}

func (s *Service) runReply(job Job, attempt int) error {
	status, err := models.GetConversationMessageStatus(s.DB, job.ReplyID)
	if err != nil {
		return err
	}
	if status != "generating" {
		return nil // finished already; only the acknowledgement was lost
	}

	key := ReplyKey(job.ReplyID)
	if attempt > 1 {
		if err := models.FinishConversationMessage(s.DB, job.ReplyID, "", "error"); err != nil {
			return err
		}
		s.Broker.Finish(key, ErrInterrupted)
		return nil
	}

	provider, err := s.Provider(s.DB, job.UserID)
	if err != nil {
		// The provider was removed or broken since the question was asked.
		if dbErr := models.FinishConversationMessage(s.DB, job.ReplyID, "", "error"); dbErr != nil {
			return dbErr
		}
		s.Broker.Finish(key, err)
		return nil
	}
	s.generate(provider, ai.ChatRequest{System: job.System, Messages: job.Messages}, job.ReplyID, key)
	return nil
}

func (s *Service) runGenerate(job Job) error {
	if job.Spec == nil {
		return errors.New("generate job without a plan")
	}
	key := GenerationKey(job.DeckID)
	provider, err := s.Provider(s.DB, job.UserID)
	if err != nil {
		s.Broker.Start(key)
		s.finishGeneration(job.UserID, job.DeckID, 0, err, func(ev GenerationEvent) { s.publishEvent(key, ev) })
		s.Broker.Finish(key, nil)
		return nil
	}
	s.generateCards(provider, job.UserID, job.DeckID, *job.Spec, key)
	return nil
}
