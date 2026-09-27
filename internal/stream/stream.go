// Package stream carries text that is still being generated (an LLM reply)
// from the goroutine producing it to the requests displaying it.
//
// The producer and the reader are deliberately decoupled: the reply is
// generated in the background, and a page request only follows it. Closing
// the tab does not cancel a paid LLM call halfway, and reloading the page
// reattaches to the same reply instead of starting a second one.
package stream

import (
	"context"
	"sync"
	"time"
)

// Broker is the seam for swapping transports. Memory is enough for a single
// process; a multi-replica deployment would implement this over NATS
// JetStream (a subject per key, replay from the start) without the tutor or
// the handlers changing.
type Broker interface {
	// Start registers key as in progress. Follow on an unknown key reports
	// found=false, so Start must come before the first reader can arrive.
	Start(key string)
	// Publish appends text to key's output.
	Publish(key, chunk string)
	// Finish marks key complete, with the error that ended it, if any.
	Finish(key string, err error)
	// Follow calls fn with key's output from the beginning, then with new
	// text as it arrives, until the producer finishes (returning its error),
	// fn fails, or ctx ends. found is false when key is not known.
	Follow(ctx context.Context, key string, fn func(chunk string) error) (found bool, err error)
	// Exists reports whether key is known (in progress, or finished within
	// the retention window), so a page can decide to stream or redirect
	// before it writes anything.
	Exists(key string) bool
}

// retainFinished is how long a finished entry stays followable. It covers a
// reader that arrives just after the producer finished, before it sees the
// saved result in the database.
const retainFinished = time.Minute

// Memory is an in-process Broker.
type Memory struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func NewMemory() *Memory {
	return &Memory{entries: make(map[string]*entry)}
}

// entry is one output: a growing text and a signal for readers. changed is
// closed and replaced on every update, so any number of readers can wait on
// it at once and none can miss an update; a slow reader simply finds more
// new text when it wakes. Nothing is ever pushed into a reader, so a slow
// reader cannot block the producer.
type entry struct {
	mu      sync.Mutex
	text    []byte
	done    bool
	err     error
	changed chan struct{}
}

func (m *Memory) Start(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = &entry{changed: make(chan struct{})}
}

func (m *Memory) get(key string) *entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[key]
}

func (m *Memory) Exists(key string) bool { return m.get(key) != nil }

func (m *Memory) Publish(key, chunk string) {
	e := m.get(key)
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.done {
		return
	}
	e.text = append(e.text, chunk...)
	e.signal()
}

func (m *Memory) Finish(key string, err error) {
	e := m.get(key)
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.done {
		e.mu.Unlock()
		return
	}
	e.done, e.err = true, err
	e.signal()
	e.mu.Unlock()

	time.AfterFunc(retainFinished, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.entries[key] == e {
			delete(m.entries, key)
		}
	})
}

// signal wakes every waiting reader. Callers hold e.mu.
func (e *entry) signal() {
	close(e.changed)
	e.changed = make(chan struct{})
}

func (m *Memory) Follow(ctx context.Context, key string, fn func(chunk string) error) (bool, error) {
	e := m.get(key)
	if e == nil {
		return false, nil
	}

	sent := 0
	for {
		e.mu.Lock()
		pending := string(e.text[sent:])
		done, doneErr := e.done, e.err
		wait := e.changed
		e.mu.Unlock()

		if pending != "" {
			if err := fn(pending); err != nil {
				return true, err
			}
			sent += len(pending)
		}
		if done {
			return true, doneErr
		}

		select {
		case <-wait:
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
}
