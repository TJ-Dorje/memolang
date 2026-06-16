package e2e_test

import (
	"strings"
	"testing"
)

func TestCSVImportPreviewThenExecute(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Import Test Deck")
	defer deleteDeck(t, page, deckURL)

	// Step 1: upload and preview
	if _, err := page.Goto(deckURL + "/import"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=csv]").SetInputFiles("testdata/spanish_verbs.csv"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}

	// Preview table and row count should appear
	if err := page.Locator("table.preview-table").WaitFor(); err != nil {
		t.Fatal(err)
	}
	header, err := page.Locator("h2").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(header, "rows") {
		t.Errorf("expected row count in preview heading, got %q", header)
	}

	// Step 2: execute import
	if err := page.Locator("button:has-text('Import')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}

	// Flash should confirm import
	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "imported") {
		t.Errorf("expected 'imported' in flash message, got body snippet %q", body[:min(200, len(body))])
	}

	// Deck meta should show cards > 0
	meta, err := page.Locator(".meta").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(meta, "0 cards") {
		t.Error("deck still shows 0 cards after import")
	}
}

func TestCSVImportEmptyFileError(t *testing.T) {
	page := newPage(t)

	deckURL := createDeck(t, page, "Empty Import Deck")
	defer deleteDeck(t, page, deckURL)

	if _, err := page.Goto(deckURL + "/import"); err != nil {
		t.Fatal(err)
	}
	// Verify the required attribute is present using a CSS attribute selector.
	// GetAttribute returns "" for both missing and valueless boolean attributes,
	// so Count() on a scoped selector is the reliable check.
	count, err := page.Locator("input[name=csv][required]").Count()
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Error("CSV input is missing the 'required' attribute")
	}
}

func TestCSVImportTokenBelongsToDeck(t *testing.T) {
	page := newPage(t)

	// Create two decks
	deck1URL := createDeck(t, page, "Token Deck 1")
	defer deleteDeck(t, page, deck1URL)
	deck2URL := createDeck(t, page, "Token Deck 2")
	defer deleteDeck(t, page, deck2URL)

	// Preview on deck1 — this stores a token bound to deck1
	if _, err := page.Goto(deck1URL + "/import"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=csv]").SetInputFiles("testdata/spanish_verbs.csv"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Import')").WaitFor(); err != nil {
		t.Fatal(err)
	}

	// Grab the token from the hidden input
	token, err := page.Locator("input[name=import_token]").GetAttribute("value")
	if err != nil || token == "" {
		t.Fatal("could not read import_token")
	}

	// Manually POST the execute step to deck2 with deck1's token
	// Playwright doesn't have a raw POST API, so we inject a form via JS and submit it.
	script := `(args) => {
		const f = document.createElement('form');
		f.method = 'POST';
		f.action = args[0];
		const inp = document.createElement('input');
		inp.name = 'import_token';
		inp.value = args[1];
		f.appendChild(inp);
		document.body.appendChild(f);
		f.submit();
	}`
	_, err = page.Evaluate(script, []any{deck2URL + "/import?step=execute", token})
	if err != nil {
		t.Fatal(err)
	}

	// Should get an error (not a successful import)
	if err := page.Locator(".form-error").WaitFor(); err != nil {
		t.Fatal(err)
	}
	errMsg, _ := page.Locator(".form-error").TextContent()
	if !strings.Contains(errMsg, "token") {
		t.Errorf("expected token error, got %q", errMsg)
	}
}

