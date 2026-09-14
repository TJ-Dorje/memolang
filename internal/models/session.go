package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// StudySession represents an active or completed study session.
type StudySession struct {
	ID        int64
	DeckID    int64
	QuizMode  string
	CardQueue []int64
	Position  int
	Correct   int
	Total     int
	StartedAt time.Time
	EndedAt   *time.Time
}

type sessionRow struct {
	ID        int64
	DeckID    int64
	QuizMode  string
	CardQueue string
	Position  int
	Correct   int
	Total     int
	StartedAt time.Time
	EndedAt   sql.NullTime
}

func CreateSession(db *sql.DB, deckID int64, quizMode string, cardIDs []int64) (StudySession, error) {
	queueJSON, err := json.Marshal(cardIDs)
	if err != nil {
		return StudySession{}, err
	}

	var row sessionRow
	err = db.QueryRow(
		`INSERT INTO study_sessions (deck_id, quiz_mode, card_queue, position, correct, total)
		 VALUES (?, ?, ?, 0, 0, 0)
		 RETURNING id, deck_id, quiz_mode, card_queue, position, correct, total, started_at, ended_at`,
		deckID, quizMode, string(queueJSON),
	).Scan(&row.ID, &row.DeckID, &row.QuizMode, &row.CardQueue, &row.Position,
		&row.Correct, &row.Total, &row.StartedAt, &row.EndedAt)
	if err != nil {
		return StudySession{}, err
	}
	return toSession(row), nil
}

func GetActiveSession(db *sql.DB, deckID int64) (*StudySession, error) {
	var row sessionRow
	err := db.QueryRow(
		`SELECT id, deck_id, quiz_mode, card_queue, position, correct, total, started_at, ended_at
		 FROM study_sessions
		 WHERE deck_id = ? AND ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`,
		deckID,
	).Scan(&row.ID, &row.DeckID, &row.QuizMode, &row.CardQueue, &row.Position,
		&row.Correct, &row.Total, &row.StartedAt, &row.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := toSession(row)
	return &s, nil
}

// GetSessionByID scopes a study session to a deck the caller has already
// proven they own. Handlers that take a session_id from the request body use
// it to bind that id to the deck in the URL. nil, nil if there is no match.
func GetSessionByID(db *sql.DB, deckID, id int64) (*StudySession, error) {
	var row sessionRow
	err := db.QueryRow(
		`SELECT id, deck_id, quiz_mode, card_queue, position, correct, total, started_at, ended_at
		 FROM study_sessions
		 WHERE id = ? AND deck_id = ?`,
		id, deckID,
	).Scan(&row.ID, &row.DeckID, &row.QuizMode, &row.CardQueue, &row.Position,
		&row.Correct, &row.Total, &row.StartedAt, &row.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := toSession(row)
	return &s, nil
}

func AdvanceSession(db *sql.DB, id int64, correct bool) error {
	if correct {
		_, err := db.Exec(
			"UPDATE study_sessions SET position = position + 1, total = total + 1, correct = correct + 1 WHERE id = ?",
			id,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE study_sessions SET position = position + 1, total = total + 1 WHERE id = ?",
		id,
	)
	return err
}

func EndSession(db *sql.DB, id int64) error {
	_, err := db.Exec("UPDATE study_sessions SET ended_at = datetime('now') WHERE id = ?", id)
	return err
}

func GetLastEndedSession(db *sql.DB, deckID int64) (*StudySession, error) {
	var row sessionRow
	err := db.QueryRow(
		`SELECT id, deck_id, quiz_mode, card_queue, position, correct, total, started_at, ended_at
		 FROM study_sessions
		 WHERE deck_id = ? AND ended_at IS NOT NULL
		 ORDER BY ended_at DESC LIMIT 1`,
		deckID,
	).Scan(&row.ID, &row.DeckID, &row.QuizMode, &row.CardQueue, &row.Position,
		&row.Correct, &row.Total, &row.StartedAt, &row.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := toSession(row)
	return &s, nil
}

func toSession(row sessionRow) StudySession {
	var queue []int64
	json.Unmarshal([]byte(row.CardQueue), &queue)
	s := StudySession{
		ID:        row.ID,
		DeckID:    row.DeckID,
		QuizMode:  row.QuizMode,
		CardQueue: queue,
		Position:  row.Position,
		Correct:   row.Correct,
		Total:     row.Total,
		StartedAt: row.StartedAt,
	}
	if row.EndedAt.Valid {
		s.EndedAt = &row.EndedAt.Time
	}
	return s
}