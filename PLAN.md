# MemoLang — Engineering Plan & Backlog

## Overview
Language flashcard web app. Users import word/verb/phrase packs via CSV, then
study them through flashcard and multiple-choice sessions. Supports two
scheduling modes per deck: Spaced Repetition (SM-2) or Linear.

---

## Tech Stack

| Layer       | Choice                           | Reason                                      |
|-------------|----------------------------------|---------------------------------------------|
| Language    | Go 1.26                          | already set up                              |
| Router      | gin-gonic/gin                    | fast, good middleware support               |
| Templates   | html/template (stdlib)           | server-rendered, no JS framework            |
| DB          | SQLite via modernc.org/sqlite    | zero-config, pure Go (no CGO required)      |
| DB access   | database/sql (stdlib)            | no ORM, plain SQL                           |
| CSS         | hand-written, minimal            | clean/minimal style chosen                  |
| JS          | ~20 lines vanilla                | only for card-flip animation                |

---

## Project Structure

```
memolang/
├── main.go                        # server entry point, route wiring
├── go.mod / go.sum
├── internal/
│   ├── db/
│   │   ├── db.go                  # open connection, run migrations
│   │   └── schema.sql             # CREATE TABLE statements
│   ├── models/
│   │   ├── deck.go                # Deck struct + CRUD
│   │   ├── card.go                # Card struct + CRUD
│   │   └── session.go             # StudySession struct + queries
│   ├── handlers/
│   │   ├── deck.go                # deck CRUD handlers
│   │   ├── card.go                # card CRUD handlers
│   │   ├── session.go             # study session handlers
│   │   └── import.go              # CSV upload + parse handler
│   └── srs/
│       └── sm2.go                 # SM-2 algorithm (pure function)
├── templates/
│   ├── layout.html                # base: <head>, nav, footer wrapper
│   ├── dashboard.html             # deck grid
│   ├── deck.html                  # deck detail + card list
│   ├── card_edit.html             # create/edit card form
│   ├── deck_form.html             # create/edit deck form
│   ├── import.html                # CSV upload + column mapping
│   ├── session.html               # flashcard + MC (conditional blocks)
│   └── session_summary.html       # end-of-session results
└── static/
    ├── style.css
    └── card.js                    # card flip toggle only
```

---

## Database Schema

```sql
-- internal/db/schema.sql

CREATE TABLE IF NOT EXISTS decks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    mode       TEXT    NOT NULL DEFAULT 'srs',   -- 'srs' | 'linear'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    front       TEXT    NOT NULL,        -- word / phrase (target language)
    back        TEXT    NOT NULL,        -- translation
    example     TEXT,                    -- example sentence (optional)
    tags        TEXT    DEFAULT '',      -- comma-separated, e.g. "verb,irregular"
    -- SM-2 state
    interval    INTEGER NOT NULL DEFAULT 1,    -- days until next review
    ease        REAL    NOT NULL DEFAULT 2.5,  -- ease factor
    repetitions INTEGER NOT NULL DEFAULT 0,
    due_date    DATE    NOT NULL DEFAULT (date('now')),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS study_sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    quiz_mode   TEXT    NOT NULL,              -- 'flashcard' | 'mc'
    card_queue  TEXT    NOT NULL,              -- JSON array of card IDs
    position    INTEGER NOT NULL DEFAULT 0,   -- current index in card_queue
    correct     INTEGER NOT NULL DEFAULT 0,
    total       INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    ended_at    DATETIME
);
```

### Key query patterns
- **Due cards (SRS):** `WHERE deck_id=? AND due_date <= date('now') ORDER BY due_date`
- **New cards (Linear):** `WHERE deck_id=? AND repetitions=0 ORDER BY id LIMIT ?`
- **Mastery %:** `100.0 * COUNT(CASE WHEN repetitions >= 3 THEN 1 END) / COUNT(*)`

---

## Routes

```
GET  /                                → dashboard
GET  /decks/new                       → new deck form
POST /decks/new                       → create deck → redirect /
GET  /decks/{id}                      → deck detail (browse cards)
GET  /decks/{id}/edit                 → edit deck form
POST /decks/{id}/edit                 → update deck → redirect /decks/{id}
POST /decks/{id}/delete               → delete deck → redirect /
GET  /decks/{id}/import               → CSV import form
POST /decks/{id}/import               → parse CSV, bulk-insert cards → redirect /decks/{id}
GET  /decks/{id}/session              → start or resume session
POST /decks/{id}/session/answer       → record answer, advance, redirect back
GET  /decks/{id}/session/summary      → show results (session ended)
GET  /cards/{id}/edit                 → edit card form
POST /cards/{id}/edit                 → update card → redirect /decks/{deck_id}
POST /cards/{id}/delete               → delete card → redirect /decks/{deck_id}
```

