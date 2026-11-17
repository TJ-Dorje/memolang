# ISS-019 — Verified accounts, reset, MFA: Implementation Tasks

Breaks down `docs/plans/verified-auth-plan.md` (read it first for the *why*). Same style as `auth-sessions-tasks.md`: file, what, done-when. Continues numbering after T-079.

---

## Phase 1 — schema and hashed session tokens

### T-080: Migration 0002

**File:** `internal/db/migrations/0002_verified_email.sql`, `internal/db/migrate.go`

**What:** `users.email_verified_at TIMESTAMPTZ` (backfill `created_at`); hash `user_sessions.token` in place and rename it `token_hash`; create `email_codes` and `auth_attempts`.

**Done when:** a migrate test shows a session created at version 1 still resolves after migrating to 2, and existing users come out verified.

### T-081: Hash session tokens in the model

**File:** `internal/models/auth_session.go`, `internal/models/user.go`

**What:** every session function takes the raw token and queries by `hashToken(token)`; `User.EmailVerifiedAt *time.Time` in `userColumns`/`scanUser`.

**Done when:** model tests show the DB holds no raw token and lookups still work.

## Phase 2 — mail

### T-082: `internal/mail`

`Mailer` interface, `SMTPMailer` (STARTTLS), `LogMailer`, a test fake; `mail.FromEnv()`. Done when: a test against an in-process SMTP listener receives the message.

### T-083: Wire the mailer

`main.go` → `app.NewRouter` → `Handler.Mail`. Done when: `task dev` logs mails, tests inject the fake.

## Phase 3 — verified registration

### T-084: `email_codes` model

`IssueCode(user, purpose, newEmail) (code, err)` (upsert), `ConsumeCode(user, purpose, code) (row, err)` with `ErrCodeInvalid`/`ErrCodeExpired`/try cap.

### T-085: Register + verify pages

Register (new / unverified-replace / verified-notify, same reply), `GET/POST /verify`, `POST /verify/resend`; login sends unverified users to `/verify`.

### T-086: e2e — register with code, cross-device not needed, typo start-over

## Phase 4 — reset, email change, notifications

### T-087: `/forgot` + `/reset`
### T-088: Email change with pending state and cancel
### T-089: Notification mails

## Phase 5 — rate limits

### T-090: `auth_attempts` model + limiter helpers
### T-091: Apply to login, register, forgot, resend; `TRUSTED_PROXIES`
### T-092: Sweeper (old attempts, expired codes, unverified accounts >7 days)

## Phase 6 — MFA

### T-093: Migration 0003 + TOTP model (enable/disable, last step)
### T-094: Setup flow with QR PNG, recovery codes + download
### T-095: `/login/mfa` half-login via `pending_logins`
### T-096: `memolang user reset-mfa <email>`
### T-097: Docs: architecture, data-model, ISS-019 closed
