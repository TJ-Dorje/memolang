package stream

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStream is a Broker on NATS JetStream, for more than one process: a
// reply produced by a worker can be followed from any web pod, and survives
// the producer's process.
//
// Each key is a subject, replies.<key>, in one stream. The stream keeps the
// ordered log of every reply for RetainReplies, which is what gives
// followers the in-memory broker's behaviour: an ordered consumer from the
// first message replays everything so far, then continues live.
type JetStream struct {
	stream jetstream.Stream
	js     jetstream.JetStream
}

const (
	// RepliesStream is the JetStream stream holding reply output.
	RepliesStream = "MEMOLANG_REPLIES"
	repliesPrefix = "replies."

	// RetainReplies is how long a reply stays followable. Longer than the
	// in-memory broker's minute: here a follower may be on another pod.
	RetainReplies = 10 * time.Minute

	// eventHeader marks control messages; its absence means a text chunk.
	eventHeader = "Memolang-Event"
	eventStart  = "start"
	eventDone   = "done"
	eventError  = "error"

	// publishTimeout bounds one publish. The Broker interface has no
	// context: producers run in the background and cannot usefully wait.
	publishTimeout = 5 * time.Second
)

// NewJetStream connects the broker to JetStream, creating or updating its
// stream.
func NewJetStream(ctx context.Context, nc *nats.Conn) (*JetStream, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	s, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      RepliesStream,
		Subjects:  []string{repliesPrefix + ">"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    RetainReplies,
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return nil, fmt.Errorf("replies stream: %w", err)
	}
	return &JetStream{stream: s, js: js}, nil
}

func subject(key string) string { return repliesPrefix + key }

// publish sends one message, logging failures: a lost chunk is a gap in a
// reply, not a reason to stop producing it.
func (b *JetStream) publish(key, event string, data []byte) {
	msg := nats.NewMsg(subject(key))
	msg.Data = data
	if event != "" {
		msg.Header.Set(eventHeader, event)
	}
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if _, err := b.js.PublishMsg(ctx, msg); err != nil {
		log.Printf("stream: publish %s (%s): %v", key, event, err)
	}
}

// Start marks key as started, so Exists is true before the first chunk.
func (b *JetStream) Start(key string) { b.publish(key, eventStart, nil) }

func (b *JetStream) Publish(key, chunk string) { b.publish(key, "", []byte(chunk)) }

// Finish ends key's output. A producer error travels as its message text;
// followers get an error with the same text.
func (b *JetStream) Finish(key string, err error) {
	if err != nil {
		b.publish(key, eventError, []byte(err.Error()))
		return
	}
	b.publish(key, eventDone, nil)
}

func (b *JetStream) Exists(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	_, err := b.stream.GetLastMsgForSubject(ctx, subject(key))
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		log.Printf("stream: exists %s: %v", key, err)
	}
	return err == nil
}

// Follow replays key's output from the first message, then follows it live,
// until the done or error message, fn failing, or ctx ending.
func (b *JetStream) Follow(ctx context.Context, key string, fn func(chunk string) error) (bool, error) {
	if !b.Exists(key) {
		return false, nil
	}
	consumer, err := b.stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject(key)},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return true, err
	}
	messages, err := consumer.Messages()
	if err != nil {
		return true, err
	}
	defer messages.Stop()
	// Next blocks without a context; stopping the iterator unblocks it.
	defer context.AfterFunc(ctx, messages.Stop)()

	for {
		msg, err := messages.Next()
		if err != nil {
			if ctx.Err() != nil {
				return true, ctx.Err()
			}
			return true, err
		}
		switch msg.Headers().Get(eventHeader) {
		case eventStart:
			continue
		case eventDone:
			return true, nil
		case eventError:
			return true, errors.New(string(msg.Data()))
		}
		if err := fn(string(msg.Data())); err != nil {
			return true, err
		}
	}
}
