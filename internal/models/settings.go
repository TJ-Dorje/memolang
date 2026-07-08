package models

import (
	"database/sql"
)

// GetSetting returns the value for key, or "" if the key does not exist.
func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// SetSetting upserts a key/value pair.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now')) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		key, value,
	)
	return err
}

// GetSettings returns all keys with the given prefix (e.g. "llm.") as a map.
func GetSettings(db *sql.DB, prefix string) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings WHERE key LIKE ? || '%'", prefix)
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

// DeleteSetting removes a key from the settings table.
func DeleteSetting(db *sql.DB, key string) error {
	_, err := db.Exec("DELETE FROM settings WHERE key = ?", key)
	return err
}
