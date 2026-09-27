package models

import (
	"database/sql"
	"time"
)

// Card represents a single flashcard.
type Card struct {
	ID          int64
	DeckID      int64
	Front       string
	Back        string
	Example     string
	Tags        string
	Interval    int
	Ease        float64
	Repetitions int
	DueDate     time.Time
	CreatedAt   time.Time
}

func CreateCard(db *sql.DB, deckID int64, front, back, example, tags string) (Card, error) {
	var c Card
	err := db.QueryRow(
		`INSERT INTO cards (deck_id, front, back, example, tags)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, deck_id, front, back, example, tags, interval_days, ease, repetitions, due_date, created_at`,
		deckID, front, back, example, tags,
	).Scan(&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Example, &c.Tags,
		&c.Interval, &c.Ease, &c.Repetitions, &c.DueDate, &c.CreatedAt)
	return c, err
}

func GetCardsByDeck(db *sql.DB, deckID int64, filter, search string) ([]Card, error) {
	base := `SELECT id, deck_id, front, back, example, tags, interval_days, ease, repetitions, due_date, created_at FROM cards WHERE deck_id = $1`
	args := []any{deckID}

	switch filter {
	case "due":
		base += ` AND due_date <= CURRENT_DATE`
	case "new":
		base += ` AND repetitions = 0`
	}

	if search != "" {
		// ILIKE: SQLite's LIKE ignored case, Postgres's does not.
		base += ` AND (front ILIKE $2 OR back ILIKE $3)`
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	switch filter {
	case "due":
		base += ` ORDER BY due_date`
	default:
		base += ` ORDER BY id`
	}

	rows, err := db.Query(base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Example, &c.Tags,
			&c.Interval, &c.Ease, &c.Repetitions, &c.DueDate, &c.CreatedAt); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// GetCardByID is the one card-level entry point reached by a raw ID from a
// URL, so it joins through to decks and scopes by owner: a card in someone
// else's deck comes back as sql.ErrNoRows, exactly like a missing one.
func GetCardByID(db *sql.DB, userID, id int64) (Card, error) {
	var c Card
	err := db.QueryRow(
		`SELECT c.id, c.deck_id, c.front, c.back, c.example, c.tags, c.interval_days, c.ease, c.repetitions, c.due_date, c.created_at
		 FROM cards c
		 JOIN decks d ON d.id = c.deck_id
		 WHERE c.id = $1 AND d.user_id = $2`, id, userID,
	).Scan(&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Example, &c.Tags,
		&c.Interval, &c.Ease, &c.Repetitions, &c.DueDate, &c.CreatedAt)
	return c, err
}

func UpdateCard(db *sql.DB, id int64, front, back, example, tags string) error {
	_, err := db.Exec(
		"UPDATE cards SET front = $1, back = $2, example = $3, tags = $4 WHERE id = $5",
		front, back, example, tags, id,
	)
	return err
}

func UpdateCardSRS(db *sql.DB, id int64, interval int, ease float64, repetitions int, dueDate time.Time) error {
	_, err := db.Exec(
		"UPDATE cards SET interval_days = $1, ease = $2, repetitions = $3, due_date = $4 WHERE id = $5",
		interval, ease, repetitions, dueDate.Format("2006-01-02"), id,
	)
	return err
}

func DeleteCard(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM cards WHERE id = $1", id)
	return err
}

func GetRandomBackValues(db *sql.DB, deckID, excludeID int64, n int) ([]string, error) {
	rows, err := db.Query(
		"SELECT back FROM cards WHERE deck_id = $1 AND id != $2 ORDER BY RANDOM() LIMIT $3",
		deckID, excludeID, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backs []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		backs = append(backs, b)
	}
	return backs, rows.Err()
}

// GetNextDueDate returns when the deck's earliest card falls due, for telling
// the user when to come back. ok is false for a deck with no cards.
func GetNextDueDate(db *sql.DB, deckID int64) (due time.Time, ok bool, err error) {
	var next sql.NullTime
	if err := db.QueryRow("SELECT MIN(due_date) FROM cards WHERE deck_id = $1", deckID).Scan(&next); err != nil {
		return time.Time{}, false, err
	}
	return next.Time, next.Valid, nil
}

func GetDueCardIDs(db *sql.DB, deckID int64, limit int) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM cards WHERE deck_id = $1 AND due_date <= CURRENT_DATE ORDER BY due_date LIMIT $2`,
		deckID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetLinearCardIDs is the queue for a linear deck: every card, never passed
// ones first in the order they were added, then the rest by due date. It used
// to return only never-passed cards, so a card rated Good left linear study
// for good and a finished deck had nothing left to study. Ordering the known
// cards by due date makes the deck rotate on its own: each review pushes a
// card's due date out, so the next session reaches different ones.
func GetLinearCardIDs(db *sql.DB, deckID int64, limit int) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM cards WHERE deck_id = $1
		 ORDER BY repetitions > 0,
		          CASE WHEN repetitions = 0 THEN id END,
		          due_date, id
		 LIMIT $2`,
		deckID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountCards returns how many cards a deck has.
func CountCards(db *sql.DB, deckID int64) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM cards WHERE deck_id = $1", deckID).Scan(&n)
	return n, err
}
