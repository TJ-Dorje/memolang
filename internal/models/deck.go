package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Deck represents a flashcard deck. Every deck is owned by exactly one user,
// and all deck queries are scoped by that owner.
type Deck struct {
	ID         int64
	UserID     int64
	Name       string
	Mode       string
	CreatedAt  time.Time
	CardCount  int
	DueCount   int
	MasteryPct int
}

func CreateDeck(db *sql.DB, userID int64, name, mode string) (Deck, error) {
	var d Deck
	err := db.QueryRow(
		"INSERT INTO decks (user_id, name, mode) VALUES ($1, $2, $3) RETURNING id, user_id, name, mode, created_at",
		userID, name, mode,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.Mode, &d.CreatedAt)
	return d, err
}

func GetAllDecks(db *sql.DB, userID int64) ([]Deck, error) {
	rows, err := db.Query(`
		SELECT d.id, d.user_id, d.name, d.mode, d.created_at,
			COUNT(c.id) AS card_count,
			COUNT(CASE WHEN c.due_date <= CURRENT_DATE THEN 1 END) AS due_count,
			CASE WHEN COUNT(c.id) > 0
				THEN CAST(100.0 * COUNT(CASE WHEN c.repetitions >= 3 THEN 1 END) / COUNT(c.id) AS INTEGER)
				ELSE 0 END AS mastery_pct
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		WHERE d.user_id = $1
		GROUP BY d.id
		ORDER BY d.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decks []Deck
	for rows.Next() {
		var d Deck
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Mode, &d.CreatedAt, &d.CardCount, &d.DueCount, &d.MasteryPct); err != nil {
			return nil, err
		}
		decks = append(decks, d)
	}
	return decks, rows.Err()
}

// GetDeckByID scopes the lookup to the owner, so a deck belonging to someone
// else is indistinguishable from one that does not exist: both come back as
// sql.ErrNoRows, which handlers already render as a 404.
func GetDeckByID(db *sql.DB, userID, id int64) (Deck, error) {
	var d Deck
	err := db.QueryRow(`
		SELECT d.id, d.user_id, d.name, d.mode, d.created_at,
			COUNT(c.id) AS card_count,
			COUNT(CASE WHEN c.due_date <= CURRENT_DATE THEN 1 END) AS due_count,
			CASE WHEN COUNT(c.id) > 0
				THEN CAST(100.0 * COUNT(CASE WHEN c.repetitions >= 3 THEN 1 END) / COUNT(c.id) AS INTEGER)
				ELSE 0 END AS mastery_pct
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		WHERE d.id = $1 AND d.user_id = $2
		GROUP BY d.id
	`, id, userID).Scan(&d.ID, &d.UserID, &d.Name, &d.Mode, &d.CreatedAt, &d.CardCount, &d.DueCount, &d.MasteryPct)
	if err != nil {
		return Deck{}, err
	}
	return d, nil
}

func UpdateDeck(db *sql.DB, userID, id int64, name, mode string) error {
	res, err := db.Exec("UPDATE decks SET name = $1, mode = $2 WHERE id = $3 AND user_id = $4", name, mode, id, userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func DeleteDeck(db *sql.DB, userID, id int64) error {
	res, err := db.Exec("DELETE FROM decks WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

// requireRowAffected turns a scoped write that matched nothing into
// sql.ErrNoRows. Without it a foreign id would report success and the handler
// would flash "Saved." having changed nothing.
func requireRowAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// isUniqueViolation reports whether err is Postgres refusing a duplicate
// (SQLSTATE 23505), matched by code rather than by message text.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
