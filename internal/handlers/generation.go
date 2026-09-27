package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"memolang/internal/assistant"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

func generationURL(deckID int64) string {
	return fmt.Sprintf("/decks/%d/generating", deckID)
}

// DeckGenerationPage shows a deck being generated: the page streams like the
// chat — top half, then a row per card as each is saved, then the outcome —
// with no JavaScript. Once the generation has finished and left the broker,
// the deck itself is the record, so the page just opens it.
func (h *Handler) DeckGenerationPage(c *gin.Context) {
	userID := currentUserID(c)
	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}
	deck, err := models.GetDeckByID(h.DB, userID, deckID)
	if err != nil {
		// Not the user's, or removed because generation produced nothing —
		// in which case the reason is waiting in the assistant's chat.
		c.Redirect(http.StatusSeeOther, deckBuilderURL)
		return
	}

	key := assistant.GenerationKey(deck.ID)
	if !h.Assistant.Broker.Exists(key) {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/decks/%d", deck.ID))
		return
	}

	data := &GenerationData{Deck: deck}
	pd := h.withChrome(c, PageData{Title: "Creating " + deck.Name, Data: data})

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	if err := h.renderPart(c, "generating_top", pd); err != nil {
		log.Printf("DeckGenerationPage: top: %v", err)
		return
	}
	c.Writer.Flush()

	// Events arrive as JSON lines. Each Publish is a whole line, but a
	// follower can receive several at once, so split on newlines.
	found, err := h.Assistant.Broker.Follow(c.Request.Context(), key, func(chunk string) error {
		for _, line := range strings.Split(strings.TrimSpace(chunk), "\n") {
			if err := h.writeGenerationEvent(c, data, line); err != nil {
				return err
			}
		}
		c.Writer.Flush()
		return nil
	})
	if !found || err != nil {
		// The stream went away mid-page (it expired, or the tab closed):
		// whatever was saved is in the deck.
		data.Interrupted = true
	}

	if err := h.renderPart(c, "generating_bottom", pd); err != nil {
		log.Printf("DeckGenerationPage: bottom: %v", err)
	}
}

// writeGenerationEvent renders one progress event into the page and records
// the outcome for the bottom half.
func (h *Handler) writeGenerationEvent(c *gin.Context, data *GenerationData, line string) error {
	if line == "" {
		return nil
	}
	var ev assistant.GenerationEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		log.Printf("DeckGenerationPage: bad event %q: %v", line, err)
		return nil
	}

	switch ev.Type {
	case "thinking":
		return h.HTML.Instance("_gen_thinking", nil).Render(c.Writer)
	case "card":
		data.Saved = ev.Saved
		return h.HTML.Instance("_gen_card", ev.Card).Render(c.Writer)
	case "done":
		data.Saved, data.Note, data.Finished = ev.Saved, ev.Message, true
	case "failed":
		data.Failed, data.Note, data.Finished = true, ev.Message, true
	}
	return nil
}
