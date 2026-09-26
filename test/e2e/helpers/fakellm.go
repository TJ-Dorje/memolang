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
			cards, _ := json.Marshal(FakeCards)
			writeCompletion(w, req.Stream, string(cards))
		default:
			streamChat(w, req.last())
		}
	})
	return mux
}

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
	if strings.Contains(last, FakePlanTrigger) {
		streamPlan(w)
		return
	}
	sendChunk(w, FakeLLMFirstChunk)
	time.Sleep(FakeLLMPause)
	sendChunk(w, "You asked: "+last+" ")
	sendChunk(w, "<b>bold</b> stays text.")
	fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

// streamPlan replies like a deck builder with everything it needs: a short
// sentence, then the plan block — its opening marker split across chunks,
// as a real stream may split it, so the page's filter is exercised.
func streamPlan(w http.ResponseWriter) {
	spec, _ := json.Marshal(map[string]any{
		"name": FakeDeckName, "language": FakeDeckLanguage,
		"prompt": FakeDeckPrompt, "count": FakeDeckCount, "mode": FakePlanMode,
	})
	// The pause keeps the reply in progress when the page loads, so the plan
	// really streams through the page's filter rather than being read back
	// finished from the database.
	sendChunk(w, "Here's your deck — check the plan and press Generate deck.\n\n")
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
