package models_test

import (
	"testing"
	"time"

	"memolang/internal/models"
)

func TestUserNameFallsBackToEmail(t *testing.T) {
	database := testDB(t)

	u, err := models.CreateUser(database, "ann@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Name() != "ann@example.com" {
		t.Errorf("Name() with no display name = %q, want the email", u.Name())
	}

	if err := models.UpdateDisplayName(database, u.ID, "Ann"); err != nil {
		t.Fatalf("UpdateDisplayName: %v", err)
	}
	got, err := models.GetUserByID(database, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Name() != "Ann" {
		t.Errorf("Name() = %q, want %q", got.Name(), "Ann")
	}
}

// The session lookup is what fills the nav, so it must carry the name too.
func TestGetUserByTokenCarriesDisplayName(t *testing.T) {
	database := testDB(t)

	u, _ := models.CreateUser(database, "bo@example.com", "hash")
	models.UpdateDisplayName(database, u.ID, "Bo")
	token, err := models.CreateUserSession(database, u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateUserSession: %v", err)
	}

	got, err := models.GetUserByToken(database, token)
	if err != nil || got == nil {
		t.Fatalf("GetUserByToken = %v, %v", got, err)
	}
	if got.DisplayName != "Bo" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Bo")
	}
}

func TestUpdatePassword(t *testing.T) {
	database := testDB(t)

	u, _ := models.CreateUser(database, "cy@example.com", "old-hash")
	if err := models.UpdatePassword(database, u.ID, "new-hash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	got, _ := models.GetUserByID(database, u.ID)
	if got.PasswordHash != "new-hash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "new-hash")
	}

	if err := models.UpdatePassword(database, 9999, "x"); err == nil {
		t.Error("UpdatePassword for a missing user reported success")
	}
}

func TestDeleteOtherUserSessions(t *testing.T) {
	database := testDB(t)

	u, _ := models.CreateUser(database, "di@example.com", "hash")
	other, _ := models.CreateUser(database, "someone@example.com", "hash")
	keep, _ := models.CreateUserSession(database, u.ID, time.Hour)
	stale1, _ := models.CreateUserSession(database, u.ID, time.Hour)
	stale2, _ := models.CreateUserSession(database, u.ID, time.Hour)
	othersSession, _ := models.CreateUserSession(database, other.ID, time.Hour)

	if n, _ := models.CountUserSessions(database, u.ID); n != 3 {
		t.Fatalf("CountUserSessions = %d, want 3", n)
	}

	ended, err := models.DeleteOtherUserSessions(database, u.ID, keep)
	if err != nil {
		t.Fatalf("DeleteOtherUserSessions: %v", err)
	}
	if ended != 2 {
		t.Errorf("ended = %d, want 2", ended)
	}

	for _, tc := range []struct {
		name, token string
		alive       bool
	}{
		{"kept session", keep, true},
		{"other session 1", stale1, false},
		{"other session 2", stale2, false},
		{"another user's session", othersSession, true},
	} {
		got, _ := models.GetUserByToken(database, tc.token)
		if (got != nil) != tc.alive {
			t.Errorf("%s: alive = %v, want %v", tc.name, got != nil, tc.alive)
		}
	}
}

func TestDeleteUserCascades(t *testing.T) {
	database := testDB(t)

	u, _ := models.CreateUser(database, "ed@example.com", "hash")
	deck, err := models.CreateDeck(database, u.ID, "Doomed", "srs")
	if err != nil {
		t.Fatalf("CreateDeck: %v", err)
	}
	if _, err := models.CreateCard(database, deck.ID, "a", "b", "", ""); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	token, _ := models.CreateUserSession(database, u.ID, time.Hour)
	models.SetSetting(database, u.ID, "llm.model", "m")

	if err := models.DeleteUser(database, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	for _, q := range []string{
		"SELECT COUNT(*) FROM users",
		"SELECT COUNT(*) FROM decks",
		"SELECT COUNT(*) FROM cards",
		"SELECT COUNT(*) FROM user_sessions",
		"SELECT COUNT(*) FROM settings",
	} {
		var n int
		if err := database.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if n != 0 {
			t.Errorf("%s = %d after DeleteUser, want 0", q, n)
		}
	}
	if got, _ := models.GetUserByToken(database, token); got != nil {
		t.Error("session still resolves after the account was deleted")
	}
}
