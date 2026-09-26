package helpers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FakeLLMPause is how long the fake LLM waits after its first chunk. It is
// what lets a test observe a reply mid-stream: the first chunk is on the page
// while the rest has not been sent yet.
const FakeLLMPause = 1500 * time.Millisecond

// FakeLLMFirstChunk opens every fake reply.
const FakeLLMFirstChunk = "Hola! "

// FakeLLMHandler is an OpenAI-compatible chat endpoint that streams a canned
// reply echoing the learner's last message, so e2e cases exercise the real
// transport, broker and streaming page without a model. The reply includes
// markup to prove it is rendered as text.
func FakeLLMHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		last := ""
		if n := len(req.Messages); n > 0 {
			last = req.Messages[n-1].Content
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(text string) {
			chunk, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]string{"content": text}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		send(FakeLLMFirstChunk)
		time.Sleep(FakeLLMPause)
		send("You asked: " + last + " ")
		send("<b>bold</b> stays text.")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	return mux
}
