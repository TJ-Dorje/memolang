package helpers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FakeLLMPause is how long the fake LLM waits after its first chunk. It is
// what lets a test observe a reply mid-stream: the first chunk is on the page
// while the rest has not been sent yet.
const FakeLLMPause = 1500 * time.Millisecond

// FakeLLMFirstChunk opens every fake chat reply.
const FakeLLMFirstChunk = "Hola! "

// The deck spec the fake returns when asked to summarise a deck-builder
// conversation, and the cards it returns when asked to generate them.
const (
	FakeDeckName     = "Spanish — Travel"
	FakeDeckLanguage = "Spanish"
	FakeDeckPrompt   = "Travel phrases for a beginner visiting Mexico."
	FakeDeckCount    = 15
)

var FakeCards = []map[string]string{
	{"front": "el billete", "back": "the ticket", "example": "Necesito un billete. (I need a ticket.)"},
	{"front": "la playa", "back": "the beach", "example": "Vamos a la playa. (Let's go to the beach.)"},
	{"front": "la cuenta", "back": "the bill", "example": "La cuenta, por favor. (The bill, please.)"},
}

type fakeRequest struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (r fakeRequest) system() string {
	if len(r.Messages) > 0 && r.Messages[0].Role == "system" {
		return r.Messages[0].Content
	}
	return ""
}

func (r fakeRequest) last() string {
	if n := len(r.Messages); n > 0 {
		return r.Messages[n-1].Content
	}
	return ""
}

// FakeLLMHandler is an OpenAI-compatible chat endpoint for e2e cases, so they
// exercise the real transports, broker and streaming pages without a model.
// It recognises the app's three kinds of request by their system prompts:
// card generation (JSON cards, not streamed), deck summarisation (a JSON
// spec) and chat (a streamed reply echoing the learner, including markup to
// prove it is rendered as text).
func FakeLLMHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req fakeRequest
		json.NewDecoder(r.Body).Decode(&req)
		system := req.system()

		// Summary first: its prompt also mentions "a flashcard generator".
		switch {
		case strings.Contains(system, "specification for generating vocabulary flashcards"):
			spec, _ := json.Marshal(map[string]any{
				"name": FakeDeckName, "language": FakeDeckLanguage,
				"prompt": FakeDeckPrompt, "count": FakeDeckCount,
			})
			writeCompletion(w, req.Stream, "```json\n"+string(spec)+"\n```")
		case strings.Contains(system, "You are a flashcard generator"):
			streamCards(w, req.last(), !strings.HasSuffix(system, "/no_think"))
		default:
			streamChat(w, req.last())
		}
	})

	// Like LM Studio behind a base URL missing /v1: every route answers 200
	// with a JSON error instead of a 404.
	mux.HandleFunc(FakeBrokenPrefix+"/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"error":"Unexpected endpoint or method. (%s %s)"}`, r.Method, strings.TrimPrefix(r.URL.Path, FakeBrokenPrefix))
	})
	return mux
}

// FakeBrokenPrefix, appended to the fake's URL, gives a base URL that
// behaves like a misconfigured LM Studio.
const FakeBrokenPrefix = "/broken"

// FakeBadPlanTrigger in the learner's message makes the fake end its reply
// with a plan block that is not valid JSON.
const FakeBadPlanTrigger = "garbled"

// writeCompletion answers in whichever shape the client asked for.
func writeCompletion(w http.ResponseWriter, stream bool, content string) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		sendChunk(w, content)
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
	})
}

// FakePlanTrigger in the learner's message makes the fake deck builder end
// its reply with a deck plan block.
const FakePlanTrigger = "ready"

// FakePlanMode is the study mode in the fake assistant's plan, deliberately
// not the default, to prove the plan's mode reaches the new deck.
const FakePlanMode = "linear"

