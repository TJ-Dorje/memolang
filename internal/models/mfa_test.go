package models_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"memolang/internal/models"
	"memolang/internal/totp"
)

// enableMFA turns MFA on for u and returns its secret and recovery codes.
func enableMFA(t *testing.T, database *sql.DB, u models.User) (string, []string) {
	t.Helper()
	setup, err := totp.NewSetup(u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.StartTOTPSetup(database, u.ID, setup.Secret); err != nil {
		t.Fatalf("StartTOTPSetup: %v", err)
	}
	// Confirm with the previous step's code, so the current one is still
	// newer than the last accepted step for the test that follows.
	code, _ := totp.Code(setup.Secret, time.Now().Add(-30*time.Second))
	codes, err := models.ConfirmTOTPSetup(database, u.ID, code)
	if err != nil {
		t.Fatalf("ConfirmTOTPSetup: %v", err)
	}
	return setup.Secret, codes
}

func TestTOTPSetupAndOneTimeUse(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	secret, codes := enableMFA(t, database, u)

	if len(codes) != 10 {
		t.Fatalf("%d recovery codes, want 10", len(codes))
	}
	got, _ := models.GetUserByID(database, u.ID)
	if !got.MFAEnabled() {
		t.Fatal("MFA not on after confirming")
	}

	code, _ := totp.Code(secret, time.Now())
	if err := models.CheckMFA(database, u.ID, code); err != nil {
		t.Fatalf("current code: %v", err)
	}
	// The same code a second time is a replay.
	if err := models.CheckMFA(database, u.ID, code); !errors.Is(err, models.ErrBadMFACode) {
		t.Fatalf("replayed code = %v, want ErrBadMFACode", err)
	}
	if err := models.CheckMFA(database, u.ID, "123456"); !errors.Is(err, models.ErrBadMFACode) && code != "123456" {
		t.Fatalf("wrong code = %v, want ErrBadMFACode", err)
	}
}

func TestSetupRefusedWhileEnabled(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	enableMFA(t, database, u)

	if err := models.StartTOTPSetup(database, u.ID, "OTHERSECRET"); !errors.Is(err, models.ErrMFAEnabled) {
		t.Fatalf("setup over enabled MFA = %v, want ErrMFAEnabled", err)
	}
}

func TestRecoveryCodesWorkOnce(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	_, codes := enableMFA(t, database, u)

	// Typed the way people copy codes off paper: upper case, no dash.
	sloppy := "  " + strings.ToUpper(strings.ReplaceAll(codes[0], "-", "")) + " "
	if err := models.CheckMFA(database, u.ID, sloppy); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	if err := models.CheckMFA(database, u.ID, codes[0]); !errors.Is(err, models.ErrBadMFACode) {
		t.Fatalf("reused recovery code = %v, want ErrBadMFACode", err)
	}
	if n, _ := models.CountRecoveryCodes(database, u.ID); n != 9 {
		t.Fatalf("codes left = %d, want 9", n)
	}

	fresh, err := models.RegenerateRecoveryCodes(database, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.CheckMFA(database, u.ID, codes[1]); !errors.Is(err, models.ErrBadMFACode) {
		t.Fatal("an old code still works after regenerating")
	}
	if err := models.CheckMFA(database, u.ID, fresh[0]); err != nil {
		t.Fatalf("new code: %v", err)
	}
}

func TestDisableMFA(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	_, codes := enableMFA(t, database, u)

	if err := models.DisableMFA(database, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := models.GetUserByID(database, u.ID)
	if got.MFAEnabled() {
		t.Fatal("MFA still on")
	}
	if err := models.CheckMFA(database, u.ID, codes[0]); !errors.Is(err, models.ErrBadMFACode) {
		t.Fatal("a recovery code survived DisableMFA")
	}
}

func TestPendingLoginTriesRunOut(t *testing.T) {
	database := testDB(t)
	u := newUnverifiedUser(t, database)
	token, err := models.CreatePendingLogin(database, u.ID, "/decks/1")
	if err != nil {
		t.Fatal(err)
	}

	for want := 4; want >= 0; want-- {
		p, err := models.TryPendingLogin(database, token)
		if err != nil || p.UserID != u.ID || p.Next != "/decks/1" || p.TriesLeft != want {
			t.Fatalf("try: %+v, %v; want %d left", p, err, want)
		}
	}
	if _, err := models.TryPendingLogin(database, token); !errors.Is(err, models.ErrPendingLoginGone) {
		t.Fatalf("sixth try = %v, want ErrPendingLoginGone", err)
	}
	if ok, _ := models.LookupPendingLogin(database, token); ok {
		t.Fatal("a spent pending login still looks live")
	}
}
