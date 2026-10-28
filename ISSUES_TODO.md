# ISSUES_TODO.md

Open items are listed first; everything below them is resolved.

| # | Status | Summary |
|---|--------|---------|
| ISS-017 | 🔲 Open — **before publishing** | AI assistant must stay on topic (language learning / deck building). Today only the prompt keeps it there; a real model will happily answer programming questions. See "ISS-017 detail" below |
| ISS-018 | 🔲 Planned — production stability | Scale past one pod: PostgreSQL instead of SQLite (prerequisite), a NATS JetStream `stream.Broker`, then a worker Deployment consuming LLM jobs so deploys and crashes don't kill in-flight generations. See `docs/plans/SCALING_PLAN.md` |
| ISS-019 | 🔲 Ready to plan — **before publishing** | Registration has no email confirmation (any address, real or not, gets an account) and login is password-only. Production needs verified emails and optional MFA. See "ISS-019 detail" below |
| ISS-021 | 🔲 Ready to plan | Projects: decks live under a project ("Learn Spanish") holding language, goal, level and agent notes — the context for ISS-020 and M11. See "ISS-021 detail" below |
| ISS-020 | 🔲 Ready to plan | "Generate more cards" for a finished deck: new cards for the same topic, none repeating what the deck has; decks capped at 300 cards, ≤50 per generation, big CSV imports split. After ISS-021. See "ISS-020 detail" below |
| ISS-001 | ✅ Fixed | `card.js` now loaded in `session.html`; inline `revealCard()` removed |
| ISS-002 | ✅ Fixed | Dead `PageData`, `render`, flash helpers deleted from `main.go` |
| ISS-003 | ✅ Fixed | Empty stub files `srs.go` and `models.go` deleted |
| ISS-004 | ✅ Fixed | MC mode reachable via "Multiple Choice" button on deck detail; `?mode=mc` param wires through to session creation |
| ISS-005 | ✅ Fixed | "Due Tomorrow" stat tile added to session summary |
| ISS-006 | ✅ Fixed | `?q=` text search on deck detail with clear button |
| ISS-007 | ✅ Fixed | CSV import no longer requires re-uploading file; parsed rows cached in-memory under a random token, consumed on execute |
| ISS-008 | ✅ Fixed | "End Session" button added to active session view; POSTs to `/decks/:id/session/end`, ends session in DB. Originally redirected to deck page — later changed to redirect to Home (see ISS-014) |
| ISS-009 | ✅ Fixed | "Delete" button added to deck detail header; confirmation dialog, POSTs to existing `/decks/:id/delete` handler |
| ISS-010 | ✅ Fixed | `testdata/tibetan_basics.csv` line 5 had an unquoted multi-value tag (`noun,place`) causing a 5-field row; quoted to `"noun,place"` |
| ISS-011 | ✅ Fixed | Playwright E2E test suite added under `test/e2e/`. Router extracted to `internal/app/app.go` so tests can mount it on a random port. 14 tests covering deck CRUD, CSV import (incl. cross-deck token rejection), flashcard + MC sessions, end-session-early, summary, and the Study dropdown. Driven via `task test:e2e` (after one-time `task playwright:install`) |
| ISS-012 | ✅ Fixed | Dashboard deck cards are now fully clickable via a stretched `.deck-link` overlay; the "Browse" button was removed. Hover highlights the card border with the accent color |
| ISS-013 | ✅ Fixed | Duplicate "+ New Deck" button removed from the global nav header; only the page-header button on the dashboard remains |
| ISS-014 | ✅ Fixed | "End Session" now redirects to Home (`/`) instead of the deck detail page, matching the user's mental model of pausing study |
| ISS-015 | ✅ Fixed | Added "← Home" back link at the top of the deck detail page so users can return to the dashboard without using the logo |
| ISS-016 | ✅ Fixed | Dashboard "Study" button replaced with a native `<details>` dropdown offering "Flashcards" and "Multiple Choice"; no JS, dropdown floats above sibling deck cards via z-index when `[open]` |

## ISS-017 detail — keep the AI assistant on topic (must land before publishing)

