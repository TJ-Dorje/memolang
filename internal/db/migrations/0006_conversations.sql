-- Generalises the tutor's tables into conversations of different kinds:
-- 'tutor' (one per user per card) and 'deck_builder' (the interview that
-- designs a new deck; one open per user). Existing tutor threads and their
-- messages move over with their ids unchanged.

CREATE TABLE conversations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('tutor', 'deck_builder')),
    card_id    INTEGER REFERENCES cards(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    -- A tutor conversation is about a card; a deck builder's is not.
    CHECK ((kind = 'tutor') = (card_id IS NOT NULL))
);

CREATE UNIQUE INDEX idx_conversations_tutor ON conversations(user_id, card_id) WHERE kind = 'tutor';
CREATE UNIQUE INDEX idx_conversations_builder ON conversations(user_id) WHERE kind = 'deck_builder';

INSERT INTO conversations (id, user_id, kind, card_id, created_at)
SELECT id, user_id, 'tutor', card_id, created_at FROM tutor_threads;

CREATE TABLE conversation_messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT    NOT NULL CHECK (role IN ('user', 'assistant')),
    content         TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'done' CHECK (status IN ('done', 'generating', 'error')),
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO conversation_messages (id, conversation_id, role, content, status, created_at)
SELECT id, thread_id, role, content, status, created_at FROM tutor_messages;

CREATE INDEX idx_conversation_messages ON conversation_messages(conversation_id, id);

DROP TABLE tutor_messages;
DROP TABLE tutor_threads;
