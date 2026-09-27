package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer streams the given raw event-stream body, flushing after every
// event so the client really does receive it in pieces.
func sseServer(t *testing.T, check func(r *http.Request, body map[string]any), stream string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body)
		if check != nil {
			check(r, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range strings.SplitAfter(stream, "\n\n") {
			fmt.Fprint(w, event)
			w.(http.Flusher).Flush()
		}
	}))
}

func collect(chunks *[]string) TokenFunc {
	return func(c string) error {
		*chunks = append(*chunks, c)
		return nil
	}
}

func TestOpenAIChatStreams(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"role":"assistant"}}]}

data: {"choices":[{"delta":{"content":"Hola"}}]}

: keep-alive

data: {"choices":[{"delta":{"content":", amigo"}}]}

data: [DONE]

`
	srv := sseServer(t, func(r *http.Request, body map[string]any) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Error("request did not ask for a stream")
		}
		msgs := body["messages"].([]any)
		if first := msgs[0].(map[string]any); first["role"] != "system" || first["content"] != "be a tutor" {
			t.Errorf("system prompt not sent first: %v", first)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
	}, stream)
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	var chunks []string
	reply, err := p.Chat(context.Background(), ChatRequest{
		System:   "be a tutor",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, collect(&chunks))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reply != "Hola, amigo" {
		t.Errorf("reply = %q", reply)
	}
	if strings.Join(chunks, "|") != "Hola|, amigo" {
		t.Errorf("chunks = %q, want each delta separately", chunks)
	}
}

func TestOpenAIChatErrorMidStream(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"Partial"}}]}

data: {"error":{"message":"model overloaded"}}

`
	srv := sseServer(t, nil, stream)
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	reply, err := p.Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}}, collect(new([]string)))
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("err = %v, want the mid-stream error", err)
	}
	if reply != "Partial" {
		t.Errorf("reply = %q, want the text received before the error", reply)
	}
}

func TestOpenAIChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	_, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string)))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want a 401 api error", err)
	}
}

func TestChatStopsWhenOnTokenFails(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"one"}}]}

data: {"choices":[{"delta":{"content":"two"}}]}

data: [DONE]

`
	srv := sseServer(t, nil, stream)
	defer srv.Close()

	stop := errors.New("reader went away")
	calls := 0
	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	_, err := p.Chat(context.Background(), ChatRequest{}, func(string) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) {
		t.Errorf("err = %v, want the onToken error", err)
	}
	if calls != 1 {
		t.Errorf("onToken called %d times after failing, want 1", calls)
	}
}

func TestAnthropicChatStreams(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: ping
data: {"type":"ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Ser "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"vs estar"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_stop
data: {"type":"message_stop"}

`
	srv := sseServer(t, func(r *http.Request, body map[string]any) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if body["stream"] != true || body["system"] != "be a tutor" {
			t.Errorf("body = %v, want stream true and the system prompt top-level", body)
		}
		if r.Header.Get("x-api-key") != "k" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
	}, stream)
	defer srv.Close()

	p := newAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	var chunks []string
	reply, err := p.Chat(context.Background(), ChatRequest{
		System:   "be a tutor",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, collect(&chunks))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reply != "Ser vs estar" || len(chunks) != 2 {
		t.Errorf("reply = %q, chunks = %q", reply, chunks)
	}
}

func TestAnthropicChatErrorEvent(t *testing.T) {
	stream := `event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}

`
	srv := sseServer(t, nil, stream)
	defer srv.Close()

	p := newAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	_, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string)))
	if err == nil || !strings.Contains(err.Error(), "Overloaded") {
		t.Fatalf("err = %v, want the error event's message", err)
	}
}

