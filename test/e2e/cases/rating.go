// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

const (
	rateAgain = 0
	rateHard  = 1
)

func textOf(t *testing.T, page playwright.Page, selector string) string {
	t.Helper()
	text, err := page.Locator(selector).TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(text)
}

// RatingButtonsShowGaps: every button says when the card comes back, and a
// new card's four choices are all different.
func RatingButtonsShowGaps(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeck(t, page, "Gap Labels Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.SingleCardCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)

	gaps, err := page.Locator(".rating-gap").AllTextContents()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"soon", "1d", "2d", "4d"}
	if strings.Join(gaps, ",") != strings.Join(want, ",") {
		t.Errorf("gap labels for a new card = %v, want %v", gaps, want)
	}
}

// HardCountsAsCorrect: Hard means "remembered with effort". It used to be
// scored as a miss and reset the card like Again.
func HardCountsAsCorrect(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeck(t, page, "Hard Pass Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)
	actions.RateFlashcard(t, page, rateHard)

	if right := textOf(t, page, ".score-right"); right != "✓ 1" {
		t.Errorf("score after Hard = %q, want ✓ 1", right)
	}
	if wrong := textOf(t, page, ".score-wrong"); wrong != "✗ 0" {
		t.Errorf("misses after Hard = %q, want ✗ 0", wrong)
	}
}

// AgainBringsTheCardBack: a missed card rejoins the end of the session.
func AgainBringsTheCardBack(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeck(t, page, "Again Requeue Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)
	before := textOf(t, page, ".session-position")
	actions.RateFlashcard(t, page, rateAgain)
	after := textOf(t, page, ".session-position")

	if before != "Card 1 of 25" || after != "Card 2 of 26" {
		t.Errorf("position %q → %q, want \"Card 1 of 25\" → \"Card 2 of 26\"", before, after)
	}
}

// AgainStopsAfterThreeAppearances: a card you keep failing cannot trap you in
// the session. With one card, three Agains must end it.
func AgainStopsAfterThreeAppearances(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeck(t, page, "Again Cap Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.SingleCardCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)
	for i := range 3 {
		if strings.Contains(page.URL(), "summary") {
			t.Fatalf("session ended after %d Again(s), want the card to come back until 3", i)
		}
		actions.RateFlashcard(t, page, rateAgain)
	}

	if !strings.Contains(page.URL(), "summary") {
		t.Errorf("after 3 Agains on the only card, url = %q, want the summary", page.URL())
	}
}
