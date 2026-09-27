package ai

import (
	"context"
	"os"
	"strings"
	"testing"
)

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
