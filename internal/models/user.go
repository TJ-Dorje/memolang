package models

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// User is an account. It is deliberately decoupled from how the account
// authenticated: PasswordHash may be empty for a future OAuth-only account.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// ErrEmailTaken is returned by CreateUser when the email already exists.
var ErrEmailTaken = errors.New("email already registered")

// NormalizeEmail matches the COLLATE NOCASE constraint on users.email, so the
// Go layer and the DB agree on what counts as the same address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CreateUser(db *sql.DB, email, passwordHash string) (User, error) {
	var u User
	err := db.QueryRow(
		"INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id, email, password_hash, created_at",
		NormalizeEmail(email), passwordHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	return u, nil
}

// GetUserByEmail returns nil, nil if no such user exists.
func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	var u User
	err := db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE email = ?",
		NormalizeEmail(email),
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID returns nil, nil if no such user exists.
func GetUserByID(db *sql.DB, id int64) (*User, error) {
	var u User
	err := db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
