package handlers

import (
	"math/rand"
	"net/http"
	"strconv"

	"memolang/internal/models"
	"memolang/internal/srs"

	"github.com/gin-gonic/gin"
)

func (h *Handler) StartSession(c *gin.Context) {
	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, deckID)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	session, err := models.GetActiveSession(h.DB, deckID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to check active session")
		return
	}

	if session == nil {
		var cardIDs []int64
		if deck.Mode == "srs" {
			cardIDs, err = models.GetDueCardIDs(h.DB, deckID, 50)
		} else {
			cardIDs, err = models.GetNewCardIDs(h.DB, deckID, 20)
		}
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to build queue")
			return
		}

		if len(cardIDs) == 0 {
			h.render(c, http.StatusOK, "session.html", PageData{
				Title: deck.Name + " — Study",
				Flash: h.getFlash(c),
				Data: SessionData{
					Deck:  deck,
					Empty: true,
				},
			})
			return
		}

		quizMode := c.Query("mode")
		if quizMode != "mc" {
			quizMode = "flashcard"
		}
		newSess, err := models.CreateSession(h.DB, deckID, quizMode, cardIDs)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to create session")
			return
		}
		session = &newSess
	}

	if session.Position >= len(session.CardQueue) {
		if session.EndedAt == nil {
			models.EndSession(h.DB, session.ID)
		}
		c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session/summary")
		return
	}

	cardID := session.CardQueue[session.Position]
	card, err := models.GetCardByID(h.DB, cardID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load card")
		return
	}

	var mcOptions []string
	if session.QuizMode == "mc" {
		mcOptions, _ = models.GetRandomBackValues(h.DB, deckID, cardID, 3)
		mcOptions = append(mcOptions, card.Back)
		rand.Shuffle(len(mcOptions), func(i, j int) {
			mcOptions[i], mcOptions[j] = mcOptions[j], mcOptions[i]
		})
	}

	progress := float64(0)
	if len(session.CardQueue) > 0 {
		progress = float64(session.Position) / float64(len(session.CardQueue)) * 100
	}

	h.render(c, http.StatusOK, "session.html", PageData{
		Title: deck.Name + " — Study",
		Flash: h.getFlash(c),
		Data: SessionData{
			Deck:      deck,
			Session:   *session,
			Card:      card,
			Progress:  progress,
			MCOptions: mcOptions,
		},
	})
}

func (h *Handler) SubmitAnswer(c *gin.Context) {
	_, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	sessionID, _ := strconv.ParseInt(c.PostForm("session_id"), 10, 64)
	cardID, _ := strconv.ParseInt(c.PostForm("card_id"), 10, 64)

	card, err := models.GetCardByID(h.DB, cardID)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return
	}

	isCorrect := false
	rating := 2

	if c.PostForm("choice") != "" {
		choice := c.PostForm("choice")
		isCorrect = (choice == card.Back)
		if isCorrect {
			rating = 2
		} else {
			rating = 0
		}
	} else {
		r, _ := strconv.Atoi(c.PostForm("rating"))
		rating = r
		isCorrect = (rating >= 2)
	}

	state := srs.CardState{Interval: card.Interval, Ease: card.Ease, Repetitions: card.Repetitions}
	newState, dueDate := srs.Update(state, rating)
	models.UpdateCardSRS(h.DB, cardID, newState.Interval, newState.Ease, newState.Repetitions, dueDate)

	if err := models.AdvanceSession(h.DB, sessionID, isCorrect); err != nil {
		c.String(http.StatusInternalServerError, "Failed to advance session")
		return
	}

	c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session")
}

func (h *Handler) EndSessionEarly(c *gin.Context) {
	sessionID, _ := strconv.ParseInt(c.PostForm("session_id"), 10, 64)
	models.EndSession(h.DB, sessionID)
	h.redirectWithFlash(c, "/", "Session ended.")
}

func (h *Handler) SessionSummary(c *gin.Context) {
	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, deckID)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	session, err := models.GetLastEndedSession(h.DB, deckID)
	if err != nil || session == nil {
		c.String(http.StatusNotFound, "No completed session found")
		return
	}

	accuracy := 0
	if session.Total > 0 {
		accuracy = session.Correct * 100 / session.Total
	}

	var dueTomorrow int
	h.DB.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE deck_id = ? AND due_date = date('now', '+1 day')`,
		deckID,
	).Scan(&dueTomorrow)

	h.render(c, http.StatusOK, "session_summary.html", PageData{
		Title: deck.Name + " — Summary",
		Flash: h.getFlash(c),
		Data: SessionSummaryData{
			Deck:        deck,
			Session:     *session,
			Accuracy:    accuracy,
			DueTomorrow: dueTomorrow,
		},
	})
}