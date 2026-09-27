# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

MemoLang is a language-learning flashcard web app (Anki-style). Users import word/verb/phrase packs via CSV and study them through flashcard or multiple-choice sessions. Two scheduling modes per deck: **SRS** (SM-2 spaced repetition) and **Linear** (sequential, unlearned cards first).

Full engineering plan and task backlog are in `docs/plans/PLAN.md` (follow-on work in `docs/plans/REFINEMENT_PLAN.md`; the card-maintenance agent, M11, in `docs/plans/ASSISTANT_AGENT_PLAN.md`; the production stability upgrade — Postgres, JetStream broker, LLM workers — M12, in `docs/plans/SCALING_PLAN.md`). Chronological journal of key tech decisions and solutions is in `JOURNAL.md`.

## Tooling

Use the native **Grep tool** for all code searches — not `grep` or `rg` via Bash. The Grep tool uses ripgrep under the hood and integrates with the editor context directly.

## Commands

Prefer `task` for common operations:

```bash
task dev          # run the server
task build        # compile binary
task test         # run all tests
task test:srs     # run SM-2 unit tests only
task db:reset     # wipe the SQLite DB (recreated clean on next start)
task tidy         # go mod tidy
```

Or directly with Go:

```bash
go run .
go build ./...
go test ./...
go test ./internal/srs/... -run TestSM2Update   # single test
go get <module> && go mod tidy
```

Server runs on `:8080`. Templates and static files are loaded from disk at runtime — no rebuild needed when editing them.

## Architecture

### Request path
```
main.go (route registration)
  → internal/handlers/*.go  (gin.Context, form parsing, redirect/flash)
    → internal/models/*.go  (plain SQL via database/sql)
    → internal/srs/sm2.go   (pure function, no DB)
  → templates/*.html        (html/template, server-rendered)
```

### Key design decisions

**No ORM, no JS framework.** All DB access is `database/sql` with plain SQL strings in `internal/models/`. All UI is server-rendered Go templates. JavaScript is progressive enhancement only and kept tiny (`static/card.js` flashcard flip, `settings.js` preset prefill, `menu.js` close-on-outside-click); every page works without it. Live output is done without JS too — see "Streaming pages" below.

**All mutations are POST + redirect.** HTML forms only support GET/POST, so every write operation (create, update, delete, submit answer) is a `POST` that redirects on success. Flash messages are passed via a short-lived cookie (`Max-Age: 5`) set before redirect and read+cleared on the next GET.

**Auth: password login, DB-backed sessions, private decks.** Everything except `/login`, `/register`, `/logout` and `/static` sits behind `middleware.RequireAuth`, which resolves the `session` cookie to a user and puts it in the gin context (`currentUserID(c)` reads it back). Decks are owned; `GetDeckByID`/`GetCardByID` scope by owner so another user's id is a 404, never a 403 (no existence leak). Handlers that take an id from the request body (`SubmitAnswer`, `EndSessionEarly`) bind it back to an owned deck via `GetSessionByID` before writing. Cookies are httpOnly + `SameSite=Lax` (the CSRF mitigation for now, since every mutation is a POST); set `SECURE_COOKIES=1` when serving over TLS.

**Streaming pages (the AI assistant: tutor, deck builder).** LLM replies stream into the page with no JavaScript: the POST stores the question, starts generation in a detached goroutine (`internal/assistant`), and 303s to the GET, which writes the page's top half, flushes, writes each reply chunk HTML-escaped and flushed as it arrives, then the bottom half (`renderChat` in `internal/handlers/chat.go`; shared partials in `templates/chat.html`). The goroutine publishes to a `stream.Broker` (`internal/stream`, in-memory; an interface so NATS could replace it for multiple replicas), so a reload reattaches to the same reply and a closed tab doesn't cancel it. `ai.Provider.Chat` streams on both transports. Model output is untrusted: always escape it. The deck builder's replies can end in a ` ```deck {json} ``` ` plan block (`assistant.ExtractDeckPlan`); `assistant.PlanFilter` hides it while streaming and the page shows it as a card whose Generate button posts only the message id. Generate runs card generation the same way (`assistant.GenerateDeck`): the deck is created at once, a background goroutine streams the model's JSON array through `cardScanner`, saving each card the moment its object closes, and publishes one JSON event line per card for `/decks/:id/generating`. Background LLM calls end on an idle watchdog (no output — reasoning included — for 2 minutes) with a 15-minute backstop, not a fixed deadline.

