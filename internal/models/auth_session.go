package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// CreateUserSession issues a login session token valid for ttl.
func CreateUserSession(db *sql.DB, userID int64, ttl time.Duration) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}

	_, err = db.Exec(
		"INSERT INTO user_sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, time.Now().Add(ttl).UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// GetUserByToken resolves a session cookie to its user. A missing or expired
// token is not an error: it returns nil, nil.
func GetUserByToken(db *sql.DB, token string) (*User, error) {
	var u User
	err := db.QueryRow(
		`SELECT u.id, u.email, u.password_hash, u.created_at
		 FROM user_sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > datetime('now')`,
		token,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func DeleteUserSession(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM user_sessions WHERE token = ?", token)
	return err
}

// newSessionToken returns 32 bytes of hex-encoded randomness. Unlike the
// short-lived in-memory cache keys elsewhere in the project, this is a
// 30-day credential, so a rand.Read failure must not pass silently.
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
