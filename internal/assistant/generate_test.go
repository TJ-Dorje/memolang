package assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"memolang/internal/ai"
	"memolang/internal/models"
)

func scanAll(chunks ...string) []Card {
	var s cardScanner
	var cards []Card
	for _, c := range chunks {
		cards = append(cards, s.Write(c)...)
	}
	return cards
}

func fronts(cards []Card) string {
	var out []string
	for _, c := range cards {
		out = append(out, c.Front)
	}
	return strings.Join(out, "|")
}

const twoCards = "```json\n[\n  {\"front\": \"Grüezi\", \"back\": \"Hello\", \"example\": \"Grüezi mitenand! (Hello everyone!)\"},\n  {\"front\": \"Tschüss\", \"back\": \"Bye\", \"example\": \"Er sagt \\\"Tschüss}\\\" {laut}. (He says bye loudly.)\"}\n]\n```"

func TestScannerReadsWholeArray(t *testing.T) {
	cards := scanAll(twoCards)
	if fronts(cards) != "Grüezi|Tschüss" {
		t.Fatalf("cards = %+v", cards)
	}
	// Braces and escaped quotes inside strings are text, not structure.
	if cards[1].Example != `Er sagt "Tschüss}" {laut}. (He says bye loudly.)` {
		t.Errorf("example = %q", cards[1].Example)
	}
}

// However the stream is split — one byte at a time included — the same
// cards come out, each as soon as its object closes.
func TestScannerAnySplit(t *testing.T) {
	var bytes []string
	for i := range len(twoCards) {
		bytes = append(bytes, twoCards[i:i+1])
	}
	if got := fronts(scanAll(bytes...)); got != "Grüezi|Tschüss" {
		t.Errorf("byte-by-byte = %q", got)
	}

	var s cardScanner
	first := s.Write(twoCards[:strings.Index(twoCards, "},")+1])
	if fronts(first) != "Grüezi" {
		t.Errorf("first card not emitted as soon as it closed: %+v", first)
	}
}

// The reply that started all this: cut off mid-card. Every complete card
// before the cut is kept.
func TestScannerCutOffReply(t *testing.T) {
	cut := "\n\n[\n  {\"front\": \"Guten Tag\", \"back\": \"Hello\", \"example\": \"Guten Tag! (Hello!)\"},\n  {\"front\": \"umsteigen\", \"back\": \"to transfer\", \"example\": \"Müssen Sie in Zürich umsteigen?"
	if got := fronts(scanAll(cut)); got != "Guten Tag" {
		t.Errorf("cards from a cut-off reply = %q, want the one complete card", got)
	}
}

func TestScannerSkipsIncompleteCards(t *testing.T) {
	got := scanAll(`[{"front":"a","back":""},{"front":" b ","back":"B"},{"nonsense":1}]`)
	if fronts(got) != "b" {
		t.Errorf("cards = %+v, want only the card with front and back, trimmed", got)
	}
}

func TestScannerWrappedArray(t *testing.T) {
	if got := fronts(scanAll(`{"cards": [{"front":"a","back":"A"}]}`)); got != "a" {
		t.Errorf("wrapped array = %q", got)
	}
}

