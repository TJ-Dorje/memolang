package handlers

import (
	"errors"
	"net/http"

	"memolang/internal/ai"
	"memolang/internal/assistant"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// The deck builder: an interview with the assistant (streamed like the
// tutor), then Create deck → the conversation is summarised into a spec →
// the old AI form, pre-filled, as a review step → the existing generation.

const deckBuilderURL = "/decks/new/assistant"

func (h *Handler) DeckBuilderPage(c *gin.Context) {
	convID, err := models.FindDeckBuilderConversation(h.DB, currentUserID(c))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load conversation")
		return
	}
	chat, err := h.loadChat(c, convID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load conversation")
		return
	}
	chat.AssistantName = "Assistant"
	chat.Greeting = assistant.DeckBuilderGreeting

	h.renderChat(c, "deckbuilder.html", "deckbuilder_top", "deckbuilder_bottom", PageData{
		Title: "Create a Deck with AI",
		Flash: h.getFlash(c),
		Data:  DeckBuilderData{ChatData: chat},
	})
}

func (h *Handler) AskDeckBuilder(c *gin.Context) {
	userID := currentUserID(c)
	open := func() (int64, error) { return models.GetOrCreateDeckBuilderConversation(h.DB, userID) }

	err := h.Assistant.Ask(userID, open, assistant.DeckBuilderPrompt(), c.PostForm("question"))
	if msg := askFlash(err); msg != "" {
		h.redirectWithFlash(c, deckBuilderURL, msg)
		return
	}
	c.Redirect(http.StatusSeeOther, deckBuilderURL)
}

func (h *Handler) ResetDeckBuilder(c *gin.Context) {
	userID := currentUserID(c)
	convID, err := models.FindDeckBuilderConversation(h.DB, userID)
	if err == nil {
		err = models.DeleteConversation(h.DB, userID, convID)
	}
	if err != nil {
		h.redirectWithFlash(c, deckBuilderURL, "Failed to clear the conversation.")
		return
	}
	h.redirectWithFlash(c, deckBuilderURL, "Started over.")
}

// ReviewDeckBuilder is the Create deck button: it shows the waiting page,
// which moves on to DeckBuilderSummary. The summary is an LLM call, so it
// runs on that GET while the spinner stays on screen.
func (h *Handler) ReviewDeckBuilder(c *gin.Context) {
	h.renderWaiting(c, "Preparing your deck...",
		"Summarising your conversation into a deck…", "You can change anything on the next page.",
		deckBuilderURL+"/summary")
}

// DeckBuilderSummary turns the interview into a DeckSpec and opens the review
// form (the AI deck form) pre-filled with it.
func (h *Handler) DeckBuilderSummary(c *gin.Context) {
	userID := currentUserID(c)
	convID, err := models.FindDeckBuilderConversation(h.DB, userID)
	if err != nil || convID == 0 {
		h.redirectWithFlash(c, deckBuilderURL, "Tell the assistant about your deck first.")
		return
	}

	spec, err := h.Assistant.SummarizeDeck(c.Request.Context(), userID, convID)
	switch {
	case errors.Is(err, assistant.ErrNothingToSummarize):
		h.redirectWithFlash(c, deckBuilderURL, "Tell the assistant about your deck first.")
		return
	case errors.Is(err, ai.ErrNotConfigured):
		h.redirectWithFlash(c, deckBuilderURL, "Set up an AI provider first: Profile → AI Provider.")
		return
	case err != nil:
		h.redirectWithFlash(c, deckBuilderURL, "Could not summarise the conversation. Try Create deck again. ("+err.Error()+")")
		return
	}

	token := h.storePending(AIFormData{
		Name:     spec.Name,
		Language: spec.Language,
		Prompt:   spec.GenerationPrompt(),
		Mode:     "srs",
	})
	c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+token)
}
