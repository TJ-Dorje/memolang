# MemoLang — Spaced Repetition (SM-2)

How SRS decks schedule cards. Linear decks don't use this: they go through cards in order, unlearned first.

## Where it lives

`internal/srs/sm2.go` — pure functions, no DB access, unit-tested in isolation (`task test:srs`).

```go
func Update(s CardState, rating int) (CardState, time.Time)
func Preview(s CardState) [4]int
// rating: 0=Again, 1=Hard, 2=Good, 3=Easy
```

Handlers call `Update`, then persist the result with `models.UpdateCardSRS(...)`.

## The rules

It is SM-2's shape — a fixed first and second gap, then gap × ease — with Anki's reading of the buttons:

- **Hard is a pass**, not a failure: the streak continues, the gap grows slowly (`hardFactor` 1.2).
- **Again** restarts the streak; its gap is always 1 day.
- **Gaps strictly increase** Again < Hard < Good < Easy, each at least a day longer than the button to its left, so every button choice makes a difference. (Original SM-2 counted Hard as a failure and gave Good and Easy identical first two gaps.)
- Ease moves by Anki's defaults (Again −0.20, Hard −0.15, Good 0, Easy +0.15) and never drops below 1.3.

## Preview is the single source of numbers

`Preview(state)` returns the four gaps without changing anything. The study screen shows them under the buttons, so `Update` must take its intervals from `Preview` — never compute them separately, or the buttons would lie.

## Again within a session

Besides rescheduling for tomorrow, Again re-queues the card at the end of the current session. A card appears at most `maxAppearances` (3) times per session (`internal/handlers/session.go`).