All mutations use POST (Go template forms only support GET/POST). No JS fetch.

Gin handler signature: `func(c *gin.Context)`. Path params via `c.Param("id")`, form values via `c.PostForm("field")`, redirect via `c.Redirect(http.StatusSeeOther, "/path")`.

---

## Session Flow (server-side)

```
GET /decks/{id}/session
  1. Check for an active study_sessions row (ended_at IS NULL) for this deck.
  2. If none: build card queue (SRS → due cards; Linear → next N unlearned),
     INSERT study_session row with card_queue as JSON array, position=0.
  3. Load card at card_queue[position].
  4. Render session.html (flashcard or MC based on deck mode).

POST /decks/{id}/session/answer  (form fields: session_id, card_id, rating OR choice)
  1. Load session row.
  2. Update SM-2 fields on the card (SRS mode) or mark seen (linear mode).
  3. Increment session.correct / session.total.
  4. Increment session.position.
  5. If position >= len(card_queue): set ended_at=now, redirect → summary.
  6. Else: redirect → GET session (next card).
```

### Multiple choice option generation
On each MC render, pick 3 random `back` values from the deck (excluding correct),
combine with correct answer, shuffle. Done in the handler, passed to template.

---

## SM-2 Algorithm  (`internal/srs/sm2.go`)

Pure function, no DB access.

```
Input:  CardState{Interval, Ease, Repetitions}, rating (0..3)
Output: updated CardState + new DueDate

Rating mapping:
  0 = Again  → SM-2 quality 0
  1 = Hard   → SM-2 quality 2
  2 = Good   → SM-2 quality 4
  3 = Easy   → SM-2 quality 5

Algorithm:
  if quality < 3 (Again or Hard):
      repetitions = 0
      interval = 1
  else:
      if repetitions == 0: interval = 1
      elif repetitions == 1: interval = 6
      else: interval = round(interval * ease)
      repetitions += 1

  ease = max(1.3, ease + 0.1 - (5-q)*(0.08 + (5-q)*0.02))
  due_date = today + interval days
```

---

## CSV Import Format

Expected columns (order determined by user via column-mapping UI):

| Column        | Required | Notes                            |
|---------------|----------|----------------------------------|
| front         | yes      | word / phrase in target language |
| back          | yes      | translation                      |
| example       | no       | example sentence                 |
| tags          | no       | comma-separated                  |

