package handlers

import (
	"errors"
	"html/template"
	"log"
	"net/http"

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
	data.Messages = msgs
	return data, nil
}

// chatPage is implemented by page data that embeds ChatData.
type chatPage interface{ ChatState() ChatData }

// renderChat renders a chat page, streaming it when a reply is in progress:
// the top half is sent and flushed, the reply is written into it as it
// arrives, then the bottom half closes the page. No JavaScript: the browser
// renders the HTML as it comes in.
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
	// written, exactly as the template would escape it.
	found, err := h.Assistant.Broker.Follow(c.Request.Context(), assistant.ReplyKey(reply.ID), func(chunk string) error {
		if _, err := c.Writer.WriteString(template.HTMLEscapeString(chunk)); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	})

	switch {
	case !found:
		// Nothing is producing this reply: the server restarted mid-reply.
		c.Writer.WriteString(template.HTMLEscapeString(reply.Content))
		if err := h.Assistant.MarkInterrupted(reply); err != nil {
			log.Printf("renderChat: mark interrupted: %v", err)
		}
		c.Writer.WriteString(`<span class="chat-error">The reply was interrupted. Ask again.</span>`)
	case err != nil:
		c.Writer.WriteString(`<span class="chat-error">The assistant could not finish this reply. Ask again.</span>`)
	}

	if err := h.renderPart(c, bottom, pd); err != nil {
		log.Printf("renderChat: %s: %v", bottom, err)
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
