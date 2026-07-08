// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"
)

func CSVImportPreviewThenExecute(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Import Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	if _, err := page.Goto(deckURL + "/import"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=csv]").SetInputFiles("testdata/spanish_verbs.csv"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}

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

	if err := page.Locator("button:has-text('Import')").Click(); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, deckURL)

	body := components.GetBody(t, page)
	if !strings.Contains(body, "imported") {
		t.Errorf("expected 'imported' in flash message, got body snippet %q", body[:min(200, len(body))])
	}

	meta := components.GetMeta(t, page)
	if strings.Contains(meta, "0 cards") {
		t.Error("deck still shows 0 cards after import")
	}
}

func CSVImportEmptyFileError(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Empty Import Deck")
	defer actions.DeleteDeck(t, page, deckURL)

	components.Goto(t, page, deckURL+"/import")

	count, _ := page.Locator("input[name=csv][required]").Count()
	if count == 0 {
		t.Error("CSV input is missing the 'required' attribute")
	}
}

func CSVImportTokenBelongsToDeck(t *testing.T) {
	page := components.NewPage(t)

	deck1URL := actions.CreateDeck(t, page, "Token Deck 1")
	defer actions.DeleteDeck(t, page, deck1URL)
	deck2URL := actions.CreateDeck(t, page, "Token Deck 2")
	defer actions.DeleteDeck(t, page, deck2URL)

	components.Goto(t, page, deck1URL+"/import")
	if err := page.Locator("input[name=csv]").SetInputFiles("testdata/spanish_verbs.csv"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Import')").WaitFor(); err != nil {
		t.Fatal(err)
	}

	token, _ := page.Locator("input[name=import_token]").GetAttribute("value")
	if token == "" {
		t.Fatal("could not read import_token")
	}

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
	_, err := page.Evaluate(script, []any{deck2URL + "/import?step=execute", token})
	if err != nil {
		t.Fatal(err)
	}

	if err := page.Locator(".form-error").WaitFor(); err != nil {
		t.Fatal(err)
	}
	errMsg, _ := page.Locator(".form-error").TextContent()
	if !strings.Contains(errMsg, "token") {
		t.Errorf("expected token error, got %q", errMsg)
	}
}
