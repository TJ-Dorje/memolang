package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropicStreamRequest struct {
	anthropicRequest
	Stream bool `json:"stream"`
}

// anthropicStreamEvent covers the fields of the stream events we act on.
// Text arrives in content_block_delta events whose delta is a text_delta;
// message_start, content_block_start/stop, message_delta and ping carry
// nothing the reply needs.
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat streams a reply from POST /v1/messages with stream: true.
func (a *anthropic) Chat(ctx context.Context, req ChatRequest, onToken TokenFunc) (string, error) {
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultChatMaxTokens
	}

	body, err := json.Marshal(anthropicStreamRequest{
		anthropicRequest: anthropicRequest{
			Model:     a.model,
			MaxTokens: maxTokens,
			System:    req.System,
			Messages:  messages,
		},
		Stream: true,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return "", fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var reply strings.Builder
	err = readSSE(resp.Body, func(_, data string) error {
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return fmt.Errorf("decode stream event: %w", err)
		}
		switch ev.Type {
		case "error":
			if ev.Error != nil {
				return fmt.Errorf("api error: %s", ev.Error.Message)
			}
			return fmt.Errorf("api error")
		case "message_stop":
			return errStreamDone
		case "content_block_delta":
			if ev.Delta.Type != "text_delta" || ev.Delta.Text == "" {
				return nil
			}
			reply.WriteString(ev.Delta.Text)
			return onToken(ev.Delta.Text)
		}
		return nil
	})
	if err != nil {
		return reply.String(), err
	}
	return reply.String(), nil
}
