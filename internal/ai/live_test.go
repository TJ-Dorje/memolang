package ai

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestLiveGeneration runs the real GenerateCards path against a real server.
// It is skipped unless LIVE_LLM_BASE_URL is set, so the normal suite stays
// hermetic and offline:
//
//	LIVE_LLM_BASE_URL=http://localhost:11434/v1 \
//	LIVE_LLM_MODEL=qwen/qwen3.8-27b \
//	go test ./internal/ai -run TestLiveGeneration -v
//
// LIVE_LLM_PROVIDER (default "custom") selects the preset, and LLM_API_KEY is
// picked up for hosted providers. It exists because the unit tests mock the
// transport, so nothing else checks the thing most likely to break in
// practice: whether a given model actually returns JSON this app can parse.
func TestLiveGeneration(t *testing.T) {
	client := liveClient(t)

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	cards, err := client.GenerateCards(context.Background(), "Spanish", "5 common kitchen verbs")
	if err != nil {
		t.Fatalf("GenerateCards: %v", err)
	}

	if len(cards) == 0 {
		t.Fatal("no cards returned")
	}

	for i, c := range cards {
		if c.Front == "" || c.Back == "" {
			t.Errorf("card %d has an empty front or back: %+v", i, c)
		}
		// The system prompt asks for "Target sentence (English translation)".
		// A model that ignores this still parses, but the example is useless
		// on the card, so it is worth failing on.
		if c.Example != "" && !strings.Contains(c.Example, "(") {
			t.Errorf("card %d example is missing the (translation) part: %q", i, c.Example)
		}
		t.Logf("%-12s %-16s %s", c.Front, c.Back, c.Example)
	}
}

// TestLiveChat streams a real tutor-style reply. The unit tests feed the
// parsers event streams written from the providers' documentation; this is
// the check that a real server's stream actually looks like that.
//
//	LIVE_LLM_BASE_URL=... go test ./internal/ai -run TestLiveChat -v
func TestLiveChat(t *testing.T) {
	client := liveClient(t)

	var chunks int
	reply, err := client.Chat(context.Background(), ChatRequest{
		System:   "You are a concise Spanish tutor. Answer in one or two sentences.",
		Messages: []ChatMessage{{Role: "user", Content: "What does 'hablar' mean?"}},
	}, func(string) error {
		chunks++
		return nil
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.TrimSpace(reply) == "" {
		t.Fatal("empty reply")
	}
	// One chunk would mean the reply arrived whole: not streamed at all.
	if chunks < 2 {
		t.Errorf("reply arrived in %d chunk(s); expected it streamed in pieces", chunks)
	}
	t.Logf("%d chunks: %s", chunks, reply)
}

// liveClient builds a provider from LIVE_LLM_* variables, skipping the test
// when LIVE_LLM_BASE_URL is unset. LIVE_LLM_PROVIDER (default "custom")
// selects the preset; LLM_API_KEY is picked up for hosted providers.
func liveClient(t *testing.T) Provider {
	t.Helper()
	baseURL := os.Getenv("LIVE_LLM_BASE_URL")
	if baseURL == "" {
		t.Skip("set LIVE_LLM_BASE_URL to run against a real provider")
	}

	provider := os.Getenv("LIVE_LLM_PROVIDER")
	if provider == "" {
		provider = "custom"
	}

	client, err := New(Config{
		Provider: provider,
		BaseURL:  baseURL,
		Model:    os.Getenv("LIVE_LLM_MODEL"),
		APIKey:   os.Getenv("LLM_API_KEY"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}
