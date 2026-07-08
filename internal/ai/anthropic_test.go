package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropic_GenerateCards(t *testing.T) {
	cardsJSON := `[{"front":"apple","back":"Apfel","example":"I eat an apple"}]`

	tests := []struct {
		name           string
		apiKey         string
		serverHandler  http.HandlerFunc
		expectCardsLen int
		expectErr      string
	}{
		{
			name:   "Happy path",
			apiKey: "sk-ant-test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "sk-ant-test" {
					t.Errorf("expected x-api-key sk-ant-test, got %q", r.Header.Get("x-api-key"))
				}
				if r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Errorf("expected anthropic-version 2023-06-01, got %q", r.Header.Get("anthropic-version"))
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(anthropicResponse{
					Content: []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					}{{
						Type: "text",
						Text: cardsJSON,
					}},
				})
			},
			expectCardsLen: 1,
		},
		{
			name:   "API error 400",
			apiKey: "invalid",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":{"message":"invalid request"}}`))
			},
			expectErr: "api error (status 400): {\"error\":{\"message\":\"invalid request\"}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverHandler)
			defer srv.Close()

			provider := newAnthropic(Config{
				BaseURL: srv.URL,
				APIKey:  tt.apiKey,
				Model:   "claude-3",
			})

			cards, err := provider.GenerateCards(context.Background(), "English", "Fruits")

			if tt.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tt.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(cards) != tt.expectCardsLen {
				t.Errorf("expected %d cards, got %d", tt.expectCardsLen, len(cards))
			}
		})
	}
}

func TestAnthropic_Ping(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		expectErr     bool
	}{
		{
			name: "Success",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectErr: false,
		},
		{
			name: "Failure",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal server error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverHandler)
			defer srv.Close()

			provider := newAnthropic(Config{
				BaseURL: srv.URL,
				APIKey:  "test",
				Model:   "claude-3",
			})

			err := provider.Ping(context.Background())

			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
		})
	}
}
