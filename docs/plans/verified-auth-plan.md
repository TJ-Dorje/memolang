# Verified accounts, password reset and MFA (ISS-019)

## Context

Today `POST /register` creates an account and logs it in at once; the only email check is "contains `@`". There is no password reset, no second factor, no rate limit, and session tokens are stored as-is. That is fine for testing, not for a public deployment — it is also the abuse path for ISS-017's shared-key cost.

Decided in `ISSUES_TODO.md` → "ISS-019 detail" (option A, in-app; grill session 2026-10-26), then refined from the user's side on 2026-11-17. The refinements are the ones marked **(UX pass)** below.

Tasks: `docs/implementations/verified-auth-tasks.md`.

## What the user sees

### Sign up

1. Register form: email, password, confirm.
2. Always the same reply: "We sent a 6-digit code to **anna@example.com**. Wrong address? Start over." — the address is shown so a typo is visible.
3. The user types the code on that page → logged in, on the dashboard.

- **(UX pass) Codes, not links.** Every email the app sends carries a 6-digit code, never a link. A code works across devices (register on the laptop, read mail on the phone), mail scanners that pre-open links (Outlook Safe Links) can't consume it, and the app needs no public base URL — so no `Host`-header link poisoning either.
- If the address already has a **verified** account, the owner gets "someone tried to register with your address — if it was you, log in or reset your password" instead of a code. The page shown is identical, so the form doesn't reveal which emails exist.
- **(UX pass) An unverified account can't squat an address.** Registering again with an address whose account is still unverified replaces that account's password and sends a fresh code — nothing is lost, since nobody proved owning it. Unverified accounts are deleted after 7 days.

### Log in

- Correct password, verified → logged in (or the MFA step, below).
- Correct password, **unverified** → a fresh code is sent and the code page shown. The "verify" state is revealed only after the right password.
- Wrong password → "Invalid email or password." for every failure.
- **(UX pass) Lockouts can't be used against you.** Failures count per (email, IP) — 5 per 15 minutes — plus a looser per-email cap (20 per 15 minutes) and a per-IP cap (50 per 15 minutes). The lockout message is "Too many attempts. Try again in 15 minutes, or reset your password." Password reset stays available during a lockout.

### Forgot password

1. `/forgot`: email → always "If that account exists, we sent a code."
2. Code + new password (one form) → all sessions ended → **logged in** (or the MFA step). A reset proves the address, so it also marks an unverified account verified.
3. A notification mail: "Your password was changed. Not you? Reset it now."

### Change email (Profile → Security)

- Requires the current password. A code goes to the **new** address; the old one gets "your email is being changed to X — not you? reset your password".
- Until confirmed, login uses the old address; the Security page shows "Pending change to X" with **Cancel**.

### Two-factor (optional, Profile → Security)

1. **Turn on:** QR code (server-rendered PNG, no JS) plus the secret as text → type one code to confirm → MFA is on, and 10 recovery codes are shown once, with **Download .txt** and an "I've saved them" button.
   - *Changed while building:* MFA switches on at the confirming code, not after an "I've saved them" checkbox. Gating on the checkbox would mean keeping the plain-text codes on the server until it is ticked; and someone who doesn't save them still has the authenticator and can generate new codes, so the gate added storage without adding safety.
2. **Login with MFA:** password → "Enter the code from your authenticator app" (or "Use a recovery code instead") → logged in. Sessions last 30 days, so this comes up about monthly per device.
3. **Security page:** recovery codes left, **Generate new codes**, **Turn off** (needs password + a current code).
4. **Lost phone and codes:** the operator runs `memolang user reset-mfa <email>`. There is deliberately no email fallback — it would make MFA as weak as the inbox.

### Security notification mails

Password changed, email changed (to the old address), MFA turned on/off, recovery codes regenerated.

## Mechanism

### Codes (`email_codes`)

- One live code per user per purpose (`verify`, `reset`, `change_email`): `UNIQUE (user_id, purpose)`, so resending replaces it.
- 6 random digits (`crypto/rand`), 15-minute expiry, at most 5 wrong tries — then the row is deleted and the user asks for a new one.
- Stored as SHA-256. Honest limit: a 6-digit code has only 10⁶ values, so the hash does not protect a code read from a leaked database; the short expiry and the try cap do. What the hash does prevent is a code appearing in logs, query dumps or backups as usable text.
- Checking is one statement that deletes on success (`DELETE … RETURNING`), so a code can't be used twice even by two requests at once. A wrong guess increments `attempts`.

