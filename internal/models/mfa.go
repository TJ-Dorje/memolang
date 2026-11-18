package models

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"strings"
	"time"

	"memolang/internal/totp"
)

// recoveryCodeCount is how many one-time codes a user gets at a time.
const recoveryCodeCount = 10

// recoveryAlphabet has no 0/o or 1/l/i, which people misread from paper.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

var (
	// ErrMFAEnabled: setup was started on an account that already has MFA.
	ErrMFAEnabled = errors.New("two-factor login is already on")
	// ErrBadMFACode: neither a current authenticator code nor an unused
	// recovery code.
	ErrBadMFACode = errors.New("code is not valid")
)

// StartTOTPSetup stores a new, not yet confirmed secret. Starting again
// replaces it; an account with MFA already on is refused, so a stolen
// session can't silently swap the owner's authenticator.
func StartTOTPSetup(db *sql.DB, userID int64, secret string) error {
	res, err := db.Exec(
		"UPDATE users SET totp_secret = $1 WHERE id = $2 AND totp_enabled_at IS NULL", secret, userID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMFAEnabled
	}
	return nil
}

// TOTPSecret returns the user's secret ("" if none) and whether it is
// confirmed (MFA on) or still in setup.
func TOTPSecret(db *sql.DB, userID int64) (secret string, enabled bool, err error) {
	err = db.QueryRow(
		"SELECT totp_secret, totp_enabled_at IS NOT NULL FROM users WHERE id = $1", userID,
	).Scan(&secret, &enabled)
	return secret, enabled, err
}

// ConfirmTOTPSetup turns MFA on if code matches the pending secret, and
// returns the first set of recovery codes, in plain text for the one time
// they are shown.
func ConfirmTOTPSetup(db *sql.DB, userID int64, code string) ([]string, error) {
	secret, enabled, err := TOTPSecret(db, userID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, ErrMFAEnabled
	}
	step, ok := totp.Match(secret, code, time.Now())
	if secret == "" || !ok {
		return nil, ErrBadMFACode
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE users SET totp_enabled_at = now(), totp_last_step = $1
		 WHERE id = $2 AND totp_secret = $3 AND totp_enabled_at IS NULL`,
		step, userID, secret,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// The secret changed (setup restarted elsewhere) or MFA got enabled
		// meanwhile.
		return nil, ErrBadMFACode
	}
	codes, err := replaceRecoveryCodes(tx, userID)
	if err != nil {
		return nil, err
	}
	return codes, tx.Commit()
}

// CheckMFA accepts either a current authenticator code (6 digits) or an
// unused recovery code, using it up. Anything else is ErrBadMFACode.
func CheckMFA(db *sql.DB, userID int64, code string) error {
	code = strings.TrimSpace(code)
	if isTOTPCode(code) {
		return checkTOTP(db, userID, code)
	}
	return useRecoveryCode(db, userID, code)
}

func isTOTPCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	return strings.Trim(code, "0123456789") == ""
}

// checkTOTP accepts code once. The UPDATE only succeeds when the matched
// step is newer than the last accepted one, so two requests with the same
// code — a replay, or a double submit — can't both pass.
func checkTOTP(db *sql.DB, userID int64, code string) error {
	secret, enabled, err := TOTPSecret(db, userID)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrBadMFACode
	}
	step, ok := totp.Match(secret, code, time.Now())
	if !ok {
		return ErrBadMFACode
	}
	res, err := db.Exec(
		"UPDATE users SET totp_last_step = $1 WHERE id = $2 AND totp_last_step < $1", step, userID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBadMFACode
	}
	return nil
}

func useRecoveryCode(db *sql.DB, userID int64, code string) error {
	res, err := db.Exec(
		`UPDATE recovery_codes SET used_at = now()
		 WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		userID, hashToken(normalizeRecoveryCode(code)),
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBadMFACode
	}
	return nil
}

// normalizeRecoveryCode lets people type a code the way they wrote it
// down: any case, with or without the dash or spaces.
func normalizeRecoveryCode(code string) string {
	code = strings.ToLower(code)
	return strings.NewReplacer("-", "", " ", "").Replace(code)
}

// RegenerateRecoveryCodes replaces every recovery code, used or not, and
// returns the new set.
func RegenerateRecoveryCodes(db *sql.DB, userID int64) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	codes, err := replaceRecoveryCodes(tx, userID)
	if err != nil {
		return nil, err
	}
	return codes, tx.Commit()
}

func replaceRecoveryCodes(tx *sql.Tx, userID int64) ([]string, error) {
	if _, err := tx.Exec("DELETE FROM recovery_codes WHERE user_id = $1", userID); err != nil {
		return nil, err
	}
	codes := make([]string, recoveryCodeCount)
	for i := range codes {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			"INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2)",
			userID, hashToken(normalizeRecoveryCode(code)),
		); err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// newRecoveryCode returns "xxxxx-xxxxx": 10 characters from a 31-letter
// alphabet, ~50 bits. Unlike a 6-digit code that is far too many to guess,
// so a plain SHA-256 hash protects it at rest.
func newRecoveryCode() (string, error) {
	max := big.NewInt(int64(len(recoveryAlphabet)))
	out := make([]byte, 0, 11)
	for i := range 10 {
		if i == 5 {
			out = append(out, '-')
		}
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out = append(out, recoveryAlphabet[n.Int64()])
	}
	return string(out), nil
}

// CountRecoveryCodes returns how many unused recovery codes remain.
func CountRecoveryCodes(db *sql.DB, userID int64) (int, error) {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM recovery_codes WHERE user_id = $1 AND used_at IS NULL", userID,
	).Scan(&n)
	return n, err
}

// DisableMFA turns two-factor login off and drops the secret and recovery
// codes. Used by the owner (after password + code) and by the operator's
// reset-mfa command for someone who lost both phone and codes.
func DisableMFA(db *sql.DB, userID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"UPDATE users SET totp_secret = '', totp_enabled_at = NULL, totp_last_step = 0 WHERE id = $1", userID,
	)
	if err != nil {
		return err
	}
	if err := requireRowAffected(res); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM recovery_codes WHERE user_id = $1", userID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM pending_logins WHERE user_id = $1", userID); err != nil {
		return err
	}
	return tx.Commit()
}
