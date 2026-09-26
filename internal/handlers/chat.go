package handlers

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/assistant"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// Shared machinery for the assistant's chat pages (tutor, deck builder).
// Each page template defines a top half ending *inside* the streaming
// reply's text element and a bottom half closing it; see templates/chat.html.

// loadChat reads a conversation for display. conversationID 0 (none yet) is
// an empty chat. The reply still generating, if any, is split out so the
// page can stream it.
func (h *Handler) loadChat(c *gin.Context, conversationID int64) (ChatData, error) {
	userID := currentUserID(c)
	var data ChatData

	cfg, err := ai.LoadConfig(h.DB, userID)
	data.Configured = err == nil && cfg.Provider != ""

	if conversationID == 0 {
		return data, nil
	}
	msgs, err := models.GetConversationMessages(h.DB, userID, conversationID)
	if err != nil {
		return data, err
	}
	if n := len(msgs); n > 0 && msgs[n-1].Status == "generating" {
		data.Streaming = &msgs[n-1]
		msgs = msgs[:n-1]
	}
	data.Messages = messageViews(msgs)
	data.HasPlan = latestPlan(data.Messages) != nil
	return data, nil
}

// messageViews takes plan blocks out of the assistant's messages and marks
// the newest plan as the one that can be generated.
func messageViews(msgs []models.ConversationMessage) []ChatMessageView {
	views := make([]ChatMessageView, len(msgs))
	for i, m := range msgs {
		views[i] = ChatMessageView{ID: m.ID, Role: m.Role, Text: strings.TrimSpace(m.Content), Status: m.Status}
		if m.Role != "assistant" {
			continue
		}
		text, spec, planErr := assistant.ExtractDeckPlan(m.Content)
		views[i].Text = text
		views[i].PlanUnreadable = planErr != nil
		if spec != nil {
			views[i].Plan = &PlanCard{MessageID: m.ID, Spec: *spec}
		}
	}
	if card := latestPlan(views); card != nil {
		card.Current = true
	}
	return views
}

func latestPlan(views []ChatMessageView) *PlanCard {
	for i := len(views) - 1; i >= 0; i-- {
		if views[i].Plan != nil {
			return views[i].Plan
		}
	}
	return nil
}

// chatPage is implemented by page data that embeds ChatData.
type chatPage interface{ ChatState() *ChatData }

// renderChat renders a chat page, streaming it when a reply is in progress:
// the top half is sent and flushed, the reply is written into it as it
// arrives, then the bottom half closes the page. No JavaScript: the browser
// renders the HTML as it comes in. pd.Data must be a pointer to page data
// embedding ChatData, so the plan of the reply that just streamed can be
// handed to the bottom half.
func (h *Handler) renderChat(c *gin.Context, full, top, bottom string, pd PageData) {
	pd = h.withChrome(c, pd)
	chat := pd.Data.(chatPage).ChatState()
	if chat.Streaming == nil {
		c.HTML(http.StatusOK, full, pd)
		return
	}
	reply := *chat.Streaming

	c.Header("Content-Type", "text/html; charset=utf-8")
	// Tells nginx-style proxies not to buffer, which would hold the stream
	// back until it ended. Harmless when there is no proxy.
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	if err := h.renderPart(c, top, pd); err != nil {
		log.Printf("renderChat: %s: %v", top, err)
		return
	}
	c.Writer.Flush()

	// Model output is untrusted: every chunk is HTML-escaped before it is
	// written, exactly as the template would escape it. The plan filter holds
	// back a deck plan block, which is shown as a card instead; the trim
	// filter keeps the model's leading and trailing blank lines out of the
	// pre-wrap text, matching the trimmed text of a stored reply.
	var plans assistant.PlanFilter
	var trim assistant.TrimFilter
	write := func(text string) error {
		if text == "" {
			return nil
		}
		if _, err := c.Writer.WriteString(template.HTMLEscapeString(text)); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	found, err := h.Assistant.Broker.Follow(c.Request.Context(), assistant.ReplyKey(reply.ID), func(chunk string) error {
		return write(trim.Write(plans.Write(chunk)))
	})

	switch {
	case !found:
		// Nothing is producing this reply: the server restarted mid-reply.
		text, _, _ := assistant.ExtractDeckPlan(reply.Content)
		write(text)
		if err := h.Assistant.MarkInterrupted(reply); err != nil {
			log.Printf("renderChat: mark interrupted: %v", err)
		}
		c.Writer.WriteString(`<span class="chat-error">The reply was interrupted. Ask again.</span>`)
	case err != nil:
		write(trim.Write(plans.Flush()))
		// The live reason is shown (escaped) because it is usually actionable
		// — most often a wrong base URL. It is not stored; a reload shows the
		// generic line.
		c.Writer.WriteString(`<span class="chat-error">The assistant could not reply: ` +
			template.HTMLEscapeString(err.Error()) + `. Check <a href="/profile/ai">Profile → AI Provider</a>, then ask again.</span>`)
	default:
		write(trim.Write(plans.Flush()))
		h.attachStreamedPlan(c, chat, reply)
	}

	if err := h.renderPart(c, bottom, pd); err != nil {
		log.Printf("renderChat: %s: %v", bottom, err)
	}
}

// attachStreamedPlan reads the reply that just finished (the assistant saves
// it before ending the stream) and, if it carries a plan, puts its card on
// the page. It is the newest plan, so any earlier card is superseded.
func (h *Handler) attachStreamedPlan(c *gin.Context, chat *ChatData, reply models.ConversationMessage) {
	msgs, err := models.GetConversationMessages(h.DB, currentUserID(c), reply.ConversationID)
	if err != nil {
		log.Printf("renderChat: reload reply: %v", err)
		return
	}
	for _, m := range msgs {
		if m.ID != reply.ID {
			continue
		}
		_, spec, planErr := assistant.ExtractDeckPlan(m.Content)
		chat.StreamedPlanUnreadable = planErr != nil
		if spec != nil {
			chat.StreamedPlan = &PlanCard{MessageID: m.ID, Spec: *spec, Current: true}
			chat.HasPlan = true
		}
		return
	}
}

// askFlash maps an Ask error to the message shown after the redirect; "" for
// success.
func askFlash(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, assistant.ErrEmptyQuestion):
		return "Type a question, or pick one of the buttons."
	case errors.Is(err, ai.ErrNotConfigured):
		return "Set up an AI provider first: Profile → AI Provider."
	case errors.Is(err, assistant.ErrBusy):
		return "The assistant is still answering. Wait for it to finish."
	default:
		log.Printf("assistant ask: %v", err)
		return "Could not ask the assistant: " + err.Error()
	}
}
