// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"
)

// WorkerDeathResumesGeneration is what the job queue is for: a worker is
// killed halfway through a deck generation — no SIGTERM, like a crash or an
// OOM kill — a new worker gets the job after the ack wait, resumes it, and
// the deck ends up complete, while the page kept streaming throughout.
// Integration only: it needs real worker containers.
func WorkerDeathResumesGeneration(t *testing.T) {
	stack := helpers.CurrentStack
	if stack == nil {
		t.Skip("needs INTEGRATION=1: kills a worker container")
	}

	page, email := freshUser(t, "workerdeath")
	useFakeLLMAt(t, email, configuration.FakeLLMURL)
	components.NavigateTo(t, page, "/decks/new/assistant")
	sendToBuilder(t, page, "Spanish, "+helpers.FakeSlowTrigger)

	click(t, page, ".plan-card button:has-text('Generate Deck')")
	components.WaitForURLCommitted(t, page, "**/decks/*/generating")

	// A couple of cards in, the worker dies.
	if err := page.Locator(".gen-card").Nth(1).WaitFor(); err != nil {
		t.Fatalf("generation never got going: %v", err)
	}
	if err := stack.KillWorker(); err != nil {
		t.Fatal(err)
	}
	if err := stack.ReplaceWorker(); err != nil {
		t.Fatal(err)
	}

	waitAttached(t, page, ".gen-result")
	if n := components.CountLocators(t, page, ".gen-card"); n != len(helpers.FakeSlowCards) {
		t.Errorf("%d card rows streamed across both attempts, want %d", n, len(helpers.FakeSlowCards))
	}

	click(t, page, "a:has-text('Open Deck')")
	components.WaitForURL(t, page, "**/decks/*")
	if n := components.CountLocators(t, page, ".card-row"); n != len(helpers.FakeSlowCards) {
		t.Errorf("deck has %d cards after the resumed generation, want %d (no duplicates, none lost)", n, len(helpers.FakeSlowCards))
	}
}
