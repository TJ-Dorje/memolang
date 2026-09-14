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
	baseURL := os.Getenv("LIVE_LLM_BASE_URL")
	if baseURL == "" {
		t.Skip("set LIVE_LLM_BASE_URL to run against a real provider")
	}

	provider := os.Getenv("LIVE_LLM_PROVIDER")
	if provider == "" {
		provider = "custom"
	}

	cfg := Config{
		Provider: provider,
		BaseURL:  baseURL,
		Model:    os.Getenv("LIVE_LLM_MODEL"),
		APIKey:   os.Getenv("LLM_API_KEY"),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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
