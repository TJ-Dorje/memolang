package actions

import (
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// NavigateToAIProvider takes the real route: account menu → Profile → the
// profile side menu's AI Provider entry.
func NavigateToAIProvider(t *testing.T, page playwright.Page) {
	t.Helper()
	components.ClickAccountMenuItem(t, page, "Profile")
	if err := page.WaitForURL("**/profile"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".profile-nav a:has-text('AI Provider')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/profile/ai"); err != nil {
		t.Fatal(err)
	}
}

func SaveSettings(t *testing.T, page playwright.Page, provider, baseURL, model, apiKey string) {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/profile/ai"); err != nil {
		t.Fatal(err)
	}
	values := []string{provider}
	if _, err := page.Locator("select[name=provider]").SelectOption(playwright.SelectOptionValues{Values: &values}); err != nil {
		t.Fatal(err)
	}
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
	if err := components.ClickButton(page, "button:has-text('Save')"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/profile/ai"); err != nil {
		t.Fatal(err)
	}
}

func TestLLMConnection(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := components.ClickButton(page, "button:has-text('Test Connection')"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/profile/ai"); err != nil {
		t.Fatal(err)
	}
}

func ClearAPIKeyField(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("input[name=api_key]").Clear(); err != nil {
		t.Fatal(err)
	}
}

func SubmitAIForm(t *testing.T, page playwright.Page, name, language, prompt string) {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/decks/new?ai_mode=true"); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=name]", name); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=language]", language); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "textarea[name=prompt]", prompt); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate("document.querySelector('main form').noValidate = true"); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button:has-text('Generate Cards')"); err != nil {
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

// SubmitSettings submits the settings form as it currently stands, without
// touching any field — for cases that set up state by other means first.
func SubmitSettings(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := components.ClickButton(page, "button:has-text('Save')"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/profile/ai"); err != nil {
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
