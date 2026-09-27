# MemoLang — Scaling Plan: Production Stability Upgrade (M12)

**Status:** planned, not started. Recorded 2026-09-27 as the upgrade path for
production stability; do it before the app has to survive restarts under load
or run more than one pod.

Tasks are written for autonomous coding agents: exact files, what changes, and
a verifiable "Done when". Follow `CLAUDE.md` (no ORM, plain SQL, server-rendered
templates, POST + redirect, no JavaScript — streaming is server-side HTML).

---

## Why

Today everything runs in **one pod**: the web handlers, the background LLM
goroutines (tutor replies, deck-builder replies, card generation), the
in-memory stream broker, and SQLite on a ReadWriteOnce PVC with
`strategy: Recreate`. That is simple and fine for one learner, but:

- **A deploy or crash kills in-flight LLM work.** A card generation three
  minutes into a thinking model's reply is lost when the pod restarts; the
  learner sees an interrupted page and a partial (or no) deck.
- **The broker is process memory.** A reply being generated is only
  followable from the pod generating it; after a restart the stream is gone
  (handled — replies are marked interrupted — but the work is lost).
- **There can only be one pod.** SQLite on an RWO volume cannot be opened
  safely by a second replica or a separate worker, so there is no rolling
  deploy (hence `Recreate`, with downtime) and no way to add capacity.

## What already makes this cheap

- **`stream.Broker` is an interface** (`internal/stream`). Its semantics —
  an ordered log per reply, replay from the start for late readers, a
  retention window after completion — are exactly NATS JetStream's. Nothing
  above the interface (tutor, deck builder, generation page, `renderChat`)
  knows the implementation.
- **LLM work is already decoupled from requests:** handlers start work and
  redirect; pages only *follow* output. Moving the work to another process
  changes where the goroutine runs, not the page flow.
- **The database is already the record of results:** replies are saved
  before the stream finishes, cards as each is parsed. A page that finds no
  live stream just renders from the database.

## The order matters

SQLite is the real blocker, not the broker. Adding NATS while still on one
SQLite pod buys nothing: with a single process the in-memory broker does the
same job with nothing extra to run.

### Step 1 — Shared database (prerequisite)

Move from SQLite to PostgreSQL so more than one process can use the data.

- Driver swap in `internal/db` (`database/sql` stays; plain SQL stays).
- SQL dialect audit: `INSERT OR IGNORE` → `ON CONFLICT DO NOTHING`,
  `RETURNING` (fine), `json_insert` in `AppendToSessionQueue` → `jsonb`
  operators, `datetime('now')`/`date('now')` → `now()`/`current_date`,
  partial unique indexes (supported), `COLLATE NOCASE` → `citext` or
  `lower()` unique index, `PRAGMA user_version` → a `schema_version` table in
  the migration runner.
- Migrations: a Postgres baseline equal to the current schema. No data
  copy: the SQLite data was test data only (decided 2026-09-27), so
  production starts on an empty database.
- Deployment: Postgres in the cluster (or managed), a Secret with the DSN,
  backups moved from the `sqlite3 .backup` CronJob to `pg_dump`.
- Only now can the app Deployment switch from `Recreate` to rolling updates.

**Done when:** the full unit + e2e suites pass against Postgres (e2e can run
Postgres in a container), production runs on it, and a deploy with two
replicas has no downtime.

### Step 2 — JetStream broker

`internal/stream/jetstream.go` implementing `Broker`, selected when
`NATS_URL` is set (in-memory otherwise, so local dev and tests need no NATS).

| `Broker` | JetStream |
|---|---|
| `Start(key)` | one stream `MEMOLANG_REPLIES`, subjects `replies.>`, created at startup; nothing per key |
| `Publish(key, chunk)` | `js.Publish("replies."+key, chunk)` — ordered per subject |
| `Finish(key, err)` | a final message with header `Status: done` or the error |
| `Follow(key, fn)` | ordered consumer on `replies.<key>`, deliver-all (replay, then live), stop at the final message; honours ctx |
| `Exists(key)` | `GetLastMsgForSubject("replies."+key)` |
| retention (`retainFinished`) | stream `MaxAge` ≈ 10 minutes; memory or file storage |

Run NATS with JetStream in the cluster (one node is enough for a homelab;
three for real HA).

**Done when:** the stream package tests run against both implementations
(same behaviour: late follower replays, many followers, cancel, producer
error), and with two web replicas a reply generated on one pod streams to a
page served by the other.

### Step 3 — Worker Deployment and a job queue

Move LLM calls out of the web pods.

- A work-queue stream (`WorkQueuePolicy`), subjects `jobs.reply`,
  `jobs.generate`, one message per request (conversation/message id or deck
  id + spec — never the API key; workers read the provider from the database).
- A `memolang worker` mode (same binary, different entrypoint) consuming
  jobs and running today's `assistant.generate` / `generateCards` bodies,
  publishing output to `replies.<key>`. Ack on completion; a worker that dies
  mid-job gets it redelivered (make jobs idempotent: skip cards already saved,
  don't re-add a reply that finished).
- Web handlers enqueue instead of starting goroutines; pages are unchanged.
- Separate Deployments for web and worker, scaled independently; web pods
  can roll without touching in-flight generations.

**Done when:** killing a worker mid-generation results in the job being
picked up again and the deck completing; deploying the web Deployment during
a generation does not interrupt it.

## Related

- ISS-017 (keep the assistant on topic) is independent, and must land before
  publishing regardless of this plan.
- `docs/plans/ASSISTANT_AGENT_PLAN.md` (M11) runs its agent loop in the same
  background-goroutine shape; after Step 3 it becomes another job type.
