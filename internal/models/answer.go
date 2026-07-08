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
	c := 0
	if correct {
		c = 1
	}
	res, err := db.Exec(
		"INSERT INTO session_answers (session_id, card_id, correct, given) VALUES (?, ?, ?, ?)",
		sessionID, cardID, c, given,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAnswerByID returns the answer row. sql.ErrNoRows → (nil, nil).
func GetAnswerByID(db *sql.DB, id int64) (*SessionAnswer, error) {
	var sa SessionAnswer
	var c int
	err := db.QueryRow(
		`SELECT id, session_id, card_id, correct, given, answered_at
		 FROM session_answers WHERE id = ?`,
		id,
	).Scan(&sa.ID, &sa.SessionID, &sa.CardID, &c, &sa.Given, &sa.AnsweredAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sa.Correct = c == 1
	return &sa, nil
}

// GetSessionAnswers returns all answers for a session ordered by id,
// with CardFront/CardBack populated via JOIN.
// If onlyWrong is true, filters WHERE correct = 0.
func GetSessionAnswers(db *sql.DB, sessionID int64, onlyWrong bool) ([]SessionAnswer, error) {
	query := `SELECT sa.id, sa.session_id, sa.card_id, sa.correct, sa.given, sa.answered_at,
		              c.front, c.back
		 FROM session_answers sa
		 JOIN cards c ON c.id = sa.card_id
		 WHERE sa.session_id = ?`
	if onlyWrong {
		query += " AND sa.correct = 0"
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
		var c int
		if err := rows.Scan(&sa.ID, &sa.SessionID, &sa.CardID, &c, &sa.Given, &sa.AnsweredAt, &sa.CardFront, &sa.CardBack); err != nil {
			return nil, err
		}
		sa.Correct = c == 1
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
