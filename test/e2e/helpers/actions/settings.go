package actions

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// NavigateToAIProvider takes the real route: account menu → Profile → the
// profile side menu's AI Providers entry.
func NavigateToAIProvider(t *testing.T, page playwright.Page) {
	t.Helper()
	components.ClickAccountMenuItem(t, page, "Profile")
	if err := page.WaitForURL("**/profile"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".profile-nav a:has-text('AI Providers')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/profile/ai"); err != nil {
		t.Fatal(err)
	}
}

// OpenProviderForm opens the form for the shared user's provider: its edit
// form when there is one (settings cases keep a single provider), else the
// add form.
func OpenProviderForm(t *testing.T, page playwright.Page) {
	t.Helper()
	components.NavigateTo(t, page, "/profile/ai")
	edit := page.Locator(".provider-card a:has-text('Edit')").First()
	n, err := edit.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		components.NavigateTo(t, page, "/profile/ai/new")
		return
	}
	if err := edit.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/edit"); err != nil {
		t.Fatal(err)
	}
}

// SaveSettings fills and saves the provider form, returning the flash the
// list shows, then reopens the form so callers can check what was stored.
func SaveSettings(t *testing.T, page playwright.Page, provider, baseURL, model, apiKey string) string {
	t.Helper()
	OpenProviderForm(t, page)
	SelectProvider(t, page, provider)
	if baseURL != "" {
		if err := components.FillInput(page, "input[name=base_url]", baseURL); err != nil {
			t.Fatal(err)
		}
	}
	if model != "" {
		if err := components.FillInput(page, "input[name=model]", model); err != nil {
			t.Fatal(err)
		}
	}
	if apiKey != "" {
		if err := components.FillInput(page, "input[name=api_key]", apiKey); err != nil {
			t.Fatal(err)
		}
	}
	return SubmitSettings(t, page)
}

// SubmitSettings submits the provider form as it stands. A successful save
// lands on the list: its flash is returned and the form reopened. A rejected
// one re-renders the form with an error, and "" is returned.
func SubmitSettings(t *testing.T, page playwright.Page) string {
	t.Helper()
	// The form re-renders at its own POST URL on error, so wait for the
	// navigation itself rather than for one particular URL.
	_, err := page.ExpectNavigation(func() error {
		return components.ClickButton(page, "form button:has-text('Save')")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(page.URL(), "/profile/ai") {
		return ""
	}
	flash := components.GetFlash(t, page)
	OpenProviderForm(t, page)
	return flash
}

func ClearAPIKeyField(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("input[name=api_key]").Clear(); err != nil {
		t.Fatal(err)
	}
}

// TickClearAPIKey checks the "Clear saved key" box. It is only rendered when a
// key is actually stored, so this doubles as an assertion that one is.
func TickClearAPIKey(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("input[name=clear_api_key]").Check(); err != nil {
		t.Fatal(err)
	}
}

// SelectProvider picks a provider from the dropdown, mirroring a real user's
// change event so the progressive-enhancement script runs.
func SelectProvider(t *testing.T, page playwright.Page, provider string) {
	t.Helper()
	values := []string{provider}
	if _, err := page.Locator("select[name=provider]").SelectOption(playwright.SelectOptionValues{Values: &values}); err != nil {
		t.Fatal(err)
	}
}

// ClearField empties an input, for cases that need to submit it blank.
func ClearField(t *testing.T, page playwright.Page, selector string) {
	t.Helper()
	if err := page.Locator(selector).Clear(); err != nil {
		t.Fatal(err)
	}
}

// ResetLLMSettings removes the shared e2e user's providers, so a settings
// case starts from a clean, order-independent state.
func ResetLLMSettings(t *testing.T) {
	t.Helper()
	_, err := configuration.DB.Exec(
		"DELETE FROM llm_providers WHERE user_id = (SELECT id FROM users WHERE email = ?)",
		configuration.TestUserEmail,
	)
	if err != nil {
		t.Fatal(err)
	}
}