// generation runs GenerateDeck for the fixture's user and collects the
// events and the saved cards.
func (f fixture) generation(t *testing.T, spec DeckSpec) (int64, []GenerationEvent) {
	t.Helper()
	deckID, err := f.svc.GenerateDeck(f.userID, spec)
	if err != nil {
		t.Fatalf("GenerateDeck: %v", err)
	}
	var raw strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f.svc.Broker.Follow(ctx, GenerationKey(deckID), func(c string) error {
		raw.WriteString(c)
		return nil
	})

	var events []GenerationEvent
	for _, line := range strings.Split(strings.TrimSpace(raw.String()), "\n") {
		var ev GenerationEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("event %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return deckID, events
}

func eventTypes(events []GenerationEvent) string {
	var out []string
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return strings.Join(out, ",")
}

var spanishSpec = DeckSpec{Name: "Travel", Language: "Spanish", Prompt: "travel", Count: 2, Mode: "linear"}

// thinkingProvider streams reasoning first, like Qwen3, then the reply.
type thinkingProvider struct {
	fakeProvider
	thoughts []string
}

func (p *thinkingProvider) Chat(ctx context.Context, req ai.ChatRequest, onToken ai.TokenFunc) (string, error) {
	for _, th := range p.thoughts {
		req.OnThinking(th)
	}
	return p.fakeProvider.Chat(ctx, req, onToken)
}

func TestGenerateDeckSavesCardsAsTheyArrive(t *testing.T) {
	p := &thinkingProvider{
		fakeProvider: fakeProvider{reply: []string{`[{"front":"hola","back":"hi"},`, `{"front":"adiós","back":"bye"}]`}},
		thoughts:     []string{"Let me think", " some more"},
	}
	f := setup(t, p)
	models.GetOrCreateDeckBuilderConversation(f.db, f.userID)

	deckID, events := f.generation(t, spanishSpec)

	if got := eventTypes(events); got != "thinking,card,card,done" {
		t.Errorf("events = %s, want one thinking, a card each, done", got)
	}
	deck, err := models.GetDeckByID(f.db, f.userID, deckID)
	if err != nil || deck.CardCount != 2 || deck.Mode != "linear" || deck.Name != "Travel" {
		t.Errorf("deck = %+v, %v", deck, err)
	}
	if !strings.Contains(p.request().System, "flashcard generator") || p.request().MaxTokens != generationMaxTokens {
		t.Errorf("request = %+v", p.request())
	}
	if id, _ := models.FindDeckBuilderConversation(f.db, f.userID); id != 0 {
		t.Error("the finished interview was not cleared")
	}
}

func TestGenerateDeckKeepsCardsWhenCutShort(t *testing.T) {
	p := &fakeProvider{reply: []string{`[{"front":"hola","back":"hi"},{"front":"adi`}, err: ErrIdle{After: time.Minute}}
	f := setup(t, p)

	deckID, events := f.generation(t, spanishSpec)

	last := events[len(events)-1]
	if last.Type != "done" || last.Saved != 1 || !strings.Contains(last.Message, "stopped early") {
		t.Errorf("last event = %+v, want done with 1 card and a note", last)
	}
	if deck, _ := models.GetDeckByID(f.db, f.userID, deckID); deck.CardCount != 1 {
		t.Errorf("deck has %d cards, want the 1 complete card", deck.CardCount)
	}
}

func TestGenerateDeckWithNothingUsable(t *testing.T) {
	p := &fakeProvider{err: errors.New("server exploded")}
	f := setup(t, p)
	convID, _ := models.GetOrCreateDeckBuilderConversation(f.db, f.userID)

	deckID, events := f.generation(t, spanishSpec)

	if got := eventTypes(events); got != "failed" || !strings.Contains(events[0].Message, "server exploded") {
		t.Errorf("events = %+v", events)
	}
	if _, err := models.GetDeckByID(f.db, f.userID, deckID); err == nil {
		t.Error("the empty deck was kept")
	}
	msgs, _ := models.GetConversationMessages(f.db, f.userID, convID)
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "server exploded") {
		t.Errorf("chat = %+v, want the failure noted in it", msgs)
	}
}

func TestGenerateDeckNotConfigured(t *testing.T) {
	f := setup(t, nil)
	f.svc.Provider = func(*sql.DB, int64) (ai.Provider, error) { return nil, ai.ErrNotConfigured }

	if _, err := f.svc.GenerateDeck(f.userID, spanishSpec); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
	if decks, _ := models.GetAllDecks(f.db, f.userID); len(decks) != 1 {
		t.Errorf("decks = %d, want only the fixture's: no empty deck for an unconfigured user", len(decks))
	}
}

// slowThinker reasons in bursts, each gap shorter than the idle limit but
// the whole well beyond it — the Qwen3 case that the old fixed 3-minute
// deadline cut off.
type slowThinker struct {
	fakeProvider
	bursts int
	gap    time.Duration
}

func (p *slowThinker) Chat(ctx context.Context, req ai.ChatRequest, onToken ai.TokenFunc) (string, error) {
	for range p.bursts {
		select {
		case <-time.After(p.gap):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		req.OnThinking("still thinking")
	}
	return p.fakeProvider.Chat(ctx, req, onToken)
}

func TestLongThinkingIsNotCutOff(t *testing.T) {
	p := &slowThinker{
		fakeProvider: fakeProvider{reply: []string{`[{"front":"hola","back":"hi"}]`}},
		bursts:       6,
		gap:          30 * time.Millisecond,
	}
	f := setup(t, p)
	f.svc.IdleTimeout = 60 * time.Millisecond // total thinking ≈ 180ms

	_, events := f.generation(t, spanishSpec)
	if last := events[len(events)-1]; last.Type != "done" || last.Saved != 1 {
		t.Errorf("last event = %+v, want done: reasoning should keep the call alive", last)
	}
}