**Problem.** The tutor and deck builder answer anything asked of them — a programming question gets solved. Off-topic use costs the learner's own key, *except* when the operator sets `LLM_API_KEY`, which overrides every user's key: then everyone's off-topic chat bills to the operator. So this is UX today and a cost/abuse issue the moment the app is public with a shared key.

**Decided approach (2026-09-27), in layers:**

1. **Scope rules in both system prompts** (`assistant.TutorPrompt`, `DeckBuilderPrompt`): only language learning / this deck; decline anything else in one sentence and steer back, with a one-line example refusal. Soft — stops drift, not a determined user.
2. **Hard input limits** in `assistant.Ask`: max question length (~500 chars, no pasted code files/essays); max ~15 learner turns per deck-builder interview.
3. **Hard output limits:** `ChatRequest.MaxTokens` ~400 for the tutor, ~250 for the builder (both use the 1024 default today).
4. **Topic gate** — the real guard. An `assistant.TopicGate` interface (`Check(question) → onTopic, probability`) called in `Ask` *before* any LLM call; off-topic questions get a fixed refusal costing zero tokens. No gate configured = allow all.
   - First adapter: **Laya** — open (Apache 2.0), self-hostable "System One" decision model: no text generation, a typed answer + calibrated probability per question (~33 ms), 100+ languages (matters: learners write in the language they're learning). Ask it e.g. *"Is this message about learning a language or building a flashcard deck?" → bool*. Enabled by `LAYA_URL`.
   - **Jev** (TypeSafe AI) is the closed, hosted equivalent (~240–280 ms) — a second adapter if self-hosting Laya doesn't work out.
   - Guard models like Llama Guard / Lakera were considered and rejected: they classify *harm* / injection, not *topic* — a programming question is perfectly "safe".
   - **Open questions before building:** Laya's model size, CPU vs GPU (the one guardrail write-up ran it on an NVIDIA L4; the homelab node is small), and its serving API. Read the model card first.
5. **Per-user rate limit** (e.g. 60 assistant messages/hour) — needed only if publishing with a shared `LLM_API_KEY`.

**Testing:** unit tests for the hard limits and the gate wiring (fake gate); a check that the prompts carry the scope rules; an opt-in live test (like `TestLiveChat`) asking a programming question and expecting a short refusal — model-dependent, for spot checks, not CI.

References: [Laya on Hugging Face](https://huggingface.co/convaiinnovations/laya) · [Jev vs Laya](https://dev.to/jamilxt/jev-vs-laya-the-same-ai-idea-one-closed-and-one-open-3c6e) · [laya-guardrails](https://github.com/javimp2003/laya-guardrails) · [gatelaya](https://github.com/Diwas2055/gatelaya)

## ISS-019 detail — verified registration and MFA (intake, 2026-10-25)

**Problem.** `POST /register` creates the account and logs it in at once; the only email check is "contains `@`". Anyone can sign up as an address they don't own, and nothing stops scripted sign-ups. Fine for testing; not for a public deployment (it is also the abuse path for ISS-017's shared-key cost). Login is email + password only — no second factor, no "forgot password" (which needs email too).

**Current state.** `internal/handlers/auth.go` (`Register`, `Login`, `startSessionCookie`), `models.CreateUser`, `user_sessions` (opaque 32-byte token, 30-day TTL, stored as-is). `users.password_hash = ''` is already reserved for OAuth-only accounts, so the schema expects a second way to log in.

**Wanted for production:**
1. **Email verification** — an account can't be used (or can't use the assistant) until the address is confirmed via an emailed, single-use, expiring link.
2. **Password reset** by email (falls out of 1: same mail + token machinery).
3. **MFA**, opt-in per user, at minimum TOTP (authenticator app) with one-time recovery codes.
4. Supporting hardening: login/register rate limiting; consider hashing session tokens at rest.

**The decision to make first — build in the app, or delegate to an identity provider:**

- **A. In-app.** Verification/reset tokens table, an SMTP sender (needs a mail provider + a secret in the cluster), TOTP via a small library (`pquerna/otp`), recovery codes, a "password ok, MFA pending" half-login step. Fits the no-JS rule: every step is a plain form. Most code to own and get right.
- **B. OIDC to a self-hosted IdP** (e.g. Authentik, Zitadel, Keycloak, Pocket ID in the homelab). The IdP does sign-up, email verification, MFA, passkeys, reset; MemoLang only does the OIDC redirect dance and keeps `users` (empty `password_hash`). Least auth code here, but one more stateful service to run and back up (it can use the CNPG operator).
- **C. Social login** (Google/GitHub OIDC) — the provider has already verified the email; no MFA work for us. Excludes people without those accounts; can be added beside A or B.

