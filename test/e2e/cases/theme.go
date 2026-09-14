// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// Every theme case uses its own browser context. The theme lives in a cookie,
// so sharing configuration.Context would leak one case's choice into the next
// and make the order significant.

// ThemeDefaultsToSystem: with no cookie set, the server stamps no data-theme
// and the stylesheet is left to prefers-color-scheme.
func ThemeDefaultsToSystem(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	components.NavigateTo(t, page, "/login")

	if theme := components.GetTheme(t, page); theme != "" {
		t.Errorf("expected no data-theme without a cookie, got %q", theme)
	}
	if active := components.ActiveThemeOption(t, page); active != "system" {
		t.Errorf("expected the system cell to be active by default, got %q", active)
	}
}

// ThemeSwitcherIsPublic: the switcher has to work logged out, because the
// login and register pages carry it too.
func ThemeSwitcherIsPublic(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	components.NavigateTo(t, page, "/login")

	components.SelectTheme(t, page, "dark")

	if theme := components.GetTheme(t, page); theme != "dark" {
		t.Errorf("data-theme = %q, want \"dark\" after choosing dark while logged out", theme)
	}
	if active := components.ActiveThemeOption(t, page); active != "dark" {
		t.Errorf("active cell = %q, want \"dark\"", active)
	}
}

// ThemePersistsAcrossNavigation: the choice is a cookie, so it has to survive
// leaving the page it was made on.
func ThemePersistsAcrossNavigation(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	components.NavigateTo(t, page, "/login")

	components.SelectTheme(t, page, "light")

	components.NavigateTo(t, page, "/register")
	if theme := components.GetTheme(t, page); theme != "light" {
		t.Errorf("data-theme = %q on /register, want \"light\" to carry over", theme)
	}
}

// ThemeSystemClearsOverride: picking system after a fixed theme has to remove
// the attribute entirely, not stamp data-theme="system" — the stylesheet has
// no such state, and leaving an override in place would strand the user away
// from their OS setting.
func ThemeSystemClearsOverride(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	components.NavigateTo(t, page, "/login")

	components.SelectTheme(t, page, "dark")
	if theme := components.GetTheme(t, page); theme != "dark" {
		t.Fatalf("setup: data-theme = %q, want \"dark\"", theme)
	}

	components.SelectTheme(t, page, "system")
	if theme := components.GetTheme(t, page); theme != "" {
		t.Errorf("data-theme = %q after choosing system, want it removed", theme)
	}
	if active := components.ActiveThemeOption(t, page); active != "system" {
		t.Errorf("active cell = %q, want \"system\"", active)
	}
}

// ThemeReturnsToOriginPage: the switcher posts a return path taken from the
// request URI, so using it deep in the app must not bounce the user home —
// query string included.
func ThemeReturnsToOriginPage(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	registerThrough(t, page, uniqueEmail("theme"), "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	deckURL := helpers.CreateDeck(t, page, "Theme Origin Deck")
	defer helpers.DeleteDeck(t, page, deckURL)

	components.Goto(t, page, deckURL+"?filter=due")
	before := page.URL()

	components.SelectTheme(t, page, "dark")

	if page.URL() != before {
		t.Errorf("theme switch left the page: got %q, want %q", page.URL(), before)
	}
	if theme := components.GetTheme(t, page); theme != "dark" {
		t.Errorf("data-theme = %q, want \"dark\"", theme)
	}
}

// setThemeCookie plants a raw cookie value, bypassing the switcher, to reach
// states the UI cannot produce.
func setThemeCookie(t *testing.T, page playwright.Page, value string) {
	t.Helper()
	if err := page.Context().AddCookies([]playwright.OptionalCookie{{
		Name:  "theme",
		Value: value,
		URL:   playwright.String(configuration.BaseURL),
	}}); err != nil {
		t.Fatal(err)
	}
}

// ThemeIgnoresUnknownCookie: the cookie is attacker-controllable and its value
// is interpolated into an HTML attribute, so the handler whitelists to
// "dark"/"light" and anything else must be dropped rather than echoed.
//
// The payload is percent-encoded on purpose. Quotes and spaces are not legal
// cookie-value characters, so a raw payload is simply never sent and the case
// passes for the wrong reason — it did, until a mutation run caught it. Gin's
// c.Cookie runs url.QueryUnescape, so this arrives at the handler decoded as
//
//	dark" onload="alert(1)
func ThemeIgnoresUnknownCookie(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	setThemeCookie(t, page, `dark%22%20onload%3D%22alert(1)`)

	components.NavigateTo(t, page, "/login")

	if theme := components.GetTheme(t, page); theme != "" {
		t.Errorf("data-theme = %q, want it dropped for an unrecognised cookie", theme)
	}
	// Defence in depth: even if the whitelist were bypassed, html/template's
	// contextual escaping must keep the value inside the attribute.
	if n := components.CountLocators(t, page, "html[onload]"); n != 0 {
		t.Errorf("the cookie value broke out of the attribute: %d element(s) with onload", n)
	}
	if active := components.ActiveThemeOption(t, page); active != "system" {
		t.Errorf("active cell = %q, want \"system\" for an unrecognised cookie", active)
	}
}

// ThemeIgnoresUnknownPlainCookie covers the ordinary case: a well-formed but
// unrecognised theme name, such as one left by a future or removed theme.
func ThemeIgnoresUnknownPlainCookie(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	setThemeCookie(t, page, "solarized")

	components.NavigateTo(t, page, "/login")

	if theme := components.GetTheme(t, page); theme != "" {
		t.Errorf("data-theme = %q, want it dropped for an unknown theme name", theme)
	}
}
