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
		"INSERT INTO user_sessions (token, user_id, expires_at) VALUES ($1, $2, $3)",
		token, userID, time.Now().Add(ttl),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// GetUserByToken resolves a session cookie to its user. A missing or expired
// token is not an error: it returns nil, nil.
func GetUserByToken(db *sql.DB, token string) (*User, error) {
	u, err := scanUser(db.QueryRow(
		`SELECT u.id, u.email, u.password_hash, u.display_name, u.created_at
		 FROM user_sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = $1 AND s.expires_at > now()`,
		token,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func DeleteUserSession(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM user_sessions WHERE token = $1", token)
	return err
}

// CountUserSessions returns how many unexpired logins the user has, the
// current one included.
func CountUserSessions(db *sql.DB, userID int64) (int, error) {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND expires_at > now()", userID,
	).Scan(&n)
	return n, err
}

// DeleteOtherUserSessions logs the user out everywhere except the session
// identified by keepToken, and returns how many were ended. Used after a
// password change and by "sign out everywhere else": a stolen session must
// not outlive either.
func DeleteOtherUserSessions(db *sql.DB, userID int64, keepToken string) (int64, error) {
	res, err := db.Exec("DELETE FROM user_sessions WHERE user_id = $1 AND token != $2", userID, keepToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
