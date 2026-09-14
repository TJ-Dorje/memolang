# Auth + User Sessions for MemoLang

## Context

MemoLang is currently a single-tenant prototype: every deck/card/study-session is global, visible and editable by anyone who can reach the server. The user wants to turn it into a real multi-user product, starting simple:

- **User model:** private collections per user (like Anki) — each user's decks are invisible to others. Smallest schema delta: cards/study_sessions/session_answers already cascade through `deck_id`, so only `decks` needs a new owner column.
- **Auth mechanism now:** username/password + DB-backed session (bcrypt + session table + httpOnly cookie), matching the project's existing "session state lives in the DB" convention (`study_sessions`).
- **Auth mechanism later:** the user wants to add WorkOS/OAuth down the line. The design below keeps `users` decoupled from *how* they authenticated, so adding WorkOS later is additive (new login route + find-or-create-by-email), not a rewrite.
- **Registration:** open self-registration (`/register`), no invite gate.

This plan covers schema, new auth package, the systemic "scope every deck/card query by owner" change across existing handlers/models, and e2e test fallout.

---

## 1. Schema changes (`internal/db/schema.sql`)

Two new tables, plus one new column on an existing table:

```sql
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL DEFAULT '',  -- empty allowed: future OAuth-only accounts
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token      TEXT PRIMARY KEY,             -- random hex, not autoincrement
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
```

`email` is `COLLATE NOCASE`: SQLite's default `UNIQUE` on `TEXT` is case-sensitive, so without
it `Foo@x.com` and `foo@x.com` become two accounts and the user gets a confusing "invalid email
or password" for whichever case they didn't register with. The model layer also lowercases on
write (§2) so both layers agree.

Named `user_sessions`, not `sessions` — avoids clashing with the existing `study_sessions` concept in both naming and in code (`models.StudySession` already owns "Session").

`decks` gains an owner column in its `CREATE TABLE` definition:
```sql
user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
```

**Migration gap (call this out explicitly):** `schema.sql` only runs `CREATE TABLE IF NOT EXISTS` — fine for brand-new DBs, but existing `memolang.db` files won't get the new `decks.user_id` column automatically (SQLite `ALTER TABLE ADD COLUMN` isn't idempotent the same way). Since there's no real user data yet, the plan is: update `CREATE TABLE decks` directly in schema.sql, and reset local/dev DBs (`task db:reset`) after this lands. If the GHCR-deployed instance has decks worth keeping, that needs a one-off manual `ALTER TABLE decks ADD COLUMN user_id ...` + backfill before upgrading — flagging this now so it isn't a surprise at rollout, not solving it here.

**Startup guard (do build this):** because the `IF NOT EXISTS` no-op is silent, an un-reset DB
doesn't fail at startup — it fails later as confusing `no such column: user_id` errors on the
first dashboard load. `internal/db/db.go` should check `PRAGMA table_info(decks)` for `user_id`
after applying the schema and `log.Fatal` with "schema out of date — run `task db:reset`" if it
is missing. Cheap, and turns a mystery runtime error into a one-line instruction.

`settings` also becomes per-user (LLM API keys are personal data): change its primary key from `key` alone to a composite `(user_id, key)`:
```sql
CREATE TABLE IF NOT EXISTS settings (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);
```

Note for §5: `SetSetting` currently upserts with `ON CONFLICT(key) DO UPDATE`
(`internal/models/settings.go:23`). Under the composite PK that conflict target no longer
matches an index and SQLite errors **at runtime** — `go build` won't catch it. It must become
`ON CONFLICT(user_id, key) DO UPDATE ...`, with `user_id` added to the INSERT column list.

---

## 2. New model files

**`internal/models/user.go`** (new):
```go
type User struct {
    ID           int64
    Email        string
    PasswordHash string
    CreatedAt    time.Time
}
func CreateUser(db *sql.DB, email, passwordHash string) (User, error)   // UNIQUE violation -> distinguishable error for "email taken"
func GetUserByEmail(db *sql.DB, email string) (*User, error)            // nil, nil if not found
func GetUserByID(db *sql.DB, id int64) (*User, error)
```

