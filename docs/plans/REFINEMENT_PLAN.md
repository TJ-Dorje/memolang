# MemoLang — Refinement Plan: LLM Configuration & Session Feedback

Continues the backlog in `PLAN.md` (last task there: T-029). Two feature tracks:

- **M7 — Configurable LLM provider + Settings page.** Replace the hardcoded
  Ollama/env-var integration with a DB-backed configuration editable in the UI,
  supporting any LLM provider.
- **M8 — In-session answer feedback + session stats.** Show the user whether
  each multiple-choice answer was right or wrong before moving on, keep a
  running score, and show a detailed right/wrong breakdown at session end.

Tasks are written for autonomous coding agents: every task lists exact files,
signatures, SQL, and a verifiable "Done when". Follow the existing conventions
in `CLAUDE.md` (no ORM, plain SQL in `internal/models/`, server-rendered
templates, POST + redirect for all mutations, flash cookies).

---

## Feature 1 (M7): Configurable LLM Provider

### Current state (what we're replacing)

- `internal/ai/client.go` talks to Ollama only, via the `github.com/ollama/ollama/api`
  SDK, configured by `OLLAMA_HOST` / `OLLAMA_MODEL` env vars.
- `AIExecute` in `internal/handlers/deck.go` calls `ai.NewClient()` per request.
- There is no settings UI of any kind.

### Target design

**Storage.** A generic `settings` key/value table (extensible for future config
beyond LLM). LLM config lives under four keys:

| Key            | Meaning                              | Example                       |
|----------------|--------------------------------------|-------------------------------|
| `llm.provider` | `ollama` \| `openai` \| `anthropic` \| `custom` | `ollama`           |
| `llm.base_url` | API base URL                         | `http://localhost:11434/v1`   |
| `llm.api_key`  | API key (empty allowed for local)    | `sk-...`                      |
| `llm.model`    | Model name                           | `llama3`, `gpt-4o-mini`, `claude-sonnet-5` |

Security note: this is a single-user, self-hosted app; the API key is stored
plaintext in SQLite, same trust level as the rest of the DB file. The UI must
never render the saved key back into the page (masked placeholder only).

**Provider abstraction.** `internal/ai` becomes:

```
internal/ai/
├── ai.go             # Config, Provider interface, New() factory, CardData
├── prompt.go         # shared system/user prompt builder + parseCards()
├── openai_compat.go  # OpenAI-compatible chat completions client
└── anthropic.go      # Anthropic Messages API client
```

Only two HTTP implementations are needed, because the OpenAI chat-completions
format is the de-facto standard: Ollama (`http://localhost:11434/v1`),
LM Studio, vLLM, OpenRouter, Groq, Mistral, and OpenAI itself all serve it.
The provider selector's `ollama`, `openai`, and `custom` choices all map to the
OpenAI-compatible client — they differ only in default base URL and whether an
API key is expected. `anthropic` maps to the native Anthropic client.

Both clients are written with plain `net/http` + `encoding/json`. **No SDK
dependencies.** The `github.com/ollama/ollama` module is removed from `go.mod`.

**Flow.** `AIExecute` loads the config from the DB per request (no caching —
settings changes apply immediately). If no provider is configured, the AI form
shows an error linking to `/settings`.

**Routes.**

```
GET  /settings        → settings page (LLM section)
POST /settings        → save LLM settings → redirect /settings, flash "Settings saved."
POST /settings/test   → test saved config with a 1-token request → redirect /settings, flash result
```

---

### T-030: Settings table + model

**Files:** `internal/db/schema.sql`, `internal/models/settings.go` (new)

**Schema** — append to `schema.sql` (the whole file is executed with
`CREATE TABLE IF NOT EXISTS` on every startup, so existing DBs pick this up
automatically, no migration tooling needed):

```sql
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Model functions** (follow the style of `internal/models/deck.go` — package
`models`, take `db *sql.DB` as first arg):

```go
// GetSetting returns the value for key, or "" if the key does not exist.
func GetSetting(db *sql.DB, key string) (string, error)

// SetSetting upserts a key/value pair.
// SQL: INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
//      ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
func SetSetting(db *sql.DB, key, value string) error

// GetSettings returns all keys with the given prefix (e.g. "llm.") as a map.
// SQL: SELECT key, value FROM settings WHERE key LIKE ? || '%'
func GetSettings(db *sql.DB, prefix string) (map[string]string, error)
```

`GetSetting` must treat `sql.ErrNoRows` as `("", nil)`, not an error.

**Done when:** `go build ./...` passes; starting the server creates the
`settings` table in `memolang.db` (verify: `sqlite3 memolang.db '.schema settings'`).

---

### T-031: Provider interface + config loader (refactor `internal/ai`)

**Files:** `internal/ai/ai.go` (new), `internal/ai/prompt.go` (new),
`internal/ai/client.go` (delete after moving code)

**`ai.go`:**

```go
package ai

// CardData is unchanged from today.
type CardData struct {
    Front   string `json:"front"`
    Back    string `json:"back"`
    Example string `json:"example"`
}

// Config holds LLM connection settings loaded from the DB.
type Config struct {
    Provider string // "ollama" | "openai" | "anthropic" | "custom"
    BaseURL  string
    APIKey   string
    Model    string
}

// Provider generates flashcards from a language + topic prompt.
type Provider interface {
    // GenerateCards returns the generated cards. Implementations must apply
    // a 3-minute timeout via context.WithTimeout.
    GenerateCards(ctx context.Context, language, promptText string) ([]CardData, error)

    // Ping performs a minimal request to verify connectivity and auth
    // (5-second timeout). Used by the settings "Test connection" button.
    Ping(ctx context.Context) error
}

// New builds a Provider from config.
// - Provider "anthropic"                     → newAnthropic(cfg)
// - Provider "ollama", "openai", "custom"    → newOpenAICompat(cfg)
// - Provider ""                              → ErrNotConfigured
// - Any other value                          → fmt.Errorf("unknown provider %q", ...)
// Validation: BaseURL and Model must be non-empty; APIKey must be non-empty
// for "openai" and "anthropic" (optional for "ollama"/"custom").
func New(cfg Config) (Provider, error)

