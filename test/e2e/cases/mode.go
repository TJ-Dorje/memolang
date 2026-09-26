// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"
)

const rateEasy = 3

// LinearDeckKeepsStudyingAfterEveryCardIsPassed is the regression case for
// Linear: it used to study only never-passed cards, so after one session of
// Easy ratings the deck had nothing left to study, ever.
func LinearDeckKeepsStudyingAfterEveryCardIsPassed(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeckWithMode(t, page, "Linear Loop Deck", "linear")
	defer actions.DeleteDeck(t, page, deckURL)
	// One card, so the first session passes the entire deck. With more cards
	// than a session holds, the second session finds unseen cards and would
	// pass even with the old query.
	actions.ImportCSV(t, page, deckURL, configuration.SingleCardCSVPath)

	if meta := components.GetMeta(t, page); !strings.Contains(meta, "Linear") {
		t.Errorf("deck page meta = %q, want it to name the Linear mode", meta)
	}

	actions.StartFlashcardSession(t, page, deckURL)
	actions.RateAllFlashcards(t, page, rateEasy)

	actions.StartFlashcardSession(t, page, deckURL)
	if components.CountLocators(t, page, ".empty-state") != 0 {
		t.Fatal("linear deck has nothing to study after every card was passed")
	}
	if !components.IsVisible(t, page, "#reveal-btn") {
		t.Error("expected a card to study in the second linear session")
	}
}

// SRSDeckSaysWhenToComeBack: with nothing due, the empty state dates the next
// review and points at Linear for anyone who wants to keep going.
func SRSDeckSaysWhenToComeBack(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeckWithMode(t, page, "SRS Wait Deck", "srs")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)
	actions.RateAllFlashcards(t, page, rateEasy)

	actions.StartFlashcardSession(t, page, deckURL)
	nextDue, err := page.Locator(".empty-state .next-due").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nextDue, "Next review:") {
		t.Errorf("empty state = %q, want the next review date", nextDue)
	}
	if components.CountLocators(t, page, ".empty-state a:has-text('Switch this deck to Linear')") != 1 {
		t.Error("expected a pointer to Linear mode in the empty state")
	}
}

// ModePickerExplainsBothModes: the descriptions are the only place the
// difference is explained, on both forms that offer the choice.
func ModePickerExplainsBothModes(t *testing.T) {
	page := components.NewPage(t)

	for _, path := range []string{"/decks/new", "/decks/new?ai_mode=true"} {
		components.NavigateTo(t, page, path)
		if n := components.CountLocators(t, page, ".mode-option .hint"); n != 2 {
			t.Errorf("%s: %d mode descriptions, want 2", path, n)
		}
		if !components.IsChecked(t, page, "input[name=mode][value=srs]") {
			t.Errorf("%s: SRS is not the default", path)
		}
	}
}
