package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompat_Ping(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		expectErr     bool
	}{
		{
			name: "Success",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
			},
			expectErr: false,
		},
		{
			// A bare 200 used to count as success, which let a wrong base
			// URL pass Test Connection.
			name: "Empty 200 is not success",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectErr: true,
		},
		{
			// LM Studio on a base URL missing /v1: every route answers 200
			// with a JSON error.
			name: "200 carrying an error is not success",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"error":"Unexpected endpoint or method. (` + r.Method + ` ` + r.URL.Path + `)"}`))
			},
			expectErr: true,
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
