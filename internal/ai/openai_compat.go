package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAICompat struct {
	baseURL string
	apiKey  string
	model   string
	noThink bool
	http    *http.Client
}

func newOpenAICompat(cfg Config) *openAICompat {
	url := cfg.BaseURL
	url = strings.TrimRight(url, "/")
	return &openAICompat{
		baseURL: url,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		noThink: cfg.DisableThinking,
		http:    &http.Client{},
	}
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// modelsResponse is the GET /models payload. Only the ids matter, plus an
// error field: some servers answer an unknown route with 200 and a JSON
// error, which must not read as "no models, all fine". It is raw because
// servers disagree on its shape (a string or an object).
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Error json.RawMessage `json:"error"`
}

// Ping verifies the configuration without provoking inference.
//
// It asks GET /models first. That endpoint is cheap, answers immediately, and
// still proves the three things that actually go wrong: the server is
// reachable, the base URL is right, and the key is accepted. It also lets us
// check the configured model exists, which a completion request cannot do
// until it has already failed.
//
// The previous implementation sent a real completion with a 5-second budget.
// Local servers such as LM Studio and Ollama load weights lazily, so the first
// request after startup spends tens of seconds paging gigabytes off disk and
// blew that deadline — reporting "connection failed" for a configuration that
// was entirely correct, and which then succeeded on a second press because the
// failed attempt had started the load.
//
// Not every OpenAI-compatible server implements /models, so a completion ping
// remains as a fallback. It gets a longer budget precisely because that path
// may have to sit through a cold model load.
func (c *openAICompat) Ping(ctx context.Context) error {
	models, err := c.listModels(ctx)
	if err == nil {
		return checkModelListed(c.model, models)
	}
	return c.pingCompletion(ctx)
}

// listModels returns the model ids the server advertises.
func (c *openAICompat) listModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Error) > 0 && string(parsed.Error) != "null" {
		return nil, fmt.Errorf("api error: %s", parsed.Error)
	}

	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// checkModelListed reports a mismatch as a ModelNotListedError, which callers
// show as a warning rather than a failure. An empty list means the server
// answered but advertises nothing, which is not evidence the model is wrong.
func checkModelListed(model string, available []string) error {
	if len(available) == 0 {
		return nil
	}

	for _, id := range available {
		// Some servers namespace ids (e.g. "models/gemini-3.6-flash") while
		// still accepting the bare name, so match on either shape.
		if id == model || strings.HasSuffix(id, "/"+model) {
			return nil
		}
	}
	return &ModelNotListedError{Model: model, Available: available}
}

// pingCompletion is the fallback for servers with no /models endpoint. Its
// budget is generous because reaching it can mean waiting out a cold load.
func (c *openAICompat) pingCompletion(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: "ping"},
		},
		Stream:    false,
		MaxTokens: 1,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// A 200 is not enough: LM Studio answers an unknown route (a base URL
	// missing /v1) with 200 and a JSON error. A real completion has choices.
	var parsed struct {
		Choices []json.RawMessage `json:"choices"`
		Error   json.RawMessage   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("unexpected response: %w — check the base URL", err)
	}
	if len(parsed.Error) > 0 && string(parsed.Error) != "null" {
		return fmt.Errorf("api error: %s — check the base URL", parsed.Error)
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("the server returned no completion — check the base URL")
	}
	return nil
}
