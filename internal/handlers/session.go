package handlers

import (
	"fmt"
	"log"
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
		started, ok := h.startNewSession(c, deck)
		if !ok {
			return
		}
		session = started
	}

	if ans, card, ok := h.feedbackFor(c, userID, session); ok {
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

	if session.Position >= len(session.CardQueue) {
		h.endSessionIfRunning(session)
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

// startNewSession builds the card queue for a deck with no active session and
// creates it. It writes its own response and reports false when the caller
// should simply return — nothing due to study, a stale ?feedback= to shake
// off, or a failure.
func (h *Handler) startNewSession(c *gin.Context, deck models.Deck) (*models.StudySession, bool) {
	// A ?feedback= pointing at a session that no longer exists: drop the query
	// and start clean.
	if c.Query("feedback") != "" {
		c.Redirect(http.StatusSeeOther, "/decks/"+c.Param("id")+"/session")
		return nil, false
	}

	cardIDs, err := h.queueFor(deck)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to build queue")
		return nil, false
	}

	if len(cardIDs) == 0 {
		// Only an SRS deck with cards can come up empty (nothing due yet);
		// NextDue tells the user when to come back. Zero means no cards at all.
		nextDue, _, err := models.GetNextDueDate(h.DB, deck.ID)
		if err != nil {
			log.Printf("startNewSession: GetNextDueDate: %v", err)
		}
		h.render(c, http.StatusOK, "session.html", PageData{
			Title: deck.Name + " — Study",
			Flash: h.getFlash(c),
			Data: SessionData{
				Deck:    deck,
				Empty:   true,
				NextDue: nextDue,
			},
		})
		return nil, false
	}

	session, err := models.CreateSession(h.DB, deck.ID, quizModeFrom(c), cardIDs)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create session")
		return nil, false
	}
	return &session, true
}

// queueFor picks the card queue according to the deck's scheduling mode: SRS
// decks study what is due, linear decks go through the whole deck, unlearned
// cards first. Both grade with SM-2, so switching mode keeps all progress.
func (h *Handler) queueFor(deck models.Deck) ([]int64, error) {
	if deck.Mode == "srs" {
		return models.GetDueCardIDs(h.DB, deck.ID, 50)
	}
	return models.GetLinearCardIDs(h.DB, deck.ID, 20)
}

// quizModeFrom treats anything that is not an explicit multiple-choice request
// as a flashcard session.
func quizModeFrom(c *gin.Context) string {
	if c.Query("mode") == "mc" {
		return "mc"
	}
	return "flashcard"
}

// feedbackFor resolves a ?feedback=<answer id> into the answer and its card,
// reporting false whenever the feedback screen should not be shown: no such
// query, an unparseable or unknown id, or an answer belonging to a different
// session.
func (h *Handler) feedbackFor(c *gin.Context, userID int64, session *models.StudySession) (*models.SessionAnswer, models.Card, bool) {
	fbID, err := strconv.ParseInt(c.Query("feedback"), 10, 64)
	if err != nil {
		return nil, models.Card{}, false
	}

	ans, err := models.GetAnswerByID(h.DB, fbID)
	if err != nil || ans == nil || ans.SessionID != session.ID {
		return nil, models.Card{}, false
	}

	card, err := models.GetCardByID(h.DB, userID, ans.CardID)
	if err != nil {
		return nil, models.Card{}, false
	}
	return ans, card, true
}

// endSessionIfRunning closes a session that ran off the end of its queue,
// unless it was already closed (ending early, or a refresh of the summary).
func (h *Handler) endSessionIfRunning(session *models.StudySession) {
	if session.EndedAt != nil {
		return
	}
	models.EndSession(h.DB, session.ID)
}

// ratingLabels index by SM-2 rating, and double as the text shown for a
// flashcard answer on the summary screen.
var ratingLabels = [4]string{"Again", "Hard", "Good", "Easy"}

// gradeAnswer turns a submitted answer into an SM-2 rating, whether it counted
// as correct, and the label to record for the summary screen. The two quiz
// modes post different fields: multiple choice posts `choice`, flashcards post
// a self-assessed `rating`.
//
// The rating is clamped rather than validated because it arrives from the
// request body, where nothing stops a caller posting 99.
func gradeAnswer(c *gin.Context, card models.Card) (isCorrect bool, rating int, given string) {
	if choice := c.PostForm("choice"); choice != "" {
		if choice == card.Back {
			return true, 2, choice
		}
		return false, 0, choice
	}

	r, _ := strconv.Atoi(c.PostForm("rating"))
	r = min(max(r, 0), len(ratingLabels)-1)
	return r >= 2, r, ratingLabels[r]
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

	isCorrect, rating, given := gradeAnswer(c, card)
	answerID, _ := models.RecordAnswer(h.DB, session.ID, cardID, isCorrect, given)

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