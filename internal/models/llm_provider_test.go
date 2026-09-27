package models_test

import (
	"database/sql"
	"errors"
	"testing"

	"memolang/internal/models"
)

func newProvider(t *testing.T, database *sql.DB, userID int64, name string) int64 {
	t.Helper()
	id, err := models.CreateLLMProvider(database, models.LLMProvider{
		UserID: userID, Name: name, Preset: "custom", BaseURL: "http://x/v1", Model: "m",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider(%s): %v", name, err)
	}
	return id
}

func activeName(t *testing.T, database *sql.DB, userID int64) string {
	t.Helper()
	p, err := models.GetActiveLLMProvider(database, userID)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil {
		return ""
	}
	return p.Name
}

func TestFirstProviderBecomesActive(t *testing.T) {
	database := testDB(t)
	u, _ := models.CreateUser(database, "p@example.com", "hash")

	if activeName(t, database, u.ID) != "" {
		t.Fatal("a new user has an active provider")
	}
	newProvider(t, database, u.ID, "Home")
	newProvider(t, database, u.ID, "Work")

	if got := activeName(t, database, u.ID); got != "Home" {
		t.Errorf("active = %q, want the first provider", got)
	}
	list, _ := models.ListLLMProviders(database, u.ID)
	if len(list) != 2 || !list[0].Active || list[1].Active {
		t.Errorf("list = %+v, want the active one first and only one active", list)
	}
}

func TestActivateSwitches(t *testing.T) {
	database := testDB(t)
	u, _ := models.CreateUser(database, "p@example.com", "hash")
	newProvider(t, database, u.ID, "Home")
	work := newProvider(t, database, u.ID, "Work")

	if err := models.ActivateLLMProvider(database, u.ID, work); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got := activeName(t, database, u.ID); got != "Work" {
		t.Errorf("active = %q, want Work", got)
	}
	// Activating the active one again is fine.
	if err := models.ActivateLLMProvider(database, u.ID, work); err != nil {
		t.Errorf("re-activate: %v", err)
	}
}

// Another user's provider id must behave like a missing one — and a failed
// switch must not leave the user with nothing active.
func TestProvidersAreScopedToTheirOwner(t *testing.T) {
	database := testDB(t)
	alice, _ := models.CreateUser(database, "alice@example.com", "hash")
	bob, _ := models.CreateUser(database, "bob@example.com", "hash")
	newProvider(t, database, alice.ID, "Alice's")
	bobs := newProvider(t, database, bob.ID, "Bob's")

	if err := models.ActivateLLMProvider(database, alice.ID, bobs); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("activating someone else's provider = %v, want ErrNoRows", err)
	}
	if got := activeName(t, database, alice.ID); got != "Alice's" {
		t.Errorf("after a refused switch alice's active = %q, want unchanged", got)
	}
	if _, err := models.GetLLMProvider(database, alice.ID, bobs); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("reading someone else's provider = %v, want ErrNoRows", err)
	}
	if err := models.DeleteLLMProvider(database, alice.ID, bobs); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("deleting someone else's provider = %v, want ErrNoRows", err)
	}
	if err := models.UpdateLLMProvider(database, models.LLMProvider{ID: bobs, UserID: alice.ID, Name: "x"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("updating someone else's provider = %v, want ErrNoRows", err)
	}
}

func TestDeletingActiveLeavesNoneActive(t *testing.T) {
	database := testDB(t)
	u, _ := models.CreateUser(database, "p@example.com", "hash")
	home := newProvider(t, database, u.ID, "Home")
	newProvider(t, database, u.ID, "Work")

	if err := models.DeleteLLMProvider(database, u.ID, home); err != nil {
		t.Fatal(err)
	}
	if got := activeName(t, database, u.ID); got != "" {
		t.Errorf("active after deleting it = %q, want none", got)
	}
}
