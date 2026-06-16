# ISSUES_TODO.md

All issues resolved as of 2026-05-15.

| # | Status | Summary |
|---|--------|---------|
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
