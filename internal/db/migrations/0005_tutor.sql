-- Tutor conversations: one thread per user per card. Deleting the user or the
-- card removes the thread and its messages.

CREATE TABLE tutor_threads (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id    INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, card_id)
);

-- status: 'done' for every user message and finished reply; 'generating'
-- while a reply streams; 'error' when generation failed or was interrupted.
CREATE TABLE tutor_messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL REFERENCES tutor_threads(id) ON DELETE CASCADE,
    role       TEXT    NOT NULL CHECK (role IN ('user', 'assistant')),
    content    TEXT    NOT NULL DEFAULT '',
    status     TEXT    NOT NULL DEFAULT 'done' CHECK (status IN ('done', 'generating', 'error')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tutor_messages_thread ON tutor_messages(thread_id, id);
