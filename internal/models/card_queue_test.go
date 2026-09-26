package models_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"memolang/internal/models"
)

// queueDeck makes a deck with the given card fronts, returning card ids in
// insertion order.
func queueDeck(t *testing.T, database *sql.DB, fronts ...string) (int64, []int64) {
	t.Helper()
	u, err := models.CreateUser(database, "queue@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	deck, err := models.CreateDeck(database, u.ID, "Queue", "linear")
	if err != nil {
		t.Fatalf("CreateDeck: %v", err)
	}
	var ids []int64
	for _, f := range fronts {
		c, err := models.CreateCard(database, deck.ID, f, f+"-back", "", "")
		if err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
		ids = append(ids, c.ID)
	}
	return deck.ID, ids
}

// pass marks a card as learned, due daysAhead from now.
func pass(t *testing.T, database *sql.DB, cardID int64, daysAhead int) {
	t.Helper()
	due := time.Now().AddDate(0, 0, daysAhead)
	if err := models.UpdateCardSRS(database, cardID, daysAhead, 2.5, 1, due); err != nil {
		t.Fatalf("UpdateCardSRS: %v", err)
	}
}

func TestLinearQueueUnlearnedFirstThenByDueDate(t *testing.T) {
	database := testDB(t)
	deckID, ids := queueDeck(t, database, "a", "b", "c", "d", "e")

	// b is learned and due far off, d learned and due sooner; a, c, e unlearned.
	pass(t, database, ids[1], 30)
	pass(t, database, ids[3], 3)

	got, err := models.GetLinearCardIDs(database, deckID, 10)
	if err != nil {
		t.Fatalf("GetLinearCardIDs: %v", err)
	}
	want := []int64{ids[0], ids[2], ids[4], ids[3], ids[1]}
	if !slices.Equal(got, want) {
		t.Errorf("queue = %v, want %v (unlearned in order, then learned by due date)", got, want)
	}
}

// The bug this replaces: once every card was passed, linear study was empty
// for good.
func TestLinearQueueNeverEmptiesOnceLearned(t *testing.T) {
	database := testDB(t)
	deckID, ids := queueDeck(t, database, "a", "b", "c")
	for i, id := range ids {
		pass(t, database, id, 10-i) // c due soonest, a last
	}

	got, err := models.GetLinearCardIDs(database, deckID, 2)
	if err != nil {
		t.Fatalf("GetLinearCardIDs: %v", err)
	}
	want := []int64{ids[2], ids[1]}
	if !slices.Equal(got, want) {
		t.Errorf("queue = %v, want %v (soonest-due learned cards, limited to 2)", got, want)
	}
}

func TestGetNextDueDate(t *testing.T) {
	database := testDB(t)
	deckID, ids := queueDeck(t, database, "a", "b")

	pass(t, database, ids[0], 6)
	pass(t, database, ids[1], 2)

	due, ok, err := models.GetNextDueDate(database, deckID)
	if err != nil || !ok {
		t.Fatalf("GetNextDueDate = %v, %v, %v", due, ok, err)
	}
	want := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	if due.Format("2006-01-02") != want {
		t.Errorf("next due = %s, want %s (the earliest card)", due.Format("2006-01-02"), want)
	}

	// A deck id with no cards: MIN() over nothing is NULL.
	_, ok, err = models.GetNextDueDate(database, deckID+1000)
	if err != nil || ok {
		t.Errorf("deck without cards: ok = %v, err = %v; want false, nil", ok, err)
	}
}
