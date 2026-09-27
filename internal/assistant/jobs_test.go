package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"memolang/internal/models"
)

// queuedReply creates a reply placeholder, as Ask does, without running it —
// a job waiting for a worker.
func (f fixture) queuedReply(t *testing.T) Job {
	t.Helper()
	convID, _ := models.GetOrCreateTutorConversation(f.db, f.userID, f.card.ID)
	models.AddConversationMessage(f.db, convID, "user", "q", "done")
	replyID, err := models.AddConversationMessage(f.db, convID, "assistant", "", "generating")
	if err != nil {
		t.Fatal(err)
	}
	f.svc.Broker.Start(ReplyKey(replyID))
	return Job{Kind: JobReply, UserID: f.userID, ReplyID: replyID, System: "s"}
}

func (f fixture) replyStatus(t *testing.T, id int64) string {
	t.Helper()
	status, err := models.GetConversationMessageStatus(f.db, id)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A redelivered reply — its first worker died mid-stream — is not generated
// again on top of the text already shown: it is marked interrupted.
func TestRedeliveredReplyIsInterrupted(t *testing.T) {
	p := &fakeProvider{reply: []string{"should not run"}}
	f := setup(t, p)
	job := f.queuedReply(t)

	if err := f.svc.RunJob(context.Background(), job, 2); err != nil {
		t.Fatal(err)
	}
	if got := f.replyStatus(t, job.ReplyID); got != "error" {
		t.Errorf("status = %q, want error (interrupted)", got)
	}
	if p.request().System != "" {
		t.Error("the model was called for a redelivered reply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := f.svc.Broker.Follow(ctx, ReplyKey(job.ReplyID), func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("followers got %v, want the interruption", err)
	}
}

// A reply that finished, whose acknowledgement was lost, is left alone.
func TestFinishedReplyIsNotRedone(t *testing.T) {
	p := &fakeProvider{reply: []string{"again?"}}
	f := setup(t, p)
	job := f.queuedReply(t)
	models.FinishConversationMessage(f.db, job.ReplyID, "the answer", "done")

	if err := f.svc.RunJob(context.Background(), job, 2); err != nil {
		t.Fatal(err)
	}
	if got := f.replyStatus(t, job.ReplyID); got != "done" {
		t.Errorf("status = %q, want done left as it was", got)
	}
}

// A redelivered generation resumes: cards the first run saved stay, are not
// duplicated, and count toward the result.
func TestRedeliveredGenerationResumes(t *testing.T) {
	p := &fakeProvider{reply: []string{`[{"front":"hola","back":"hi"},{"front":"adiós","back":"bye"}]`}}
	f := setup(t, p)
	deck, _ := models.CreateDeck(f.db, f.userID, "Travel", "srs")
	models.CreateCard(f.db, deck.ID, "hola", "hi", "", "") // saved by the lost first run
	f.svc.Broker.Start(GenerationKey(deck.ID))

	job := Job{Kind: JobGenerate, UserID: f.userID, DeckID: deck.ID, Spec: &spanishSpec}
	if err := f.svc.RunJob(context.Background(), job, 2); err != nil {
		t.Fatal(err)
	}

	if n, _ := models.CountCards(f.db, deck.ID); n != 2 {
		t.Errorf("deck has %d cards, want 2: the saved one kept, the new one added, no duplicate", n)
	}
	var last GenerationEvent
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	f.svc.Broker.Follow(ctx, GenerationKey(deck.ID), func(chunk string) error {
		for _, line := range strings.Split(strings.TrimSpace(chunk), "\n") {
			jsonUnmarshal(t, line, &last)
		}
		return nil
	})
	if last.Type != "done" || last.Saved != 2 {
		t.Errorf("last event = %+v, want done with 2 cards in total", last)
	}
}

func TestUnknownJobKind(t *testing.T) {
	f := setup(t, &fakeProvider{})
	if err := f.svc.RunJob(context.Background(), Job{Kind: "mystery"}, 1); err == nil {
		t.Error("an unknown job kind was accepted")
	}
}

func jsonUnmarshal(t *testing.T, line string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(line), v); err != nil {
		t.Fatalf("event %q: %v", line, err)
	}
}
