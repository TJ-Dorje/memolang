// Package cases holds the e2e test case bodies. They are plain exported
// functions, not tests: new case funcs MUST be registered in
// test/e2e/main_test.go, or they will silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"
)

func CreateDeck(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "E2E Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	heading := components.GetHeading(t, page)
	if heading != "E2E Test Deck" {
		t.Errorf("expected heading 'E2E Test Deck', got %q", heading)
	}
}

func CreateDeckNameRequired(t *testing.T) {
	page := components.NewPage(t)

	components.NavigateTo(t, page, "/decks/new")
	if _, err := page.Evaluate("document.querySelector('main form').noValidate = true"); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}

	errMsg := components.GetFormError(t, page)
	if !strings.Contains(errMsg, "required") {
		t.Errorf("expected 'required' in error message, got %q", errMsg)
	}
}

func DeleteDeck(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Deck To Delete")
	defer actions.DeleteDeck(t, page, deckURL)

	components.ClickDeleteButton(t, page)
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	body := components.GetBody(t, page)
	if strings.Contains(body, "Deck To Delete") {
		t.Error("deleted deck still visible on dashboard")
	}
}

func DeckCardClickable(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Clickable Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	components.NavigateTo(t, page, "/")
	components.ClickDeckCard(t, page)
	components.WaitForURL(t, page, deckURL)

	heading := components.GetHeading(t, page)
	if heading != "Clickable Deck" {
		t.Errorf("expected 'Clickable Deck', got %q", heading)
	}
}

func StudyDropdownOffersBothModes(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Study Modes Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	components.NavigateTo(t, page, "/")

	options := page.Locator(".study-options")
	visible, _ := options.IsVisible()
	if visible {
		t.Error("study options should be hidden before clicking Study")
	}

	if err := page.Locator(".study-menu summary").Click(); err != nil {
		t.Fatal(err)
	}

	flashcard := page.Locator(".study-options a:has-text('Flashcards')")
	mc := page.Locator(".study-options a:has-text('Multiple Choice')")
	if v, _ := flashcard.IsVisible(); !v {
		t.Error("Flashcards option not visible after clicking Study")
	}
	if v, _ := mc.IsVisible(); !v {
		t.Error("Multiple Choice option not visible after clicking Study")
	}

	if err := flashcard.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}
}

func DashboardLayout(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Layout Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	components.NavigateTo(t, page, "/")

	browseCount, _ := page.Locator("a:has-text('Browse')").Count()
	if browseCount > 0 {
		t.Error("Browse button should not exist on dashboard")
	}

	navCount, _ := page.Locator("nav a:has-text('New Deck')").Count()
	if navCount > 0 {
		t.Error("+ New Deck button should not appear in the nav header")
	}

	headerCount, _ := page.Locator(".page-header a:has-text('New Deck')").Count()
	if headerCount == 0 {
		t.Error("+ New Deck button missing from page header")
	}
}

func EditDeck(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Original Name")
	defer actions.DeleteDeck(t, page, deckURL)

	components.ClickEditButton(t, page)
	components.WaitForURL(t, page, "**/edit")

	if err := page.Locator("input[name=name]").Clear(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=name]").Fill("Renamed Deck"); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, deckURL)

	heading := components.GetHeading(t, page)
	if heading != "Renamed Deck" {
		t.Errorf("expected 'Renamed Deck', got %q", heading)
	}
}
