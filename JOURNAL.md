# Project Journal

## 2026-06-14
- Added "Generate using AI" button to the deck creation process.
- Implemented an intermediate "AI Configuration" form that allows users to define parameters like target language and quantity.
- Integrated Ollama as a backend for LLM generation, allowing local usage via even-less logic.
- Built a robust multi-step flow: 1) Input selection -> 2) Transition with CSS animations/spinners during processing -> 3. auto-generation of deck cards -> 4. Success state.
- Added dynamic system prompt injection to ensure the LLM uses the selected target language in its response format.
- Implemented logic to preserve user input and display specific error messages when AI generation fails, enabling easy retry functionality without losing context.

## 2026-09-15
- Implemented M9 (auth + user sessions, T-050..T-060): `users` and `user_sessions` tables, bcrypt password login, DB-backed session cookie, and per-user scoping of decks, cards and settings. App is multi-user now; decks are private per owner.
- Ownership is checked once per request at whatever entry point first reads an id, then `deck_id` is trusted downstream. `GetDeckByID`/`GetCardByID` fold "not yours" into `sql.ErrNoRows` so handlers 404 without leaking whether the id exists — no separate 403 path.
- Reviewing the plan against the code first turned up three things worth the delay: `SubmitAnswer` discarded its deck param and `EndSessionEarly` never had one, so both accepted a body-supplied `session_id` for anyone's session (fixed with `GetSessionByID(db, deckID, id)`); `SetSetting`'s `ON CONFLICT(key)` would have failed at runtime, not compile time, under the new composite PK; and SQLite's `UNIQUE` on TEXT is case-sensitive, so `email` needed `COLLATE NOCASE` plus lowercasing on write.
- Scoped `UPDATE`/`DELETE` now check `RowsAffected` — a zero-row write returns nil error in `database/sql`, which would have flashed "Saved." for a deck the user doesn't own.
- `schema.sql` is `CREATE TABLE IF NOT EXISTS` only and cannot add `decks.user_id` to an existing DB, so `db.Open` checks `PRAGMA table_info(decks)` *before* applying the schema and refuses to start with "run `task db:reset`". Without the pre-check the failure surfaced later as `no such column: user_id` from an index creation.
- E2E: one shared logged-in browser context created in `TestMain` keeps all 24 pre-auth cases unchanged; new `cases/auth.go` covers register/logout/wrong-password/case-insensitive login/protected-route redirect plus a two-user deck privacy check. Two selector collisions had to be resolved — the nav logout button matched page-wide `button[type=submit]` (dropped the explicit attribute, submit is the in-form default) and `querySelector('form')` started resolving to the nav form (scoped to `main form`).
- CSRF is still just `SameSite=Lax`; login has no rate limiting and registration is open. Deliberate for this stage, listed in the architecture doc's known gaps.