First row may be a header (auto-detected: if first cell matches known column
names it's skipped).

---

## Template Strategy

All pages extend `layout.html` using Go template `define`/`template` blocks:

```
layout.html  defines: "layout" block with {{template "content" .}}
each page    defines: "content" block
handler      parses both files, executes "layout"
```

Pass a shared `PageData` struct to every template:
```go
type PageData struct {
    Title   string
    Flash   string      // one-time success/error message via cookie
    Data    interface{} // page-specific payload
}
```

Flash messages: set a `flash` cookie on redirect, read+clear it on next GET.

---

---

# Backlog

Milestones are ordered by dependency. Complete M0 before M1, etc.

---

## M0 — Project Setup

### T-001: Add SQLite dependency
**What:** Add `modernc.org/sqlite` to go.mod (pure Go, no CGO).  
**Also add:** `github.com/mattn/go-sqlite3` is an alternative but requires CGO — avoid it.  
**Done when:** `go build ./...` succeeds with the new dep.

### T-002: Restructure directories
**What:** Create `internal/db/`, `internal/models/`, `internal/handlers/`, `internal/srs/` with placeholder `.go` files (package declarations only).  
**Done when:** `go build ./...` compiles with new structure.

### T-003: DB init + schema
**File:** `internal/db/db.go`  
**What:**
- `Open(path string) (*sql.DB, error)` — opens SQLite file, sets `PRAGMA foreign_keys = ON`, `PRAGMA journal_mode = WAL`.
- Reads and executes `schema.sql` on startup (CREATE TABLE IF NOT EXISTS — idempotent).  
**Done when:** running the server creates `memolang.db` with the three tables.

### T-004: Base layout template
**File:** `templates/layout.html`  
**What:** HTML5 boilerplate with `<head>` (charset, viewport, CSS link), `<header>` with nav (MemoLang logo → `/`, "+ New Deck" → `/decks/new`), `<main>{{template "content" .}}`, `<footer>`.  
Flash message display: if `.Flash` non-empty, render a small notice banner below nav.

### T-005: Refactor main.go
**What:** Move route registration to a `routes(db *sql.DB) http.Handler` function. Instantiate DB, build handlers (pass `db`), register all routes from the routes table above. Remove placeholder `aboutHandler`.  
**Done when:** server starts, `GET /` returns 200.

### T-006: Minimal style.css
**File:** `static/style.css`  
**What:** CSS reset + base styles for layout, nav, cards grid, buttons, form inputs, progress bar, flash message. No framework — ~150 lines max. Clean/minimal: white background, `#333` text, one accent color (`#4f6ef7`), generous padding.

---

## M1 — Deck Management

### T-007: Deck model
**File:** `internal/models/deck.go`  
**Struct:**
```go
type Deck struct {
    ID        int64
    Name      string
    Mode      string    // "srs" | "linear"
    CreatedAt time.Time
    // computed, not stored:
    CardCount  int
    DueCount   int
    MasteryPct int
}
```
**Functions:**
- `CreateDeck(db, name, mode) (Deck, error)`
- `GetAllDecks(db) ([]Deck, error)` — includes computed fields via JOIN/subquery
- `GetDeckByID(db, id) (Deck, error)`
- `UpdateDeck(db, id, name, mode) error`
- `DeleteDeck(db, id) error`

### T-008: Dashboard handler + template
**File:** `internal/handlers/deck.go` → `DashboardHandler`  
**Template:** `templates/dashboard.html`  
**What:** Calls `GetAllDecks`, renders deck grid. Each card shows: name, card count, due count badge (hidden if 0), mastery progress bar, [Study] and [Browse] buttons.  
**Empty state:** if no decks, show "No decks yet. Import your first pack." with link to `/decks/new`.

### T-009: Create deck handler + template
**Handler:** `NewDeckForm` (GET) + `CreateDeck` (POST)  
**Template:** `templates/deck_form.html`  
**Form fields:** Name (text, required), Mode (radio: SRS / Linear).  
**POST:** validate name not empty, insert, redirect to `/` with flash "Deck created."  
**Error:** re-render form with inline error if name empty.

### T-010: Edit deck handler
**Handler:** `EditDeckForm` (GET) + `UpdateDeck` (POST)  
**Template:** reuse `deck_form.html` (pre-filled values).  
**POST:** update name/mode, redirect to `/decks/{id}` with flash "Saved."

### T-011: Delete deck handler
**Handler:** `DeleteDeck` (POST `/decks/{id}/delete`)  
**What:** Delete deck (CASCADE deletes cards + sessions), redirect to `/` with flash "Deck deleted."  
**Template note:** delete button is a `<form method="POST">` with a confirmation via `onclick="return confirm(...)"`.

---

## M2 — Card Management

### T-012: Card model
**File:** `internal/models/card.go`  
**Struct:**
```go
type Card struct {
    ID          int64
    DeckID      int64
    Front       string
    Back        string
    Example     string
    Tags        string
    Interval    int
    Ease        float64
    Repetitions int
    DueDate     time.Time
    CreatedAt   time.Time
}
```
**Functions:**
- `CreateCard(db, deckID, front, back, example, tags) (Card, error)`
- `GetCardsByDeck(db, deckID, filter string) ([]Card, error)` — filter: "all"|"due"|"new"
- `GetCardByID(db, id) (Card, error)`
- `UpdateCard(db, id, front, back, example, tags) error`
- `UpdateCardSRS(db, id, interval int, ease float64, repetitions int, dueDate time.Time) error`
- `DeleteCard(db, id) error`
- `GetRandomBackValues(db, deckID, excludeID int64, n int) ([]string, error)` — for MC options

### T-013: Deck detail handler + template
**Handler:** `DeckDetail` (GET `/decks/{id}`)  
**Template:** `templates/deck.html`  
**What:** Shows deck stats (card count, mastery %, due count), filter tabs [All | Due | New], card list rows (front | back | example snippet | [Edit] button), [Import CSV] and [Start Session] buttons.  
**Search:** `?q=` query param filters card list by front/back LIKE.

### T-014: Edit card handler + template
**Handler:** `EditCardForm` (GET) + `UpdateCard` (POST)  
**Template:** `templates/card_edit.html`  
**Fields:** Front (text), Back (text), Example (textarea, optional), Tags (text, optional).  
**POST:** update, redirect to `/decks/{deck_id}` with flash "Card saved."

### T-015: Delete card handler
**Handler:** `DeleteCard` (POST `/cards/{id}/delete`)  
**What:** Delete card, redirect to `/decks/{deck_id}`.  
Form must include hidden `deck_id` field for the redirect target.

---

## M3 — CSV Import

### T-016: Import page handler + template
**Handler:** `ImportForm` (GET `/decks/{id}/import`)  
**Template:** `templates/import.html`  
**What:** File upload input (`<input type="file" accept=".csv">`), column mapping selects (Front, Back, Example), submit button "Preview". Deck name shown as context.  
**Note:** Preview requires a second step — see T-017.

### T-017: CSV parse + preview
**Handler:** `ImportPreview` (POST, same route, `?step=preview`)  
**What:**
1. Receive uploaded file (multipart form, max 5MB).
2. Parse with `encoding/csv`.
3. Auto-detect header row: if first cell is one of `front|word|term|back|translation` (case-insensitive), treat as header.
4. Read column-mapping selections from form.
5. Re-render `import.html` with: preview table (first 5 rows mapped), total row count, hidden fields for column indices.  
**Validation:** return error if file is not valid CSV, or if required columns (front, back) are not mapped.

### T-018: Import execute
**Handler:** `ImportExecute` (POST, same route, `?step=execute`)  
**What:**
1. Re-parse uploaded CSV (or pass mapped rows via hidden fields for small files).
2. Bulk-insert cards with `INSERT INTO cards ...` in a single transaction.
3. Redirect to `/decks/{id}` with flash "{N} cards imported."  
**Note:** Skip duplicate fronts (same deck) using `INSERT OR IGNORE` with a UNIQUE index on `(deck_id, front)`.

---

## M4 — SM-2 Algorithm

### T-019: SM-2 pure function
**File:** `internal/srs/sm2.go`  
**What:** Implement algorithm as described in Engineering Plan section above.  
```go
type CardState struct {
    Interval    int
    Ease        float64
    Repetitions int
}
func Update(s CardState, rating int) (CardState, time.Time)
```
Rating values: `0=Again, 1=Hard, 2=Good, 3=Easy`.  
Returns updated state and the next due date.  
**Include unit tests** in `internal/srs/sm2_test.go` covering: first review (Good), failed card resets interval, ease floor at 1.3, Easy increases interval.

---

## M5 — Study Session

### T-020: Session model
**File:** `internal/models/session.go`  
**Struct:**
```go
type StudySession struct {
    ID        int64
    DeckID    int64
    QuizMode  string    // "flashcard" | "mc"
    CardQueue []int64   // deserialized from JSON
    Position  int
    Correct   int
    Total     int
    StartedAt time.Time
    EndedAt   *time.Time
}
```
**Functions:**
- `CreateSession(db, deckID, quizMode string, cardIDs []int64) (StudySession, error)`
- `GetActiveSession(db, deckID) (*StudySession, error)` — returns nil if none
- `AdvanceSession(db, id, correct bool) error` — increments position, correct, total
- `EndSession(db, id) error` — sets ended_at

### T-021: Session start handler
**Handler:** `StartSession` (GET `/decks/{id}/session`)  
**What:**
1. Check for active session via `GetActiveSession`.
2. If none: build queue.
   - SRS mode: `SELECT id FROM cards WHERE deck_id=? AND due_date <= date('now') ORDER BY due_date LIMIT 50`
   - Linear mode: `SELECT id FROM cards WHERE deck_id=? AND repetitions=0 ORDER BY id LIMIT 20`
   - If queue empty: render "Nothing due today" message with [Back] button.
3. Create session, load first card.
4. If MC mode: call `GetRandomBackValues` for 3 distractors, shuffle with correct.
5. Render `session.html`.

### T-022: Session template
**File:** `templates/session.html`  
**What:** Single template with two conditional blocks based on `.QuizMode`:

*Flashcard block:*
- Progress bar (`position / total`)
- Card box: front text + example sentence
- [Reveal] button (JS toggles visibility of back side)
- After reveal: rating buttons [Again] [Hard] [Good] [Easy] as a POST form

*MC block:*
- Progress bar
- Question: "What does '{{.Card.Front}}' mean?"
- 4 option buttons (A/B/C/D), each a POST form with `choice` field
- No reveal step needed

### T-023: Answer handler
**Handler:** `SubmitAnswer` (POST `/decks/{id}/session/answer`)  
**Form fields:** `session_id`, `card_id`, `rating` (flashcard) OR `choice` (MC, value is the selected `back` string)  
**What:**
1. Load session and card.
2. Flashcard: use SM-2 `Update()`, call `UpdateCardSRS`.  
   MC: mark correct if `choice == card.Back`. For SRS mode also apply SM-2 (correct=Good, wrong=Again).
3. `AdvanceSession` (or `EndSession` if last card).
4. Redirect to `GET /decks/{id}/session` (or summary).

### T-024: Session summary handler + template
**Handler:** `SessionSummary` (GET `/decks/{id}/session/summary`)  
**Template:** `templates/session_summary.html`  
**What:** Loads the most recently ended session for the deck. Shows: total reviewed, correct count, accuracy %, next due count (cards due tomorrow). Buttons: [Study again] (start new session) | [Back to deck].

---

## M6 — UI Polish

### T-025: Progress bar component
**What:** CSS-only progress bar. In templates, render as:
```html
<div class="progress"><div class="progress-fill" style="width: {{.MasteryPct}}%"></div></div>
```
Style in `style.css`: height 6px, border-radius, accent color fill.

### T-026: Card flip animation
**File:** `static/card.js`  
**What:** ~15 lines. On `DOMContentLoaded`, find `.card-reveal-btn`, on click: toggle `.card--flipped` class on parent `.card`, show `.card-back` div, hide the button.  
No other JS in the app.

### T-027: Mobile-friendly layout
**What:** In `style.css`, add:
- Viewport meta already in layout.html.
- Deck grid: `display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr))`
- Session card: max-width 480px, centered.
- Nav: wraps on small screens.
- Buttons: `min-height: 44px` (touch target).

### T-028: Flash message cookie
**File:** `internal/handlers/` (shared helper)  
**What:** Two helper functions used by all POST handlers:
```go
func SetFlash(w http.ResponseWriter, msg string)
func GetFlash(w http.ResponseWriter, r *http.Request) string
```
Use a short-lived cookie (`Max-Age: 5`). `GetFlash` reads + immediately clears the cookie.

---

## Definition of Done (per task)
- Code compiles (`go build ./...`)
- No unhandled errors (all `err != nil` return HTTP 500 or re-render form)
- Template renders without `<no value>` gaps
- Route is reachable end-to-end in browser

---

## Build Order Summary

```
M0: T-001 → T-002 → T-003 → T-004 → T-005 → T-006
M1: T-007 → T-008 → T-009 → T-010 → T-011
M2: T-012 → T-013 → T-014 → T-015
M3: T-016 → T-017 → T-018
M4: T-019  (can be done in parallel with M2/M3)
M5: T-020 → T-021 → T-022 → T-023 → T-024  (requires M4)
M6: T-025 → T-026 → T-027 → T-028  (can be done alongside M1-M5)
```

Total: 28 tasks across 6 milestones.

---

## Status (as of 2026-05-14)

**All 28 tasks are implemented.** The app builds and runs end-to-end.

| Milestone | Tasks | Status |
|-----------|-------|--------|
| M0 — Project Setup | T-001..006 | ✅ Complete |
| M1 — Deck Management | T-007..011 | ✅ Complete |
| M2 — Card Management | T-012..015 | ✅ Complete |
| M3 — CSV Import | T-016..018 | ✅ Complete |
| M4 — SM-2 Algorithm | T-019 | ✅ Complete + unit tests passing |
| M5 — Study Session | T-020..024 | ✅ Complete |
| M6 — UI Polish | T-025..028 | ✅ Complete |

Known issues and deviations from spec are tracked in `ISSUES_TODO.md`.

## DevOps

### T-029: Use Infisical for secrets
**What:** When secrets are needed (API keys, passwords, etc.), create them in Infisical and reference them via `InfisicalSecret` CRD + `secretKeyRef` in the k8s deployment manifest. No YAML should contain hardcoded secrets, passwords, or tokens.  
**Template:** follow the pattern in `homelab/apps/n8n/infisical-secret.yaml`.
