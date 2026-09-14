package models_test

import (
	"errors"
	"testing"

	"memolang/internal/models"
)

func TestCreateAndGetUser(t *testing.T) {
	database := testDB(t)

	created, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("CreateUser returned a zero ID")
	}

	got, err := models.GetUserByEmail(database, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("GetUserByEmail = %v, want user %d", got, created.ID)
	}

	byID, err := models.GetUserByID(database, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID == nil || byID.Email != "alice@example.com" {
		t.Fatalf("GetUserByID = %v, want alice@example.com", byID)
	}
}

func TestGetUserByEmailMissing(t *testing.T) {
	database := testDB(t)

	got, err := models.GetUserByEmail(database, "nobody@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got != nil {
		t.Fatalf("GetUserByEmail = %v, want nil", got)
	}
}

func TestUserEmailIsCaseInsensitive(t *testing.T) {
	database := testDB(t)

	created, err := models.CreateUser(database, "Foo@x.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := models.GetUserByEmail(database, "foo@x.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("GetUserByEmail(foo@x.com) = %v, want user %d", got, created.ID)
	}

	if _, err := models.CreateUser(database, "FOO@X.COM", "hash"); !errors.Is(err, models.ErrEmailTaken) {
		t.Fatalf("CreateUser with a differently-cased duplicate = %v, want ErrEmailTaken", err)
	}
}
