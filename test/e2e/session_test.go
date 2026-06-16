package e2e_test

import (
	"strings"
	"testing"
)

func TestFlashcardReveal(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Flashcard Test Deck")
	defer deleteDeck(t, page, deckURL)
	importCards(t, page, deckURL, "testdata/spanish_verbs.csv")

	// Start flashcard session
	if _, err := page.Goto(deckURL + "/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}

	// Back side and answer form are hidden initially
	backVisible, _ := page.Locator("#card-back").IsVisible()
	if backVisible {
		t.Error("card back should be hidden before reveal")
	}
	formVisible, _ := page.Locator("#answer-form").IsVisible()
	if formVisible {
		t.Error("answer form should be hidden before reveal")
	}

	// Click Reveal
	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}

	// Back and form should now be visible
	backVisible, _ = page.Locator("#card-back").IsVisible()
	if !backVisible {
		t.Error("card back should be visible after reveal")
	}
	formVisible, _ = page.Locator("#answer-form").IsVisible()
	if !formVisible {
		t.Error("answer form should be visible after reveal")
	}
}

func TestFlashcardRatingAdvances(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Rating Test Deck")
	defer deleteDeck(t, page, deckURL)
	importCards(t, page, deckURL, "testdata/spanish_verbs.csv")

	if _, err := page.Goto(deckURL + "/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}

	// Reveal and rate Good
	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[value='2']").Click(); err != nil { // Good
		t.Fatal(err)
	}

	// Should still be on session URL (next card)
	if err := page.WaitForURL("**/session"); err != nil {
		t.Fatal(err)
	}

	// Progress bar should exist
	progress, err := page.Locator(".progress").IsVisible()
	if err != nil {
		t.Fatal(err)
	}
	if !progress {
		t.Error("progress bar not visible on next card")
	}
}

func TestMultipleChoiceSession(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "MC Test Deck")
	defer deleteDeck(t, page, deckURL)
	importCards(t, page, deckURL, "testdata/spanish_verbs.csv")

	if _, err := page.Goto(deckURL + "/session?mode=mc"); err != nil {
		t.Fatal(err)
	}

	// MC options should be present (no reveal button)
	revealVisible, _ := page.Locator("#reveal-btn").IsVisible()
	if revealVisible {
		t.Error("reveal button should not appear in MC mode")
	}

	options := page.Locator(".mc-option")
	count, err := options.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 MC options, got %d", count)
	}

	// Click any option
	if err := options.First().Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/session"); err != nil {
		t.Fatal(err)
	}
}

func TestEndSessionEarly(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "End Early Deck")
	defer deleteDeck(t, page, deckURL)
	importCards(t, page, deckURL, "testdata/spanish_verbs.csv")

	if _, err := page.Goto(deckURL + "/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}

	// End session before finishing — should redirect to Home, not the deck page
	if err := page.Locator("button:has-text('End Session')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(baseURL + "/"); err != nil {
		t.Fatal(err)
	}

	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Session ended") {
		t.Errorf("expected 'Session ended' flash, got body snippet %q", body[:min(300, len(body))])
	}
}

func TestSessionSummary(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Summary Test Deck")
	defer deleteDeck(t, page, deckURL)
	importCards(t, page, deckURL, "testdata/spanish_verbs.csv")

	// Start MC session and answer all cards with first option
	if _, err := page.Goto(deckURL + "/session?mode=mc"); err != nil {
		t.Fatal(err)
	}

	// Answer cards until we hit the summary page
	for range 55 { // more than the 50-card SRS queue limit
		currentURL := page.URL()
		if strings.Contains(currentURL, "summary") {
			break
		}
		option := page.Locator(".mc-option").First()
		visible, _ := option.IsVisible()
		if !visible {
			break
		}
		if err := option.Click(); err != nil {
			break
		}
		page.WaitForLoadState()
	}

	// Should be on summary page
	if err := page.WaitForURL("**/summary"); err != nil {
		t.Fatal(err)
	}

	// Summary page should have stat tiles
	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	for _, stat := range []string{"Reviewed", "Correct", "Accuracy"} {
		if !strings.Contains(body, stat) {
			t.Errorf("summary page missing stat %q", stat)
		}
	}
}
