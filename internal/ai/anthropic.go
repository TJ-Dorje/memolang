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

type anthropic struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func newAnthropic(cfg Config) *anthropic {
	url := cfg.BaseURL
	if url == "" {
		url = "https://api.anthropic.com"
	}
	url = strings.TrimRight(url, "/")
	return &anthropic{
		baseURL: url,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		http:    &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *anthropic) GenerateCards(ctx context.Context, language, promptText string) ([]CardData, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	reqBody := anthropicRequest{
		Model:     a.model,
		MaxTokens: 8192,
		System:    systemPrompt(language),
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt(language, promptText)},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var anthroResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if anthroResp.Error != nil {
		return nil, fmt.Errorf("api error: %s", anthroResp.Error.Message)
	}

	var fullText strings.Builder
	for _, block := range anthroResp.Content {
		if block.Type == "text" {
			fullText.WriteString(block.Text)
		}
	}

	return parseCards(fullText.String())
}

func (a *anthropic) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	reqBody := anthropicRequest{
		Model:     a.model,
		MaxTokens: 1,
		Messages: []anthropicMessage{
			{Role: "user", Content: "ping"},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
