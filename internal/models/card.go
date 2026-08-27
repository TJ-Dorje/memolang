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
		 VALUES (?, ?, ?, ?, ?)
		 RETURNING id, deck_id, front, back, example, tags, interval, ease, repetitions, due_date, created_at`,
		deckID, front, back, example, tags,
	).Scan(&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Example, &c.Tags,
		&c.Interval, &c.Ease, &c.Repetitions, &c.DueDate, &c.CreatedAt)
	return c, err
}

func GetCardsByDeck(db *sql.DB, deckID int64, filter, search string) ([]Card, error) {
	base := `SELECT id, deck_id, front, back, example, tags, interval, ease, repetitions, due_date, created_at FROM cards WHERE deck_id = ?`
	args := []any{deckID}

	switch filter {
	case "due":
		base += ` AND due_date <= date('now')`
	case "new":
		base += ` AND repetitions = 0`
	}

	if search != "" {
		base += ` AND (front LIKE ? OR back LIKE ?)`
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

func GetCardByID(db *sql.DB, id int64) (Card, error) {
	var c Card
	err := db.QueryRow(
		`SELECT id, deck_id, front, back, example, tags, interval, ease, repetitions, due_date, created_at
		 FROM cards WHERE id = ?`, id,
	).Scan(&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Example, &c.Tags,
		&c.Interval, &c.Ease, &c.Repetitions, &c.DueDate, &c.CreatedAt)
	return c, err
}

func UpdateCard(db *sql.DB, id int64, front, back, example, tags string) error {
	_, err := db.Exec(
		"UPDATE cards SET front = ?, back = ?, example = ?, tags = ? WHERE id = ?",
		front, back, example, tags, id,
	)
	return err
}

func UpdateCardSRS(db *sql.DB, id int64, interval int, ease float64, repetitions int, dueDate time.Time) error {
	_, err := db.Exec(
		"UPDATE cards SET interval = ?, ease = ?, repetitions = ?, due_date = ? WHERE id = ?",
		interval, ease, repetitions, dueDate.Format("2006-01-02"), id,
	)
	return err
}

func DeleteCard(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM cards WHERE id = ?", id)
	return err
}

func GetRandomBackValues(db *sql.DB, deckID, excludeID int64, n int) ([]string, error) {
	rows, err := db.Query(
		"SELECT back FROM cards WHERE deck_id = ? AND id != ? ORDER BY RANDOM() LIMIT ?",
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

func GetDueCardIDs(db *sql.DB, deckID int64, limit int) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM cards WHERE deck_id = ? AND due_date <= date('now') ORDER BY due_date LIMIT ?`,
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

func GetNewCardIDs(db *sql.DB, deckID int64, limit int) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM cards WHERE deck_id = ? AND repetitions = 0 ORDER BY id LIMIT ?`,
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
