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

// SettingsValidation: Base URL and Model are still required where nothing can
// supply them — Custom. This used to assert the same for a pristine form, but
// presets now prefill every other provider, so an empty submission there is
// meant to succeed. PresetSaveNeedsOnlyAKey covers that side.
func SettingsValidation(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	components.NavigateTo(t, page, "/settings")
	actions.SelectProvider(t, page, "custom")
	actions.ClearField(t, page, "input[name=base_url]")
	actions.ClearField(t, page, "input[name=model]")

	if err := components.ClickButton(page, "button:has-text('Save')"); err != nil {
		t.Fatal(err)
	}

	errMsg := components.GetFormError(t, page)
	if !strings.Contains(errMsg, "required") {
		t.Errorf("expected 'required' in error message, got %q", errMsg)
	}
}

// PresetPrefillsBaseURLAndModel: a first-time user must land on a form that is
// already filled in, not two empty boxes.
func PresetPrefillsBaseURLAndModel(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	components.NavigateTo(t, page, "/settings")

	baseURL := components.GetInputValue(t, page, "input[name=base_url]")
	if baseURL == "" {
		t.Error("base_url is empty on a pristine form; expected the first preset's URL")
	}
	model := components.GetInputValue(t, page, "input[name=model]")
	if model == "" {
		t.Error("model is empty on a pristine form; expected the first preset's default")
	}
}

// PresetSaveNeedsOnlyAKey is the whole point of presets: pick a provider,
// paste a key, save. The server fills the rest even with the fields blanked,
// which is also the no-JavaScript path.
func PresetSaveNeedsOnlyAKey(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	components.NavigateTo(t, page, "/settings")
	actions.SelectProvider(t, page, "gemini")
	actions.ClearField(t, page, "input[name=base_url]")
	actions.ClearField(t, page, "input[name=model]")
	if err := components.FillInput(page, "input[name=api_key]", "test-key"); err != nil {
		t.Fatal(err)
	}
	actions.SubmitSettings(t, page)

	flash := components.GetFlash(t, page)
	if !strings.Contains(flash, "saved") {
		t.Fatalf("expected a successful save, got flash %q", flash)
	}

	baseURL := components.GetInputValue(t, page, "input[name=base_url]")
	if !strings.Contains(baseURL, "generativelanguage.googleapis.com") {
		t.Errorf("base_url = %q, want the Gemini preset filled in server-side", baseURL)
	}
	model := components.GetInputValue(t, page, "input[name=model]")
	if !strings.HasPrefix(model, "gemini-") {
		t.Errorf("model = %q, want the Gemini preset's default", model)
	}
}

// PresetSwitchDoesNotClobberCustom: the enhancement script must restore the
// user's own values when they return to the provider they had configured, or
// switching to look at another option would quietly destroy their setup.
func PresetSwitchDoesNotClobberCustom(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:9999/v1", "my-local-model", "")

	actions.SelectProvider(t, page, "gemini")
	switched := components.GetInputValue(t, page, "input[name=base_url]")
	if !strings.Contains(switched, "generativelanguage.googleapis.com") {
		t.Errorf("switching to Gemini did not refill base_url, got %q", switched)
	}

	actions.SelectProvider(t, page, "custom")
	restored := components.GetInputValue(t, page, "input[name=base_url]")
	if restored != "http://localhost:9999/v1" {
		t.Errorf("switching back lost the stored custom URL: got %q", restored)
	}
	restoredModel := components.GetInputValue(t, page, "input[name=model]")
	if restoredModel != "my-local-model" {
		t.Errorf("switching back lost the stored custom model: got %q", restoredModel)
	}
}

// ClearAPIKeyRemovesIt is the counterpart to ReSaveKeepsAPIKey: the keep path
// was covered, the clear path was not, which is why a dropped error on the
// delete went unnoticed.
func ClearAPIKeyRemovesIt(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-to-be-cleared")

	placeholder := components.GetPlaceholder(t, page, "input[name=api_key]")
	if !strings.Contains(placeholder, "saved") {
		t.Fatalf("setup: expected a saved key, placeholder %q", placeholder)
	}

	actions.TickClearAPIKey(t, page)
	actions.SubmitSettings(t, page)

	flash := components.GetFlash(t, page)
	if !strings.Contains(flash, "saved") {
		t.Errorf("expected a success flash, got %q", flash)
	}

	placeholder = components.GetPlaceholder(t, page, "input[name=api_key]")
	if strings.Contains(placeholder, "saved") {
		t.Errorf("key still stored after clearing; placeholder %q", placeholder)
	}
	if n := components.CountLocators(t, page, "input[name=clear_api_key]"); n != 0 {
		t.Errorf("clear checkbox still rendered with no key stored (%d)", n)
	}
}

// ClearAndNewKeyIsRejected: asking to clear and supplying a new key at once is
// contradictory. It used to write the new key and then delete it, discarding
// what the user typed without saying anything.
func ClearAndNewKeyIsRejected(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-original")

	actions.TickClearAPIKey(t, page)
	if err := components.FillInput(page, "input[name=api_key]", "sk-replacement"); err != nil {
		t.Fatal(err)
	}
	actions.SubmitSettings(t, page)

	formErr := components.GetFormError(t, page)
	if !strings.Contains(formErr, "not both") {
		t.Errorf("expected a not-both error, got %q", formErr)
	}

	// The original key must survive a rejected submission untouched.
	placeholder := components.GetPlaceholder(t, page, "input[name=api_key]")
	if !strings.Contains(placeholder, "saved") {
		t.Errorf("rejected submission lost the stored key; placeholder %q", placeholder)
	}
}
