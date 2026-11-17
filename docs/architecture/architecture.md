# MemoLang — Architecture

Snapshot of the system as it is today, not what's planned — see `docs/plans/` for upcoming work. Tables and migrations are in [`data-model.md`](data-model.md); the scheduling algorithm is in [`../design/srs.md`](../design/srs.md).

## Overview

MemoLang is a multi-user, server-rendered Go web app for language-learning flashcards (Anki-style). Users import word/verb/phrase packs via CSV (or have an LLM build a deck) and study them through flashcard or multiple-choice sessions. Each deck has a scheduling mode: **SRS** (SM-2 spaced repetition) or **Linear** (sequential, unlearned cards first).

One binary, two modes: `memolang` is the web app; `memolang worker` runs LLM jobs taken from a NATS JetStream queue.

## Request path

```
main.go (entrypoint: opens DB, picks job queue + broker, listens :8080)
  → internal/app/app.go          (route table — gin.Engine)
    → internal/middleware/auth.go (session cookie → user on the gin context)
    → internal/handlers/*.go     (gin.Context in, form parsing, redirect/flash)
      → internal/models/*.go     (plain SQL via database/sql, one file per entity)
      → internal/srs/sm2.go      (pure function, no DB — SM-2 math)
      → internal/assistant/      (tutor + deck builder: prompts, jobs, card scanning)
        → internal/ai/           (LLM provider clients)
        → internal/stream/       (Broker: live output from producer to page)
        → internal/jobs/         (JetStream work queue, worker side)
    → templates/*.html           (html/template, server-rendered)
```

## Stack

| Layer        | Choice |
|--------------|--------|
| Language     | Go 1.27 |
| Router       | gin-gonic/gin |
| Templates    | html/template (stdlib) |
| DB           | PostgreSQL 18 via pgx (`github.com/jackc/pgx/v5/stdlib`), behind `database/sql`; no ORM |
| Queue/stream | NATS JetStream (optional; in-process without `NATS_URL`) |
| CSS          | hand-written (`static/style.css`) |
| JS           | progressive enhancement only: `card.js` (flip), `settings.js` (preset prefill), `menu.js` (close on outside click) |
| E2E testing  | Playwright (Go bindings), against the real server; `task test:integration` runs it against the built image as web + worker containers on Postgres + NATS |

## Key design decisions

**No ORM, no JS framework.** All DB access is `database/sql` with plain SQL strings in `internal/models/`. All UI is server-rendered Go templates; every page works without JavaScript. Live output is done without JS too (see streaming pages below).

**All mutations are POST + redirect.** HTML forms only support GET/POST, so every write is a `POST` that redirects on success (PRG). Flash messages ride a short-lived cookie (`Max-Age: 5`), set before redirect, read + cleared on the next GET (`Handler.redirectWithFlash` / `Handler.getFlash` in `internal/handlers/handlers.go`).

**Auth: password login, DB-backed sessions, private decks.** Everything except `/login`, `/register`, `/verify`, `/forgot`, `/reset`, `/logout` and `/static` sits behind `middleware.RequireAuth`, which resolves the httpOnly `session` cookie to a user and puts it on the gin context (`currentUserID(c)` reads it back). Ownership is checked at whatever entry point first reads an id from the URL or body: `GetDeckByID(db, userID, id)` and `GetCardByID(db, userID, id)` fold "not yours" into `sql.ErrNoRows`, rendered as 404 — never 403, so nothing leaks about whether the id exists. `SubmitAnswer` and `EndSessionEarly` take an id from the form, so they bind it to an owned deck with `GetSessionByID` before writing. Scoped `UPDATE`/`DELETE` go through `requireRowAffected`, turning a zero-row write into `sql.ErrNoRows` rather than a false "Saved.". Passwords are bcrypt; login answers "Invalid email or password." to every failure. Session tokens are stored as `sha256` hashes (`models.hashToken`), never raw. Cookies are `SameSite=Lax` (the CSRF mitigation for now, since every mutation is a POST); `SECURE_COOKIES=1` sets the Secure flag behind TLS.

**Verified email (ISS-019, `docs/plans/verified-auth-plan.md`).** Registration creates an unverified account and redirects to `/verify`, where the user types a 6-digit code from the email (`models.IssueCode` / `VerifyEmail`: hashed, 15 min, 5 tries, single use, one live code per user and purpose); a correct code logs them in. An unverified account can't log in — the right password re-sends a code and shows `/verify`. Replies never reveal which addresses have accounts: registering a verified address mails its owner a notice and shows the same page; registering an unverified one takes it over (new password, new code). Mails carry codes, never links (`handlers/authmail.go`), and go through `internal/mail`.

**Password reset and email change.** `/forgot` always answers "if that account exists, we sent a code"; `/reset` takes the code and the new password in one form (`models.ResetPassword`: sets the password, verifies the address, ends every session) and logs the user in. Changing the email (Profile → Security) needs the current password, mails a code to the new address and a notice to the old; the address changes only when the code is typed (`models.ChangeEmail`), and the pending change shows with Cancel. A new address that belongs to another account gets a notice instead of a code. Password and email changes send a notice mail after the fact.

**Rate limits** (`handlers/ratelimit.go`, counts in `auth_attempts`, so they hold across pods and restarts). Failed logins: 5 per (email, IP), 20 per email, 50 per IP, each per 15 minutes, checked before the password; the lockout message points to password reset, which stays open. Mails sent on anonymous requests (codes, register notices): 5 per address and 30 per IP per hour, claimed before the address is looked up and before a code is issued. A sweeper in the web process (`models.SweepAuth`, hourly) deletes old events, expired codes, and unverified accounts older than 7 days.

