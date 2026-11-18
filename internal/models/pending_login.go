package models

import (
	"database/sql"
	"errors"
	"time"
)

const (
	// PendingLoginTTL is how long the second-factor step may take.
	PendingLoginTTL = 5 * time.Minute
	// maxMFAAttempts codes may be tried per password login.
	maxMFAAttempts = 5
)

// ErrPendingLoginGone: the second-factor step expired or ran out of tries;
// the user starts again with the password.
var ErrPendingLoginGone = errors.New("login step expired")

// CreatePendingLogin records "password OK, waiting for the second factor"
// and returns the token for its cookie. Like a session token, only the hash
// is stored.
func CreatePendingLogin(db *sql.DB, userID int64, next string) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	_, err = db.Exec(
		"INSERT INTO pending_logins (token_hash, user_id, next, expires_at) VALUES ($1, $2, $3, $4)",
		hashToken(token), userID, next, time.Now().Add(PendingLoginTTL),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// PendingLogin is a login waiting for its second factor.
type PendingLogin struct {
	UserID int64
	Next   string
	// TriesLeft after the try being made now.
	TriesLeft int
}

// TryPendingLogin spends one try of the pending login and returns it, or
// ErrPendingLoginGone if it expired or has no tries left. The try is
// counted before the code is checked, in one atomic UPDATE, so parallel
// requests can't get more than maxMFAAttempts guesses between them.
func TryPendingLogin(db *sql.DB, token string) (PendingLogin, error) {
	var p PendingLogin
	var attempts int
	err := db.QueryRow(
		`UPDATE pending_logins SET attempts = attempts + 1
		 WHERE token_hash = $1 AND expires_at > now() AND attempts < $2
		 RETURNING user_id, next, attempts`,
		hashToken(token), maxMFAAttempts,
	).Scan(&p.UserID, &p.Next, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingLogin{}, ErrPendingLoginGone
	}
	p.TriesLeft = maxMFAAttempts - attempts
	return p, err
}

// LookupPendingLogin reports whether token is a live pending login, without
// spending a try (for showing the code page).
func LookupPendingLogin(db *sql.DB, token string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pending_logins
		 WHERE token_hash = $1 AND expires_at > now() AND attempts < $2`,
		hashToken(token), maxMFAAttempts,
	).Scan(&n)
	return n == 1, err
}

func DeletePendingLogin(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM pending_logins WHERE token_hash = $1", hashToken(token))
	return err
}
