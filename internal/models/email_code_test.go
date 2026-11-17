package models_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"memolang/internal/models"
)

func newUnverifiedUser(t *testing.T, database *sql.DB) models.User {
	t.Helper()
	u, err := models.CreateUser(database, "anna@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func isVerified(t *testing.T, database *sql.DB, id int64) bool {
	t.Helper()
	u, err := models.GetUserByID(database, id)
	if err != nil || u == nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	return u.Verified()
}

// wrongCode returns a 6-digit code that differs from code.
func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func TestVerifyEmailWithIssuedCode(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)

	code, err := models.IssueCode(database, u.ID, models.CodeVerify, "")
	if err != nil {
		t.Fatalf("IssueCode: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code %q, want 6 digits", code)
	}

	if err := models.VerifyEmail(database, u.ID, code); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if !isVerified(t, database, u.ID) {
		t.Fatal("user not verified after a correct code")
	}

	// Single use: the same code again is gone.
	if err := models.VerifyEmail(database, u.ID, code); !errors.Is(err, models.ErrCodeGone) {
		t.Fatalf("reusing a code = %v, want ErrCodeGone", err)
	}
}

func TestWrongCodesRunOut(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	code, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")

	for i := 1; i <= 4; i++ {
		if err := models.VerifyEmail(database, u.ID, wrongCode(code)); !errors.Is(err, models.ErrCodeWrong) {
			t.Fatalf("wrong try %d = %v, want ErrCodeWrong", i, err)
		}
	}
	// The fifth wrong try uses up the code…
	if err := models.VerifyEmail(database, u.ID, wrongCode(code)); !errors.Is(err, models.ErrCodeGone) {
		t.Fatalf("fifth wrong try = %v, want ErrCodeGone", err)
	}
	// …so even the right one no longer works.
	if err := models.VerifyEmail(database, u.ID, code); !errors.Is(err, models.ErrCodeGone) {
		t.Fatalf("right code after the tries ran out = %v, want ErrCodeGone", err)
	}
	if isVerified(t, database, u.ID) {
		t.Fatal("verified after the code ran out of tries")
	}
}

func TestExpiredCodeIsGone(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	code, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")

	if _, err := database.Exec("UPDATE email_codes SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if err := models.VerifyEmail(database, u.ID, code); !errors.Is(err, models.ErrCodeGone) {
		t.Fatalf("expired code = %v, want ErrCodeGone", err)
	}
}

// Resending replaces the code: the old one stops working and the try count
// starts over.
func TestReissueReplacesCode(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	first, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")
	second, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")
	if first == second {
		t.Skip("two random codes collided (1 in a million)")
	}

	if err := models.VerifyEmail(database, u.ID, first); !errors.Is(err, models.ErrCodeWrong) {
		t.Fatalf("old code = %v, want ErrCodeWrong", err)
	}
	if err := models.VerifyEmail(database, u.ID, second); err != nil {
		t.Fatalf("new code: %v", err)
	}
}

func TestCodeIsStoredHashed(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	code, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")

	var n int
	if err := database.QueryRow("SELECT COUNT(*) FROM email_codes WHERE code_hash = $1", code).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("the raw code is stored")
	}
}

func TestReplaceUnverifiedPassword(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)

	if err := models.ReplaceUnverifiedPassword(database, u.ID, "new"); err != nil {
		t.Fatalf("unverified: %v", err)
	}

	code, _ := models.IssueCode(database, u.ID, models.CodeVerify, "")
	if err := models.VerifyEmail(database, u.ID, code); err != nil {
		t.Fatal(err)
	}
	if err := models.ReplaceUnverifiedPassword(database, u.ID, "hijack"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("verified account = %v, want sql.ErrNoRows", err)
	}
}

func TestResetPasswordEndsSessionsAndVerifies(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	if _, err := models.CreateUserSession(database, u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}

	code, _ := models.IssueCode(database, u.ID, models.CodeReset, "")
	if err := models.ResetPassword(database, u.ID, code, "newhash"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	got, _ := models.GetUserByID(database, u.ID)
	if got.PasswordHash != "newhash" || !got.Verified() {
		t.Errorf("after reset: hash %q, verified %v", got.PasswordHash, got.Verified())
	}
	if n, _ := models.CountUserSessions(database, u.ID); n != 0 {
		t.Errorf("sessions after reset = %d, want 0", n)
	}
}

// A reset code can't verify an email, nor the other way round: each code
// only does what it was sent for.
func TestCodesAreBoundToPurpose(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	code, _ := models.IssueCode(database, u.ID, models.CodeReset, "")

	if err := models.VerifyEmail(database, u.ID, code); !errors.Is(err, models.ErrCodeGone) {
		t.Fatalf("reset code used to verify = %v, want ErrCodeGone", err)
	}
}

func TestChangeEmail(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)

	code, err := models.IssueCode(database, u.ID, models.CodeChangeEmail, "New@Example.com")
	if err != nil {
		t.Fatalf("IssueCode: %v", err)
	}
	if pending, _ := models.PendingEmailChange(database, u.ID); pending != "new@example.com" {
		t.Fatalf("pending = %q, want the normalized new address", pending)
	}

	got, err := models.ChangeEmail(database, u.ID, code)
	if err != nil || got != "new@example.com" {
		t.Fatalf("ChangeEmail = %q, %v", got, err)
	}
	user, _ := models.GetUserByID(database, u.ID)
	if user.Email != "new@example.com" || !user.Verified() {
		t.Errorf("after change: %q verified=%v", user.Email, user.Verified())
	}
	if pending, _ := models.PendingEmailChange(database, u.ID); pending != "" {
		t.Errorf("pending after confirm = %q, want none", pending)
	}
}

func TestChangeEmailToTakenAddress(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	if _, err := models.CreateUser(database, "taken@example.com", "x"); err != nil {
		t.Fatal(err)
	}

	code, _ := models.IssueCode(database, u.ID, models.CodeChangeEmail, "taken@example.com")
	if _, err := models.ChangeEmail(database, u.ID, code); !errors.Is(err, models.ErrEmailTaken) {
		t.Fatalf("ChangeEmail to a taken address = %v, want ErrEmailTaken", err)
	}
}
