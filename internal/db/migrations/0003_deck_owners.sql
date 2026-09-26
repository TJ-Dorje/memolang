-- Gives decks an owner and makes settings per-user. SQLite's ALTER TABLE can
-- add neither a NOT NULL REFERENCES column nor a new primary key, so both
-- tables are rebuilt: create *_new, copy, drop the old one, rename.
--
-- temp.migration_owner is created and filled by claimOrphanDecks (migrate.go)
-- in the same transaction. It holds the owner's id, or nothing when there
-- were no decks to own — in which case pre-auth settings are dropped.

CREATE TABLE decks_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    mode       TEXT    NOT NULL DEFAULT 'srs',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO decks_new (id, user_id, name, mode, created_at)
SELECT d.id, o.id, d.name, d.mode, d.created_at
FROM decks d CROSS JOIN temp.migration_owner o;

DROP TABLE decks;
ALTER TABLE decks_new RENAME TO decks;

CREATE INDEX idx_decks_user ON decks(user_id);

CREATE TABLE settings_new (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);

INSERT INTO settings_new (user_id, key, value, updated_at)
SELECT o.id, s.key, s.value, s.updated_at
FROM settings s CROSS JOIN temp.migration_owner o;

DROP TABLE settings;
ALTER TABLE settings_new RENAME TO settings;

DROP TABLE temp.migration_owner;
