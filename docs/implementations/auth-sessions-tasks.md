# M9 — Auth & User Sessions: Implementation Tasks

Breaks down `docs/plans/auth-sessions-plan.md` into sequenced, independently-executable
tasks, in the same style as `docs/plans/PLAN.md` / `REFINEMENT_PLAN.md` (file, what,
done-when). Read the plan doc first for the *why* behind each decision — this doc is
just the *what*, in task-sized pieces.

Continues the project's task numbering from `REFINEMENT_PLAN.md` (last task there:
T-045).

---

## T-050: Schema — users, user_sessions, decks.user_id, settings composite key

**File:** `internal/db/schema.sql`

**What:**
```sql
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
```

- Add `user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE` to the
  existing `CREATE TABLE decks (...)` statement.
- Change `settings` from PK `key` to composite PK `(user_id, key)`:
  ```sql
  CREATE TABLE IF NOT EXISTS settings (
      user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      key        TEXT NOT NULL,
      value      TEXT NOT NULL,
      updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      PRIMARY KEY (user_id, key)
  );
  ```

**Migration note:** this only affects fresh DBs (`CREATE TABLE IF NOT EXISTS` is a
no-op against an existing `decks`/`settings` table with the old shape). Run
`task db:reset` on local/dev DBs after this lands. Do not attempt an `ALTER TABLE`
migration path as part of this task — flagged as a known gap in the plan doc, not
solved here.

**Done when:** `task db:reset` then starting the server creates all 5 tables with
the new shapes (`sqlite3 memolang.db '.schema users'` /
`.schema user_sessions` / `.schema decks` / `.schema settings`).

---

## T-051: `users` model

**File:** `internal/models/user.go` (new)

```go
type User struct {
    ID           int64
    Email        string
    PasswordHash string
    CreatedAt    time.Time
}

func CreateUser(db *sql.DB, email, passwordHash string) (User, error)
func GetUserByEmail(db *sql.DB, email string) (*User, error)  // nil, nil if not found
func GetUserByID(db *sql.DB, id int64) (*User, error)         // nil, nil if not found
```

`CreateUser` must let a `UNIQUE` constraint violation on `email` surface distinguishably
(check the sqlite error, or just let the caller inspect `err.Error()` for "UNIQUE" —
match whatever pattern `internal/models/deck.go`/`card.go` already use for error
propagation, they don't wrap errors specially, so don't over-engineer this).

**Done when:** `go build ./...` passes; a unit test (`internal/models/user_test.go`,
new — first test file in this package) covers create + get-by-email + get-by-email
on a missing user returns `(nil, nil)`.

---

## T-052: Login-session model

**File:** `internal/models/auth_session.go` (new — deliberately not `session.go`,
which already owns `StudySession`)

```go
func CreateUserSession(db *sql.DB, userID int64, ttl time.Duration) (token string, err error)
func GetUserByToken(db *sql.DB, token string) (*User, error) // join user_sessions->users, WHERE expires_at > datetime('now'); nil,nil if missing/expired
func DeleteUserSession(db *sql.DB, token string) error
```

Token generation: same style as `randomToken()` in `internal/handlers/import.go`
(`crypto/rand`, hex-encoded, 16+ bytes).

**Done when:** unit tests cover: token roundtrips to the right user; an expired
session (`expires_at` in the past) returns `(nil, nil)` from `GetUserByToken`, not
an error; `DeleteUserSession` makes a subsequent lookup return `(nil, nil)`.

---

## T-053: Auth middleware

**File:** `internal/middleware/auth.go` (new package)

```go
package middleware

func RequireAuth(db *sql.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        token, err := c.Cookie("session")
        if err != nil {
            c.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(c.Request.URL.Path))
            c.Abort()
            return
        }
        user, err := models.GetUserByToken(db, token)
        if err != nil || user == nil {
            c.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(c.Request.URL.Path))
            c.Abort()
            return
        }
        c.Set("user", user)
        c.Set("userID", user.ID)
        c.Next()
    }
}
```

