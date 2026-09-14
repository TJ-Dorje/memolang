package components

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

// GetTheme reads the data-theme attribute the server stamped on <html>.
// Returns "" when the attribute is absent, which is the follow-the-OS state.
//
// Evaluate rather than GetAttribute so a missing attribute and an empty one
// collapse to the same "" instead of depending on how the binding maps null.
func GetTheme(t *testing.T, page playwright.Page) string {
	t.Helper()
	result, err := page.Evaluate("() => document.documentElement.getAttribute('data-theme') || ''")
	if err != nil {
		t.Fatal(err)
	}
	theme, _ := result.(string)
	return theme
}

// SelectTheme clicks a cell of the nav theme switcher: "system", "light" or
// "dark". It waits for the redirect to land so the caller reads the new page.
func SelectTheme(t *testing.T, page playwright.Page, theme string) {
	t.Helper()
	before := page.URL()
	if err := page.Locator(".theme-toggle button[value='" + theme + "']").Click(); err != nil {
		t.Fatal(err)
	}
	// The switcher is a POST + 303 back to the page it was used on, so the
	// URL is unchanged and there is no navigation to wait on by URL.
	if err := page.WaitForURL(before); err != nil {
		t.Fatal(err)
	}
}

// ActiveThemeOption returns the value of the highlighted switcher cell, or ""
// when no cell is highlighted.
//
// The count check is what keeps that "" case fast: GetAttribute on a locator
// matching nothing blocks for the full 30s timeout, which turns an ordinary
// assertion failure into a stalled run.
func ActiveThemeOption(t *testing.T, page playwright.Page) string {
	t.Helper()
	active := page.Locator(".theme-toggle button.active")
	n, err := active.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		return ""
	}
	value, err := active.First().GetAttribute("value")
	if err != nil {
		t.Fatal(err)
	}
	return value
}
