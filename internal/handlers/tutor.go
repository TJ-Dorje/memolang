package handlers

import (
	"fmt"
	"net/http"

	"memolang/internal/assistant"
	"memolang/internal/models"

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

// TutorPage shows the conversation about a card, streaming the latest reply
// if it is still being generated.
func (h *Handler) TutorPage(c *gin.Context) {
	card, deck, ok := h.tutorCard(c)
	if !ok {
		return
	}

	convID, err := models.FindTutorConversation(h.DB, currentUserID(c), card.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load conversation")
		return
	}
	chat, err := h.loadChat(c, convID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load conversation")
		return
	}
	chat.AssistantName = "Tutor"

	h.renderChat(c, "tutor.html", "tutor_top", "tutor_bottom", PageData{
		Title: "Tutor — " + card.Front,
		Flash: h.getFlash(c),
		Data:  &TutorData{ChatData: chat, Card: card, Deck: deck, Presets: tutorPresetList},
	})
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

	open := func() (int64, error) { return models.GetOrCreateTutorConversation(h.DB, userID, card.ID) }
	err := h.Assistant.Ask(userID, open, assistant.TutorPrompt(card, deck.Name), question)
	if msg := askFlash(err); msg != "" {
		h.redirectWithFlash(c, tutorURL(card.ID), msg)
		return
	}
	c.Redirect(http.StatusSeeOther, tutorURL(card.ID))
}

func (h *Handler) ResetTutor(c *gin.Context) {
	userID := currentUserID(c)
	card, _, ok := h.tutorCard(c)
	if !ok {
		return
	}
	convID, err := models.FindTutorConversation(h.DB, userID, card.ID)
	if err == nil {
		err = models.DeleteConversation(h.DB, userID, convID)
	}
	if err != nil {
		h.redirectWithFlash(c, tutorURL(card.ID), "Failed to clear the conversation.")
		return
	}
	h.redirectWithFlash(c, tutorURL(card.ID), "Conversation cleared.")
}
