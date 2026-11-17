package models_test

import (
	"database/sql"
	"errors"
	"testing"

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