**Constraint to note:** passkeys/WebAuthn need `navigator.credentials` — JavaScript — which the project otherwise avoids. Under A, TOTP is the no-JS MFA; under B the IdP's pages carry the JS, not ours.

**Decided (2026-10-25): option A, in-app.** TOTP is the MFA; passkeys can come later.

**Decided (grill session 2026-10-26):**
- **Mail goes out over plain SMTP**, configured by env (host, port, user, password from the cluster Secret, from-address). The provider is chosen later and changing it is config only. In dev, mail is written to the log.
- **An unverified account can't log in.** Register shows "check your inbox". Login refuses with a "resend link" button. Changing your email address requires verifying the new one.
- **MFA is optional:** TOTP is turned on in Profile → Security, and recovery codes are shown once when it's enabled.
- **Existing accounts are marked verified** by the migration. Production holds only the owner's accounts.
- **Replies don't reveal which emails have accounts:**
  - Register always says "check your inbox". An address that already has an account gets a "someone tried to register" mail instead.
  - Login says "verify your email" only after the correct password.
  - Password reset always says "if that account exists, we sent a link".
- **Rate limits are kept in Postgres** (an attempts table keyed by IP and by email), so they hold across several web pods. Starting values: 5 failed logins per email per 15 minutes, and 5 TOTP tries per login.
- **Tokens are stored hashed (SHA-256):** session tokens, plus the new verification and reset tokens. A leaked database or backup can't be replayed as live logins.

## ISS-020 detail — "Generate more cards" for a finished deck (intake, 2026-10-25)

**Problem.** Once a learner has worked through a deck, the only way to get more material is a new deck from the deck builder, which knows nothing about what they already have. Wanted: a "Generate N more" action (session summary + deck page) that adds new cards to the same deck, none of them repeating what is already there.

**Relation to M11.** This is narrower than the card-maintenance agent (`docs/plans/ASSISTANT_AGENT_PLAN.md`): no tools, no proposals, one generation job. Ship it first as an extension of `JobGenerate`; M11 can later expose the same function as an `add_cards` tool.

**Limits (decided 2026-10-25):**
- **A deck holds at most 300 cards**, however they arrive (generation, CSV import, manual add). At 300, "Generate more" and "Add card" are refused with a message.
- **One generation asks for at most 50 cards** — models time out above that. "Generate more" offers up to `min(50, 300 − cards in deck)`.
- **Only CSV import can bring more than 300 at once**; it is split into several decks of ≤300 (500 rows → two decks of 250, named "Name (1/2)", "Name (2/2)"). The import preview says so before executing.
- Existing decks over 300 are test data only (production started empty), so no migration of them is needed.

**Gap today.** A deck does not remember how it was made: `GenerateDeck` passes the `DeckSpec` (language, prompt) only to the job, and `decks` stores just name + mode. Language and learner goal belong on the project (ISS-021); the deck keeps its own topic prompt (empty for CSV/manual decks — the form asks once). **Build ISS-021 first.**

**Prompt design — cheap, and never trusting the model to dedupe:**
1. **Send fronts only**, one per line, in the user message: "The deck already has these — do not repeat any of them, nor their inflected forms or near-synonyms." Backs and examples add tokens and nothing to dedupe. Cost is small: a front is ~2–5 tokens, so 200 cards ≈ 1k input tokens against an 8k output budget.
2. **Dedupe on the server anyway.** Compare a normalized front (lower-case, trimmed, punctuation stripped) against the deck's set as each card streams out of `cardScanner`; skip repeats (the `(deck_id, front)` unique index catches exact ones). Models do repeat themselves, especially small local ones.
3. **Over-ask, stop early.** Request `count + ~25%`, and cancel the stream once `count` new cards are saved — skipped duplicates don't leave the deck short, and the extra output is never paid for.
4. **The whole list always fits:** with the 300-card cap the fronts are ≤ ~1.5k tokens, so no truncation strategy is needed for the deck itself. Within a project (ISS-021), dedupe on the server against *every* deck in the project, but send the model only this deck's fronts plus the sibling decks' names/topics.
5. **Optional progression signal:** mark which existing cards the learner knows well (high repetitions/ease) vs. struggles with, and ask for the next step up in difficulty, not just "more of the same".
6. **Prompt caching** (Anthropic `cache_control`; OpenAI caches long prefixes automatically): keep the fixed instructions first and the front list ordered by id (append-only), so repeat requests share a prefix. Worth it only if this is used often; not needed for v1.

