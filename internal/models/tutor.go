package models

import (
	"database/sql"
	"time"
)

// TutorMessage is one turn of a tutor conversation. Status is "done",
// "generating" (a reply still streaming) or "error".
type TutorMessage struct {
	ID        int64
	ThreadID  int64
	Role      string
	Content   string
	Status    string
	CreatedAt time.Time
}

// Tutor threads are keyed by (user, card). Callers must already have checked
// the card belongs to the user (GetCardByID); these functions trust card ids.

// GetOrCreateTutorThread returns the user's thread for a card, creating it on
// first use.
func GetOrCreateTutorThread(db *sql.DB, userID, cardID int64) (int64, error) {
	if _, err := db.Exec(
		"INSERT INTO tutor_threads (user_id, card_id) VALUES (?, ?) ON CONFLICT (user_id, card_id) DO NOTHING",
		userID, cardID,
	); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow("SELECT id FROM tutor_threads WHERE user_id = ? AND card_id = ?", userID, cardID).Scan(&id)
	return id, err
}

// GetTutorMessages returns the user's conversation about a card, oldest
// first; empty when there is none yet. It never creates a thread, so merely
// viewing the tutor page writes nothing.
func GetTutorMessages(db *sql.DB, userID, cardID int64) ([]TutorMessage, error) {
	rows, err := db.Query(
		`SELECT m.id, m.thread_id, m.role, m.content, m.status, m.created_at
		 FROM tutor_messages m
		 JOIN tutor_threads t ON t.id = m.thread_id
		 WHERE t.user_id = ? AND t.card_id = ?
		 ORDER BY m.id`,
		userID, cardID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []TutorMessage
	for rows.Next() {
		var m TutorMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Content, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func AddTutorMessage(db *sql.DB, threadID int64, role, content, status string) (int64, error) {
	var id int64
	err := db.QueryRow(
		"INSERT INTO tutor_messages (thread_id, role, content, status) VALUES (?, ?, ?, ?) RETURNING id",
		threadID, role, content, status,
	).Scan(&id)
	return id, err
}

// FinishTutorMessage stores a reply's final text and status.
func FinishTutorMessage(db *sql.DB, id int64, content, status string) error {
	res, err := db.Exec("UPDATE tutor_messages SET content = ?, status = ? WHERE id = ?", content, status, id)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

// ResetTutorThread deletes the user's conversation about a card.
func ResetTutorThread(db *sql.DB, userID, cardID int64) error {
	_, err := db.Exec("DELETE FROM tutor_threads WHERE user_id = ? AND card_id = ?", userID, cardID)
	return err
}
