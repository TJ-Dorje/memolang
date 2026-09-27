// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"fmt"
	"strings"
	"testing"

	"memolang/internal/models"
	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// ProfileLeadsToAIProvider: the LLM settings are a section of the profile,
// reached through the account menu.
func ProfileLeadsToAIProvider(t *testing.T) {
	page := components.NewPage(t)
	components.NavigateTo(t, page, "/")

	actions.NavigateToAIProvider(t, page)

	if heading := components.GetHeading(t, page); heading != "AI Providers" {
		t.Errorf("expected heading 'AI Providers', got %q", heading)
	}
}

// LegacySettingsRedirects keeps old bookmarks to /settings working.
func LegacySettingsRedirects(t *testing.T) {
	page := components.NewPage(t)

	components.NavigateTo(t, page, "/settings")
	components.WaitForURL(t, page, "**/profile/ai")

	if heading := components.GetHeading(t, page); heading != "AI Providers" {
		t.Errorf("expected /settings to land on 'AI Providers', got %q", heading)
	}
}

func SaveSettings(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	flash := actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-test-key")
	if !strings.HasPrefix(flash, "Saved") {
		t.Errorf("expected a Saved flash, got %q", flash)
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

	if components.IsChecked(t, page, "input[name=clear_api_key]") {
		t.Error("clear_api_key checkbox should not be checked")
	}

	placeholder := components.GetPlaceholder(t, page, "input[name=api_key]")
	if !strings.Contains(placeholder, "saved") {
		t.Errorf("expected api_key still saved, placeholder %q", placeholder)
	}
}

// SettingsValidation: Base URL and Model are still required where nothing can
// supply them — Custom. Presets prefill every other provider, so an empty
// submission there is meant to succeed; PresetSaveNeedsOnlyAKey covers that.
func SettingsValidation(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.OpenProviderForm(t, page)
	actions.SelectProvider(t, page, "custom")
	actions.ClearField(t, page, "input[name=base_url]")
	actions.ClearField(t, page, "input[name=model]")
	actions.SubmitSettings(t, page)

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

	actions.OpenProviderForm(t, page)

	if baseURL := components.GetInputValue(t, page, "input[name=base_url]"); baseURL == "" {
		t.Error("base_url is empty on a pristine form; expected the first preset's URL")
	}
	if model := components.GetInputValue(t, page, "input[name=model]"); model == "" {
		t.Error("model is empty on a pristine form; expected the first preset's default")
	}
}

// PresetSaveNeedsOnlyAKey is the whole point of presets: pick a provider,
// paste a key, save. The server fills the rest even with the fields blanked,
// which is also the no-JavaScript path — and names the provider after its
// preset.
func PresetSaveNeedsOnlyAKey(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.OpenProviderForm(t, page)
	actions.SelectProvider(t, page, "gemini")
	actions.ClearField(t, page, "input[name=base_url]")
	actions.ClearField(t, page, "input[name=model]")
	if err := components.FillInput(page, "input[name=api_key]", "test-key"); err != nil {
		t.Fatal(err)
	}
	flash := actions.SubmitSettings(t, page)
	if !strings.HasPrefix(flash, "Saved") {
		t.Fatalf("expected a successful save, got flash %q", flash)
	}

	if baseURL := components.GetInputValue(t, page, "input[name=base_url]"); !strings.Contains(baseURL, "generativelanguage.googleapis.com") {
		t.Errorf("base_url = %q, want the Gemini preset filled in server-side", baseURL)
	}
	if model := components.GetInputValue(t, page, "input[name=model]"); !strings.HasPrefix(model, "gemini-") {
		t.Errorf("model = %q, want the Gemini preset's default", model)
	}
	if name := components.GetInputValue(t, page, "input[name=name]"); name != "Google Gemini" {
		t.Errorf("name = %q, want the preset's short label", name)
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
	if switched := components.GetInputValue(t, page, "input[name=base_url]"); !strings.Contains(switched, "generativelanguage.googleapis.com") {
		t.Errorf("switching to Gemini did not refill base_url, got %q", switched)
	}

	actions.SelectProvider(t, page, "custom")
	if restored := components.GetInputValue(t, page, "input[name=base_url]"); restored != "http://localhost:9999/v1" {
		t.Errorf("switching back lost the stored custom URL: got %q", restored)
	}
	if restoredModel := components.GetInputValue(t, page, "input[name=model]"); restoredModel != "my-local-model" {
		t.Errorf("switching back lost the stored custom model: got %q", restoredModel)
	}
}

// ClearAPIKeyRemovesIt is the counterpart to ReSaveKeepsAPIKey: the keep path
// was covered, the clear path was not, which is why a dropped error on the
// delete once went unnoticed.
func ClearAPIKeyRemovesIt(t *testing.T) {
	page := components.NewPage(t)
	actions.ResetLLMSettings(t)

	actions.SaveSettings(t, page, "custom", "http://localhost:1234/v1", "test-model", "sk-to-be-cleared")

	if placeholder := components.GetPlaceholder(t, page, "input[name=api_key]"); !strings.Contains(placeholder, "saved") {
		t.Fatalf("setup: expected a saved key, placeholder %q", placeholder)
	}

	actions.TickClearAPIKey(t, page)
	if flash := actions.SubmitSettings(t, page); !strings.HasPrefix(flash, "Saved") {
		t.Errorf("expected a success flash, got %q", flash)
	}

	if placeholder := components.GetPlaceholder(t, page, "input[name=api_key]"); strings.Contains(placeholder, "saved") {
		t.Errorf("key still stored after clearing; placeholder %q", placeholder)
	}
	if n := components.CountLocators(t, page, "input[name=clear_api_key]"); n != 0 {
		t.Errorf("clear checkbox still rendered with no key stored (%d)", n)
	}
}

// ClearAndNewKeyIsRejected: asking to clear and supplying a new key at once is
// contradictory. It once wrote the new key and then deleted it, discarding
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

	if formErr := components.GetFormError(t, page); !strings.Contains(formErr, "not both") {
		t.Errorf("expected a not-both error, got %q", formErr)
	}

	// The original key must survive a rejected submission untouched.
	actions.OpenProviderForm(t, page)
	if placeholder := components.GetPlaceholder(t, page, "input[name=api_key]"); !strings.Contains(placeholder, "saved") {
		t.Errorf("rejected submission lost the stored key; placeholder %q", placeholder)
	}
}

// providersUser registers a fresh user with two providers on the fake LLM:
// "Home" (active, first) and "Work".
func providersUser(t *testing.T, prefix string) (page playwright.Page, email string) {
	t.Helper()
	page, email = freshUser(t, prefix)
	u, err := models.GetUserByEmail(configuration.DB, email)
	if err != nil || u == nil {
		t.Fatalf("look up %s: %v", email, err)
	}
	for _, name := range []string{"Home", "Work"} {
		_, err := models.CreateLLMProvider(configuration.DB, models.LLMProvider{
			UserID: u.ID, Name: name, Preset: "custom", BaseURL: configuration.FakeLLMURL, Model: "fake-model",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return page, email
}

func activeCardName(t *testing.T, page playwright.Page) string {
	t.Helper()
	return textOf(t, page, ".provider-active .provider-name")
}

// ProvidersSwitchActive: several saved providers, one active, switched with
// "Use This".
func ProvidersSwitchActive(t *testing.T) {
	page, _ := providersUser(t, "providers")
	components.NavigateTo(t, page, "/profile/ai")

	if n := components.CountLocators(t, page, ".provider-card"); n != 2 {
		t.Fatalf("%d provider cards, want 2", n)
	}
	if got := activeCardName(t, page); got != "Home" {
		t.Errorf("active = %q, want the first provider", got)
	}

	click(t, page, ".provider-card:not(.provider-active) button:has-text('Use This')")
	components.WaitForURL(t, page, "**/profile/ai")

	if flash := components.GetFlash(t, page); flash != "Now using “Work”." {
		t.Errorf("flash = %q", flash)
	}
	if got := activeCardName(t, page); got != "Work" {
		t.Errorf("active = %q, want Work after switching", got)
	}
	if n := components.CountLocators(t, page, ".provider-active"); n != 1 {
		t.Errorf("%d active providers, want exactly 1", n)
	}
}

// ProviderSwitcherInChat: the chat pages switch provider without leaving the
// conversation.
func ProviderSwitcherInChat(t *testing.T) {
	page, _ := providersUser(t, "switcher")
	components.NavigateTo(t, page, "/decks/new/assistant")

	if label := textOf(t, page, ".provider-switch summary"); label != "AI: Home" {
		t.Errorf("switcher label = %q, want the active provider", label)
	}

	click(t, page, ".provider-switch summary")
	click(t, page, ".provider-switch button:has-text('Work')")
	components.WaitForURL(t, page, "**/decks/new/assistant")

	if label := textOf(t, page, ".provider-switch summary"); label != "AI: Work" {
		t.Errorf("after switching, label = %q, want Work", label)
	}
	if flash := components.GetFlash(t, page); flash != "Now using “Work”." {
		t.Errorf("flash = %q", flash)
	}
}

// DeletingActiveProviderLeavesNone: nothing is silently activated in its
// place — that could start billing a different key — and the assistant says
// a provider must be chosen.
func DeletingActiveProviderLeavesNone(t *testing.T) {
	page, _ := providersUser(t, "deleteactive")
	components.NavigateTo(t, page, "/profile/ai")

	click(t, page, ".provider-active button:has-text('Delete')") // confirm() auto-accepted
	components.WaitForURL(t, page, "**/profile/ai")

	if flash := components.GetFlash(t, page); !strings.Contains(flash, "No provider is active") {
		t.Errorf("flash = %q", flash)
	}
	if n := components.CountLocators(t, page, ".provider-active"); n != 0 {
		t.Error("another provider was activated in the deleted one's place")
	}

	components.NavigateTo(t, page, "/decks/new/assistant")
	if n := components.CountLocators(t, page, ".form-error a[href='/profile/ai']"); n != 1 {
		t.Error("assistant did not ask for a provider to be chosen")
	}
	if label := textOf(t, page, ".provider-switch summary"); label != "AI: none active" {
		t.Errorf("switcher label = %q", label)
	}
}

// OtherUsersProviderIs404: provider ids are scoped to their owner.
func OtherUsersProviderIs404(t *testing.T) {
	_, email := providersUser(t, "providerowner")
	u, _ := models.GetUserByEmail(configuration.DB, email)
	list, _ := models.ListLLMProviders(configuration.DB, u.ID)

	intruder := helpers.NewAnonymousPage(t)
	registerThrough(t, intruder, uniqueEmail("providerintruder"), "supersecret")
	components.WaitForURL(t, intruder, configuration.BaseURL+"/")

	resp, err := intruder.Goto(configuration.BaseURL + fmt.Sprintf("/profile/ai/%d/edit", list[0].ID))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status() != 404 {
		t.Errorf("GET another user's provider = %d, want 404", resp.Status())
	}
}
