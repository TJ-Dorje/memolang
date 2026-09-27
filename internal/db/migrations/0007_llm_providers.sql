-- A user can keep several LLM providers and switch between them; exactly one
-- (or none) is active, and the assistant and card generation use that one.
-- Replaces the single set of llm.* keys in settings.

CREATE TABLE llm_providers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    preset     TEXT    NOT NULL,
    base_url   TEXT    NOT NULL DEFAULT '',
    model      TEXT    NOT NULL DEFAULT '',
    api_key    TEXT    NOT NULL DEFAULT '',
    active     INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_llm_providers_user ON llm_providers(user_id);

-- At most one active provider per user, enforced by the database rather
-- than trusted to the code that switches.
CREATE UNIQUE INDEX idx_llm_providers_active ON llm_providers(user_id) WHERE active = 1;

-- Each existing configuration becomes that user's first provider, active.
-- The names are the preset labels as of this migration (without the
-- "(free tier)" style suffixes), so the list reads well; they can be renamed.
INSERT INTO llm_providers (user_id, name, preset, base_url, model, api_key, active)
SELECT p.user_id,
       CASE p.value
           WHEN 'gemini'    THEN 'Google Gemini'
           WHEN 'groq'      THEN 'Groq'
           WHEN 'lmstudio'  THEN 'LM Studio'
           WHEN 'ollama'    THEN 'Ollama'
           WHEN 'openai'    THEN 'OpenAI'
           WHEN 'anthropic' THEN 'Anthropic'
           ELSE 'Custom'
       END,
       p.value,
       COALESCE((SELECT value FROM settings WHERE user_id = p.user_id AND key = 'llm.base_url'), ''),
       COALESCE((SELECT value FROM settings WHERE user_id = p.user_id AND key = 'llm.model'), ''),
       COALESCE((SELECT value FROM settings WHERE user_id = p.user_id AND key = 'llm.api_key'), ''),
       1
FROM settings p
WHERE p.key = 'llm.provider' AND p.value != '';

DELETE FROM settings WHERE key LIKE 'llm.%';