`CreateUser` and `GetUserByEmail` both `strings.ToLower(strings.TrimSpace(email))` before
touching the DB, matching the `COLLATE NOCASE` constraint from §1.

**`internal/models/auth_session.go`** (new — named to avoid clashing with `session.go`'s `StudySession`):
```go
func CreateUserSession(db *sql.DB, userID int64, ttl time.Duration) (token string, err error) // crypto/rand token
func GetUserByToken(db *sql.DB, token string) (*User, error)  // joins user_sessions -> users, checks expires_at > now; nil,nil if missing/expired
func DeleteUserSession(db *sql.DB, token string) error         // logout
```

Token generation: 32 bytes from `crypto/rand`, hex-encoded. The project has two existing
token helpers — `randomToken()` (`internal/handlers/import.go:217`, 12 bytes) and
`storePending()` (`internal/handlers/handlers.go:30`, 16 bytes) — but both are for short-lived
in-memory cache keys and both **ignore `rand.Read`'s error**. Don't copy either verbatim: a
30-day session token gets 32 bytes and a checked error (a silent `rand.Read` failure would
hand out an all-zero token).

---

## 3. Middleware (`internal/middleware/auth.go`, new package)

```go
func RequireAuth(db *sql.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        token, err := c.Cookie("session")
        if err != nil { redirectToLogin(c); return }
        user, err := models.GetUserByToken(db, token)
        if err != nil || user == nil { redirectToLogin(c); return }
        c.Set("user", user)
        c.Set("userID", user.ID)
        c.Next()
    }
}
```

Redirect target: `/login?next=<original path>` so login can bounce back.

---

## 4. Routing (`internal/app/app.go`)

Split into public and protected groups:

```go
r.GET("/login", h.LoginForm)
r.POST("/login", h.Login)
r.GET("/register", h.RegisterForm)
r.POST("/register", h.Register)
r.POST("/logout", h.Logout)   // protected in practice (needs a valid session to matter) but harmless if not

protected := r.Group("/")
protected.Use(middleware.RequireAuth(database))
{
    // every existing route (Dashboard, decks/*, cards/*, settings/*) moves here unchanged
}
```

`/static` stays outside the group.

---

## 5. Handler + model signature changes (the systemic part)

Ownership is checked **once, at the entry point of each request** — wherever a deck or card is first loaded from a URL param — not re-threaded through every downstream query. Once a deck is proven owned, its `deck_id` is trusted for the rest of that request (cards/sessions/answers all cascade through it already).

- `models.GetAllDecks(db, userID)` — add `WHERE d.user_id = ?`
- `models.GetDeckByID(db, userID, id)` — add `AND d.user_id = ?`; not-owned or not-found both become `sql.ErrNoRows` → handlers already 404 on that error, so this is IDOR-safe for free (no existence leak, no separate 403 path needed)
- `models.CreateDeck(db, userID, name, mode)` — insert owner at creation
- `models.UpdateDeck(db, userID, id, name, mode)` / `models.DeleteDeck(db, userID, id)` — add `AND user_id = ?` to the `WHERE` clause directly, **and check `RowsAffected()`**: an `UPDATE`/`DELETE` matching zero rows returns a nil error, so a foreign deck ID would otherwise flash "Saved."/"Deck deleted." while changing nothing. Return `sql.ErrNoRows` on zero rows so handlers 404, consistent with `GetDeckByID`
- `models.GetSessionByID(db, deckID, id)` — **new**, see the study-session note below
- `models.GetCardByID(db, userID, id)` — join `cards` → `decks`, filter `decks.user_id = ?` (this is the one card-level entry point reached by raw ID: `EditCardForm`, `UpdateCard`, `DeleteCard`)
- Everything already scoped by an already-verified `deckID` stays unchanged: `GetCardsByDeck`, `GetDueCardIDs`, `GetNewCardIDs`, `GetRandomBackValues`, `CreateCard`, `CreateSession`, `GetActiveSession`, `GetLastEndedSession`, `GetSessionAnswers`

**Exception — the study-session handlers are not scoped by a verified `deckID` today.** Two
handlers in `internal/handlers/session.go` take IDs straight from the POST body and never
check them against the deck in the URL, so the "verify once at the entry point" rule does not
cover them as written:

- `SubmitAnswer` (`session.go:136`) parses the deck param and **discards it** (`_, err :=`).
  `session_id` and `card_id` come from the form. Scoping `GetCardByID` catches a foreign
  `card_id`, but `session_id` still flows unchecked into `models.RecordAnswer` and
  `models.AdvanceSession` — user B can advance and pollute user A's session counters using a
  card of their own.
- `EndSessionEarly` (`session.go:201`) reads only `session_id` from the form and calls
  `models.EndSession`. There is no deck check at all, even though the route
  (`/decks/:id/session/end`) carries the deck ID — user B can end user A's active session.

Fix, in both handlers: use the deck param, load the deck through the now-scoped
`GetDeckByID(db, userID, deckID)`, then confirm the session belongs to that deck via a new

```go
func GetSessionByID(db *sql.DB, deckID, id int64) (*StudySession, error) // WHERE id = ? AND deck_id = ?
```

in `internal/models/session.go`; 404 on nil. `SubmitAnswer` additionally asserts
`card.DeckID == deckID`, which closes the cross-deck-within-one-user case for free. Ownership
is still checked once per request — this just adds the session handlers to the set of entry
points where that check actually happens.
- `models.GetSetting/SetSetting/GetSettings/DeleteSetting` — add `userID` as first param, filter/insert on it
- `ai.LoadConfig(db, userID)` — passes through to `GetSettings(db, userID, "llm.")`

Handlers affected (mechanical — thread `userID := currentUserID(c)` through, one new line per handler): `Dashboard`, `NewDeckForm`, `CreateDeck`, `CreateDeckAI`, `AIExecute`, `EditDeckForm`, `UpdateDeck`, `DeleteDeck`, `DeckDetail`, `ImportForm`, `ImportSubmit`, `EditCardForm`, `UpdateCard`, `DeleteCard`, `StartSession`, `SubmitAnswer`, `EndSessionEarly`, `SessionSummary`, `SettingsPage`, `SaveSettings`, `TestLLMConnection`.

Add a small helper next to `getInt64` in `handlers/handlers.go`:
```go
func currentUserID(c *gin.Context) int64 {
    v, _ := c.Get("userID")
    id, _ := v.(int64)
    return id
}
```

**Nav display without touching every handler:** add `User *models.User` to `PageData`, and have `h.render()` itself pull `c.Get("user")` and populate it — so `layout.html` can show the logged-in email/logout button without every individual handler needing a new field.

---

## 6. New handlers (`internal/handlers/auth.go`)

- `RegisterForm` (GET) / `Register` (POST): validate email format + password length, `bcrypt.GenerateFromPassword`, `CreateUser`, on unique-email conflict re-render with inline error (same pattern as `CreateDeck`'s empty-name re-render), then auto-login (create session, set cookie, redirect to `/`).
- `LoginForm` (GET) / `Login` (POST): look up by email, reject an empty `PasswordHash` before calling bcrypt (§1 allows `''` for future OAuth-only accounts; bcrypt would reject it anyway, but an explicit guard keeps that from depending on a library detail), then `bcrypt.CompareHashAndPassword`; generic "Invalid email or password" error on either failure (don't leak which one) — re-render `login.html` with `SettingsData`-style `Error` field on failure.
- `Logout` (POST): `DeleteUserSession`, clear cookie, redirect `/login`.

Session cookie: `httpOnly=true`, `SameSite=Lax` (call `c.SetSameSite(http.SameSiteLaxMode)` before `c.SetCookie`) — this is the lightweight CSRF mitigation for this stage: Lax blocks the cookie on cross-site POSTs, which covers every mutating route in the app (all mutations are already POST per CLAUDE.md convention) without building a CSRF-token system. `secure` is **not** hardcoded false: unlike the 5-second flash cookie, this one carries a 30-day
credential and the app ships as a container image (`Dockerfile`, GHCR). Read it from an env var
(`SECURE_COOKIES=1`), defaulting to false so local `task dev` over plain HTTP still works.
Set an explicit cookie `MaxAge` equal to the session TTL as well — omitting it yields a
browser-session cookie that dies on browser close while the DB row lives 30 days.

Password hashing: `golang.org/x/crypto/bcrypt` — already an *indirect* dependency (pulled transitively); importing it directly and running `task tidy` promotes it to direct, no new dependency actually added.

---

## 7. Templates

- `templates/login.html`, `templates/register.html` — new, same `_header`/`_footer` + plain POST form pattern as `deck_form.html`.
- `templates/layout.html` nav: when `.User` is set, show `{{.User.Email}}` + a `[Logout]` form (POST); otherwise show `[Login] [Register]`.

---

## 8. e2e test fallout

Every existing e2e case currently hits protected routes with no auth — they'll all start hitting the login redirect. Rather than adding a login flow to every single case:

- Add a `test/e2e/helpers/auth.go` (or extend `configuration`) that, once per `TestMain`, registers one test user directly via `models.CreateUser` (bcrypt-hash a fixed test password) or via an HTTP POST to `/register`, then sets the resulting session cookie on the shared Playwright browser context so every subsequent case is already "logged in."
- Add a small new `test/e2e/cases/auth.go` covering: register → redirected to dashboard logged in; logout → redirected to login; login with wrong password → inline error; protected route without a session → redirected to `/login`.
- Register the new cases in `main_test.go`'s `TestE2E` per the existing manual-registration convention (comment already there: "New case funcs MUST be registered...").

---

## 9. Docs

Update `CLAUDE.md`'s "Database schema (3 tables)" line — it'll be wrong after this (5-6 tables). Update `PLAN.md`/leave `REFINEMENT_PLAN.md` alone (that's closed); this probably deserves its own `M9` entry in `PLAN.md` once implemented, but that's a follow-up doc task, not part of this plan.

---

## Verification

1. `go build ./... && go vet ./...`
2. `go test ./internal/...` (add unit tests for `bcrypt` roundtrip and `GetUserByToken` expiry behavior alongside existing patterns). Include a test that `SetSetting` upserts twice for the same `(user_id, key)` without error — the `ON CONFLICT` target change is a runtime-only failure the compiler cannot catch.
3. `task test:e2e` — full suite including new `auth.go` cases
4. Manual IDOR sweep as user B against user A's data — all must 404, and A's rows must be unchanged afterwards:
   - `GET /decks/{A-deck}` and `/decks/{A-deck}/edit`
   - `GET /cards/{A-card}/edit`, `POST /cards/{A-card}/delete`
   - `POST /decks/{A-deck}/session/end` with A's `session_id`
   - `POST /decks/{A-deck}/session/answer` with A's `session_id`
   - `/settings` shows only B's own LLM config
5. Register `Foo@x.com`, then log in as `foo@x.com` — must succeed (case-insensitive email).
6. `task db:reset` before first run post-migration (see migration-gap note above); confirm the startup guard fires on a stale DB.

## Out of scope for this pass (explicitly deferred)

- WorkOS/OAuth integration itself — this plan only makes it a clean additive change later
- Full CSRF-token middleware — `SameSite=Lax` is judged sufficient for now given prototype stage
- Session expiry cleanup job (expired rows just sit inert; `GetUserByToken` already ignores them)
- Password reset / email verification flows
- Rate-limiting login attempts
