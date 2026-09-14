package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestPingUsesModelsEndpoint is the regression guard for the cold-start bug:
// Ping must not provoke inference when the server can answer GET /models,
// because a lazily-loaded local model turns that into a 30s+ wait that blew
// the old 5s deadline and reported a working setup as broken.
func TestPingUsesModelsEndpoint(t *testing.T) {
	var completions atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
			return
		}
		completions.Add(1)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	client := newOpenAICompat(Config{BaseURL: srv.URL, Model: "test-model"})
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	if n := completions.Load(); n != 0 {
		t.Errorf("Ping sent %d completion request(s); it must only list models", n)
	}
}

// TestPingFallsBackWithoutModelsEndpoint: not every OpenAI-compatible server
// implements /models, and those must still be testable.
func TestPingFallsBackWithoutModelsEndpoint(t *testing.T) {
	var completions atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			http.NotFound(w, r)
			return
		}
		completions.Add(1)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	client := newOpenAICompat(Config{BaseURL: srv.URL, Model: "test-model"})
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	if n := completions.Load(); n != 1 {
		t.Errorf("expected exactly one fallback completion, got %d", n)
	}
}

// TestPingReportsUnlistedModel: a wrong model id should surface at Test
// Connection, not three minutes into a generation.
func TestPingReportsUnlistedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"gemini-3.6-flash"},{"id":"gemini-3.6-pro"}]}`))
	}))
	defer srv.Close()

	client := newOpenAICompat(Config{BaseURL: srv.URL, Model: "gemini-2.0-flash"})
	err := client.Ping(context.Background())

	var notListed *ModelNotListedError
	if !errors.As(err, &notListed) {
		t.Fatalf("expected a ModelNotListedError, got %v", err)
	}
	if notListed.Model != "gemini-2.0-flash" {
		t.Errorf("Model = %q, want the configured id", notListed.Model)
	}
	if len(notListed.Available) != 2 {
		t.Errorf("Available = %v, want both advertised ids", notListed.Available)
	}
}

// TestPingFailsOnBadCredentials: a real failure must stay a real failure and
// not be softened into a warning.
func TestPingFailsOnBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid api key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newOpenAICompat(Config{BaseURL: srv.URL, Model: "test-model"})
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected an error for a 401")
	}

	var notListed *ModelNotListedError
	if errors.As(err, &notListed) {
		t.Errorf("a 401 was reported as a model warning: %v", err)
	}
}

func TestCheckModelListed(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		available []string
		wantErr   bool
	}{
		{"exact match", "gpt-4o-mini", []string{"gpt-4o", "gpt-4o-mini"}, false},
		{"namespaced id still matches", "gemini-3.6-flash", []string{"models/gemini-3.6-flash"}, false},
		{"absent", "gpt-5", []string{"gpt-4o"}, true},
		// An empty listing is not evidence of anything, so it must not warn.
		{"empty listing", "anything", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkModelListed(tt.model, tt.available)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkModelListed(%q, %v) = %v, wantErr %v", tt.model, tt.available, err, tt.wantErr)
			}
		})
	}
}
