-- The schema as it stood before accounts existed.
--
-- Kept as IF NOT EXISTS on purpose: databases from before migrations report
-- user_version 0 whether they are brand new or pre-auth, so this file runs on
-- both. On a pre-auth database it is a no-op that only fills in any table the
-- old schema.sql had not created yet.

CREATE TABLE IF NOT EXISTS decks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    mode       TEXT    NOT NULL DEFAULT 'srs',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    front       TEXT    NOT NULL,
    back        TEXT    NOT NULL,
    example     TEXT,
    tags        TEXT    DEFAULT '',
    interval    INTEGER NOT NULL DEFAULT 1,
    ease        REAL    NOT NULL DEFAULT 2.5,
    repetitions INTEGER NOT NULL DEFAULT 0,
    due_date    DATE    NOT NULL DEFAULT (date('now')),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cards_deck_front ON cards(deck_id, front);

CREATE TABLE IF NOT EXISTS study_sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    quiz_mode   TEXT    NOT NULL,
    card_queue  TEXT    NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    correct     INTEGER NOT NULL DEFAULT 0,
    total       INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    ended_at    DATETIME
);

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS session_answers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  INTEGER NOT NULL REFERENCES study_sessions(id) ON DELETE CASCADE,
    card_id     INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    correct     INTEGER NOT NULL,
    given       TEXT    NOT NULL DEFAULT '',
    answered_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_session_answers_session ON session_answers(session_id);