**Session state lives in the DB.** The active study session (card queue as JSON, position, score) is stored in `study_sessions` so it survives a page refresh. `GetActiveSession(db, deckID)` returns nil if none is active. Login sessions (`user_sessions`, `internal/models/auth_session.go`) are a separate concept — hence the naming split from `models.StudySession`.

**Templates share a layout.** `layout.html` wraps every page via `define`/`template` blocks. Every handler passes a `PageData{Title, Flash, User, Data}` struct; `h.render` fills in `User` (for the nav) and shared chrome, and is the only `c.HTML` call. `Data` holds the page-specific payload (one struct per page in `internal/handlers/data.go`).

**LLM provider abstraction.** `internal/ai` defines `Provider` (`Ping`, `Chat` — streaming on both transports) with two implementations over plain `net/http` + `encoding/json`, no SDKs: native Anthropic Messages API, and OpenAI-compatible chat completions (Ollama, OpenAI, LM Studio, custom). A user's configurations live in `llm_providers`; `ai.LoadConfig` reads the active one per request, so a switch applies immediately.

## Streaming pages (AI tutor, deck builder)

LLM replies stream into the page with no JavaScript:

1. The POST stores the question, hands generation to the job queue, and 303s to the GET.
2. The GET writes the page's top half, flushes, writes each reply chunk HTML-escaped and flushed as it arrives, then the bottom half (`renderChat` in `internal/handlers/chat.go`; shared partials in `templates/chat.html`).
3. The producer publishes to a `stream.Broker`, so a reload reattaches to the same reply and a closed tab doesn't cancel a paid call.

Model output is untrusted: always escape it.

The deck builder's replies can end in a ` ```deck {json} ``` ` plan block (`assistant.ExtractDeckPlan`); `assistant.PlanFilter` hides it while streaming and the page shows it as a card whose Generate button posts only the message id. Generate (`assistant.GenerateDeck`) creates the deck at once; card generation streams the model's JSON array through `cardScanner`, saving each card the moment its object closes and publishing one JSON event line per card for `/decks/:id/generating`.

Background LLM calls end on an idle watchdog (no output — reasoning included — for 2 minutes) with a 15-minute backstop, not a fixed deadline.

## Background work: web + worker

Chat replies and card generation are an `assistant.Job` (ids and the prepared request — never an API key) handed to a `JobQueue`.

- **Without `NATS_URL`** (`task dev`, tests): the web app runs jobs in-process with the in-memory broker (`stream.Memory`).
- **With `NATS_URL`**: the web app enqueues on a JetStream work queue (`internal/jobs`, stream `MEMOLANG_JOBS`) and follows output through the JetStream broker (`stream.JetStream`, stream `MEMOLANG_REPLIES`). Workers heartbeat running jobs; a dead worker's job is redelivered after `JOBS_ACK_WAIT` (default 1m). On redelivery a chat reply is marked interrupted, while a deck generation resumes. Workers expose `/healthz` on :8081.

## CSV import flow

Two-step POST: `?step=preview` parses the upload and re-renders the form with a preview table (rows held in the process-local `importCache` under a token); `?step=execute` takes the cached rows and bulk-inserts in one transaction. Duplicate fronts within a deck are skipped via `ON CONFLICT (deck_id, front) DO NOTHING`.

## Configuration

| Env var          | Effect |
|------------------|--------|
| `DATABASE_URL`   | Postgres DSN. Required in production; the dev build (`-tags dev`) falls back to embedded Postgres in `./.pgdata` |
| `NATS_URL`       | Enables the JetStream queue + broker; required for `memolang worker` |
| `JOBS_ACK_WAIT`  | Redelivery delay for a job whose worker stopped heartbeating (default 1m) |
| `SECURE_COOKIES` | `1` sets the Secure flag on cookies (TLS deployments) |
| `SMTP_HOST`      | Enables sending mail over SMTP (`internal/mail`); unset = mail is written to the log (dev). `task dev:mail` points it at a local Mailpit |
| `SMTP_PORT`      | Default 587 |
| `SMTP_USER`, `SMTP_PASSWORD` | SMTP login (password from the cluster Secret); no user = no auth (Mailpit) |
| `SMTP_TLS`       | `starttls` (default, required — no clear-text fallback), `tls` (implicit, port 465) or `none` (local catcher only) |
| `TRUSTED_PROXIES` | Comma-separated IPs/CIDRs of the ingress, allowed to set `X-Forwarded-For`; unset = trust none (the socket address is the client IP). Per-IP rate limits depend on it |
| `AUTH_IP_LIMITS` | `off` disables per-IP auth rate limits — test suites only, logs a warning. Per-email limits always apply |
| `MAIL_FROM`      | Sender, e.g. `MemoLang <no-reply@mail.example.com>`; required with `SMTP_HOST` |

## Deployment

`Dockerfile` — two-stage build, `golang:1.27-alpine` builder → `alpine:3.21` runtime. `.github/workflows/docker.yml` builds and pushes `ghcr.io/tj-dorje/memolang` on every push to `main`. The same image runs as web (`memolang`) and worker (`memolang worker`) against Postgres and NATS JetStream.

## Known gaps (as-is, not a todo list)

- No CSRF tokens — `SameSite=Lax` only
- No MFA yet (ISS-019 phase 6)
- Expired `user_sessions` rows are never swept
- OAuth not implemented — `users` is decoupled from how an account authenticated, so it stays additive
- `importCache` is process-local: a CSV preview and its execute must hit the same replica
