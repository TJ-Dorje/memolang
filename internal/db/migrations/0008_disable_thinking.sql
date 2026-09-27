-- Per-provider opt-out of the model's reasoning phase. Thinking models
-- (Qwen3 and alike) can spend most of a reply's time reasoning, which does
-- not make flashcards better; when set, the app asks them not to think
-- (ai.Config.DisableThinking).
ALTER TABLE llm_providers ADD COLUMN disable_thinking INTEGER NOT NULL DEFAULT 0 CHECK (disable_thinking IN (0, 1));
