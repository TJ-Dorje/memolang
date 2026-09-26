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
	// DisplayName is optional; "" means not set. Use Name for display.
	DisplayName string
	CreatedAt   time.Time
}

// Name is what the UI calls the user: their display name, or their email
// until they set one.
func (u User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Email
}

// ErrEmailTaken is returned by CreateUser when the email already exists.
var ErrEmailTaken = errors.New("email already registered")

// userColumns and scanUser keep every query that loads a User in step, so a
// new column is added in one place instead of four.
const userColumns = "id, email, password_hash, display_name, created_at"

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	return u, err
}

// NormalizeEmail matches the COLLATE NOCASE constraint on users.email, so the
// Go layer and the DB agree on what counts as the same address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CreateUser(db *sql.DB, email, passwordHash string) (User, error) {
	u, err := scanUser(db.QueryRow(
		"INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING "+userColumns,
		NormalizeEmail(email), passwordHash,
	))
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
	u, err := scanUser(db.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE email = ?", NormalizeEmail(email),
	))
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
	u, err := scanUser(db.QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func UpdateDisplayName(db *sql.DB, userID int64, name string) error {
	res, err := db.Exec("UPDATE users SET display_name = ? WHERE id = ?", name, userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func UpdatePassword(db *sql.DB, userID int64, passwordHash string) error {
	res, err := db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

// DeleteUser removes the account. Everything it owns goes with it through
// ON DELETE CASCADE: login sessions, settings, and decks, which in turn take
// their cards, study sessions and answers.
func DeleteUser(db *sql.DB, userID int64) error {
	res, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}
