package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

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

// renderWaiting shows the spinner page, which immediately navigates (meta
// refresh, no JavaScript) to next: a slow GET that does the LLM work. The
// browser keeps this page on screen until next responds.
func (h *Handler) renderWaiting(c *gin.Context, title, message, sub, next string) {
	h.render(c, http.StatusOK, "ai_processing.html", PageData{
		Title: title,
		Data:  WaitingData{Message: message, Sub: sub, Next: next},
	})
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