// ErrNotConfigured is returned by New when no provider is set.
var ErrNotConfigured = errors.New("llm provider not configured")

// LoadConfig reads the llm.* keys via models.GetSettings(db, "llm.").
func LoadConfig(db *sql.DB) (Config, error)
```

**`prompt.go`:** move the existing system-prompt string and `parseCards()` from
`client.go` verbatim (they are provider-agnostic):

```go
// systemPrompt returns the flashcard-generation system prompt for language.
func systemPrompt(language string) string

// userPrompt returns "Target language: %s\nTopic: %s".
func userPrompt(language, promptText string) string

// parseCards — unchanged from client.go (strips ``` fences, tries bare array,
// falls back to {"cards": [...]} wrapper).
func parseCards(raw string) ([]CardData, error)
```

This task may leave the package temporarily without a concrete provider
(T-032/T-033 add them); to keep the build green, `New` can return
`ErrNotConfigured`/"unknown provider" for everything until those land, and
`handlers/deck.go` is updated in T-035. If splitting breaks the build, do
T-031 and T-032 in one PR.

**Done when:** `go build ./...` passes; `client.go` and the
`github.com/ollama/ollama` import are gone from `internal/ai` (dep removal from
go.mod happens in T-032 when the last usage disappears).

---

### T-032: OpenAI-compatible provider

**File:** `internal/ai/openai_compat.go` (new), plus `go.mod` cleanup

**What:** implement `Provider` against the OpenAI chat-completions API using
only `net/http` and `encoding/json`.

```go
type openAICompat struct {
    baseURL string // normalized: trailing "/" stripped
    apiKey  string // may be empty (local servers)
    model   string
    http    *http.Client
}

func newOpenAICompat(cfg Config) *openAICompat
```

**Request** — `POST {baseURL}/chat/completions`:

```json
{
  "model": "<model>",
  "messages": [
    {"role": "system", "content": "<systemPrompt(language)>"},
    {"role": "user",   "content": "<userPrompt(language, promptText)>"}
  ],
  "stream": false
}
```

Headers: `Content-Type: application/json`; `Authorization: Bearer <apiKey>`
only when `apiKey != ""`.

**Response** — parse only what we need:

```go
var resp struct {
    Choices []struct {
        Message struct {
            Content string `json:"content"`
        } `json:"message"`
    } `json:"choices"`
    Error *struct {
        Message string `json:"message"`
    } `json:"error"`
}
```

Then `return parseCards(resp.Choices[0].Message.Content)`.

**Error handling requirements:**
- Non-2xx status: return error including status code and the first 200 bytes of
  the body (API error messages are the main debugging tool).
- `resp.Error != nil`: return its message.
- Empty `Choices`: return a clear error, not an index panic.

**`Ping`:** same endpoint, messages `[{"role":"user","content":"ping"}]`,
`"max_tokens": 1`, 5-second timeout. Success = any 2xx response.

**Also:** remove `github.com/ollama/ollama` from `go.mod` (`task tidy`).

**Unit tests** (`internal/ai/openai_compat_test.go`) using `net/http/httptest`:
1. Happy path: fake server returns a valid choices payload with a JSON card
   array → cards parsed.
2. Auth header present when apiKey set, absent when empty.
3. 401 with error body → error contains status and body text.

**Done when:** `go build ./...` and `go test ./internal/ai/...` pass;
`grep ollama go.mod` is empty.

---

### T-033: Anthropic provider

**File:** `internal/ai/anthropic.go` (new)

**What:** implement `Provider` against the Anthropic Messages API, plain
`net/http` (no SDK).

**Request** — `POST {baseURL}/v1/messages` (default baseURL
`https://api.anthropic.com`):

```json
{
  "model": "<model>",
  "max_tokens": 8192,
  "system": "<systemPrompt(language)>",
  "messages": [
    {"role": "user", "content": "<userPrompt(language, promptText)>"}
  ]
}
```

Headers: `Content-Type: application/json`, `x-api-key: <apiKey>`,
`anthropic-version: 2023-06-01`. Note the differences from OpenAI: system
prompt is a top-level `system` field (not a message), `max_tokens` is
mandatory, auth header is `x-api-key` (not `Authorization`).

**Response:**

```go
var resp struct {
    Content []struct {
        Type string `json:"type"`
        Text string `json:"text"`
    } `json:"content"`
    Error *struct {
        Message string `json:"message"`
    } `json:"error"`
}
```

Concatenate `Text` of all blocks with `Type == "text"`, then `parseCards`.
Same error-handling requirements as T-032 (status + body excerpt, nil-safe).

**`Ping`:** same endpoint, `"max_tokens": 1`, user message `"ping"`, 5-second
timeout.

**Unit tests** mirroring T-032 (httptest): happy path, headers
(`x-api-key`, `anthropic-version`), API error surfaced.

**Done when:** `go test ./internal/ai/...` passes.

---

### T-034: Settings page (handler + template + nav)

**Files:** `internal/handlers/settings.go` (new), `templates/settings.html`
(new), `internal/handlers/data.go` (add `SettingsData`),
`internal/app/app.go` (routes), `templates/layout.html` (nav link)

**Data struct** (in `data.go`):

```go
// SettingsData is the data payload for the settings page.
type SettingsData struct {
    Provider   string
    BaseURL    string
    Model      string
    HasAPIKey  bool   // true if a key is saved; never expose the key itself
    Error      string
}
```

**`GET /settings` → `SettingsPage`:** load via `ai.LoadConfig(h.DB)`, render
`settings.html` with `PageData{Title: "Settings", Flash: h.getFlash(c), Data: SettingsData{...}}`.

**`POST /settings` → `SaveSettings`:** read form fields `provider`, `base_url`,
`api_key`, `model`.
- Validate: `provider` must be one of `ollama|openai|anthropic|custom`;
  `base_url` and `model` required. On failure re-render the form with
  `SettingsData.Error` set (status 200, same pattern as `CreateDeck`).
