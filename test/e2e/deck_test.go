package e2e_test

import (
	"strings"
	"testing"
)

func TestCreateDeck(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "E2E Test Deck")
	defer deleteDeck(t, page, deckURL)

	heading, err := page.Locator("h1").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if heading != "E2E Test Deck" {
		t.Errorf("expected heading 'E2E Test Deck', got %q", heading)
	}
}

func TestCreateDeckNameRequired(t *testing.T) {
	page := newPage(t)

	if _, err := page.Goto(baseURL + "/decks/new"); err != nil {
		t.Fatal(err)
	}
	// Disable browser HTML5 validation so the POST reaches the server
	if _, err := page.Evaluate("document.querySelector('form').noValidate = true"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}

	// Server should re-render the form with an error
	errMsg, err := page.Locator(".form-error").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errMsg, "required") {
		t.Errorf("expected 'required' in error message, got %q", errMsg)
	}
}

func TestDeleteDeck(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Deck To Delete")

	// Delete it (scoped to page-header to avoid matching card-level delete buttons)
	if err := page.Locator(".page-header button:has-text('Delete')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(baseURL + "/"); err != nil {
		t.Fatal(err)
	}

	// Should not appear on dashboard
	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	_ = deckURL
	if strings.Contains(body, "Deck To Delete") {
		t.Error("deleted deck still visible on dashboard")
	}
}

func TestDeckCardClickable(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Clickable Deck")
	defer deleteDeck(t, page, deckURL)

	// Go to dashboard and click the stretched link that covers the card body
	if _, err := page.Goto(baseURL + "/"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".deck-card .deck-link").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}

	heading, err := page.Locator("h1").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if heading != "Clickable Deck" {
		t.Errorf("expected 'Clickable Deck', got %q", heading)
	}
}

func TestStudyDropdownOffersBothModes(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Study Modes Deck")
	defer deleteDeck(t, page, deckURL)

	if _, err := page.Goto(baseURL + "/"); err != nil {
		t.Fatal(err)
	}

	// Options should be hidden before Study is clicked
	options := page.Locator(".study-options")
	visible, _ := options.IsVisible()
	if visible {
		t.Error("study options should be hidden before clicking Study")
	}

	// Click Study to expand
	if err := page.Locator(".study-menu summary").Click(); err != nil {
		t.Fatal(err)
	}

	// Both options should now be visible
	flashcard := page.Locator(".study-options a:has-text('Flashcards')")
	mc := page.Locator(".study-options a:has-text('Multiple Choice')")
	if v, _ := flashcard.IsVisible(); !v {
		t.Error("Flashcards option not visible after clicking Study")
	}
	if v, _ := mc.IsVisible(); !v {
		t.Error("Multiple Choice option not visible after clicking Study")
	}

	// Click Flashcards — should navigate to session
	if err := flashcard.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/session?mode=flashcard"); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardLayout(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Layout Test Deck")
	defer deleteDeck(t, page, deckURL)

	if _, err := page.Goto(baseURL + "/"); err != nil {
		t.Fatal(err)
	}

	// Browse button must be gone
	browseCount, err := page.Locator("a:has-text('Browse')").Count()
	if err != nil {
		t.Fatal(err)
	}
	if browseCount > 0 {
		t.Error("Browse button should not exist on dashboard")
	}

	// + New Deck must NOT be in the nav
	navCount, err := page.Locator("nav a:has-text('New Deck')").Count()
	if err != nil {
		t.Fatal(err)
	}
	if navCount > 0 {
		t.Error("+ New Deck button should not appear in the nav header")
	}

	// + New Deck must still exist in the page-header
	headerCount, err := page.Locator(".page-header a:has-text('New Deck')").Count()
	if err != nil {
		t.Fatal(err)
	}
	if headerCount == 0 {
		t.Error("+ New Deck button missing from page header")
	}
}

func TestEditDeck(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Original Name")
	defer deleteDeck(t, page, deckURL)

	// Click Edit
	if err := page.Locator("a:has-text('Edit')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/edit"); err != nil {
		t.Fatal(err)
	}

	// Clear and re-fill name
	nameInput := page.Locator("input[name=name]")
	if err := nameInput.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := nameInput.Fill("Renamed Deck"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}

	heading, err := page.Locator("h1").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if heading != "Renamed Deck" {
		t.Errorf("expected 'Renamed Deck', got %q", heading)
	}
}
