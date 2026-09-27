package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