**Flow (no JS, reuses existing pages):** `POST /decks/:id/generate-more` (count) → enqueue `JobGenerate` with the stored spec + exclusion list → 303 to the existing `/decks/:id/generating` progress page.

**Decided (grill session 2026-10-26):**
- **Default count 20**, at most `min(50, 300 − cards in deck)`.
- **Button on the deck page always** (while under 300), and **on the session summary after every session**: a plain link normally, and the main button with "You've seen every card in this deck" once nothing unseen is left.
- **A full deck offers a new deck in the same project** that continues the topic ("Food 2"), checked for repeats against the whole project.
- **Difficulty progression is later** (with M11). v1 sends topic, level and existing fronts.
- Repeat check keeps accents: in Spanish *si* and *sí* are different words. Normalizing means lower-case, trim, collapse spaces, strip surrounding punctuation only.

## ISS-021 detail — Projects: a container for decks (intake, 2026-10-25)

**Idea.** A learner creates a **project** ("Learn Spanish") and its decks live under it. The project is the context everything else is missing today: the target language, why and at what level the learner studies, which topics are already covered. It is the structured space the card-maintenance agent (M11) and "Generate more" (ISS-020) work in.

**Sketch:**
- `projects` — `user_id`, `name`, `language` (target), `goal` / `level` (free text from the interview), `notes` (agent-maintained memory, see below), timestamps. Owned and scoped like decks: another user's id is a 404.
- `decks.project_id` — FK, `ON DELETE CASCADE`; a deck's project must have the same owner. Each deck keeps its own topic prompt.
- **State: store only what can't be derived.** Progress (cards per deck, mastered / due / new counts) is computed from `cards` on demand, never stored. Stored state is what the learner said (language, goal, level) and the agent's `notes` (e.g. "struggles with ser/estar; wants travel vocabulary next") — written only through proposals the learner applies, like every M11 change.
- **UI (no JS):** dashboard lists projects → project page lists its decks with progress + "New deck in this project". The deck builder interview moves into a project: language and level are already known, so it starts at "which topic next?" — shorter, and one open builder conversation per project instead of per user.
- **AI context:** prompts for a project's decks get the project's language/goal/level, the deck list with topics, and the notes — compact and structured, instead of re-asking or guessing.
- **M11 scope moves from deck to project:** the agent's tools are bounded to one project the user owns (still the security boundary), so it can e.g. propose a new deck for a gap, or flag a card duplicated across two decks.

**Order:** ISS-021 → ISS-020 (stores its language/goal here, dedupes project-wide) → M11 (`docs/plans/ASSISTANT_AGENT_PLAN.md` needs its scope updated from deck to project).

**Decided (grill session 2026-10-26):**
- **Every deck is in a project** (`decks.project_id NOT NULL`). Production started empty, so nothing to migrate. Forms that create a deck (new, CSV import, deck builder) default to the user's last-used project.
- **Exactly one target language per project.**
- **Study stays per deck in v1.** A project-wide "study all due" session is a later change (`study_sessions` is keyed by deck).
- **At most 20 decks per project** (≤ 6,000 cards). A CSV split that would go past 20 is refused at preview.
- **The learner's own language is a per-user profile setting** (default English), replacing the hardcoded "translation in English" in `CardGenerationPrompt`. Every project uses it.
- **Projects are created with a short form**: name, target language, level, goal. No LLM call, works without a provider. The deck builder then starts inside the project at "which topic?".
- **Notes are visible and editable by the learner** on the project page, as plain text. The agent changes them only by proposal.
- **Decks can't move between projects in v1.**
