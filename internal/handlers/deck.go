package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"

	"memolang/internal/ai"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Dashboard(c *gin.Context) {
	userID := currentUserID(c)

	decks, err := models.GetAllDecks(h.DB, userID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load decks")
		return
	}
	if decks == nil {
		decks = []models.Deck{}
	}
	h.render(c, http.StatusOK, "dashboard.html", PageData{
		Title: "MemoLang",
		Flash: h.getFlash(c),
		Data:  DashboardData{Decks: decks},
	})
}

func (h *Handler) NewDeckForm(c *gin.Context) {
	if c.Query("ai_mode") != "true" {
		h.render(c, http.StatusOK, "deck_form.html", PageData{
			Title: "New Deck",
			Flash: h.getFlash(c),
		})
		return
	}

	// The old AI form is gone: AI decks are designed with the assistant.
	// Kept as a redirect for bookmarks.
	c.Redirect(http.StatusSeeOther, deckBuilderURL)
}

// pendingAIForm restores a filled AI form by token. ok is false when the
// token is absent, already consumed, or holds something else. The pending map
// is shared with the CSV import flow, so the type assertion has to be
// checked: an import token replayed here would otherwise panic.
func (h *Handler) pendingAIForm(token string) (AIFormData, bool) {
	if token == "" {
		return AIFormData{}, false
	}
	v, ok := h.loadPending(token)
	if !ok {
		return AIFormData{}, false
	}
	form, ok := v.(AIFormData)
	return form, ok
}

func (h *Handler) AIProcessing(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Redirect(http.StatusSeeOther, "/decks/new")
		return
	}
	h.renderWaiting(c, "Generating...",
		"Asking the AI to generate your cards…", "This may take a minute for large requests.",
		"/decks/new-ai/execute?token="+token)
}

// renderWaiting shows the spinner page, which immediately navigates (meta
// refresh, no JavaScript) to next: a slow GET that does the LLM work. The
// browser keeps this page on screen until next responds.
func (h *Handler) renderWaiting(c *gin.Context, title, message, sub, next string) {
	h.render(c, http.StatusOK, "ai_processing.html", PageData{
		Title: title,
		Data:  WaitingData{Message: message, Sub: sub, Next: next},
	})
}

func (h *Handler) AIExecute(c *gin.Context) {
	userID := currentUserID(c)

	token := c.Query("token")
	if token == "" {
		c.Redirect(http.StatusSeeOther, "/decks/new")
		return
	}

	// Checked assertion: the pending map is shared with the CSV import flow,
	// so a replayed import token must not panic here.
	fd, ok := h.pendingAIForm(token)
	if !ok {
		c.Redirect(http.StatusSeeOther, "/decks/new/assistant")
		return
	}

	cfg, err := ai.LoadConfig(h.DB, userID)
	if err != nil {
		log.Printf("AIExecute: LoadConfig failed: %v", err)
		h.redirectWithFlash(c, deckBuilderURL, "Failed to load LLM settings: "+err.Error())
		return
	}

	provider, err := ai.New(cfg)
	if err != nil {
		log.Printf("AIExecute: ai.New failed: %v", err)
		msg := "LLM configuration error: " + err.Error()
		if errors.Is(err, ai.ErrNotConfigured) {
			msg = "Set up an AI provider first: Profile → AI Providers."
		}
		h.redirectWithFlash(c, deckBuilderURL, msg)
		return
	}

	log.Printf("AIExecute: calling GenerateCards...")
	cards, err := provider.GenerateCards(c.Request.Context(), fd.Language, fd.Prompt)
	if err != nil {
		log.Printf("AIExecute: GenerateCards failed: %v", err)
		// The plan is still in the chat, so Generate deck can simply be pressed again.
		h.redirectWithFlash(c, deckBuilderURL, "Card generation failed. Press Generate deck to try again.")
		return
	}

	log.Printf("AIExecute: got %d cards", len(cards))
	if len(cards) == 0 {
		h.redirectWithFlash(c, deckBuilderURL, "The AI returned no cards. Ask the assistant to adjust the plan, then generate again.")
		return
	}

	deck, err := models.CreateDeck(h.DB, userID, fd.Name, fd.Mode)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create deck")
		return
	}

	for _, card := range cards {
		_, err := models.CreateCard(h.DB, deck.ID, card.Front, card.Back, card.Example, "")
		if err != nil {
			log.Printf("failed to insert card %q: %v", card.Front, err)
		}
	}

	// The interview that designed this deck is finished; the next "Create
	// with AI" starts a fresh one.
	if convID, err := models.FindDeckBuilderConversation(h.DB, userID); err == nil && convID != 0 {
		if err := models.DeleteConversation(h.DB, userID, convID); err != nil {
			log.Printf("AIExecute: close deck builder conversation: %v", err)
		}
	}

	h.redirectWithFlash(c, fmt.Sprintf("/decks/%d", deck.ID),
		fmt.Sprintf("Generated %d cards.", len(cards)))
}