func streamChat(w http.ResponseWriter, last string) {
	w.Header().Set("Content-Type", "text/event-stream")
	switch {
	case strings.Contains(last, FakeBadPlanTrigger):
		sendChunk(w, "Here is your plan.\n\n```deck\n{name: German, count: twenty}\n```")
		fmt.Fprint(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
		return
	case strings.Contains(last, FakeFailTrigger):
		streamPlanFor(w, FakeFailingPrompt)
		return
	case strings.Contains(last, FakeSlowTrigger):
		streamPlanFor(w, FakeSlowPrompt)
		return
	case strings.Contains(last, FakePlanTrigger):
		streamPlanFor(w, FakeDeckPrompt)
		return
	}
	// Leading and trailing blank lines, as Qwen3 models produce, which the
	// page must not render as empty space.
	sendChunk(w, "\n\n")
	sendChunk(w, FakeLLMFirstChunk)
	time.Sleep(FakeLLMPause)
	sendChunk(w, "You asked: "+last+" ")
	sendChunk(w, "<b>bold</b> stays text.")
	sendChunk(w, "\n\n")
	fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

// streamPlan replies like a deck builder with everything it needs: a short
// sentence, then the plan block — its opening marker split across chunks,
// as a real stream may split it, so the page's filter is exercised.
// streamPlanFor is streamPlan with a chosen generation prompt; the deck
// builder's fake uses FakeFailingPrompt to make generation fail.
func streamPlanFor(w http.ResponseWriter, prompt string) {
	spec, _ := json.Marshal(map[string]any{
		"name": FakeDeckName, "language": FakeDeckLanguage,
		"prompt": prompt, "count": FakeDeckCount, "mode": FakePlanMode,
	})
	// The pause keeps the reply in progress when the page loads, so the plan
	// really streams through the page's filter rather than being read back
	// finished from the database.
	sendChunk(w, "\nHere's your deck — check the plan and press Generate deck.\n\n")
	time.Sleep(FakeLLMPause)
	for _, chunk := range []string{"``", "`de", "ck\n", string(spec), "\n```"} {
		sendChunk(w, chunk)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

func sendChunk(w http.ResponseWriter, text string) {
	chunk, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"content": text}}},
	})
	fmt.Fprintf(w, "data: %s\n\n", chunk)
	w.(http.Flusher).Flush()
}

// FakeFailTrigger in the learner's message makes the fake deck builder
// propose a plan whose generation then fails (see FakeFailingPrompt).
const FakeFailTrigger = "doomed"

// FakeFailingPrompt, in a plan, makes the fake generator answer with no
// cards at all.
const FakeFailingPrompt = "A topic the generator refuses."

// streamCards answers a card-generation request like a thinking model on
// LM Studio: reasoning first (unless the request asked it not to think),
// then the JSON array split mid-card, with a pause after the first card so
// a test can see it on the page before the rest arrives.
func streamCards(w http.ResponseWriter, last string, thinking bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	if strings.Contains(last, FakeSlowPrompt) {
		streamSlowCards(w)
		return
	}
	if strings.Contains(last, FakeFailingPrompt) {
		// Paused, so the failure lands while the progress page is open. An
		// instant failure deletes the empty deck before the page loads, and
		// the page then sends the learner straight back to the chat.
		time.Sleep(FakeLLMPause)
		sendChunk(w, "Sorry, I can't make flashcards about that.")
		fmt.Fprint(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
		return
	}
	if thinking {
		sendReasoning(w, "The learner wants travel phrases. ")
		sendReasoning(w, "Twenty would be too many for a test.")
	}

	cards, _ := json.Marshal(FakeCards)
	text := "\n\n" + string(cards)
	firstCard := strings.Index(text, "},") + 1
	sendChunk(w, text[:firstCard+10]) // the first card, and a little of the second
	time.Sleep(FakeLLMPause)
	sendChunk(w, text[firstCard+10:])
	fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

func sendReasoning(w http.ResponseWriter, text string) {
	chunk, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]string{"reasoning_content": text}}},
	})
	fmt.Fprintf(w, "data: %s\n\n", chunk)
	w.(http.Flusher).Flush()
}

// FakeSlowTrigger in the learner's message makes the fake deck builder
// propose a plan whose generation is slow (see FakeSlowPrompt).
const FakeSlowTrigger = "slowly"

// FakeSlowPrompt, in a plan, makes the fake generator write FakeSlowCards
// one per second — time enough to kill a worker halfway through.
const FakeSlowPrompt = "A long topic, generated slowly."

// FakeSlowCards is what a slow generation produces.
var FakeSlowCards = func() []map[string]string {
	var cards []map[string]string
	for i := 1; i <= 8; i++ {
		cards = append(cards, map[string]string{
			"front": fmt.Sprintf("palabra %d", i), "back": fmt.Sprintf("word %d", i),
		})
	}
	return cards
}()

func streamSlowCards(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	sendChunk(w, "[")
	for i, card := range FakeSlowCards {
		obj, _ := json.Marshal(card)
		sep := ","
		if i == len(FakeSlowCards)-1 {
			sep = "]"
		}
		sendChunk(w, string(obj)+sep)
		time.Sleep(time.Second)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}