// LM Studio on a base URL missing /v1 answers the chat route with 200 and a
// JSON error. That used to parse as a stream with no events — an empty reply
// saved as a success.
func TestChatRejectsNonStreamSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"Unexpected endpoint or method. (POST /chat/completions)"}`))
	}))
	defer srv.Close()

	for name, p := range map[string]Provider{
		"openai":    newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"}),
		"anthropic": newAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}),
	} {
		_, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string)))
		if err == nil || !strings.Contains(err.Error(), "Unexpected endpoint") || !strings.Contains(err.Error(), "base URL") {
			t.Errorf("%s: err = %v, want the server's error and a base URL hint", name, err)
		}
	}
}

func TestChatStreamCutShort(t *testing.T) {
	// No [DONE] and no finish_reason: the connection just ended.
	srv := sseServer(t, nil, "data: {\"choices\":[{\"delta\":{\"content\":\"Half a\"}}]}\n\n")
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	reply, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string)))
	if !errors.Is(err, errStreamCut) {
		t.Errorf("err = %v, want errStreamCut", err)
	}
	if reply != "Half a" {
		t.Errorf("reply = %q, want the partial text kept", reply)
	}
}

func TestChatFinishReasonCompletesWithoutDone(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"Hola"}}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

`
	srv := sseServer(t, nil, stream)
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	if reply, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string))); err != nil || reply != "Hola" {
		t.Errorf("Chat = %q, %v; want a complete reply", reply, err)
	}
}

func TestChatEmptyReply(t *testing.T) {
	srv := sseServer(t, nil, "data: {\"choices\":[{\"delta\":{\"content\":\"\\n\\n\"}}]}\n\ndata: [DONE]\n\n")
	defer srv.Close()

	p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})
	if _, err := p.Chat(context.Background(), ChatRequest{}, collect(new([]string))); !errors.Is(err, ErrEmptyReply) {
		t.Errorf("err = %v, want ErrEmptyReply", err)
	}
}

func TestReadSSEMultilineDataAndNoTrailingBlank(t *testing.T) {
	var got []string
	err := readSSE(strings.NewReader("data: a\ndata: b\n\ndata: last"), func(_, data string) error {
		got = append(got, data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "a\nb|last" {
		t.Errorf("events = %q", got)
	}
}

// Reasoning must reach OnThinking (so callers know a thinking model is
// working) and never leak into the reply.
func TestChatReportsThinking(t *testing.T) {
	openAIStream := `data: {"choices":[{"delta":{"reasoning_content":"Let me think. "}}]}

data: {"choices":[{"delta":{"reasoning":"Still thinking."}}]}

data: {"choices":[{"delta":{"content":"Answer"}}]}

data: [DONE]

`
	anthropicStream := `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think. Still thinking."}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}

event: message_stop
data: {"type":"message_stop"}

`
	for name, tc := range map[string]struct {
		stream string
		build  func(url string) Provider
	}{
		"openai":    {openAIStream, func(u string) Provider { return newOpenAICompat(Config{BaseURL: u, Model: "m"}) }},
		"anthropic": {anthropicStream, func(u string) Provider { return newAnthropic(Config{BaseURL: u, APIKey: "k", Model: "m"}) }},
	} {
		srv := sseServer(t, nil, tc.stream)
		var thought strings.Builder
		reply, err := tc.build(srv.URL).Chat(context.Background(), ChatRequest{
			OnThinking: func(c string) { thought.WriteString(c) },
		}, collect(new([]string)))
		srv.Close()

		if err != nil || reply != "Answer" {
			t.Errorf("%s: reply = %q, %v; want just the answer", name, reply, err)
		}
		if thought.String() != "Let me think. Still thinking." {
			t.Errorf("%s: thinking = %q", name, thought.String())
		}
	}
}

func TestDisableThinkingAppendsSwitch(t *testing.T) {
	for _, disable := range []bool{false, true} {
		var system string
		srv := sseServer(t, func(r *http.Request, body map[string]any) {
			system = body["messages"].([]any)[0].(map[string]any)["content"].(string)
		}, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")

		p := newOpenAICompat(Config{BaseURL: srv.URL, Model: "m", DisableThinking: disable})
		p.Chat(context.Background(), ChatRequest{System: "be brief"}, collect(new([]string)))
		srv.Close()

		if got := strings.HasSuffix(system, "/no_think"); got != disable {
			t.Errorf("DisableThinking=%v: system prompt %q", disable, system)
		}
	}
}
