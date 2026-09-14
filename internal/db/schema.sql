CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);

CREATE TABLE IF NOT EXISTS decks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    mode       TEXT    NOT NULL DEFAULT 'srs',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_decks_user ON decks(user_id);

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
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
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
