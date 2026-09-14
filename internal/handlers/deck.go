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
	if c.Query("ai_mode") == "true" {
		formData := AIFormData{Mode: "srs"}
		if tok := c.Query("token"); tok != "" {
			if v, ok := h.loadPending(tok); ok {
				formData = v.(AIFormData)
			}
		}
		h.render(c, http.StatusOK, "ai_form.html", PageData{
			Title: "Generate Cards with AI",
			Flash: h.getFlash(c),
			Data:  formData,
		})
		return
	}
	h.render(c, http.StatusOK, "deck_form.html", PageData{
		Title: "New Deck",
		Flash: h.getFlash(c),
	})
}

func (h *Handler) CreateDeckAI(c *gin.Context) {
	name := c.PostForm("name")
	language := c.PostForm("language")
	promptText := c.PostForm("prompt")
	mode := c.PostForm("mode")
	if mode == "" {
		mode = "srs"
	}

	formData := AIFormData{
		Name:     name,
		Language: language,
		Prompt:   promptText,
		Mode:     mode,
	}

	log.Printf("CreateDeckAI: name=%q language=%q prompt=%q mode=%q", name, language, promptText, mode)

	if name == "" || language == "" || promptText == "" {
		formData.Error = "Deck name, language, and prompt are required"
		h.render(c, http.StatusOK, "ai_form.html", PageData{
			Title: "Generate Cards with AI",
			Data:  formData,
		})
		return
	}

	token := h.storePending(formData)
	c.Redirect(http.StatusSeeOther, "/decks/new-ai/processing?token="+token)
}

func (h *Handler) AIProcessing(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Redirect(http.StatusSeeOther, "/decks/new")
		return
	}
	h.render(c, http.StatusOK, "ai_processing.html", PageData{
		Title: "Generating...",
		Data:  map[string]string{"Token": token},
	})
}

func (h *Handler) AIExecute(c *gin.Context) {
	userID := currentUserID(c)

	token := c.Query("token")
	if token == "" {
		c.Redirect(http.StatusSeeOther, "/decks/new")
		return
	}

	v, ok := h.loadPending(token)
	if !ok {
		c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true")
		return
	}
	fd := v.(AIFormData)

	cfg, err := ai.LoadConfig(h.DB, userID)
	if err != nil {
		log.Printf("AIExecute: LoadConfig failed: %v", err)
		fd.Error = "Failed to load LLM settings: " + err.Error()
		tok := h.storePending(fd)
		c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
		return
	}

	provider, err := ai.New(cfg)
	if err != nil {
		log.Printf("AIExecute: ai.New failed: %v", err)
		if errors.Is(err, ai.ErrNotConfigured) {
			fd.Error = "No LLM provider configured. Set one up in Settings first."
		} else {
			fd.Error = "LLM configuration error: " + err.Error()
		}
		tok := h.storePending(fd)
		c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
		return
	}

	log.Printf("AIExecute: calling GenerateCards...")
	cards, err := provider.GenerateCards(c.Request.Context(), fd.Language, fd.Prompt)
	if err != nil {
		log.Printf("AIExecute: GenerateCards failed: %v", err)
		fd.Error = "AI generation failed. You can retry."
		tok := h.storePending(fd)
		c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
		return
	}

	log.Printf("AIExecute: got %d cards", len(cards))
	if len(cards) == 0 {
		fd.Error = "AI returned no cards. Try a different prompt."
		tok := h.storePending(fd)
		c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
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
