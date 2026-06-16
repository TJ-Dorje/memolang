package handlers

import (
	"net/http"
	"strconv"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) EditCardForm(c *gin.Context) {
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid card ID")
		return
	}

	card, err := models.GetCardByID(h.DB, id)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return
	}

	h.render(c, http.StatusOK, "card_edit.html", PageData{
		Title: "Edit Card",
		Flash: h.getFlash(c),
		Data:  CardEditData{Card: &card},
	})
}

func (h *Handler) UpdateCard(c *gin.Context) {
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid card ID")
		return
	}

	card, err := models.GetCardByID(h.DB, id)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return
	}

	front := c.PostForm("front")
	back := c.PostForm("back")
	example := c.PostForm("example")
	tags := c.PostForm("tags")

	if front == "" || back == "" {
		h.render(c, http.StatusOK, "card_edit.html", PageData{
			Title: "Edit Card",
			Data:  CardEditData{Error: "Front and back are required", Card: &card},
		})
		return
	}

	if err := models.UpdateCard(h.DB, id, front, back, example, tags); err != nil {
		c.String(http.StatusInternalServerError, "Failed to update card")
		return
	}

	h.redirectWithFlash(c, "/decks/"+strconv.FormatInt(card.DeckID, 10), "Card saved.")
}

func (h *Handler) DeleteCard(c *gin.Context) {
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid card ID")
		return
	}

	card, err := models.GetCardByID(h.DB, id)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return
	}

	if err := models.DeleteCard(h.DB, id); err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete card")
		return
	}

	h.redirectWithFlash(c, "/decks/"+strconv.FormatInt(card.DeckID, 10), "Card deleted.")
}