**Session state lives in the DB.** The active study session (card queue as JSON, current position, score) is stored in `study_sessions` so it survives page refresh. `GetActiveSession(db, deckID)` returns nil if none is active.

**Templates use a shared layout.** `layout.html` wraps every page via Go template `define`/`template` blocks. Every handler passes a `PageData{Title, Flash, User, Data}` struct (`h.render` fills in `User` from the gin context for the nav). `Data` holds the page-specific payload.

### Stack

| Layer | Choice |
|---|---|
| Router | `gin-gonic/gin` |
| Templates | `html/template` (stdlib) |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| DB access | `database/sql` (stdlib) |

### Gin handler conventions

```go
// Path param
id := c.Param("id")

// Form value
name := c.PostForm("name")

// Render template
c.HTML(http.StatusOK, "page.html", PageData{...})

// Redirect after POST
c.Redirect(http.StatusSeeOther, "/decks/"+id)
```

### SM-2 algorithm

Lives in `internal/srs/sm2.go` as a pure function with no DB access:

```go
func Update(s CardState, rating int) (CardState, time.Time)
// rating: 0=Again, 1=Hard, 2=Good, 3=Easy
```

After calling `Update`, persist the result with `models.UpdateCardSRS(...)`.

It is SM-2 with Anki's reading of the buttons: Hard is a *pass* (slow growth), and Again < Hard < Good < Easy always give strictly increasing gaps. `srs.Preview(state)` returns the four gaps without changing anything; the study screen shows them under the buttons, so `Update` must take its numbers from `Preview`. Again also re-queues the card at the end of the session (max 3 appearances per card).

### Database schema (10 tables)

- `users` — email (UNIQUE, `COLLATE NOCASE`), password_hash (bcrypt; empty is reserved for future OAuth-only accounts), display_name (optional; `User.Name()` falls back to email)
- `user_sessions` — login sessions: token (PK), user_id, expires_at
- `decks` — user_id (owner), name, mode (`srs`|`linear`)
- `cards` — front, back, example, tags, SM-2 fields (interval, ease, repetitions, due_date)
- `study_sessions` — card_queue (JSON int array), position, correct/total counters, ended_at
- `session_answers` — per-answer record backing the feedback and summary screens
- `settings` — per-user key/value store (composite PK `(user_id, key)`); unused since the `llm.*` keys moved to `llm_providers`
- `llm_providers` — a user's saved LLM configurations (name, preset, base_url, model, api_key, disable_thinking); at most one `active` per user (partial unique index). `ai.LoadConfig` reads the active one; switching is `ActivateLLMProvider` (one transaction)
- `conversations` — assistant chats: kind `tutor` (one per user per card) or `deck_builder` (one open per user); `conversation_messages` — role, content, status (`done` | `generating` | `error`)

Foreign keys with `ON DELETE CASCADE` are enforced via `_pragma=foreign_keys(1)` in the DSN, so every pooled connection gets it (an `Exec`'d pragma only affects one connection).

### Migrations

`db.Open` runs `internal/db/migrations/NNNN_*.sql` in order, tracking the version in `PRAGMA user_version` (see `internal/db/migrate.go`). To change the schema, add the next numbered file and a `{version, file}` entry to `migrations`; never edit a shipped one. Each migration is one transaction with foreign keys off (checked with `foreign_key_check` before commit), because SQLite table rebuilds (create `x_new`, copy, drop, rename) would otherwise cascade-delete child rows. A migration can have a Go `prepare` step for work SQL cannot do.

Upgrading a pre-auth database that has decks needs `MIGRATE_OWNER_EMAIL` + `MIGRATE_OWNER_PASSWORD` for one start: `0003` creates that account and gives it the old decks and settings. Without decks, no owner is needed and old settings are dropped.

### CSV import flow

Two-step POST: `?step=preview` parses the file and re-renders the form with a preview table; `?step=execute` does the bulk insert in a single transaction. Duplicate fronts within a deck are skipped via `INSERT OR IGNORE` (requires UNIQUE index on `(deck_id, front)`).
