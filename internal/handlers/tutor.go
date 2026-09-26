package handlers

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"memolang/internal/ai"
	"memolang/internal/models"
	"memolang/internal/tutor"

	"github.com/gin-gonic/gin"
)

// tutorPresets are the one-click questions. They are looked up by key rather
// than posted as text, so the form stays small and the wording lives here.
var tutorPresets = map[string]string{
	"explain":  "Explain this word: what it means, how it's used, and any grammar I should know.",
	"examples": "Give me three more example sentences using it, each with a translation.",
	"mnemonic": "Give me a memory trick to remember it.",
	"quiz":     "Quiz me on this word with one short question. Don't give the answer until I reply.",
}

// tutorCard loads the card and its deck for the current user, answering 404
// for anything not theirs. ok is false when the response is already written.
func (h *Handler) tutorCard(c *gin.Context) (models.Card, models.Deck, bool) {
	userID := currentUserID(c)
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid card ID")
		return models.Card{}, models.Deck{}, false
	}
	card, err := models.GetCardByID(h.DB, userID, id)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return models.Card{}, models.Deck{}, false
	}
	deck, err := models.GetDeckByID(h.DB, userID, card.DeckID)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return models.Card{}, models.Deck{}, false
	}
	return card, deck, true
}

func tutorURL(cardID int64) string {
	return fmt.Sprintf("/cards/%d/tutor", cardID)
}

// TutorPage shows the conversation about a card. When the latest reply is
// still being generated, the page is streamed: the top of the page (with the
// history) is sent and flushed, the reply is written into it as it arrives,
// then the bottom of the page (the question form) closes it. No JavaScript:
// the browser renders the HTML as it comes in.
func (h *Handler) TutorPage(c *gin.Context) {
	userID := currentUserID(c)
	card, deck, ok := h.tutorCard(c)
	if !ok {
		return
	}

	msgs, err := models.GetTutorMessages(h.DB, userID, card.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load conversation")
		return
	}

	data := TutorData{Card: card, Deck: deck, Presets: tutorPresetList}
	cfg, err := ai.LoadConfig(h.DB, userID)
	data.Configured = err == nil && cfg.Provider != ""

	if n := len(msgs); n > 0 && msgs[n-1].Status == "generating" {
		data.Streaming = &msgs[n-1]
		msgs = msgs[:n-1]
	}
	data.Messages = msgs

	pd := h.withChrome(c, PageData{Title: "Tutor — " + card.Front, Flash: h.getFlash(c), Data: data})

	if data.Streaming == nil {
		c.HTML(http.StatusOK, "tutor.html", pd)
		return
	}
	h.streamTutorPage(c, pd, *data.Streaming)
}

func (h *Handler) streamTutorPage(c *gin.Context, pd PageData, reply models.TutorMessage) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	// Tells nginx-style proxies not to buffer, which would hold the stream
	// back until it ended. Harmless when there is no proxy.
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	if err := h.renderPart(c, "tutor_top", pd); err != nil {
		log.Printf("TutorPage: render top: %v", err)
		return
	}
	c.Writer.Flush()

	// Model output is untrusted: every chunk is HTML-escaped before it is
	// written, exactly as the template would escape it.
	found, err := h.Tutor.Broker.Follow(c.Request.Context(), tutor.ReplyKey(reply.ID), func(chunk string) error {
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
		if err := h.Tutor.MarkInterrupted(reply); err != nil {
			log.Printf("TutorPage: mark interrupted: %v", err)
		}
		c.Writer.WriteString(`<span class="tutor-error">The reply was interrupted. Ask again.</span>`)
	case err != nil:
		c.Writer.WriteString(`<span class="tutor-error">The tutor could not finish this reply. Ask again.</span>`)
	}

	if err := h.renderPart(c, "tutor_bottom", pd); err != nil {
		log.Printf("TutorPage: render bottom: %v", err)
	}
}

func (h *Handler) AskTutor(c *gin.Context) {
	userID := currentUserID(c)
	card, deck, ok := h.tutorCard(c)
	if !ok {
		return
	}

	question := c.PostForm("question")
	if preset, ok := tutorPresets[c.PostForm("preset")]; ok {
		question = preset
	}

	err := h.Tutor.Ask(userID, card, deck.Name, question)
	switch {
	case errors.Is(err, tutor.ErrEmptyQuestion):
		h.redirectWithFlash(c, tutorURL(card.ID), "Type a question, or pick one of the buttons.")
	case errors.Is(err, ai.ErrNotConfigured):
		h.redirectWithFlash(c, tutorURL(card.ID), "Set up an AI provider first: Profile → AI Provider.")
	case errors.Is(err, tutor.ErrBusy):
		h.redirectWithFlash(c, tutorURL(card.ID), "The tutor is still answering. Wait for it to finish.")
	case err != nil:
		log.Printf("AskTutor: %v", err)
		h.redirectWithFlash(c, tutorURL(card.ID), "Could not ask the tutor: "+err.Error())
	default:
		c.Redirect(http.StatusSeeOther, tutorURL(card.ID))
	}
}

func (h *Handler) ResetTutor(c *gin.Context) {
	card, _, ok := h.tutorCard(c)
	if !ok {
		return
	}
	if err := models.ResetTutorThread(h.DB, currentUserID(c), card.ID); err != nil {
		h.redirectWithFlash(c, tutorURL(card.ID), "Failed to clear the conversation.")
		return
	}
	h.redirectWithFlash(c, tutorURL(card.ID), "Conversation cleared.")
}
