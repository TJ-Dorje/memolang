package models

import (
	"database/sql"
	"time"
)

// SessionAnswer is one recorded answer within a study session.
type SessionAnswer struct {
	ID         int64
	SessionID  int64
	CardID     int64
	Correct    bool
	Given      string
	AnsweredAt time.Time
	// joined from cards (populated by GetSessionAnswers only):
	CardFront string
	CardBack  string
}

// RecordAnswer inserts a row and returns its ID.
func RecordAnswer(db *sql.DB, sessionID, cardID int64, correct bool, given string) (int64, error) {
	var id int64
	err := db.QueryRow(
		"INSERT INTO session_answers (session_id, card_id, correct, given) VALUES ($1, $2, $3, $4) RETURNING id",
		sessionID, cardID, correct, given,
	).Scan(&id)
	return id, err
}

// GetAnswerByID returns the answer row. sql.ErrNoRows → (nil, nil).
func GetAnswerByID(db *sql.DB, id int64) (*SessionAnswer, error) {
	var sa SessionAnswer
	err := db.QueryRow(
		`SELECT id, session_id, card_id, correct, given, answered_at
		 FROM session_answers WHERE id = $1`,
		id,
	).Scan(&sa.ID, &sa.SessionID, &sa.CardID, &sa.Correct, &sa.Given, &sa.AnsweredAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &sa, nil
}

// GetSessionAnswers returns all answers for a session ordered by id,
// with CardFront/CardBack populated via JOIN.
// If onlyWrong is true, only the wrong ones.
func GetSessionAnswers(db *sql.DB, sessionID int64, onlyWrong bool) ([]SessionAnswer, error) {
	query := `SELECT sa.id, sa.session_id, sa.card_id, sa.correct, sa.given, sa.answered_at,
		              c.front, c.back
		 FROM session_answers sa
		 JOIN cards c ON c.id = sa.card_id
		 WHERE sa.session_id = $1`
	if onlyWrong {
		query += " AND NOT sa.correct"
	}
	query += " ORDER BY sa.id"

	rows, err := db.Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []SessionAnswer
	for rows.Next() {
		var sa SessionAnswer
		if err := rows.Scan(&sa.ID, &sa.SessionID, &sa.CardID, &sa.Correct, &sa.Given, &sa.AnsweredAt, &sa.CardFront, &sa.CardBack); err != nil {
			return nil, err
		}
		answers = append(answers, sa)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if answers == nil {
		answers = []SessionAnswer{}
	}
	return answers, nil
}
