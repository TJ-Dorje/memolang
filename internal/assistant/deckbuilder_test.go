package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"memolang/internal/models"
)

func (f fixture) openBuilder() (int64, error) {
	return models.GetOrCreateDeckBuilderConversation(f.db, f.userID)
}

func TestSummarizeDeck(t *testing.T) {
	p := &fakeProvider{reply: []string{"Interview reply."}}
	f := setup(t, p)

	if err := f.svc.Ask(f.userID, f.openBuilder, DeckBuilderPrompt(), "Spanish, for a trip to Mexico"); err != nil {
		t.Fatal(err)
	}
	id, _ := models.FindDeckBuilderConversation(f.db, f.userID)
	msgs, _ := models.GetConversationMessages(f.db, f.userID, id)
	f.svc.Broker.Follow(context.Background(), ReplyKey(msgs[len(msgs)-1].ID), func(string) error { return nil })

	p.reply = []string{"Here you go:\n```json\n", `{"name":"Spanish — Travel","language":"Spanish","prompt":"Travel phrases for Mexico, beginner.","count":15}`, "\n```"}
	spec, err := f.svc.SummarizeDeck(context.Background(), f.userID, id)
	if err != nil {
		t.Fatalf("SummarizeDeck: %v", err)
	}
	want := DeckSpec{Name: "Spanish — Travel", Language: "Spanish", Prompt: "Travel phrases for Mexico, beginner.", Count: 15}
	if spec != want {
		t.Errorf("spec = %+v, want %+v", spec, want)
	}

	sent := p.request()
	if len(sent.Messages) != 1 {
		t.Fatalf("summary sent %d messages, want the transcript as one", len(sent.Messages))
	}
	transcript := sent.Messages[0].Content
	for _, part := range []string{"Assistant: " + DeckBuilderGreeting, "Learner: Spanish, for a trip to Mexico", "Assistant: Interview reply."} {
		if !strings.Contains(transcript, part) {
			t.Errorf("transcript lacks %q:\n%s", part, transcript)
		}
	}
}

func TestSummarizeEmptyConversation(t *testing.T) {
	f := setup(t, &fakeProvider{})
	id, _ := f.openBuilder()
	if _, err := f.svc.SummarizeDeck(context.Background(), f.userID, id); !errors.Is(err, ErrNothingToSummarize) {
		t.Errorf("err = %v, want ErrNothingToSummarize", err)
	}
}

func TestParseDeckSpecDefaultsAndClamps(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want DeckSpec
	}{
		{`{"language":"German","prompt":"verbs"}`, DeckSpec{Name: "German deck", Language: "German", Prompt: "verbs", Count: 20}},
		{`{"name":"x","language":"L","prompt":"p","count":500}`, DeckSpec{Name: "x", Language: "L", Prompt: "p", Count: 50}},
		{`{"name":"x","language":"L","prompt":"p","count":1}`, DeckSpec{Name: "x", Language: "L", Prompt: "p", Count: 5}},
		{`Sure! {"name":" y ","language":"L","prompt":"p","count":10} Hope that helps.`, DeckSpec{Name: "y", Language: "L", Prompt: "p", Count: 10}},
	} {
		got, err := parseDeckSpec(tc.raw)
		if err != nil {
			t.Errorf("parseDeckSpec(%q): %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDeckSpec(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}

	if _, err := parseDeckSpec("I could not do that."); err == nil {
		t.Error("parseDeckSpec accepted text with no JSON")
	}
}

func TestGenerationPromptCarriesCount(t *testing.T) {
	got := DeckSpec{Prompt: "Kitchen verbs.", Count: 12}.GenerationPrompt()
	if !strings.HasSuffix(got, "Generate 12 cards.") || !strings.HasPrefix(got, "Kitchen verbs.") {
		t.Errorf("GenerationPrompt = %q", got)
	}
}
