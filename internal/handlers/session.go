package handlers

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"

	"memolang/internal/models"
	"memolang/internal/srs"

	"github.com/gin-gonic/gin"
)

func (h *Handler) StartSession(c *gin.Context) {
	userID := currentUserID(c)

	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, userID, deckID)
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
		if c.Query("feedback") != "" {
			c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session")
			return
		}
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

	if fbID, err := strconv.ParseInt(c.Query("feedback"), 10, 64); err == nil {
		ans, err := models.GetAnswerByID(h.DB, fbID)
		if err == nil && ans != nil && ans.SessionID == session.ID {
			card, err := models.GetCardByID(h.DB, userID, ans.CardID)
			if err == nil {
				h.render(c, http.StatusOK, "session.html", PageData{
					Title: deck.Name + " — Study",
					Flash: h.getFlash(c),
					Data: SessionData{
						Deck: deck, Session: *session,
						Feedback: true, Answer: *ans, AnswerCard: card,
						Progress: float64(session.Position) / float64(len(session.CardQueue)) * 100,
					},
				})
				return
			}
		}
	}

	if session.Position >= len(session.CardQueue) {
		if session.EndedAt == nil {
			models.EndSession(h.DB, session.ID)
		}
		c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session/summary")
		return
	}

	cardID := session.CardQueue[session.Position]
	card, err := models.GetCardByID(h.DB, userID, cardID)
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
	userID := currentUserID(c)

	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	// session_id and card_id arrive in the request body, so both have to be
	// bound back to a deck this user actually owns before anything is written.
	if _, err := models.GetDeckByID(h.DB, userID, deckID); err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	sessionID, _ := strconv.ParseInt(c.PostForm("session_id"), 10, 64)
	cardID, _ := strconv.ParseInt(c.PostForm("card_id"), 10, 64)

	session, err := models.GetSessionByID(h.DB, deckID, sessionID)
	if err != nil || session == nil {
		c.String(http.StatusNotFound, "Session not found")
		return
	}

	card, err := models.GetCardByID(h.DB, userID, cardID)
	if err != nil {
		c.String(http.StatusNotFound, "Card not found")
		return
	}
	if card.DeckID != deckID {
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
		if r < 0 {
			r = 0
		} else if r > 3 {
			r = 3
		}
		rating = r
		isCorrect = (rating >= 2)
	}

	var answerID int64
	if c.PostForm("choice") != "" {
		answerID, _ = models.RecordAnswer(h.DB, session.ID, cardID, isCorrect, c.PostForm("choice"))
	} else {
		labels := []string{"Again", "Hard", "Good", "Easy"}
		given := "Unknown"
		if rating >= 0 && rating <= 3 {
			given = labels[rating]
		}
		answerID, _ = models.RecordAnswer(h.DB, session.ID, cardID, isCorrect, given)
	}

	state := srs.CardState{Interval: card.Interval, Ease: card.Ease, Repetitions: card.Repetitions}
	newState, dueDate := srs.Update(state, rating)
	models.UpdateCardSRS(h.DB, cardID, newState.Interval, newState.Ease, newState.Repetitions, dueDate)

	if err := models.AdvanceSession(h.DB, session.ID, isCorrect); err != nil {
		c.String(http.StatusInternalServerError, "Failed to advance session")
		return
	}

	if c.PostForm("choice") != "" {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/decks/%s/session?feedback=%d", c.Param("id"), answerID))
	} else {
		c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session")
	}
}

func (h *Handler) EndSessionEarly(c *gin.Context) {
	userID := currentUserID(c)

	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	if _, err := models.GetDeckByID(h.DB, userID, deckID); err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	sessionID, _ := strconv.ParseInt(c.PostForm("session_id"), 10, 64)
	session, err := models.GetSessionByID(h.DB, deckID, sessionID)
	if err != nil || session == nil {
		c.String(http.StatusNotFound, "Session not found")
		return
	}

	models.EndSession(h.DB, session.ID)
	h.redirectWithFlash(c, "/", "Session ended.")
}

func (h *Handler) SessionSummary(c *gin.Context) {
	userID := currentUserID(c)

	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, userID, deckID)
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

	wrong := session.Total - session.Correct
	var missedCards []models.SessionAnswer
	if wrong > 0 {
		missedCards, _ = models.GetSessionAnswers(h.DB, session.ID, true)
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
			Wrong:       wrong,
			MissedCards: missedCards,
		},
	})
}