package ai

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// ChatMessage is one turn of a conversation.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// ChatRequest is a whole conversation to continue. It is a struct rather than
// loose arguments so later capabilities (tools, for the card-maintenance
// agent) can be added without changing every Chat signature.
type ChatRequest struct {
	System   string
	Messages []ChatMessage
	// MaxTokens caps the reply; 0 means the transport's default.
	MaxTokens int
}

// TokenFunc receives reply text as the model generates it. Returning an error
// aborts the stream, and Chat returns that error.
type TokenFunc func(chunk string) error

// defaultChatMaxTokens bounds a reply when the request does not. A tutor
// answer that runs past this is rambling, and on a paid provider it is also
// the cost ceiling for one message.
const defaultChatMaxTokens = 1024

// errStreamDone ends readSSE early without being an error.
var errStreamDone = errors.New("stream done")

// maxSSELine is the longest single event line accepted. Streaming chunks are
// small; this only stops a misbehaving server from growing the buffer forever.
const maxSSELine = 1 << 20

// readSSE parses a server-sent-events stream and calls onEvent once per event,
// with its "event:" name (empty when the server sends none, as OpenAI does)
// and its "data:" payload. Both providers stream this way; they differ only in
// what the JSON inside the data means. onEvent returning errStreamDone stops
// reading cleanly.
func readSSE(r io.Reader, onEvent func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxSSELine)

	var event string
	var data []string
	dispatch := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		err := onEvent(event, strings.Join(data, "\n"))
		event, data = "", nil
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return stopOK(err)
			}
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		// Anything else (": keep-alive" comments, id:, retry:) is ignored.
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	// A stream may end without a trailing blank line.
	return stopOK(dispatch())
}

func stopOK(err error) error {
	if errors.Is(err, errStreamDone) {
		return nil
	}
	return err
}
