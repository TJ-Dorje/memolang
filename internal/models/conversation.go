package models

import (
	"database/sql"
	"time"
)

// Conversation kinds.
const (
	KindTutor       = "tutor"        // about one card; one per user per card
	KindDeckBuilder = "deck_builder" // designing a new deck; one open per user
)

// ConversationMessage is one turn of an assistant conversation. Status is
// "done", "generating" (a reply still streaming) or "error".
type ConversationMessage struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
	Status         string
	CreatedAt      time.Time
}

// Callers must already have checked that a card belongs to the user
// (GetCardByID); these functions trust card ids and scope everything else by
// user id.

// FindTutorConversation returns the user's conversation about a card; 0 when
// there is none. It never creates one, so merely viewing a page writes nothing.
func FindTutorConversation(db *sql.DB, userID, cardID int64) (int64, error) {
	return findConversation(db,
		"SELECT id FROM conversations WHERE user_id = ? AND kind = 'tutor' AND card_id = ?", userID, cardID)
}

// FindDeckBuilderConversation returns the user's open deck-builder
// conversation; 0 when there is none.
func FindDeckBuilderConversation(db *sql.DB, userID int64) (int64, error) {
	return findConversation(db,
		"SELECT id FROM conversations WHERE user_id = ? AND kind = 'deck_builder'", userID)
}

func findConversation(db *sql.DB, query string, args ...any) (int64, error) {
	var id int64
	err := db.QueryRow(query, args...).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// GetOrCreateTutorConversation returns the user's conversation about a card,
// creating it on first use.
func GetOrCreateTutorConversation(db *sql.DB, userID, cardID int64) (int64, error) {
	if _, err := db.Exec(
		"INSERT INTO conversations (user_id, kind, card_id) VALUES (?, 'tutor', ?) ON CONFLICT DO NOTHING",
		userID, cardID,
	); err != nil {
		return 0, err
	}
	return FindTutorConversation(db, userID, cardID)
}

// GetOrCreateDeckBuilderConversation returns the user's open deck-builder
// conversation, creating it on first use.
func GetOrCreateDeckBuilderConversation(db *sql.DB, userID int64) (int64, error) {
	if _, err := db.Exec(
		"INSERT INTO conversations (user_id, kind) VALUES (?, 'deck_builder') ON CONFLICT DO NOTHING",
		userID,
	); err != nil {
		return 0, err
	}
	return FindDeckBuilderConversation(db, userID)
}

// GetConversationMessages returns a conversation's messages, oldest first.
// userID scopes it: another user's conversation id yields nothing.
func GetConversationMessages(db *sql.DB, userID, conversationID int64) ([]ConversationMessage, error) {
	rows, err := db.Query(
		`SELECT m.id, m.conversation_id, m.role, m.content, m.status, m.created_at
		 FROM conversation_messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE c.user_id = ? AND c.id = ?
		 ORDER BY m.id`,
		userID, conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func AddConversationMessage(db *sql.DB, conversationID int64, role, content, status string) (int64, error) {
	var id int64
	err := db.QueryRow(
		"INSERT INTO conversation_messages (conversation_id, role, content, status) VALUES (?, ?, ?, ?) RETURNING id",
		conversationID, role, content, status,
	).Scan(&id)
	return id, err
}

// FinishConversationMessage stores a reply's final text and status.
func FinishConversationMessage(db *sql.DB, id int64, content, status string) error {
	res, err := db.Exec("UPDATE conversation_messages SET content = ?, status = ? WHERE id = ?", content, status, id)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

// DeleteConversation removes one of the user's conversations and its
// messages. Deleting one that does not exist is not an error.
func DeleteConversation(db *sql.DB, userID, conversationID int64) error {
	_, err := db.Exec("DELETE FROM conversations WHERE id = ? AND user_id = ?", conversationID, userID)
	return err
}
