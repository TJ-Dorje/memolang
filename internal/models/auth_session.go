package models

import (
	"crypto/rand"
	"crypto/sha256"
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
		"INSERT INTO user_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)",
		hashToken(token), userID, time.Now().Add(ttl),
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
		`SELECT u.id, u.email, u.password_hash, u.display_name, u.created_at, u.email_verified_at
		 FROM user_sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`,
		hashToken(token),
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
	_, err := db.Exec("DELETE FROM user_sessions WHERE token_hash = $1", hashToken(token))
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
	res, err := db.Exec("DELETE FROM user_sessions WHERE user_id = $1 AND token_hash != $2", userID, hashToken(keepToken))
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

// hashToken is how a secret token is stored: hex(sha256(token)). The cookie
// (or email) carries the raw value; the database only ever sees the hash, so
// a leaked database or backup can't be replayed. A plain fast hash is right
// here, unlike for passwords: the token is 256 random bits, not guessable,
// and it is checked on every request.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
