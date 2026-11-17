package models_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"memolang/internal/models"
)

func TestUserSessionRoundtrip(t *testing.T) {
	database := testDB(t)

	user, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, err := models.CreateUserSession(database, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateUserSession: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(token))
	}

	got, err := models.GetUserByToken(database, token)
	if err != nil {
		t.Fatalf("GetUserByToken: %v", err)
	}
	if got == nil || got.ID != user.ID {
		t.Fatalf("GetUserByToken = %v, want user %d", got, user.ID)
	}

	if err := models.DeleteUserSession(database, token); err != nil {
		t.Fatalf("DeleteUserSession: %v", err)
	}
	got, err = models.GetUserByToken(database, token)
	if err != nil {
		t.Fatalf("GetUserByToken after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("GetUserByToken after delete = %v, want nil", got)
	}
}

func TestGetUserByTokenExpired(t *testing.T) {
	database := testDB(t)

	user, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, err := models.CreateUserSession(database, user.ID, -time.Hour)
	if err != nil {
		t.Fatalf("CreateUserSession: %v", err)
	}

	got, err := models.GetUserByToken(database, token)
	if err != nil {
		t.Fatalf("GetUserByToken on an expired session returned an error: %v", err)
	}
	if got != nil {
		t.Fatalf("GetUserByToken on an expired session = %v, want nil", got)
	}
}

func TestGetUserByTokenUnknown(t *testing.T) {
	database := testDB(t)

	got, err := models.GetUserByToken(database, "not-a-real-token")
	if err != nil {
		t.Fatalf("GetUserByToken: %v", err)
	}
	if got != nil {
		t.Fatalf("GetUserByToken = %v, want nil", got)
	}
}

// The cookie value must never be stored: a leaked database or backup would
// otherwise be a list of live logins.
func TestUserSessionStoresOnlyTokenHash(t *testing.T) {
	database := testDB(t)

	user, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := models.CreateUserSession(database, user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateUserSession: %v", err)
	}

	var raw, hashed int
	if err := database.QueryRow("SELECT COUNT(*) FROM user_sessions WHERE token_hash = $1", token).Scan(&raw); err != nil {
		t.Fatalf("count raw: %v", err)
	}
	sum := sha256.Sum256([]byte(token))
	if err := database.QueryRow("SELECT COUNT(*) FROM user_sessions WHERE token_hash = $1", hex.EncodeToString(sum[:])).Scan(&hashed); err != nil {
		t.Fatalf("count hashed: %v", err)
	}
	if raw != 0 || hashed != 1 {
		t.Fatalf("rows by raw token = %d, by sha256 = %d; want 0 and 1", raw, hashed)
	}
}

func TestNewUserIsUnverified(t *testing.T) {
	database := testDB(t)

	user, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Verified() {
		t.Fatal("a freshly created user is verified, want unverified")
	}
}
