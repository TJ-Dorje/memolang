package models

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// CodePurpose says what an emailed code proves. Each user has at most one
// live code per purpose.
type CodePurpose string

const (
	CodeVerify      CodePurpose = "verify"
	CodeReset       CodePurpose = "reset"
	CodeChangeEmail CodePurpose = "change_email"
)

const (
	// CodeTTL is how long an emailed code works.
	CodeTTL = 15 * time.Minute
	// maxCodeAttempts wrong tries end a code; the user asks for a new one.
	maxCodeAttempts = 5
)

var (
	// ErrCodeWrong: the code didn't match; it still works for the
	// remaining tries.
	ErrCodeWrong = errors.New("code is wrong")
	// ErrCodeGone: there is no usable code — never sent, already used,
	// expired, or out of tries. Only a new code helps.
	ErrCodeGone = errors.New("code expired or used up")
)

// IssueCode creates a fresh 6-digit code for the user and purpose, replacing
// any earlier one, and returns it for the email. Only its hash is stored.
// newEmail is the address being confirmed, for CodeChangeEmail only.
func IssueCode(db *sql.DB, userID int64, purpose CodePurpose, newEmail string) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", err
	}
	_, err = db.Exec(
		`INSERT INTO email_codes (user_id, purpose, code_hash, new_email, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, purpose) DO UPDATE
		 SET code_hash = EXCLUDED.code_hash, new_email = EXCLUDED.new_email,
		     attempts = 0, created_at = now(), expires_at = EXCLUDED.expires_at`,
		userID, purpose, hashToken(code), NormalizeEmail(newEmail), time.Now().Add(CodeTTL),
	)
	if err != nil {
		return "", err
	}
	return code, nil
}

// consumeCode checks code inside tx and, when it matches, deletes it so it
// can't be used again; it returns the row's new_email. The row is locked
// (FOR UPDATE), so two requests racing with the right code can't both
// succeed, and two wrong guesses can't both count as one.
// A wrong or dead code is recorded in tx too; withCode commits it anyway.
func consumeCode(tx *sql.Tx, userID int64, purpose CodePurpose, code string) (string, error) {
	var hash, newEmail string
	var attempts int
	var expiresAt time.Time
	err := tx.QueryRow(
		`SELECT code_hash, new_email, attempts, expires_at FROM email_codes
		 WHERE user_id = $1 AND purpose = $2 FOR UPDATE`,
		userID, purpose,
	).Scan(&hash, &newEmail, &attempts, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrCodeGone
	}
	if err != nil {
		return "", err
	}

	if time.Now().After(expiresAt) || attempts >= maxCodeAttempts {
		_, err := tx.Exec("DELETE FROM email_codes WHERE user_id = $1 AND purpose = $2", userID, purpose)
		return "", errors.Join(ErrCodeGone, err)
	}

	// Constant-time compare: response timing must not hint how many leading
	// characters of the hash were right.
	if subtle.ConstantTimeCompare([]byte(hash), []byte(hashToken(code))) != 1 {
		return "", recordWrongCode(tx, userID, purpose, attempts)
	}

	if _, err := tx.Exec("DELETE FROM email_codes WHERE user_id = $1 AND purpose = $2", userID, purpose); err != nil {
		return "", err
	}
	return newEmail, nil
}

// recordWrongCode counts a failed try, deleting the code on the last one.
func recordWrongCode(tx *sql.Tx, userID int64, purpose CodePurpose, attempts int) error {
	if attempts+1 >= maxCodeAttempts {
		_, err := tx.Exec("DELETE FROM email_codes WHERE user_id = $1 AND purpose = $2", userID, purpose)
		return errors.Join(ErrCodeGone, err)
	}
	_, err := tx.Exec(
		"UPDATE email_codes SET attempts = attempts + 1 WHERE user_id = $1 AND purpose = $2",
		userID, purpose,
	)
	return errors.Join(ErrCodeWrong, err)
}

// withCode runs apply after a code checks out, all in one transaction: the
// code is used up if and only if apply succeeds. A wrong or dead code still
// commits, so its try is counted (or its row deleted).
func withCode(db *sql.DB, userID int64, purpose CodePurpose, code string, apply func(tx *sql.Tx, newEmail string) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newEmail, err := consumeCode(tx, userID, purpose, code)
	if errors.Is(err, ErrCodeWrong) || errors.Is(err, ErrCodeGone) {
		if cerr := tx.Commit(); cerr != nil {
			return cerr
		}
		return err
	}
	if err != nil {
		return err
	}

	if err := apply(tx, newEmail); err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyEmail marks the user's address confirmed if code is their live
// verification code.
func VerifyEmail(db *sql.DB, userID int64, code string) error {
	return withCode(db, userID, CodeVerify, code, func(tx *sql.Tx, _ string) error {
		_, err := tx.Exec(
			"UPDATE users SET email_verified_at = now() WHERE id = $1 AND email_verified_at IS NULL", userID,
		)
		return err
	})
}

// newCode returns 6 uniformly random digits. rand.Int avoids the bias a
// plain "random number % 1,000,000" would have.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// ResetPassword sets a new password if code is the user's live reset code,
// and ends every session: whoever had the old password must not stay logged
// in. Typing a code sent to the address also proves owning it, so an
// unverified account comes out verified.
func ResetPassword(db *sql.DB, userID int64, code, passwordHash string) error {
	return withCode(db, userID, CodeReset, code, func(tx *sql.Tx, _ string) error {
		if _, err := tx.Exec(
			`UPDATE users SET password_hash = $1, email_verified_at = COALESCE(email_verified_at, now())
			 WHERE id = $2`, passwordHash, userID,
		); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM user_sessions WHERE user_id = $1", userID)
		return err
	})
}

// ChangeEmail switches the user to the address their live change_email code
// was sent to, and returns it. ErrEmailTaken if another account took that
// address meanwhile.
func ChangeEmail(db *sql.DB, userID int64, code string) (string, error) {
	var changedTo string
	err := withCode(db, userID, CodeChangeEmail, code, func(tx *sql.Tx, newEmail string) error {
		_, err := tx.Exec("UPDATE users SET email = $1, email_verified_at = now() WHERE id = $2", newEmail, userID)
		if isUniqueViolation(err) {
			return ErrEmailTaken
		}
		changedTo = newEmail
		return err
	})
	return changedTo, err
}

// PendingEmailChange returns the address an unexpired email change is
// waiting to confirm, or "" if there is none.
func PendingEmailChange(db *sql.DB, userID int64) (string, error) {
	var newEmail string
	err := db.QueryRow(
		`SELECT new_email FROM email_codes
		 WHERE user_id = $1 AND purpose = $2 AND expires_at > now()`,
		userID, CodeChangeEmail,
	).Scan(&newEmail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return newEmail, err
}

// CancelCode drops the user's live code for purpose, if any.
func CancelCode(db *sql.DB, userID int64, purpose CodePurpose) error {
	_, err := db.Exec("DELETE FROM email_codes WHERE user_id = $1 AND purpose = $2", userID, purpose)
	return err
}