**Done when:** `go build ./...` passes (middleware not yet wired into routes —
that's T-055).

---

## T-054: Register / Login / Logout handlers + templates

**Files:** `internal/handlers/auth.go` (new), `templates/login.html` (new),
`templates/register.html` (new), `go.mod` (bcrypt becomes a direct import)

**Handlers** (follow `CreateDeck`'s re-render-on-error pattern):

- `RegisterForm` (GET `/register`): render `register.html`.
- `Register` (POST `/register`): validate email non-empty + contains `@`,
  password length >= 8. `bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)`.
  `models.CreateUser`; on unique-email conflict, re-render with
  `Error: "An account with that email already exists."` (status 200). On success:
  create a session (`models.CreateUserSession`, TTL 30 days), set cookie (see
  cookie spec below), redirect to `/`.
- `LoginForm` (GET `/login`): render `login.html`, preserving `?next=` in a hidden
  field so `Login` can redirect back after success.
- `Login` (POST `/login`): `GetUserByEmail`, `bcrypt.CompareHashAndPassword`.
  Generic error on either failure: `"Invalid email or password."` — don't
  distinguish "no such user" from "wrong password". On success: create session,
  set cookie, redirect to `c.PostForm("next")` if non-empty and starts with `/`
  (avoid open-redirect), else `/`.
- `Logout` (POST `/logout`): `models.DeleteUserSession(token)`, clear cookie
  (`Max-Age: -1`), redirect `/login`.

**Cookie spec** (used by `Register`/`Login`, and cleared by `Logout`):
name `session`, `httpOnly=true`, `secure=false` (matches the existing flash-cookie
convention), call `c.SetSameSite(http.SameSiteLaxMode)` before `c.SetCookie` —
this is the project's CSRF mitigation for now (see plan doc §6).

**Templates:** same `_header`/`_footer` + plain POST form pattern as
`templates/deck_form.html`. `register.html` fields: email, password, confirm
password (client never needs JS — just re-render with an error if they don't
match, don't bother with `<input pattern>` tricks). `login.html` fields: email,
password, hidden `next`.

**Done when:** `go build ./...` passes; manually registering, logging out, and
logging back in works against a fresh DB (routes aren't wired yet — do this by
temporarily calling the handlers directly in a throwaway test, or just wait for
T-055 and test both together).

---

## T-055: Wire routing, nav, and the `currentUserID`/`PageData.User` plumbing

**Files:** `internal/app/app.go`, `internal/handlers/handlers.go`,
`templates/layout.html`

**Routing:** split `app.go` into public routes (`/login`, `/register`, `/logout`,
`/static`) and a protected group wrapping every existing route:
```go
protected := r.Group("/")
protected.Use(middleware.RequireAuth(database))
{
    protected.GET("/", h.Dashboard)
    // ... every other existing route, unchanged handler wiring
}
```

**`internal/handlers/handlers.go`:**
- Add `User *models.User` field to `PageData`.
- Add helper next to `getInt64`:
  ```go
  func currentUserID(c *gin.Context) int64 {
      v, _ := c.Get("userID")
      id, _ := v.(int64)
      return id
  }
  ```
- Modify `h.render()` to pull `c.Get("user")` and populate `pd.User` automatically
  if the caller didn't already set it — so individual handlers don't need a new
  field just to make the nav work.

**`templates/layout.html`:** in the nav, `{{if .User}}` show `.User.Email` +
a `[Logout]` POST-form button; `{{else}}` show `[Login]` / `[Register]` links.

**Done when:** hitting any existing route (e.g. `/`) while logged out redirects to
`/login`; after logging in, the nav shows the user's email and a working logout
button; `task test:e2e` is expected to start failing here (that's T-059's job to fix)
— confirm the failures are all "redirected to /login", not something else.

---

## T-056: Scope decks (model + handlers)

**Files:** `internal/models/deck.go`, `internal/handlers/deck.go`

**Model signature changes:**
- `GetAllDecks(db, userID)` — add `WHERE d.user_id = ?`
- `GetDeckByID(db, userID, id)` — add `AND d.user_id = ?` (not-found and
  not-owned both fall through to `sql.ErrNoRows` — IDOR-safe for free, no
  separate 403 path needed)
- `CreateDeck(db, userID, name, mode)` — insert `user_id`
- `UpdateDeck(db, userID, id, name, mode)` / `DeleteDeck(db, userID, id)` — add
  `AND user_id = ?` to the `WHERE` clause

**Handlers:** thread `userID := currentUserID(c)` through `Dashboard`,
`NewDeckForm`, `CreateDeck`, `CreateDeckAI`, `AIExecute`, `EditDeckForm`,
`UpdateDeck`, `DeleteDeck`, `DeckDetail` — one new line + pass-through per
handler, no behavior change beyond the added scoping.

**Done when:** two different logged-in users each create a deck; neither can see,
edit, or delete the other's deck (verify via curl/manual test: hit
`/decks/{other-user's-id}` while logged in as the first user, expect 404).

---

## T-057: Scope cards (model + handlers)

**Files:** `internal/models/card.go`, `internal/handlers/card.go`,
`internal/handlers/import.go`

**Model:** `GetCardByID(db, userID, id)` — join `cards` → `decks`, filter
`decks.user_id = ?`. This is the only card-level entry point reached by a raw ID
from a URL param (`EditCardForm`, `UpdateCard`, `DeleteCard`); everything else
(`GetCardsByDeck`, `GetDueCardIDs`, `GetNewCardIDs`, `GetRandomBackValues`,
`CreateCard`) is always called with a `deckID` that was already proven owned
earlier in the same request (via T-056's `GetDeckByID`) — leave those signatures
unchanged.

**Handlers:** thread `userID` through `EditCardForm`, `UpdateCard`, `DeleteCard`
(card.go) and `ImportForm`, `ImportSubmit` (import.go — these load the deck via
the now-scoped `GetDeckByID`, no card-level change needed beyond that).

**Done when:** a card ID belonging to user A's deck returns 404 when
`EditCardForm`/`UpdateCard`/`DeleteCard` is hit as user B.

---

## T-058: Scope settings (model + ai.LoadConfig + handlers)

**Files:** `internal/models/settings.go`, `internal/ai/ai.go`,
`internal/handlers/settings.go`

**Model:** add `userID int64` as the first param to `GetSetting`, `SetSetting`,
`GetSettings`, `DeleteSetting`; filter/insert on it (matches T-050's composite PK).

**`ai.LoadConfig(db, userID)`** — passes through to `GetSettings(db, userID, "llm.")`.

**Handlers:** thread `userID := currentUserID(c)` through `SettingsPage`,
`SaveSettings`, `TestLLMConnection`, and update the `ai.LoadConfig` call site in
`AIExecute` (deck.go) to pass `userID` too.

**Done when:** two logged-in users each save different LLM settings; each only
ever sees their own saved config on `/settings` and their own config is what
`AIExecute`/`TestLLMConnection` uses.

---

## T-059: e2e test fallout — auth helper + new auth cases

**Files:** `test/e2e/helpers/auth.go` (new), `test/e2e/main_test.go`,
`test/e2e/cases/auth.go` (new), `test/e2e/configuration/environment.go`
(if a shared test-user constant is needed)

**What:**
1. In `TestMain` (or a helper it calls once), register one fixed test user
   (direct `models.CreateUser` call with a pre-hashed password, or a real HTTP
   POST to `/register` — either is fine, prefer whichever is less code) and set
   the resulting session cookie on the shared Playwright browser context so
   every existing case in `cases/deck.go`, `import.go`, `session.go`,
   `settings.go` is already "logged in" and needs no per-case changes.
2. New `test/e2e/cases/auth.go`, registered in `main_test.go`'s `TestE2E` per
   the existing "New case funcs MUST be registered" convention:
   - `Register` → lands on dashboard, logged in
   - `Logout` → redirected to `/login`
   - `LoginWrongPassword` → inline error, still on `/login`
   - `ProtectedRouteWithoutSession` → hitting `/` with no cookie redirects to
     `/login`

**Done when:** `task test:e2e` passes (24+ existing cases still green under the
shared logged-in context, plus the new auth cases).

---

## T-060: Docs cleanup

**Files:** `CLAUDE.md`, `docs/architecture/architecture.md`

**What:** update `CLAUDE.md`'s schema description (currently says "3 tables";
will be 6: decks, cards, study_sessions, session_answers, settings, users,
user_sessions — recount). Update `docs/architecture/architecture.md`'s data-model
section and "Known gaps" list (remove the "no auth" bullet, note the new tables).

**Done when:** both docs match the shipped schema — no more, no less than what
`schema.sql` actually defines.

---

## Build order

```
T-050 (schema)
  → T-051, T-052 (parallel — both new model files, no interdependency)
    → T-053 (middleware, depends on T-052's GetUserByToken)
      → T-054 (handlers, depends on T-051/T-052)
        → T-055 (wiring, depends on T-053/T-054)
          → T-056 → T-057 (cards depend on decks being scoped first)
          → T-058 (independent of T-056/T-057, can run in parallel with them)
            → T-059 (needs all handler changes done first)
              → T-060 (last, docs only)
```

T-056/T-057/T-058 touch disjoint files (deck.go+card handlers vs card.go vs
settings.go) except for the shared `currentUserID` helper from T-055 — safe to
parallelize across two agents/sessions once T-055 lands.
