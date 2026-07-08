package actions

import (
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

func NavigateToSettings(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("nav a:has-text('Settings')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/settings"); err != nil {
		t.Fatal(err)
	}
}

func SaveSettings(t *testing.T, page playwright.Page, provider, baseURL, model, apiKey string) {
	t.Helper()
	if _, err := page.Goto(configuration.BaseURL + "/settings"); err != nil {
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
	if err := page.WaitForURL("**/settings"); err != nil {
		t.Fatal(err)
	}
}

func TestLLMConnection(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := components.ClickButton(page, "button:has-text('Test Connection')"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/settings"); err != nil {
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
	if _, err := page.Evaluate("document.querySelector('form').noValidate = true"); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button:has-text('Generate Cards')"); err != nil {
		t.Fatal(err)
	}
}
