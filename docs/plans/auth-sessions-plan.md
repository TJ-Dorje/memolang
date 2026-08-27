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
    email         TEXT NOT NULL UNIQUE,
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

Named `user_sessions`, not `sessions` — avoids clashing with the existing `study_sessions` concept in both naming and in code (`models.StudySession` already owns "Session").

`decks` gains an owner column in its `CREATE TABLE` definition:
```sql
user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
```

**Migration gap (call this out explicitly):** `schema.sql` only runs `CREATE TABLE IF NOT EXISTS` — fine for brand-new DBs, but existing `memolang.db` files won't get the new `decks.user_id` column automatically (SQLite `ALTER TABLE ADD COLUMN` isn't idempotent the same way). Since there's no real user data yet, the plan is: update `CREATE TABLE decks` directly in schema.sql, and reset local/dev DBs (`task db:reset`) after this lands. If the GHCR-deployed instance has decks worth keeping, that needs a one-off manual `ALTER TABLE decks ADD COLUMN user_id ...` + backfill before upgrading — flagging this now so it isn't a surprise at rollout, not solving it here.

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

**`internal/models/auth_session.go`** (new — named to avoid clashing with `session.go`'s `StudySession`):
```go
func CreateUserSession(db *sql.DB, userID int64, ttl time.Duration) (token string, err error) // crypto/rand token, same style as import.go's randomToken()
func GetUserByToken(db *sql.DB, token string) (*User, error)  // joins user_sessions -> users, checks expires_at > now; nil,nil if missing/expired
func DeleteUserSession(db *sql.DB, token string) error         // logout
```

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
- `models.UpdateDeck(db, userID, id, name, mode)` / `models.DeleteDeck(db, userID, id)` — add `AND user_id = ?` to the `WHERE` clause directly
- `models.GetCardByID(db, userID, id)` — join `cards` → `decks`, filter `decks.user_id = ?` (this is the one card-level entry point reached by raw ID: `EditCardForm`, `UpdateCard`, `DeleteCard`)
- Everything already scoped by an already-verified `deckID` stays unchanged: `GetCardsByDeck`, `GetDueCardIDs`, `GetNewCardIDs`, `GetRandomBackValues`, `CreateCard`, `CreateSession`, `GetActiveSession`, `GetLastEndedSession`, `RecordAnswer`, `GetSessionAnswers`
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
- `LoginForm` (GET) / `Login` (POST): look up by email, `bcrypt.CompareHashAndPassword`; generic "Invalid email or password" error on either failure (don't leak which one) — re-render `login.html` with `SettingsData`-style `Error` field on failure.
- `Logout` (POST): `DeleteUserSession`, clear cookie, redirect `/login`.

Session cookie: `httpOnly=true`, `SameSite=Lax` (call `c.SetSameSite(http.SameSiteLaxMode)` before `c.SetCookie`) — this is the lightweight CSRF mitigation for this stage: Lax blocks the cookie on cross-site POSTs, which covers every mutating route in the app (all mutations are already POST per CLAUDE.md convention) without building a CSRF-token system. `secure=false` to match the existing flash-cookie convention (app likely sits behind a reverse proxy that terminates TLS); revisit if/when the app is exposed directly.

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
2. `go test ./internal/...` (add unit tests for `bcrypt` roundtrip and `GetUserByToken` expiry behavior alongside existing patterns)
3. `task test:e2e` — full suite including new `auth.go` cases
4. Manual: register a user, confirm decks created by that user aren't visible after logging in as a second user (the core IDOR check)
5. `task db:reset` before first run post-migration (see migration-gap note above)

## Out of scope for this pass (explicitly deferred)

- WorkOS/OAuth integration itself — this plan only makes it a clean additive change later
- Full CSRF-token middleware — `SameSite=Lax` is judged sufficient for now given prototype stage
- Session expiry cleanup job (expired rows just sit inert; `GetUserByToken` already ignores them)
- Password reset / email verification flows
- Rate-limiting login attempts