### Session tokens hashed at rest

- The cookie carries the raw 32-byte token; `user_sessions.token_hash` holds `hex(sha256(token))`. A leaked database or backup can't be replayed as live logins. Unlike codes, the token has 256 bits, so SHA-256 alone is enough (no bcrypt: it's not a password, and it's checked on every request).
- Migration 0002 hashes the existing rows in SQL (`sha256()` is built into Postgres), so nobody is logged out.
- The same hashing applies to `pending_logins` (the "password OK, waiting for MFA" step).

### Rate limits (`auth_attempts`)

- One row per counted event: `(kind, key, created_at)`; a check is `COUNT(*)` over the window. In Postgres, so limits hold across several web pods.
- Kinds: login failures per `email+ip`, per `email`, per `ip`; mails sent per `email` (5/hour — every new code resets its 5 tries, so this caps guessing at 25 tries an hour) and per `ip` (30/hour); code tries are capped on the code row itself.
- Behind the ingress, the client IP comes from `X-Forwarded-For`, trusted only from the proxies in `TRUSTED_PROXIES` (gin `SetTrustedProxies`). Unset = trust none, use the socket address.
- Old rows (>24 h) and expired unverified accounts (>7 days) are deleted by a periodic sweep in the web process.
- Why Postgres, not in memory (`x/time/rate`): in-memory counts are per pod (N pods allow N× the tries) and reset on every deploy. Writes happen only on failures and sent mails, so the cost is one indexed `COUNT` per check. An in-memory per-IP flood guard in front is possible later; it isn't needed at this scale.
- `AUTH_IP_LIMITS=off` disables only the per-IP limits, for the test suites (every case connects from one address); startup logs a warning. Per-email limits always apply.

### Mail (`internal/mail`)

- `Mailer` interface; `SMTPMailer` (stdlib `net/smtp`, STARTTLS) and `LogMailer` (dev default: the mail goes to the log). Tests use an in-memory fake.
- Env: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD` (cluster Secret), `MAIL_FROM`. No `SMTP_HOST` → `LogMailer`, with a startup warning outside dev.
- Sent synchronously with a 10 s timeout; a failure is logged and the page offers "resend". Plain-text mail only.
- **No domain yet:** dev uses `LogMailer`, manual and integration runs can point `SMTP_HOST` at **Mailpit** (a local catcher with a web inbox). Production needs a domain with SPF/DKIM/DMARC and a transactional provider (Postmark or Resend) — config only.

### MFA

- TOTP via `github.com/pquerna/otp` (RFC 6238, 30 s steps, ±1 step skew). `users.totp_last_step` rejects replaying a code within its window.
- Secrets are stored as-is, like LLM API keys today; encrypting secrets at rest is a separate issue for both.
- Recovery codes: 10 × 10 random characters, stored as SHA-256 (enough entropy, unlike 6 digits), each single-use.
- Half-login: after the right password, a `pending_logins` row (hashed token, 5-minute TTL, 5 tries) and a short-lived cookie; `/login/mfa` turns it into a real session.

## Schema

`0002_verified_email.sql` — `users.email_verified_at` (existing accounts backfilled as verified), `user_sessions.token` → hashed `token_hash`, `email_codes` (its `new_email` column carries an email change in flight, so "Pending change to X" is the live `change_email` row), `auth_attempts`.

`0003_mfa.sql` — `users.totp_secret`, `totp_enabled_at`, `totp_last_step`, `recovery_codes`, `pending_logins`.

## Phases

1. Migration 0002 + session-token hashing.
2. `internal/mail` + config.
3. Verified registration and login.
4. Password reset, email change, notification mails.
5. Rate limits, trusted proxies, sweeper.
6. TOTP MFA (migration 0003), recovery codes, `reset-mfa` command.

Each phase lands with its tests and doc updates; e2e cases read codes from the test mailer.

**Status (2026-11-18): all six phases built** on branch `feat/verified-auth`. Before opening registration publicly: a domain with SPF/DKIM/DMARC, an SMTP provider (`SMTP_*`), `TRUSTED_PROXIES` set to the ingress CIDR, and ISS-017.

## Not now

Session list with devices (later), passkeys (need JS), social login, encrypted secrets at rest, breached-password check.
