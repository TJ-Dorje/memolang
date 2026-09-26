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

// openAIStreamChunk is one `data:` event of a streamed chat completion.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat streams a reply from POST /chat/completions with stream: true. The
// server answers with server-sent events, each carrying a JSON chunk whose
// choices[0].delta.content is the next piece of text, and ends with the
// literal event `data: [DONE]`.
func (c *openAICompat) Chat(ctx context.Context, req ChatRequest, onToken TokenFunc) (string, error) {
	messages := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, chatMessage{Role: m.Role, Content: m.Content})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultChatMaxTokens
	}

	body, err := json.Marshal(chatRequest{
		Model:     c.model,
		Messages:  messages,
		Stream:    true,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
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
		if data == "[DONE]" {
			return errStreamDone
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("api error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			return nil // role-only or keep-alive chunk
		}
		text := chunk.Choices[0].Delta.Content
		reply.WriteString(text)
		return onToken(text)
	})
	if err != nil {
		return reply.String(), err
	}
	return reply.String(), nil
}
