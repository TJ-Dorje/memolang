package models

import (
	"database/sql"
	"time"
)

// Rate limiting is counted in Postgres, not in memory, so a limit holds
// across several web pods and survives a restart — a deploy must not hand an
// attacker a fresh set of tries. One row per counted event; a check counts
// the rows for (kind, key) inside the window.

// RecordAttempt counts one event, e.g. a failed login for an address.
func RecordAttempt(db *sql.DB, kind, key string) error {
	_, err := db.Exec("INSERT INTO auth_attempts (kind, key) VALUES ($1, $2)", kind, key)
	return err
}

// CountAttempts returns how many events (kind, key) has had in the last
// window.
func CountAttempts(db *sql.DB, kind, key string, window time.Duration) (int, error) {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM auth_attempts WHERE kind = $1 AND key = $2 AND created_at > $3",
		kind, key, time.Now().Add(-window),
	).Scan(&n)
	return n, err
}

// ClearAttempts forgets (kind, key)'s events, e.g. failed logins once the
// right password is given from the same place.
func ClearAttempts(db *sql.DB, kind, key string) error {
	_, err := db.Exec("DELETE FROM auth_attempts WHERE kind = $1 AND key = $2", kind, key)
	return err
}

const (
	// attemptRetention outlives every rate-limit window.
	attemptRetention = 24 * time.Hour
	// UnverifiedAccountTTL is how long an account may wait for its code
	// before it is deleted and the address is free again.
	UnverifiedAccountTTL = 7 * 24 * time.Hour
)

// SweepAuth deletes what has outlived its use: rate-limit events past every
// window, expired codes and pending logins, and accounts that never confirmed their address.
// Safe to run from several pods at once.
func SweepAuth(db *sql.DB) error {
	if _, err := db.Exec("DELETE FROM auth_attempts WHERE created_at < $1", time.Now().Add(-attemptRetention)); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM email_codes WHERE expires_at < now()"); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM pending_logins WHERE expires_at < now()"); err != nil {
		return err
	}
	_, err := db.Exec(
		"DELETE FROM users WHERE email_verified_at IS NULL AND created_at < $1",
		time.Now().Add(-UnverifiedAccountTTL),
	)
	return err
}
