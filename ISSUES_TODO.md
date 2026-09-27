# ISSUES_TODO.md

Open items are listed first; everything below them is resolved.

| # | Status | Summary |
|---|--------|---------|
| ISS-017 | 🔲 Open — **before publishing** | AI assistant must stay on topic (language learning / deck building). Today only the prompt keeps it there; a real model will happily answer programming questions. See "ISS-017 detail" below |
| ISS-018 | 🔲 Planned — production stability | Scale past one pod: PostgreSQL instead of SQLite (prerequisite), a NATS JetStream `stream.Broker`, then a worker Deployment consuming LLM jobs so deploys and crashes don't kill in-flight generations. See `docs/plans/SCALING_PLAN.md` |
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
