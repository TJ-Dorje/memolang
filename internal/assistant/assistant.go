// Package assistant runs the app's LLM conversations — the per-card tutor
// and the deck builder — streaming each reply through a stream.Broker while
// it is generated in the background.
package assistant

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"memolang/internal/ai"
	"memolang/internal/models"
	"memolang/internal/stream"
)

// ErrBusy means a reply in this conversation is still being generated. One
// question at a time keeps the conversation in order and bounds spend.
var ErrBusy = errors.New("a reply is still being written")

// ErrEmptyQuestion is returned for a blank question.
var ErrEmptyQuestion = errors.New("empty question")

// DefaultTimeout bounds one reply. Generous, because a local model may first
// have to load its weights.
const DefaultTimeout = 3 * time.Minute

// maxHistory is how many earlier messages are sent back as context. Older
// turns are dropped so a long conversation doesn't grow the cost of every
// new question without limit.
const maxHistory = 20

// Service is safe for concurrent use.
type Service struct {
	DB     *sql.DB
	Broker stream.Broker
	// Provider returns the user's configured LLM. Tests swap in a fake.
	Provider func(db *sql.DB, userID int64) (ai.Provider, error)
	Timeout  time.Duration
}

func New(db *sql.DB, broker stream.Broker) *Service {
	return &Service{DB: db, Broker: broker, Provider: userProvider, Timeout: DefaultTimeout}
}

func userProvider(db *sql.DB, userID int64) (ai.Provider, error) {
	cfg, err := ai.LoadConfig(db, userID)
	if err != nil {
		return nil, err
	}
	return ai.New(cfg)
}

// ReplyKey names a reply in the broker.
func ReplyKey(messageID int64) string {
	return "assistant-reply-" + strconv.FormatInt(messageID, 10)
}

// Ask records the question and starts generating the reply in the
// background. open returns the conversation's id, creating it if needed; it
// is only called once the question is known to be answerable, so an
// unconfigured user leaves nothing behind. Ask returns once the reply is
// registered with the broker, so a page loaded straight afterwards can
// follow it.
func (s *Service) Ask(userID int64, open func() (int64, error), system, question string) error {
	question = strings.TrimSpace(question)
	if question == "" {
		return ErrEmptyQuestion
	}

	provider, err := s.Provider(s.DB, userID)
	if err != nil {
		return err
	}

	conversationID, err := open()
	if err != nil {
		return err
	}
	history, err := models.GetConversationMessages(s.DB, userID, conversationID)
	if err != nil {
		return err
	}
	if n := len(history); n > 0 && history[n-1].Status == "generating" {
		return ErrBusy
	}

	if _, err := models.AddConversationMessage(s.DB, conversationID, "user", question, "done"); err != nil {
		return err
	}
	replyID, err := models.AddConversationMessage(s.DB, conversationID, "assistant", "", "generating")
	if err != nil {
		return err
	}

	req := ai.ChatRequest{System: system, Messages: conversation(history, question)}

	key := ReplyKey(replyID)
	s.Broker.Start(key)
	go s.generate(provider, req, replyID, key)
	return nil
}

// generate runs detached from any request: its context is its own, so a
// closed tab neither cancels the call nor loses the reply.
func (s *Service) generate(provider ai.Provider, req ai.ChatRequest, replyID int64, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()

	text, err := provider.Chat(ctx, req, func(chunk string) error {
		s.Broker.Publish(key, chunk)
		return nil
	})

	status := "done"
	if err != nil {
		status = "error"
		log.Printf("assistant: reply %d failed: %v", replyID, err)
	}
	// Saved before Finish, so anyone who sees the stream end and reloads
	// finds the final state in the database.
	if dbErr := models.FinishConversationMessage(s.DB, replyID, text, status); dbErr != nil {
		log.Printf("assistant: saving reply %d: %v", replyID, dbErr)
	}
	logUnreadablePlan(replyID, text)
	s.Broker.Finish(key, err)
}

// MarkInterrupted records a reply left "generating" with nothing producing it
// any more — the server restarted mid-reply — so the conversation can go on.
func (s *Service) MarkInterrupted(msg models.ConversationMessage) error {
	return models.FinishConversationMessage(s.DB, msg.ID, msg.Content, "error")
}

// conversation builds the messages to send: stored history plus the new
// question. Failed and unfinished replies are skipped (they would teach the
// model a broken turn), and only the last maxHistory turns are kept.
//
// Providers want strict user/assistant alternation starting with the user.
// Skipping a failed reply leaves its question next to the following one, so
// consecutive user turns are merged — after the new question is appended, or
// it would still follow the orphaned one — and a leading assistant turn left
// by trimming is dropped.
func conversation(msgs []models.ConversationMessage, question string) []ai.ChatMessage {
	var out []ai.ChatMessage
	for _, m := range msgs {
		if m.Status != "done" || m.Content == "" {
			continue
		}
		out = append(out, ai.ChatMessage{Role: m.Role, Content: m.Content})
	}
	out = append(out, ai.ChatMessage{Role: "user", Content: question})

	if len(out) > maxHistory {
		out = out[len(out)-maxHistory:]
	}
	if out[0].Role == "assistant" {
		out = out[1:]
	}
	return mergeUserRuns(out)
}

// mergeUserRuns joins consecutive user messages into one.
func mergeUserRuns(msgs []ai.ChatMessage) []ai.ChatMessage {
	var out []ai.ChatMessage
	for _, m := range msgs {
		n := len(out)
		if n > 0 && out[n-1].Role == "user" && m.Role == "user" {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, m)
	}
	return out
}
