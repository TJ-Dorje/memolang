# CLAUDE.md

Entry point for agents working in this repository. It holds the rules; the explanations live in `docs/`.

MemoLang is a multi-user, server-rendered Go flashcard app (Anki-style) on PostgreSQL, with an optional NATS JetStream queue and `memolang worker` processes for LLM jobs.

## Where to look

| Question | Read |
|---|---|
| How the system fits together: request path, auth, streaming pages, web + worker jobs, CSV import, config, deployment | `docs/architecture/architecture.md` |
| Tables, constraints, SQL conventions, migrations, test databases | `docs/architecture/data-model.md` |
| SM-2 scheduling, `Update`/`Preview`, the study buttons | `docs/design/srs.md` |
| Backlog and plans | `docs/plans/PLAN.md` (follow-on: `REFINEMENT_PLAN.md`; card-maintenance agent, M11: `ASSISTANT_AGENT_PLAN.md`; Postgres + JetStream + workers, M12: `SCALING_PLAN.md`) |
| Task breakdowns for a plan | `docs/implementations/` |
| Why a past decision was made | `JOURNAL.md` |

Read the relevant doc before changing that area. When a change alters what a doc describes, update the doc in the same change.

## Agent instructions

- **Search**: use `rg` via Bash; fall back to `grep -rn` if `rg` is not installed (`command -v rg`).
- **Commands**: prefer `task` over raw `go` invocations.
- Templates and static files load from disk at runtime — no rebuild needed when editing them.

## Commands

```bash
task dev               # run the server on :8080 (embedded Postgres in ./.pgdata unless DATABASE_URL is set)
task build             # compile binary
task test              # all tests (embedded Postgres and NATS, no Docker)
task test:srs          # SM-2 unit tests only
task test:integration  # e2e against the real image as web + worker containers on Postgres + NATS (needs Docker)
task db:reset          # wipe the embedded dev database
task tidy              # go mod tidy

go test ./internal/srs/... -run TestHardIsAPass   # single test
```

## Invariants — don't break these

- **No ORM, no JS framework.** Plain SQL in `internal/models/`; server-rendered templates. JS is progressive enhancement only — every page works without it, live output included.
- **Every mutation is a POST that redirects** (PRG), with flash via a short-lived cookie.
- **Ownership:** look up decks and cards through the owner-scoped getters, so another user's id is a 404, never a 403. An id from a request body is bound back to an owned deck before writing.
- **Model output is untrusted:** always HTML-escape it.
- **Jobs never carry an API key:** an `assistant.Job` holds ids and the prepared request only.
- **Schema changes are a new numbered migration.** Never edit a shipped one.
- **Postgres SQL:** `$1` placeholders; detect duplicates by SQLSTATE `23505` (`isUniqueViolation`), never by error text.
- **SRS:** `srs.Update` takes its intervals from `srs.Preview` — the buttons show `Preview`'s numbers.
- **Handlers render through `h.render`** with a `PageData{Title, Flash, User, Data}`.
- **DB tests:** a package touching the database has `TestMain` calling `pgtest.Main(m)`, and each test gets its own database from `testdb.New(t)`.
