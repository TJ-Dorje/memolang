package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"memolang/test/e2e/configuration"

	playwright "github.com/playwright-community/playwright-go"
)

func NewPage(t *testing.T) playwright.Page {
	t.Helper()
	// configuration.Context already carries the shared test user's session
	// cookie, so cases open pages already logged in.
	page, err := configuration.Context.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.OnDialog(func(d playwright.Dialog) { d.Accept() })
	t.Cleanup(func() { page.Close() })
	return page
}

func CreateDeck(t *testing.T, page playwright.Page, name string) string {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/decks/new"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=name]").Fill(name); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/decks/**"); err != nil {
		t.Fatal(err)
	}
	return page.URL()
}

func DeleteDeck(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if _, err := page.Goto(deckURL); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".page-header button:has-text('Delete')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(configuration.BaseURL + "/"); err != nil {
		t.Fatal(err)
	}
}

func ImportCards(t *testing.T, page playwright.Page, deckURL, csvPath string) {
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

func FindRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}

func ChdirRoot() {
	if err := os.Chdir(FindRoot()); err != nil {
		panic(fmt.Sprintf("ChdirRoot: %v", err))
	}
}
