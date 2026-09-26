package actions

import (
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

func CreateDeck(t *testing.T, page playwright.Page, name string) string {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/decks/new"); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=name]", name); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/decks/**"); err != nil {
		t.Fatal(err)
	}
	return page.URL()
}

// CreateDeckWithMode is CreateDeck with an explicit study mode ("srs" or
// "linear") picked in the form.
func CreateDeckWithMode(t *testing.T, page playwright.Page, name, mode string) string {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/decks/new"); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=name]", name); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=mode][value=" + mode + "]").Check(); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/decks/**"); err != nil {
		t.Fatal(err)
	}
	return page.URL()
}

// RateAllFlashcards reveals and rates every card left in the session until
// it reaches the summary.
func RateAllFlashcards(t *testing.T, page playwright.Page, rating int) {
	t.Helper()
	for range maxClickLoop {
		if contains(page.URL(), "summary") {
			return
		}
		if err := page.Locator("#reveal-btn").Click(); err != nil {
			t.Fatal(err)
		}
		// Wait for the navigation the rating causes. WaitForURL is no use
		// here: the next card lives at the same /session URL, so it returns at
		// once and the next pass reads the old page. ExpectNavigation is marked
		// deprecated as racy, but that applies to waiting after the fact;
		// triggering the click inside its callback is the race-free use.
		_, err := page.ExpectNavigation(func() error {
			return page.Locator("button[value='" + string(rune('0'+rating)) + "']").Click()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("session did not reach its summary within %d cards", maxClickLoop)
}

// DeleteDeck removes the deck; no-op if deckURL is empty or the deck is
// already gone (safe to use in defer after a test deleted it itself).
func DeleteDeck(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if deckURL == "" {
		return
	}
	if _, err := page.Goto(deckURL); err != nil {
		t.Fatal(err)
	}
	btn := page.Locator(".page-header button:has-text('Delete')")
	if n, err := btn.Count(); err != nil || n == 0 {
		return
	}
	if err := btn.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(configuration.BaseURL + "/"); err != nil {
		t.Fatal(err)
	}
}

func EditDeck(t *testing.T, page playwright.Page, deckURL, newName string) {
	t.Helper()
	if _, err := page.Goto(deckURL); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("a:has-text('Edit')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/edit"); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=name]", newName); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}
}

func StartFlashcardSession(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if _, err := page.Goto(deckURL + "/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}
}

func StartMCSession(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if _, err := page.Goto(deckURL + "/session?mode=mc"); err != nil {
		t.Fatal(err)
	}
}

func EndSessionEarly(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("button:has-text('End Session')").Click(); err != nil {
		t.Fatal(err)
	}
}

func SubmitFlashcardRating(t *testing.T, page playwright.Page, rating int) {
	t.Helper()
	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[value='" + string(rune('0'+rating)) + "']").Click(); err != nil {
		t.Fatal(err)
	}
}

func SubmitMCAnswer(t *testing.T, page playwright.Page, choiceIndex int) {
	t.Helper()
	options := page.Locator(".mc-option")
	if err := options.Nth(choiceIndex).Click(); err != nil {
		t.Fatal(err)
	}
}

func CompleteSession(t *testing.T, page playwright.Page) {
	t.Helper()
	for range maxClickLoop {
		currentURL := page.URL()
		if contains(currentURL, "summary") {
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
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

const maxClickLoop = 55

func ImportCSV(t *testing.T, page playwright.Page, deckURL, csvPath string) {
	t.Helper()
	if _, err := page.Goto(deckURL + "/import"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=csv]").SetInputFiles(csvPath); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Import')").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Import')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}
}