func (h *Handler) CreateDeck(c *gin.Context) {
	userID := currentUserID(c)

	name := c.PostForm("name")
	mode := c.PostForm("mode")
	if mode == "" {
		mode = "srs"
	}

	if name == "" {
		h.render(c, http.StatusOK, "deck_form.html", PageData{
			Title: "New Deck",
			Data:  DeckFormData{Error: "Name is required"},
		})
		return
	}

	deck, err := models.CreateDeck(h.DB, userID, name, mode)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create deck")
		return
	}

	h.redirectWithFlash(c, fmt.Sprintf("/decks/%d", deck.ID), "Deck created.")
}

func (h *Handler) EditDeckForm(c *gin.Context) {
	userID := currentUserID(c)

	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, userID, id)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	h.render(c, http.StatusOK, "deck_form.html", PageData{
		Title: "Edit Deck",
		Flash: h.getFlash(c),
		Data:  DeckFormData{Deck: &deck},
	})
}

func (h *Handler) UpdateDeck(c *gin.Context) {
	userID := currentUserID(c)

	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	name := c.PostForm("name")
	mode := c.PostForm("mode")

	if name == "" {
		deck, err := models.GetDeckByID(h.DB, userID, id)
		if err != nil {
			c.String(http.StatusNotFound, "Deck not found")
			return
		}
		h.render(c, http.StatusOK, "deck_form.html", PageData{
			Title: "Edit Deck",
			Data:  DeckFormData{Error: "Name is required", Deck: &deck},
		})
		return
	}

	if err := models.UpdateDeck(h.DB, userID, id, name, mode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.String(http.StatusNotFound, "Deck not found")
			return
		}
		c.String(http.StatusInternalServerError, "Failed to update deck")
		return
	}

	h.redirectWithFlash(c, "/decks/"+c.Param("id"), "Saved.")
}

func (h *Handler) DeleteDeck(c *gin.Context) {
	userID := currentUserID(c)

	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	if err := models.DeleteDeck(h.DB, userID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.String(http.StatusNotFound, "Deck not found")
			return
		}
		c.String(http.StatusInternalServerError, "Failed to delete deck")
		return
	}

	h.redirectWithFlash(c, "/", "Deck deleted.")
}

func (h *Handler) DeckDetail(c *gin.Context) {
	userID := currentUserID(c)

	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, userID, id)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	filter := c.Query("filter")
	if filter == "" {
		filter = "all"
	}
	search := c.Query("q")

	cards, err := models.GetCardsByDeck(h.DB, id, filter, search)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load cards")
		return
	}
	if cards == nil {
		cards = []models.Card{}
	}

	h.render(c, http.StatusOK, "deck.html", PageData{
		Title: deck.Name,
		Flash: h.getFlash(c),
		Data: DeckDetailData{
			Deck:   deck,
			Cards:  cards,
			Filter: filter,
			Search: search,
		},
	})
}
