package components

import (
	"testing"

	"memolang/test/e2e/configuration"

	playwright "github.com/playwright-community/playwright-go"
)

func VerifyNavLink(t *testing.T, page playwright.Page, name string) bool {
	t.Helper()
	link := page.Locator("nav a:has-text('" + name + "')")
	visible, err := link.IsVisible()
	if err != nil {
		t.Fatal(err)
	}
	return visible
}

func ClickNavLink(t *testing.T, page playwright.Page, name string) {
	t.Helper()
	if err := page.Locator("nav a:has-text('" + name + "')").Click(); err != nil {
		t.Fatal(err)
	}
}

func NavigateTo(t *testing.T, page playwright.Page, path string) {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + path); err != nil {
		t.Fatal(err)
	}
}

func NavigateToDeck(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if _, err := page.Goto(deckURL); err != nil {
		t.Fatal(err)
	}
}

func WaitForURL(t *testing.T, page playwright.Page, pattern string) {
	t.Helper()
	if err := page.WaitForURL(pattern); err != nil {
		t.Fatal(err)
	}
}

func Goto(t *testing.T, page playwright.Page, url string) {
	t.Helper()
	if _, err := page.Goto(url); err != nil {
		t.Fatal(err)
	}
}