- Persist `llm.provider`, `llm.base_url`, `llm.model` via `models.SetSetting`.
- **API key semantics:** if the `api_key` field is non-empty, save it; if
  empty, keep the existing saved key (so re-saving the form doesn't wipe it).
  A separate checkbox `clear_api_key` (value `"1"`) explicitly clears the key.
- Redirect via `h.redirectWithFlash(c, "/settings", "Settings saved.")`.

**Template** (`settings.html`, wrapped in `_header`/`_footer` like other pages):
- `<h1>Settings</h1>`, section heading "LLM Provider".
- One `<form method="POST" action="/settings">` with:
  - `<select name="provider">` — options: `ollama` ("Ollama (local)"),
    `openai` ("OpenAI"), `anthropic` ("Anthropic"), `custom`
    ("Custom (OpenAI-compatible)"); current value selected.
  - `<input type="text" name="base_url">` with the saved value. Below it a
    muted hint line: "Ollama: `http://localhost:11434/v1` · OpenAI:
    `https://api.openai.com/v1` · Anthropic: `https://api.anthropic.com`".
  - `<input type="password" name="api_key" placeholder="{{if .Data.HasAPIKey}}•••••••• (saved — leave blank to keep){{else}}API key (optional for local){{end}}">` —
    the `value` attribute is always empty.
  - `{{if .Data.HasAPIKey}}` a checkbox `clear_api_key` labeled "Clear saved key".
  - `<input type="text" name="model">` e.g. `llama3`, `gpt-4o-mini`,
    `claude-sonnet-5`.
  - Buttons: `[Save]` (submit, `btn btn-primary`).
- A second, separate one-button form: `<form method="POST" action="/settings/test">`
  with `[Test Connection]` (`btn`) — wired in T-036 (render the button now;
  it can 404 until T-036 lands).
- No JavaScript. Provider→URL prefill is handled by the hint text, not JS.

**Nav:** add `<a href="/settings">Settings</a>` to the header nav in
`layout.html`, after the existing links.

**Routes** in `app.go`:

```go
r.GET("/settings", h.SettingsPage)
r.POST("/settings", h.SaveSettings)
```

**Done when:** `/settings` renders with empty defaults on a fresh DB; saving
values persists them (visible after restart); saving with blank key keeps the
old key; validation errors re-render inline.

---

### T-035: Wire AI generation to configured provider

**File:** `internal/handlers/deck.go` (modify `AIExecute`),
`templates/ai_form.html` (error rendering — check it displays `.Data.Error`;
adjust only if needed)

**What:** in `AIExecute`, replace `ai.NewClient()` with:

```go
cfg, err := ai.LoadConfig(h.DB)
if err == nil {
    provider, err = ai.New(cfg)
}
```

- If `errors.Is(err, ai.ErrNotConfigured)`: set
  `fd.Error = "No LLM provider configured. Set one up in Settings first."` —
  and make `ai_form.html` render the error text with an inline link to
  `/settings` when this message appears (simplest: always render
  `<a href="/settings">Settings</a>` link under the error box).
- Other `ai.New` / `GenerateCards` errors: keep the existing
  store-pending-and-redirect retry flow, but include `err.Error()` in the
  message (current code already does this for connection errors).
- Pass `c.Request.Context()` as the context to `GenerateCards`.

**Done when:** with no settings saved, submitting the AI form shows the
"configure in Settings" error; with a valid Ollama config saved in the UI
(no env vars set), deck generation works end-to-end.

---

### T-036: Test-connection endpoint

**Files:** `internal/handlers/settings.go`, `internal/app/app.go`

**`POST /settings/test` → `TestLLMConnection`:**

```go
cfg, err := ai.LoadConfig(h.DB)      // then ai.New(cfg)
// on any error: h.redirectWithFlash(c, "/settings", "Connection failed: " + err.Error())
// else provider.Ping(c.Request.Context())
// on ping error: flash "Connection failed: " + err.Error()
// on success:    flash "Connection OK — " + cfg.Provider + " / " + cfg.Model
```

Route: `r.POST("/settings/test", h.TestLLMConnection)`.

**Done when:** with Ollama running and configured, the button flashes
"Connection OK"; with a bogus base URL it flashes a failure message containing
the underlying error; the page never hangs (5-second Ping timeout).

---

### T-037: E2E tests for settings

**File:** `test/e2e/cases/settings_test.go` (new; follow the structure of
`test/e2e/cases/deck_test.go` and use `test/e2e/helpers`)

**Scenarios:**
1. Nav link "Settings" is visible on the dashboard and leads to `/settings`.
2. Save settings: pick provider `custom`, fill base URL/model/api key, submit →
   flash "Settings saved."; reload → values persisted, api_key input empty
   with "(saved)" placeholder.
3. Re-save with blank api_key → key retained (placeholder still says saved).
4. AI form without configuration (fresh DB) → error mentions Settings and
   links to `/settings`.

Do not test against a real LLM. For an optional integration test of the
generate flow, point base URL at a stub HTTP server started inside the test
that returns a canned chat-completions response.

**Done when:** `task test:e2e` passes.

---

## Feature 2 (M8): Answer Feedback + Session Stats

### Current state (what's wrong)

- Multiple choice: clicking an option POSTs and immediately shows the next
  card. The user never learns whether they were right, or what the correct
  answer was.
- The session header shows only position ("Card 3 of 20") — no running score.
- The summary shows aggregate counts only (`correct`/`total` on the session
  row). There is no record of *which* cards were missed, so no review list.

### Target design

**Per-answer recording.** New table `session_answers` — one row per answered
card. This is the source of truth for the feedback screen and the summary
breakdown; the existing `correct`/`total` counters on `study_sessions` stay
(cheap for the header score) and must be kept consistent by writing both in
the same handler.

```sql
CREATE TABLE IF NOT EXISTS session_answers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  INTEGER NOT NULL REFERENCES study_sessions(id) ON DELETE CASCADE,
    card_id     INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    correct     INTEGER NOT NULL,            -- 0 | 1
    given       TEXT    NOT NULL DEFAULT '', -- MC: chosen back text; flashcard: rating label
    answered_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session_answers_session ON session_answers(session_id);
```

**Feedback flow (MC only, server-rendered, zero JS).** Flashcard mode is
self-graded — the user has already seen the back before rating — so it gets no
feedback screen, only the running score.

```
POST /decks/{id}/session/answer   (MC)
  1. Record answer row (correct = choice == card.Back).
  2. Apply SM-2, advance session position/counters (existing logic).
  3. Redirect → GET /decks/{id}/session?feedback={answerID}      ← changed

GET /decks/{id}/session?feedback={answerID}
  1. Load active session (existing).
  2. NEW — before the position/summary checks: if ?feedback is present, load
     the answer row, verify answer.session_id == session.ID (else ignore the
     param and fall through), load its card, render the FEEDBACK view.
  3. Feedback view: banner "✓ Correct!" (green) or "✗ Wrong" (red); the card
     front; "Your answer: <given>" struck through in red when wrong;
     "Correct answer: <card.Back>" in green; [Next →] button = plain link to
     /decks/{id}/session (no query param).
  4. [Next] hits the normal flow: next card, or — if that was the last card —
     the existing position>=len branch ends the session and redirects to the
     summary. Ordering matters: the feedback check MUST run before the
     end-of-queue check, otherwise the last card's feedback is skipped.
```

The feedback view does not try to re-show all four shuffled options (they were
generated randomly at render time and aren't stored); it shows chosen vs.
correct, which is the information that matters.

**Running score.** Session header gains "✓ N ✗ M" (N = `session.Correct`,
M = `session.Total - session.Correct`), both modes.

**Summary breakdown.** Below the stat tiles: "Missed cards" section listing
each wrong answer (front — correct back — your answer for MC), from
`session_answers`. Add a "Wrong" stat tile. If nothing was missed, show
"Perfect session! 🎉" instead of the list.

---

### T-040: `session_answers` table + model

**Files:** `internal/db/schema.sql` (append DDL above),
`internal/models/answer.go` (new)

```go
package models

// SessionAnswer is one recorded answer within a study session.
type SessionAnswer struct {
    ID         int64
    SessionID  int64
    CardID     int64
    Correct    bool
    Given      string
    AnsweredAt time.Time
    // joined from cards (populated by GetSessionAnswers only):
    CardFront string
    CardBack  string
}

// RecordAnswer inserts a row and returns its ID (use RETURNING id).
func RecordAnswer(db *sql.DB, sessionID, cardID int64, correct bool, given string) (int64, error)

// GetAnswerByID returns the answer row (no card join). sql.ErrNoRows → (nil, nil).
func GetAnswerByID(db *sql.DB, id int64) (*SessionAnswer, error)

// GetSessionAnswers returns all answers for a session ordered by id,
// with CardFront/CardBack populated via JOIN cards ON cards.id = card_id.
// If onlyWrong is true, filters WHERE correct = 0.
func GetSessionAnswers(db *sql.DB, sessionID int64, onlyWrong bool) ([]SessionAnswer, error)
```

Store `Correct` as `0/1` INTEGER; scan into bool via intermediate int.

**Done when:** `go build ./...` passes; table + index exist after server start.

---

### T-041: Record answers + feedback redirect

**File:** `internal/handlers/session.go` (modify `SubmitAnswer`)

**What:**
1. In the MC branch (`c.PostForm("choice") != ""`): after computing
   `isCorrect`, call
   `answerID, _ := models.RecordAnswer(h.DB, sessionID, cardID, isCorrect, choice)`.
2. In the flashcard branch: record too (for summary consistency), with `given`
   = the rating label: `[]string{"Again", "Hard", "Good", "Easy"}[rating]`
   (guard rating to 0..3 range first; current code doesn't — clamp it).
3. Keep SM-2 update + `AdvanceSession` exactly as-is.
4. Redirect:
   - MC: `c.Redirect(http.StatusSeeOther, fmt.Sprintf("/decks/%s/session?feedback=%d", c.Param("id"), answerID))`
   - Flashcard: unchanged (`/decks/{id}/session`).

**Done when:** answering an MC card lands on
`/decks/{id}/session?feedback=N`; a `session_answers` row exists per answer
for both modes (`sqlite3 memolang.db 'SELECT * FROM session_answers'`).

---

### T-042: Feedback view (handler branch + template)

**Files:** `internal/handlers/session.go` (modify `StartSession`),
`internal/handlers/data.go` (extend `SessionData`), `templates/session.html`,
`static/style.css`

**`SessionData` additions:**

```go
type SessionData struct {
    // ... existing fields ...
    Feedback     bool          // render the feedback view
    Answer       models.SessionAnswer
    AnswerCard   models.Card   // the card that was just answered
}
```

**Handler:** in `StartSession`, after loading the active session and **before**
the `session.Position >= len(session.CardQueue)` check:

```go
if fbID, err := strconv.ParseInt(c.Query("feedback"), 10, 64); err == nil {
    ans, err := models.GetAnswerByID(h.DB, fbID)
    if err == nil && ans != nil && ans.SessionID == session.ID {
        card, err := models.GetCardByID(h.DB, ans.CardID)
        if err == nil {
            h.render(c, http.StatusOK, "session.html", PageData{
                Title: deck.Name + " — Study",
                Flash: h.getFlash(c),
                Data: SessionData{Deck: deck, Session: *session,
                    Feedback: true, Answer: *ans, AnswerCard: card,
                    Progress: /* same formula as existing */},
            })
            return
        }
    }
    // invalid/foreign feedback ID: fall through to the normal flow
}
```

Note `GetActiveSession` returns nil for ended sessions; if `session == nil`
the feedback param is irrelevant — keep the existing nil-session path
untouched (it creates a new session), but only enter it when `feedback` is
absent; with a stale feedback param and no active session, redirect to
`/decks/{id}/session` (drop the param) to avoid auto-starting a new session
from a stale tab.

**Template:** in `session.html`, add a new top-level branch:

```
{{if .Data.Feedback}}
  <feedback view>
{{else if .Data.Empty}} ... existing ... {{else}} ... existing ... {{end}}
```

Feedback view markup (reuse `.session-card`):

```html
<div class="session-card feedback {{if .Data.Answer.Correct}}feedback-correct{{else}}feedback-wrong{{end}}">
    <div class="feedback-banner">{{if .Data.Answer.Correct}}✓ Correct!{{else}}✗ Wrong{{end}}</div>
    <div class="front-text">{{.Data.AnswerCard.Front}}</div>
    {{if not .Data.Answer.Correct}}
    <div class="feedback-given">Your answer: <s>{{.Data.Answer.Given}}</s></div>
    {{end}}
    <div class="feedback-answer">Correct answer: <strong>{{.Data.AnswerCard.Back}}</strong></div>
    {{if .Data.AnswerCard.Example}}<div class="example-text">{{.Data.AnswerCard.Example}}</div>{{end}}
    <a href="/decks/{{.Data.Deck.ID}}/session" class="btn btn-primary">Next →</a>
</div>
```

Keep the progress bar + header row rendered above it (same as the question
view) so the layout doesn't jump.

**CSS** (`style.css`): `.feedback-banner` — bold, 1.25rem;
`.feedback-correct .feedback-banner { color: #2e9e44; }`
`.feedback-wrong .feedback-banner { color: #d33; }`
`.feedback-given { color: #d33; }` `.feedback-answer { color: #2e9e44; }`
Match existing variable usage where colors already exist.

**Done when:** MC flow is question → feedback (colored, shows correct answer)
→ Next → next question; answering the last card shows its feedback, and Next
leads to the summary; a tampered `?feedback=` from another session shows the
normal question view.

---

### T-043: Running score in session header

**Files:** `templates/session.html`, `static/style.css` (if needed)

**What:** in the header row (the flex div with "Card X of Y" and End Session),
add between them:

```html
<span class="session-score">
    <span class="score-right">✓ {{$session.Correct}}</span>
    <span class="score-wrong">✗ {{sub $session.Total $session.Correct}}</span>
</span>
```

There is no `sub` template func yet — add it next to `add` in
`internal/app/app.go`: `"sub": func(a, b int) int { return a - b }`.
Style: `.score-right { color: #2e9e44; } .score-wrong { color: #d33; }`.
Render in both question and feedback views (both use `$session`).

**Done when:** score updates after each answer in both quiz modes; the
feedback screen already reflects the just-given answer (counters were
incremented before the redirect).

---

### T-044: Summary breakdown (missed cards)

**Files:** `internal/handlers/session.go` (modify `SessionSummary`),
`internal/handlers/data.go` (extend `SessionSummaryData`),
`templates/session_summary.html`

**Data:**

```go
type SessionSummaryData struct {
    // ... existing fields ...
    Wrong       int                     // Session.Total - Session.Correct
    MissedCards []models.SessionAnswer  // GetSessionAnswers(db, session.ID, true)
}
```

**Handler:** after loading the last ended session, compute `Wrong` and load
`MissedCards`. Ignore the error by passing an empty slice (summary must render
even for pre-migration sessions that have no answer rows).

**Template:** add a "Wrong" stat tile (value `.Data.Wrong`) after "Correct".
Below the stats:

```html
{{if .Data.MissedCards}}
<h2>Missed cards</h2>
<table class="card-table">
    <tr><th>Front</th><th>Correct answer</th><th>Your answer</th></tr>
    {{range .Data.MissedCards}}
    <tr><td>{{.CardFront}}</td><td>{{.CardBack}}</td><td>{{.Given}}</td></tr>
    {{end}}
</table>
{{else if gt .Data.Session.Total 0}}
<p class="perfect-session">Perfect session! 🎉</p>
{{end}}
```

Reuse the existing table styles from `deck.html` if a shared class exists;
otherwise minimal new CSS.

**Done when:** finishing an MC session with ≥1 wrong answer lists exactly
those cards with the wrong choice shown; an all-correct session shows the
perfect-session message; old sessions (no answer rows) still render.

---

### T-045: E2E tests for feedback + stats

**File:** `test/e2e/cases/session_test.go` (modify — existing MC tests assume
answer → immediately next card; they will break and must be updated)

**Scenarios:**
1. MC correct answer → feedback view shows "✓ Correct!"; click Next → next card.
2. MC wrong answer → feedback shows "✗ Wrong", the chosen text struck through,
   and the correct back text; click Next → next card.
   (Determinism: create a 2-card deck; the correct answer is always the
   current card's back, so click the option matching / not matching it.)
3. Running score: after one right and one wrong answer, header shows "✓ 1" and
   "✗ 1".
4. Last-card feedback: answering the final card shows feedback first; Next
   lands on the summary.
5. Summary: wrong card listed under "Missed cards" with the given answer;
   all-correct run shows the perfect-session message.
6. Flashcard mode regression: rating buttons still advance directly (no
   feedback screen), score visible.

**Done when:** `task test:e2e` passes including the updated old tests.

---

## Build order & parallelism

```
M7: T-030 → T-031 → T-032 → T-033 (T-033 parallel with T-034)
         └→ T-034 → T-035 → T-036 → T-037

M8: T-040 → T-041 → T-042 → T-043 (T-043 parallel with T-044)
                          └→ T-044 → T-045
```

M7 and M8 are fully independent tracks — they touch disjoint files except
`schema.sql`, `data.go`, and `app.go` (trivial merges) — and can be worked in
parallel by two agents.

## Status (re-reviewed 2026-07-03; fix round completed same day)

`go build ./...` ✅ · `go test ./internal/...` ✅ · `task test:e2e` ✅ (24/24, green 5 consecutive runs, ~6.5s)

**All blockers fixed 2026-07-03 (by reviewer): F-103, F-104, F-105, F-106, F-107, N-4. Both feature tracks are done.** Open: minors N-1..N-3 only.

| Task | Status | Notes |
|------|--------|-------|
| T-030 Settings table + model | ✅ Complete | Verified against spec. `DeleteSetting` added beyond spec — fine, used by clear-key flow. |
| T-031 Provider interface + config loader | ✅ Complete | F-101 fixed; `AIExecute` wired to `LoadConfig`/`New`, context passed. Placeholder provider stub removed. |
| T-032 OpenAI-compatible provider | ✅ Complete | F-102 fixed: 3 httptest scenarios + Ping tests, all pass. |
| T-033 Anthropic provider | ✅ Complete | `/v1/messages`, `x-api-key`, `anthropic-version`, top-level `system`, `max_tokens`, text-block concat — all per spec. Tests pass. Minor: see N-1. |
| T-034 Settings page | ✅ Complete | Handler + template + nav link + routes match spec incl. keep-key-if-blank and clear checkbox. Minor: see N-2. |
| T-035 Wire AI generation | ✅ Complete | `ErrNotConfigured` → "configure in Settings" message; `ai_form.html` links to `/settings`. |
| T-036 Test connection | ✅ Complete | Save-then-test semantics, 5s ping timeout, flash on both outcomes. |
| T-037 E2E settings tests | ✅ Complete | F-104/F-106/F-107 fixed: helper submits (select by value), tests reset `llm.*` state, all pass. |
| T-040 `session_answers` + model | ✅ Complete | Table + index in schema; model matches spec (bool↔int, join, onlyWrong). |
| T-041 Record answers + redirect | ✅ Complete | Both modes recorded; rating clamped 0..3; MC redirects to `?feedback=<id>`. |
| T-042 Feedback view | ✅ Complete | F-103 fixed (feedback check moved above end-of-queue check); covered by `LastCardFeedback` e2e regression test with a 1-card deck. |
| T-043 Running score | ✅ Complete | `sub` func added; ✓/✗ rendered in question + feedback views, both modes. |
| T-044 Summary breakdown | ✅ Complete | Wrong tile, missed-cards table, perfect-session message. Minor: see N-3. |
| T-045 E2E feedback tests | ✅ Complete | F-105 fixed: deterministic option picks via CSV front→back map (`actions.AnswerMC`/`LoadCSVAnswers`), 1-card fixture for last-card case, missed-cards table asserted with the actual wrong answer. |

### Fix tasks — round 2

#### F-103: Last-card feedback never shown 🔴 functional bug

**File:** `internal/handlers/session.go`, function `StartSession`

**Problem:** block order is

```go
if session.Position >= len(session.CardQueue) { ... redirect summary ... }   // line ~74
if fbID, err := strconv.ParseInt(c.Query("feedback"), ...); err == nil { ... } // line ~82
```

After answering the final card, `position == len(queue)`, so the first branch
ends the session and redirects to the summary — the `?feedback=` param is
discarded and the last card's right/wrong screen is skipped. T-042's "Done
when" explicitly requires: *"answering the last card shows its feedback, and
Next leads to the summary."*

**Fix:** swap the two blocks — move the entire `if fbID...` feedback block to
**before** the `session.Position >= len(session.CardQueue)` check. No other
change needed: the feedback view's [Next] link goes to `/session` without the
param, which then correctly hits the end-of-queue branch and redirects to the
summary.

**Done when:** in a 1-card MC deck, answering the card shows the feedback
screen; clicking Next lands on the summary. (This is F-105's
`TestLastCardFeedback` scenario — fix that test in the same PR.)

#### F-104: E2E suite cannot compile or run — cases never wired to TestMain 🔴

**Files:** `test/e2e/main_test.go`, all four files in `test/e2e/cases/`,
`test/e2e/configuration/environment.go`

**Problems:**
1. Compile error: `test/e2e/cases/settings_test.go:7: "memolang/test/e2e/configuration" imported and not used`.
2. Structural: `TestMain` (boots the app server, opens the temp DB, launches
   the Playwright browser, sets `configuration.BaseURL`/`configuration.Browser`)
   lives in package `e2e_test` under `test/e2e/`, which contains **zero
   tests**. Go compiles one test binary per package and `m.Run()` only
   discovers tests inside its own binary, so the tests in `test/e2e/cases`
   (package `cases_test`, a separate binary) start with a nil browser and
   empty base URL. The suite has never actually run.

**Fix — suite-runner pattern (decided): `main_test.go` stays the single
entry point and drives the cases via `t.Run`.**

1. **Make `cases` an importable library, not a test package.** `_test.go`
   files only compile into their own package's test binary and a `_test`
   package can never be imported, so:
   - rename `test/e2e/cases/{deck,import,session,settings}_test.go` →
     `{deck,import,session,settings}.go`;
   - change `package cases_test` → `package cases`;
   - make every case an **exported** func with the same body:
     `func TestFlashcardReveal(t *testing.T)` → `func FlashcardReveal(t *testing.T)`
     (drop the `Test` prefix — they are no longer auto-discovered);
   - remove the unused `configuration` import in `settings.go` (F-106 re-adds
     a used one);
   - keep importing `testing` — that is legal in a normal package.
2. **Register every case in `test/e2e/main_test.go`** as subtests, grouped
   per area, preserving current in-file order:

   ```go
   func TestE2E(t *testing.T) {
       t.Run("Deck", func(t *testing.T) {
           t.Run("CreateDeck", cases.CreateDeck)
           // ... every func from deck.go
       })
       t.Run("Import", func(t *testing.T) { /* every func from import.go */ })
       t.Run("Session", func(t *testing.T) { /* every func from session.go */ })
       t.Run("Settings", func(t *testing.T) { /* every func from settings.go */ })
   }
   ```

   `TestMain` stays in `main_test.go` unchanged — now it and the cases live
   in one binary, so the bootstrap actually precedes the cases.
   Filtering works as `go test ./test/e2e -run 'TestE2E/Settings'`.
3. **Registration is now manual — make omissions impossible to miss.** Every
   exported func in `cases` must appear in `TestE2E`; add a comment at the
   top of each `cases/*.go` file: "New case funcs MUST be registered in
   test/e2e/main_test.go, or they will silently never run."
4. Export the opened `*sql.DB` for tests needing direct DB resets: add
   `var DB *sql.DB` to `test/e2e/configuration/environment.go`, set it in
   `TestMain` (needed by F-106).
5. Update Taskfile `test:e2e` cmd to `go test ./test/e2e -v -count=1 -timeout 120s`
   (the `/...` wildcard is no longer needed; `cases` has no tests of its own).

**Done when:** `go vet ./test/e2e/...` passes; `task test:e2e` boots the
server/browser once and executes all registered cases under `TestE2E/*`
(failures from F-105/F-106 scenarios are handled by those tasks — this task
is about the harness running at all); the number of registered subtests
equals the number of exported funcs in `cases`.

**STATUS: ✅ FIXED (2026-07-03, by reviewer).** Cases renamed to
`cases/{deck,import,session,settings}.go` (package `cases`, exported funcs),
`TestE2E` runner added to `main_test.go`, `configuration.DB` exported,
Taskfile updated. First-ever real run: **24 executed, 14 pass, 10 fail.**
The failures are the F-105/F-106 predictions plus latent helper bugs the
suite never caught while it wasn't running — see F-107.

#### F-107: First real suite run exposed latent test/helper bugs 🟡

Failing cases from the 2026-07-03 run, beyond F-105/F-106 scope:

| Case | Symptom | Likely cause to investigate |
|------|---------|------------------------------|
| `Deck/DeleteDeck` | 30s timeout | `components.ClickDeleteButton` / confirm-dialog handling (Playwright needs a dialog handler for `confirm()`) |
| `Deck/StudyDropdownOffersBothModes` | fast fail | selector drift (`.study-menu`/`.study-options`) or `DeleteDeck(t, page, "")` misuse |
| `Import/CSVImportEmptyFileError` | fast fail | test navigates to relative `"/import"` instead of `deckURL + "/import"` |
| `Import/CSVImportTokenBelongsToDeck` | fast fail | needs investigation (form injection via Evaluate) |
| `Session/LastCardFeedback`, `Session/SummaryMissedCards` | 30s timeout in `WaitForSummary` | helper waits for summary without answering the cards — rewrite per F-105 anyway |
| `Settings/SaveSettings`, `Settings/ReSaveKeepsAPIKey` | 30s timeout at flash read; **no POST /settings in server log** | `actions.SaveSettings` never actually submits the form — fix the helper first, then F-106 ordering effects will surface |

Note: `Settings/AIFormWithoutConfig` and `Settings/SettingsValidation`
currently PASS only because `actions.SaveSettings` is broken (no config ever
saved). Once the helper is fixed, the F-106 order-dependence will kick in —
do F-106 together with this task.

**Done when:** every failure above either passes or is covered by an F-105/
F-106 rewrite; `task test:e2e` fully green 5 consecutive runs.

**STATUS: ✅ ALL FIX TASKS CLOSED (2026-07-03, by reviewer).**
F-103: feedback block moved above end-of-queue check in `session.go`.
F-105: `actions.LoadCSVAnswers` + `actions.AnswerMC` (deterministic option
picks), `testdata/single_card.csv` fixture, `LastCardFeedback` +
`SummaryMissedCards` + `RunningScore` rewritten (score asserts match rendered
"✓ N"/"✗ N" text).
F-106: `actions.ResetLLMSettings` (via `configuration.DB`) at the top of
every settings case.
F-107 root causes: `actions.SaveSettings` selected the provider by *label*
while passing the *value* → select hung 30s and the form never submitted
(fixed: select by value); `actions.DeleteDeck` made defer-safe (no-ops on
empty URL or already-deleted deck); import cases used `NavigateTo` (which
prepends BaseURL) with absolute/wrong paths (fixed: `Goto` with `deckURL`).
Result: **24/24 cases green, 5 consecutive runs, ~6.5s per run.**
Remaining: only minor notes N-1..N-3 (N-4 done via gofmt).

#### F-105: MC feedback e2e tests are nondeterministic and self-contradictory 🔴

**File:** `test/e2e/cases/session_test.go`

**Problems:**
- `TestMultipleChoiceFeedbackCorrect`, `TestMultipleChoiceFeedbackWrong`, and
  `TestRunningScore` all click `.mc-option` **First()** — but options are
  shuffled server-side. The same click is expected to yield "Correct" in one
  test and "Wrong" in another; at least one must fail on any given run.
- `TestLastCardFeedback` never asserts a feedback screen — it just waits for
  the summary, so it passes even with the F-103 bug present.
- `TestSummaryMissedCards` only checks stat labels, not the missed-cards table.

**Fix (deterministic strategy from T-045 spec):** the CSV at
`configuration.DefaultCSVPath` is a known fixture — build a `front → back`
map from it in a helper (`helpers/actions` or a new
`configuration.CSVAnswers()`). Then:
1. *Correct case:* read the on-screen front (`.front-text`), click the option
   whose exact text equals `map[front]` → assert "✓ Correct!" banner,
   `feedback-correct` class, score ✓=1.
2. *Wrong case:* click any option whose text ≠ `map[front]` (if all rendered
   options happen to equal the answer — impossible with 3 distinct
   distractors — fail loudly) → assert "✗ Wrong", struck-through given text
   equal to the clicked option, "Correct answer:" text contains `map[front]`.
3. *Last card:* import a **single-row CSV** (add a 1-card fixture, e.g.
   `testdata/single_card.csv`) so the first answer is the last; answer it →
   assert feedback banner is visible on `**/session?feedback=*` → click Next →
   assert `**/summary`.
4. *Missed cards:* deliberately answer wrong (strategy 2), finish session,
   assert the summary table contains that card's front and the wrongly chosen
   text under "Your answer".

**Done when:** `task test:e2e` passes 5 consecutive runs (shuffle can't flake
it).

#### F-106: Settings e2e tests depend on execution order / dirty state 🟡

**File:** `test/e2e/cases/settings_test.go`

**Problems (both stem from the shared server DB persisting across tests):**
- `TestAIFormWithoutConfig` requires *no* LLM config, but `TestSaveSettings`
  runs earlier in the same file and saves one — the AI form then proceeds to
  call the (unreachable) provider and shows "AI generation failed", not the
  "configure in Settings" error. Deterministic failure.
- `TestSettingsValidation` expects a "required" error after clicking Save,
  but the form is pre-filled with previously saved base URL/model, so the
  save succeeds.

**Fix:** add a state reset helper using `configuration.DB` (exported in
F-104): `func ResetLLMSettings(t *testing.T)` executing
`DELETE FROM settings WHERE key LIKE 'llm.%'`. Call it at the top of
`TestAIFormWithoutConfig` and `TestSettingsValidation` (and generally at the
top of every settings test so each is order-independent). For
`TestSettingsValidation`, after the reset also reload `/settings` before
clicking Save so the empty form is actually rendered.

**Done when:** `go test ./test/e2e/cases/ -run 'TestSettings|TestAIForm' -count=1`
passes, and still passes with `-shuffle=on`.

#### Minor notes (fix opportunistically, no dedicated task)

- **N-1** `internal/ai/anthropic.go`: `anthropicRequest.System` lacks
  `omitempty` — `Ping` sends `"system":""`. Anthropic tolerates it today; add
  `json:"system,omitempty"` for hygiene.
- **N-2** `internal/handlers/settings.go`: on validation-error re-renders,
  `HasAPIKey: apiKey != ""` reflects the *submitted* (unsaved) key, so the
  placeholder can claim a key is saved when it isn't. Load actual state via
  `models.GetSetting(db, "llm.api_key")` for those re-renders.
- **N-3** `internal/handlers/session.go` `SessionSummary`: sessions from
  before this migration have `wrong > 0` but no `session_answers` rows →
  template's `{{else if}}` shows "Perfect session! 🎉" despite wrong answers.
  Cosmetic, legacy-only; guard with `{{if gt .Data.Wrong 0}}` around the
  else-branch if it bothers anyone.
- **N-4** `internal/handlers/data.go`: `SessionSummaryData` declaration is
  indented with stray tabs (compiles, but run `gofmt -w` on the package).

Unplanned change observed: `main.go` now reads `DB_PATH` env var (Docker support). Harmless, keep.

### Review findings → fix tasks

Pick these up **before** continuing with T-033+. Each is small and independent
of the other, but F-101 unblocks everything (the app currently does not
compile).

#### F-101: Fix broken build — wire `AIExecute` to the new provider API 🔴 blocker

**File:** `internal/handlers/deck.go` (function `AIExecute`, around line 108)

**Problem:** `ai.NewClient()` was deleted in T-031 but `AIExecute` still calls
it:

```
internal/handlers/deck.go:108:22: undefined: ai.NewClient
```

**Fix:** this is T-035 pulled forward. Replace the `ai.NewClient()` block with:

```go
cfg, err := ai.LoadConfig(h.DB)
if err != nil {
    log.Printf("AIExecute: LoadConfig failed: %v", err)
    fd.Error = "Failed to load LLM settings: " + err.Error()
    tok := h.storePending(fd)
    c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
    return
}

provider, err := ai.New(cfg)
if err != nil {
    log.Printf("AIExecute: ai.New failed: %v", err)
    if errors.Is(err, ai.ErrNotConfigured) {
        fd.Error = "No LLM provider configured. Set one up in Settings first."
    } else {
        fd.Error = "LLM configuration error: " + err.Error()
    }
    tok := h.storePending(fd)
    c.Redirect(http.StatusSeeOther, "/decks/new?ai_mode=true&token="+tok)
    return
}
```

And change the generate call to pass context (new signature):

```go
cards, err := provider.GenerateCards(c.Request.Context(), fd.Language, fd.Prompt)
```

Add `"errors"` to the imports. Check `templates/ai_form.html` renders
`.Data.Error`; add a `<a href="/settings">Settings</a>` link under the error
box (the `/settings` route arrives with T-034 — a dead link for a few commits
is acceptable, or gate it until T-034 if preferred).

**Done when:** `go build ./...` passes; submitting the AI form on a fresh DB
(no settings) shows the "configure in Settings" error instead of a 500/panic.
This completes most of T-035 — after T-034 lands, re-check T-035's own "Done
when" end-to-end.

#### F-102: Add missing unit tests for OpenAI-compatible provider 🟡 required for T-032 sign-off

**File:** `internal/ai/openai_compat_test.go` (new)

**Problem:** T-032 spec requires three `httptest` scenarios; none were written.

**Fix:** implement exactly the three tests from T-032:

1. **Happy path** — `httptest.NewServer` returns
   `{"choices":[{"message":{"content":"[{\"front\":\"хлеб\",\"back\":\"bread\",\"example\":\"…\"}]"}}]}`;
   build provider via `newOpenAICompat(Config{BaseURL: srv.URL, Model: "m"})`;
   assert one card with correct fields, no error.
2. **Auth header** — handler captures `r.Header.Get("Authorization")`; with
   `APIKey: "sk-test"` expect `Bearer sk-test`; with empty key expect the
   header absent.
3. **API error surfaced** — server responds `401` with body
   `{"error":{"message":"invalid api key"}}`; assert returned error is non-nil
   and its text contains `401` and the body excerpt.

**Done when:** `go test ./internal/ai/...` passes; all three scenarios present.

#### Review notes (no action needed)

- `internal/models/settings.go`, `internal/ai/ai.go`, `internal/ai/prompt.go`,
  `internal/ai/openai_compat.go` reviewed line-by-line — faithful to spec
  (timeouts, conditional auth header, error bodies capped at 200 bytes,
  nil-safe `choices`, trailing-slash normalization, ollama dep removed).
- Anthropic placeholder in `ai.go` is fine until T-033; T-033 must delete
  `placeholderProvider` and `newAnthropic` stub when adding the real client.
- `DB_PATH` env var in `main.go`: keep.

## Out of scope (explicitly deferred)

- Streaming generation progress to the browser (current UX: spinner page).
- Multiple saved LLM profiles / per-deck provider choice.
- Encrypting the API key at rest.
- Feedback screen for flashcard mode (self-graded by design).
- Historical stats across sessions (charts, streaks) — `session_answers` is
  the foundation for this later.
