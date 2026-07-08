package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompat_GenerateCards(t *testing.T) {
	cardsJSON := `[{"front":"apple","back":"Apfel","example":"I eat an apple"}]`
	
	tests := []struct {
		name           string
		apiKey         string
		serverHandler  http.HandlerFunc
		expectCardsLen int
		expectErr      string
		expectAuth     bool
	}{
		{
			name:   "Happy path",
			apiKey: "sk-test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer sk-test" {
					t.Errorf("expected Authorization header Bearer sk-test, got %q", r.Header.Get("Authorization"))
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(chatResponse{
					Choices: []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					}{{
						Message: struct {
							Content string `json:"content"`
						}{Content: cardsJSON},
					}},
				})
			},
			expectCardsLen: 1,
			expectAuth:     true,
		},
		{
			name:   "No API key",
			apiKey: "",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Errorf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(chatResponse{
					Choices: []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					}{{
						Message: struct {
							Content string `json:"content"`
						}{Content: cardsJSON},
					}},
				})
			},
			expectCardsLen: 1,
			expectAuth:     false,
		},
		{
			name:   "API error 401",
			apiKey: "invalid",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			},
			expectErr: "api error (status 401): {\"error\":{\"message\":\"invalid api key\"}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverHandler)
			defer srv.Close()

			provider := newOpenAICompat(Config{
				BaseURL: srv.URL,
				APIKey:  tt.apiKey,
				Model:   "test-model",
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

func TestOpenAICompat_Ping(t *testing.T) {
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

			provider := newOpenAICompat(Config{
				BaseURL: srv.URL,
				Model:   "test-model",
			})

			err := provider.Ping(context.Background())

			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
		})
	}
}
