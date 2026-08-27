# MemoLang — Architecture (as of 2026-08-27)

Snapshot of the current system. This describes what exists today, not what's planned — see `docs/plans/` for upcoming work (e.g. `auth-sessions-plan.md`).

## Overview

MemoLang is a single-tenant, server-rendered Go web app for language-learning flashcards (Anki-style). No JS framework, no ORM, no external services besides an optional LLM provider for AI-generated decks.

## Request path

```
main.go (entrypoint: opens DB, builds router, listens :8080)
  → internal/app/app.go       (route table — gin.Engine)
    → internal/handlers/*.go  (gin.Context in, form parsing, redirect/flash)
      → internal/models/*.go  (plain SQL via database/sql, one file per entity)
      → internal/srs/sm2.go   (pure function, no DB — SM-2 spaced-repetition math)
      → internal/ai/*.go      (LLM provider clients, only touched by AI deck generation)
    → templates/*.html        (html/template, server-rendered, layout.html + per-page content blocks)
```

## Stack

| Layer       | Choice                              |
|-------------|--------------------------------------|
| Language    | Go 1.27                              |
| Router      | gin-gonic/gin                        |
| Templates   | html/template (stdlib)               |
| DB          | SQLite via modernc.org/sqlite (pure Go, no CGO) |
| DB access   | database/sql (stdlib), no ORM        |
| CSS         | hand-written (`static/style.css`)    |
| JS          | ~15 lines (`static/card.js`, card-flip animation only) |
| E2E testing | Playwright (Go bindings), boots the real server against a temp SQLite file |

## Data model (current tables)

```
decks            — id, name, mode ('srs' | 'linear'), created_at
cards            — id, deck_id → decks, front, back, example, tags,
                    SM-2 fields (interval, ease, repetitions, due_date), created_at
study_sessions   — id, deck_id → decks, quiz_mode ('flashcard' | 'mc'),
                    card_queue (JSON int array), position, correct, total,
                    started_at, ended_at
session_answers  — id, session_id → study_sessions, card_id → cards,
                    correct, given, answered_at   (per-answer record for
                    feedback screens + summary breakdown)
settings         — key, value, updated_at   (global key/value store;
                    currently holds only llm.* keys)
```

No `users` table yet — everything is global/single-tenant. Foreign keys use `ON DELETE CASCADE`, enforced via `PRAGMA foreign_keys = ON` set at connection time (`internal/db/db.go`). Schema is a single `schema.sql` file executed with `CREATE TABLE IF NOT EXISTS` on every startup — idempotent for new tables, but there's no migration tooling for altering existing tables (see the auth plan's "migration gap" note for why this matters going forward).

## Key design decisions

**No ORM, no JS framework.** All DB access is `database/sql` with plain SQL strings in `internal/models/`. All UI is server-rendered Go templates.

**All mutations are POST + redirect.** HTML forms only support GET/POST, so every write is a `POST` that redirects on success (PRG pattern). Flash messages ride a short-lived cookie (`Max-Age: 5`), set before redirect, read + cleared on the next GET (`Handler.redirectWithFlash` / `Handler.getFlash` in `internal/handlers/handlers.go`).

**Session state lives in the DB.** The active study session (card queue as JSON, position, running score) is stored in `study_sessions` so it survives page refresh. `GetActiveSession(db, deckID)` returns nil if none is active. (Note: "session" here means *study* session, not a login session — there is no login session concept yet.)

**Templates share a layout.** `templates/layout.html` defines `_header`/`_footer` blocks; every page template wraps its content between them. Every handler passes a `PageData{Title, Flash, Data}` struct to `h.render()`; `Data` holds the page-specific payload (one struct per page in `internal/handlers/data.go`).

**In-memory pending-state caches.** Two flows use a process-local `sync.Map` keyed by a random token instead of persisting to the DB: the AI deck-generation form (`Handler.pending` — survives the processing-page redirect chain) and CSV import preview (`importCache` in `internal/handlers/import.go` — holds parsed rows between preview and execute steps). Both are single-instance-only caches; they don't survive a restart and wouldn't be shared across replicas.

**LLM provider abstraction.** `internal/ai/` defines a `Provider` interface (`GenerateCards`, `Ping`) with two implementations: `anthropic.go` (native Anthropic Messages API) and `openai_compat.go` (OpenAI chat-completions format, used for Ollama/OpenAI/LM Studio/custom — all speak the same wire format). Both are plain `net/http` + `encoding/json`, no SDKs. Config is loaded per-request from the `settings` table (`ai.LoadConfig`) — no caching, so settings changes apply immediately.

**SM-2 algorithm.** `internal/srs/sm2.go` is a pure function (`Update(state, rating) (newState, dueDate)`) with no DB access — easy to unit test in isolation, called by handlers which then persist the result via `models.UpdateCardSRS`.

## CSV import flow

Two-step POST: `?step=preview` parses the uploaded file and re-renders the form with a preview table (rows cached in-memory under a token); `?step=execute` re-reads the cached rows and bulk-inserts in a single transaction. Duplicate fronts within a deck are skipped via `INSERT OR IGNORE` (backed by a `UNIQUE INDEX` on `(deck_id, front)`).

## Deployment

`Dockerfile` — two-stage build, `golang:1.27-alpine` builder → `alpine:3.21` runtime, `CGO_ENABLED=0`. `.github/workflows/docker.yml` builds and pushes to `ghcr.io/tj-dorje/memolang` on every push to `main` (tags: `latest` + commit sha). `DB_PATH` env var overrides the default `memolang.db` file path (Docker support).

## Known gaps (as-is, not a todo list)

- Single-tenant: no auth, no per-user data isolation (see `docs/plans/auth-sessions-plan.md`)
- No migration tooling beyond idempotent `CREATE TABLE IF NOT EXISTS`
- No CSRF protection (no auth session to protect yet, so lower stakes today)
- In-memory caches (`pending`, `importCache`) are single-instance-only
