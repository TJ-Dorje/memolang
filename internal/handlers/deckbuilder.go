package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"memolang/internal/ai"
	"memolang/internal/assistant"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// The deck builder: an interview with the assistant (streamed like the
// tutor). When it has everything, the assistant ends a reply with a deck plan,
// which the page shows as a card; Generate deck on the newest card runs the
// existing generation. If the model never produces a plan, Create deck plan
// summarises the conversation into one and adds it to the chat.

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
		Data:  &DeckBuilderData{ChatData: chat},
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

// PlanDeckBuilder is the fallback for a model that never writes a plan: it
// shows the waiting page, which moves on to DeckBuilderSummary. The summary
// is an LLM call, so it runs on that GET while the spinner stays on screen.
func (h *Handler) PlanDeckBuilder(c *gin.Context) {
	h.renderWaiting(c, "Drafting your deck...",
		"Turning your conversation into a deck plan…", "You'll see it in the chat.",
		deckBuilderURL+"/summary")
}

// DeckBuilderSummary summarises the interview into a plan and adds it to the
// chat as an assistant message, where it shows as a card like any plan the
// assistant wrote itself.
func (h *Handler) DeckBuilderSummary(c *gin.Context) {
	userID := currentUserID(c)
	convID, err := models.FindDeckBuilderConversation(h.DB, userID)
	if err != nil || convID == 0 {
		h.redirectWithFlash(c, deckBuilderURL, "Tell the assistant about your deck first.")
		return
	}
	if busy, _ := h.replyInProgress(userID, convID); busy {
		h.redirectWithFlash(c, deckBuilderURL, "The assistant is still answering. Wait for it to finish.")
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
		log.Printf("DeckBuilderSummary: %v", err)
		h.redirectWithFlash(c, deckBuilderURL, "Could not draft a plan from the conversation. Try again, or tell the assistant a bit more.")
		return
	}

	plan := "Here's the plan from our conversation. Press Generate deck, or tell me what to change.\n\n" +
		assistant.FormatDeckPlan(spec)
	if _, err := models.AddConversationMessage(h.DB, convID, "assistant", plan, "done"); err != nil {
		h.redirectWithFlash(c, deckBuilderURL, "Could not save the plan. Try again.")
		return
	}
	c.Redirect(http.StatusSeeOther, deckBuilderURL)
}

// GenerateFromPlan generates the deck on a plan card. It takes only the
// message id and reads the plan back from the learner's own conversation, so
// nothing about the deck is trusted from the form, and only the newest plan
// counts: an older card still on screen cannot generate a superseded plan.
func (h *Handler) GenerateFromPlan(c *gin.Context) {
	userID := currentUserID(c)
	messageID, _ := strconv.ParseInt(c.PostForm("message_id"), 10, 64)

	convID, err := models.FindDeckBuilderConversation(h.DB, userID)
	if err != nil || convID == 0 {
		h.redirectWithFlash(c, deckBuilderURL, "That plan is no longer available.")
		return
	}
	msgs, err := models.GetConversationMessages(h.DB, userID, convID)
	if err != nil {
		h.redirectWithFlash(c, deckBuilderURL, "Could not load the plan. Try again.")
		return
	}

	current := latestPlan(messageViews(msgs))
	switch {
	case current == nil:
		h.redirectWithFlash(c, deckBuilderURL, "There is no deck plan yet.")
		return
	case current.MessageID != messageID:
		h.redirectWithFlash(c, deckBuilderURL, "That plan was replaced by a newer one — use the latest card.")
		return
	}

	spec := current.Spec
	token := h.storePending(AIFormData{
		Name:     spec.Name,
		Language: spec.Language,
		Prompt:   spec.GenerationPrompt(),
		Mode:     spec.Mode,
	})
	c.Redirect(http.StatusSeeOther, "/decks/new-ai/processing?token="+token)
}

// replyInProgress reports whether the conversation's latest reply is still
// being generated.
func (h *Handler) replyInProgress(userID, convID int64) (bool, error) {
	msgs, err := models.GetConversationMessages(h.DB, userID, convID)
	if err != nil {
		return false, err
	}
	n := len(msgs)
	return n > 0 && msgs[n-1].Status == "generating", nil
}
