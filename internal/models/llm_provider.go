package models

import (
	"database/sql"
	"time"
)

// LLMProvider is one of a user's saved LLM configurations. At most one per
// user is Active; the assistant and card generation use that one.
type LLMProvider struct {
	ID      int64
	UserID  int64
	Name    string
	Preset  string
	BaseURL string
	Model   string
	APIKey  string
	Active  bool
	// DisableThinking asks the model to skip its reasoning phase.
	DisableThinking bool
	CreatedAt       time.Time
}

const llmProviderColumns = "id, user_id, name, preset, base_url, model, api_key, active, disable_thinking, created_at"

func scanLLMProvider(row rowScanner) (LLMProvider, error) {
	var p LLMProvider
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Preset, &p.BaseURL, &p.Model, &p.APIKey, &p.Active, &p.DisableThinking, &p.CreatedAt)
	return p, err
}

// ListLLMProviders returns the user's providers, the active one first, then
// in the order they were added.
func ListLLMProviders(db *sql.DB, userID int64) ([]LLMProvider, error) {
	rows, err := db.Query(
		"SELECT "+llmProviderColumns+" FROM llm_providers WHERE user_id = ? ORDER BY active DESC, id",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LLMProvider
	for rows.Next() {
		p, err := scanLLMProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetLLMProvider returns one of the user's providers; sql.ErrNoRows for an id
// that is not theirs, exactly like one that does not exist.
func GetLLMProvider(db *sql.DB, userID, id int64) (LLMProvider, error) {
	return scanLLMProvider(db.QueryRow(
		"SELECT "+llmProviderColumns+" FROM llm_providers WHERE id = ? AND user_id = ?", id, userID,
	))
}

// GetActiveLLMProvider returns the user's active provider, or nil, nil when
// none is active.
func GetActiveLLMProvider(db *sql.DB, userID int64) (*LLMProvider, error) {
	p, err := scanLLMProvider(db.QueryRow(
		"SELECT "+llmProviderColumns+" FROM llm_providers WHERE user_id = ? AND active = 1", userID,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateLLMProvider saves a new provider for p.UserID. It becomes active when
// the user has no active provider yet, so the first one just works.
func CreateLLMProvider(db *sql.DB, p LLMProvider) (int64, error) {
	var id int64
	err := db.QueryRow(
		`INSERT INTO llm_providers (user_id, name, preset, base_url, model, api_key, disable_thinking, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?,
		         NOT EXISTS (SELECT 1 FROM llm_providers WHERE user_id = ? AND active = 1))
		 RETURNING id`,
		p.UserID, p.Name, p.Preset, p.BaseURL, p.Model, p.APIKey, p.DisableThinking, p.UserID,
	).Scan(&id)
	return id, err
}

// UpdateLLMProvider saves p's editable fields; the key is written as given,
// so callers decide whether to keep, replace or clear it.
func UpdateLLMProvider(db *sql.DB, p LLMProvider) error {
	res, err := db.Exec(
		`UPDATE llm_providers SET name = ?, preset = ?, base_url = ?, model = ?, api_key = ?, disable_thinking = ?
		 WHERE id = ? AND user_id = ?`,
		p.Name, p.Preset, p.BaseURL, p.Model, p.APIKey, p.DisableThinking, p.ID, p.UserID,
	)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

// ActivateLLMProvider makes one of the user's providers the active one. Both
// updates run in one transaction: the unique index allows one active row per
// user, so the old one must be cleared before the new one is set, and a
// failure must not leave the user with none.
func ActivateLLMProvider(db *sql.DB, userID, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE llm_providers SET active = 0 WHERE user_id = ? AND active = 1", userID); err != nil {
		return err
	}
	res, err := tx.Exec("UPDATE llm_providers SET active = 1 WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if err := requireRowAffected(res); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLLMProvider removes one of the user's providers. Deleting the active
// one leaves none active: silently activating another could start billing a
// different key, so the user picks.
func DeleteLLMProvider(db *sql.DB, userID, id int64) error {
	res, err := db.Exec("DELETE FROM llm_providers WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}
