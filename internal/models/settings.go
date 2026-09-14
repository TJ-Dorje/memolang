package models

import (
	"database/sql"
)

// Settings are per-user: LLM API keys are personal data, so every accessor is
// scoped by owner, matching the composite (user_id, key) primary key.

// GetSetting returns the value for key, or "" if the key does not exist.
func GetSetting(db *sql.DB, userID int64, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE user_id = ? AND key = ?", userID, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// SetSetting upserts a key/value pair for one user. The conflict target must
// name both PK columns — (key) alone matches no index under the composite key
// and fails at runtime.
func SetSetting(db *sql.DB, userID int64, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO settings (user_id, key, value, updated_at) VALUES (?, ?, ?, datetime('now')) "+
			"ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		userID, key, value,
	)
	return err
}

// GetSettings returns all of this user's keys with the given prefix (e.g. "llm.") as a map.
func GetSettings(db *sql.DB, userID int64, prefix string) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings WHERE user_id = ? AND key LIKE ? || '%'", userID, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

// DeleteSetting removes one of this user's keys from the settings table.
func DeleteSetting(db *sql.DB, userID int64, key string) error {
	_, err := db.Exec("DELETE FROM settings WHERE user_id = ? AND key = ?", userID, key)
	return err
}
