package models_test

import (
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
