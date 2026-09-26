package models_test

import (
	"slices"
	"testing"

	"memolang/internal/models"
)

func TestAppendToSessionQueue(t *testing.T) {
	database := testDB(t)

	u, _ := models.CreateUser(database, "q@example.com", "hash")
	deck, _ := models.CreateDeck(database, u.ID, "Q", "srs")
	session, err := models.CreateSession(database, deck.ID, "flashcard", []int64{4, 5})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := models.AppendToSessionQueue(database, session.ID, 4); err != nil {
		t.Fatalf("AppendToSessionQueue: %v", err)
	}

	got, err := models.GetSessionByID(database, deck.ID, session.ID)
	if err != nil || got == nil {
		t.Fatalf("GetSessionByID = %v, %v", got, err)
	}
	if want := []int64{4, 5, 4}; !slices.Equal(got.CardQueue, want) {
		t.Errorf("queue = %v, want %v", got.CardQueue, want)
	}

	if err := models.AppendToSessionQueue(database, 9999, 4); err == nil {
		t.Error("appending to a missing session reported success")
	}
}
