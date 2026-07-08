// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"
)

func SettingsNavLink(t *testing.T) {
	page := components.NewPage(t)

	components.NavigateTo(t, page, "/")

	visible := components.VerifyNavLink(t, page, "Settings")
	if !visible {
		t.Error("Settings link not visible in nav")
	}

	components.ClickNavLink(t, page, "Settings")
	components.WaitForURL(t, page, "**/settings")

	heading := components.GetHeading(t, page)
	if heading != "Settings" {
		t.Errorf("expected heading 'Settings', got %q", heading)
	}
}

func SaveSettings(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-test-key")

	flash := components.GetFlash(t, page)
	if flash != "Settings saved." {
		t.Errorf("expected flash 'Settings saved.', got %q", flash)
	}

	baseURLInput := components.GetInputValue(t, page, "input[name=base_url]")
	if baseURLInput != "http://localhost:1234/v1" {
		t.Errorf("expected base_url persisted, got %q", baseURLInput)
	}

	modelInput := components.GetInputValue(t, page, "input[name=model]")
	if modelInput != "test-model" {
		t.Errorf("expected model persisted, got %q", modelInput)
	}

	apiKeyInput := components.GetInputValue(t, page, "input[name=api_key]")
	if apiKeyInput != "" {
		t.Errorf("expected api_key input empty, got %q", apiKeyInput)
	}

	placeholder := components.GetPlaceholder(t, page, "input[name=api_key]")
	if !strings.Contains(placeholder, "saved") {
		t.Errorf("expected api_key placeholder to indicate saved key, got %q", placeholder)
	}
}

func ReSaveKeepsAPIKey(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-another-key")

	actions.ClearAPIKeyField(t, page)
	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "")

	checked := components.IsChecked(t, page, "input[name=clear_api_key]")
	if checked {
		t.Error("clear_api_key checkbox should not be checked")
	}

	placeholder := components.GetPlaceholder(t, page, "input[name=api_key]")
	if !strings.Contains(placeholder, "saved") {
		t.Errorf("expected api_key still saved, placeholder %q", placeholder)
	}
}

func AIFormWithoutConfig(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SubmitAIForm(t, page, "Test Deck", "German", "Common words")
	components.WaitForURL(t, page, "**/decks/new**")

	errMsg := components.GetFormError(t, page)
	if !strings.Contains(errMsg, "Settings") {
		t.Errorf("expected error to mention Settings, got %q", errMsg)
	}

	settingsCount, _ := page.Locator(".form-error a[href='/settings']").Count()
	if settingsCount == 0 {
		t.Error("expected Settings link in error box")
	}
}

func SettingsValidation(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	components.NavigateTo(t, page, "/settings")

	if err := components.ClickButton(page, "button:has-text('Save')"); err != nil {
		t.Fatal(err)
	}

	errMsg := components.GetFormError(t, page)
	if !strings.Contains(errMsg, "required") {
		t.Errorf("expected 'required' in error message, got %q", errMsg)
	}
}
