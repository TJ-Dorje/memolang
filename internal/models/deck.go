package models

import (
	"database/sql"
	"time"
)

// Deck represents a flashcard deck.
type Deck struct {
	ID         int64
	Name       string
	Mode       string
	CreatedAt  time.Time
	CardCount  int
	DueCount   int
	MasteryPct int
}

func CreateDeck(db *sql.DB, name, mode string) (Deck, error) {
	var d Deck
	err := db.QueryRow(
		"INSERT INTO decks (name, mode) VALUES (?, ?) RETURNING id, name, mode, created_at",
		name, mode,
	).Scan(&d.ID, &d.Name, &d.Mode, &d.CreatedAt)
	return d, err
}

func GetAllDecks(db *sql.DB) ([]Deck, error) {
	rows, err := db.Query(`
		SELECT d.id, d.name, d.mode, d.created_at,
			COUNT(c.id) AS card_count,
			COUNT(CASE WHEN c.due_date <= date('now') THEN 1 END) AS due_count,
			CASE WHEN COUNT(c.id) > 0
				THEN CAST(100.0 * COUNT(CASE WHEN c.repetitions >= 3 THEN 1 END) / COUNT(c.id) AS INTEGER)
				ELSE 0 END AS mastery_pct
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		GROUP BY d.id
		ORDER BY d.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decks []Deck
	for rows.Next() {
		var d Deck
		if err := rows.Scan(&d.ID, &d.Name, &d.Mode, &d.CreatedAt, &d.CardCount, &d.DueCount, &d.MasteryPct); err != nil {
			return nil, err
		}
		decks = append(decks, d)
	}
	return decks, rows.Err()
}

func GetDeckByID(db *sql.DB, id int64) (Deck, error) {
	var d Deck
	err := db.QueryRow(`
		SELECT d.id, d.name, d.mode, d.created_at,
			COUNT(c.id) AS card_count,
			COUNT(CASE WHEN c.due_date <= date('now') THEN 1 END) AS due_count,
			CASE WHEN COUNT(c.id) > 0
				THEN CAST(100.0 * COUNT(CASE WHEN c.repetitions >= 3 THEN 1 END) / COUNT(c.id) AS INTEGER)
				ELSE 0 END AS mastery_pct
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		WHERE d.id = ?
		GROUP BY d.id
	`, id).Scan(&d.ID, &d.Name, &d.Mode, &d.CreatedAt, &d.CardCount, &d.DueCount, &d.MasteryPct)
	if err != nil {
		return Deck{}, err
	}
	return d, nil
}

func UpdateDeck(db *sql.DB, id int64, name, mode string) error {
	_, err := db.Exec("UPDATE decks SET name = ?, mode = ? WHERE id = ?", name, mode, id)
	return err
}

func DeleteDeck(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM decks WHERE id = ?", id)
	return err
}