package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// A user keeps any number of LLM providers under Profile → AI Providers and
// switches which one is active; the assistant and card generation use the
// active one (ai.LoadConfig).

const providersURL = "/profile/ai"

func providerURL(id int64, action string) string {
	return fmt.Sprintf("%s/%d/%s", providersURL, id, action)
}

// ProvidersPage lists the user's providers.
func (h *Handler) ProvidersPage(c *gin.Context) {
	list, err := models.ListLLMProviders(h.DB, currentUserID(c))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load providers")
		return
	}

	views := make([]ProviderView, len(list))
	for i, p := range list {
		views[i] = providerView(p)
	}
	h.render(c, http.StatusOK, "profile_ai.html", PageData{
		Title: "AI Providers",
		Flash: h.getFlash(c),
		Data:  ProvidersData{Providers: views, EnvKey: ai.APIKeyFromEnv() != ""},
	})
}

// providerView is what the list shows. The key itself never reaches a
// template, only whether one is stored.
func providerView(p models.LLMProvider) ProviderView {
	label := p.Preset
	if preset, ok := ai.PresetByID(p.Preset); ok {
		label = preset.Label
	}
	return ProviderView{
		ID: p.ID, Name: p.Name, PresetLabel: label,
		BaseURL: p.BaseURL, Model: p.Model,
		HasAPIKey: p.APIKey != "", Active: p.Active,
		DisableThinking: p.DisableThinking,
	}
}

// NewProviderForm shows a pristine form, prefilled from the first preset so
// a first-time user sees a working base URL and model rather than two empty
// boxes.
func (h *Handler) NewProviderForm(c *gin.Context) {
	preset, baseURL, model := presetDefaults("", "", "")
	h.renderProviderForm(c, SettingsData{Provider: preset, BaseURL: baseURL, Model: model})
}

func (h *Handler) EditProviderForm(c *gin.Context) {
	p, ok := h.ownedProvider(c)
	if !ok {
		return
	}
	preset, baseURL, model := presetDefaults(p.Preset, p.BaseURL, p.Model)
	h.renderProviderForm(c, SettingsData{
		ID: p.ID, Name: p.Name, Provider: preset, BaseURL: baseURL, Model: model,
		HasAPIKey: p.APIKey != "", DisableThinking: p.DisableThinking,
	})
}

func (h *Handler) renderProviderForm(c *gin.Context, data SettingsData) {
	data.EnvKey = ai.APIKeyFromEnv() != ""
	data.Presets = ai.Presets()
	title := "Add AI Provider"
	if data.ID != 0 {
		title = "Edit AI Provider"
	}
	h.render(c, http.StatusOK, "provider_form.html", PageData{Title: title, Flash: h.getFlash(c), Data: data})
}

// ownedProvider loads the provider named in the URL, answering 404 for one
// that is not the current user's. ok is false when the response is written.
func (h *Handler) ownedProvider(c *gin.Context) (models.LLMProvider, bool) {
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid provider ID")
		return models.LLMProvider{}, false
	}
	p, err := models.GetLLMProvider(h.DB, currentUserID(c), id)
	if errors.Is(err, sql.ErrNoRows) {
		c.String(http.StatusNotFound, "Provider not found")
		return models.LLMProvider{}, false
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load provider")
		return models.LLMProvider{}, false
	}
	return p, true
}

// providerForm is a submitted provider form, normalised.
type providerForm struct {
	name, preset, baseURL, model, apiKey string
	clearKey, disableThinking            bool
}

func readProviderForm(c *gin.Context) providerForm {
	return providerForm{
		name:   strings.TrimSpace(c.PostForm("name")),
		preset: c.PostForm("provider"),
		// Trimmed before storing, not just before validating: a trailing
		// space in a base URL survives into every request and fails opaquely.
		baseURL:  strings.TrimSpace(c.PostForm("base_url")),
		model:    strings.TrimSpace(c.PostForm("model")),
		apiKey:   c.PostForm("api_key"),
		clearKey: c.PostForm("clear_api_key") == "1",

		disableThinking: c.PostForm("disable_thinking") == "1",
	}
}

// validate applies the form's rules and fills what the preset can supply,
// returning a message for the learner when something is wrong.
func (f *providerForm) validate() string {
	preset, known := ai.PresetByID(f.preset)
	if !known {
		return "Invalid provider. Choose one from the list."
	}

	// Fall back to the preset's defaults so a user who never touched these
	// fields — or who has JavaScript off and never saw them prefilled — still
	// ends up with a working configuration. Custom has no defaults, so it
	// still fails the required check below, which is the point of it.
	if f.baseURL == "" {
		f.baseURL = preset.BaseURL
	}
	if f.model == "" {
		f.model = preset.DefaultModel
	}
	if f.baseURL == "" || f.model == "" {
		return "Base URL and Model are required."
	}
	if f.name == "" {
		f.name = shortLabel(preset.Label)
	}

	// Contradictory input. It once wrote the key and then deleted it, so a
	// key the user had just typed vanished without a word.
	if f.clearKey && f.apiKey != "" {
		return "Enter a new API key or tick “Clear saved key”, not both."
	}
	return ""
}

// shortLabel drops a preset label's parenthetical ("Groq (free tier)" →
// "Groq") to make a default provider name.
func shortLabel(label string) string {
	if i := strings.Index(label, " ("); i > 0 {
		return label[:i]
	}
	return label
}

// formError re-renders the form with a message, keeping what was typed —
// except the key, which is never echoed back — and reporting whether a key
// is actually stored rather than trusting this submission.
func (h *Handler) formError(c *gin.Context, id int64, f providerForm, hasKey bool, msg string) {
	h.renderProviderForm(c, SettingsData{
		ID: id, Name: f.name, Provider: f.preset, BaseURL: f.baseURL, Model: f.model,
		HasAPIKey: hasKey, DisableThinking: f.disableThinking, Error: msg,
	})
}

func (h *Handler) CreateProvider(c *gin.Context) {
	f := readProviderForm(c)
	if msg := f.validate(); msg != "" {
		h.formError(c, 0, f, false, msg)
		return
	}

	_, err := models.CreateLLMProvider(h.DB, models.LLMProvider{
		UserID: currentUserID(c), Name: f.name, Preset: f.preset,
		BaseURL: f.baseURL, Model: f.model, APIKey: f.apiKey,
		DisableThinking: f.disableThinking,
	})
	if err != nil {
		h.formError(c, 0, f, false, "Failed to save the provider: "+err.Error())
		return
	}
	h.redirectWithFlash(c, providersURL, "Saved “"+f.name+"”.")
}

func (h *Handler) UpdateProvider(c *gin.Context) {
	p, ok := h.ownedProvider(c)
	if !ok {
		return
	}
	f := readProviderForm(c)
	if msg := f.validate(); msg != "" {
		h.formError(c, p.ID, f, p.APIKey != "", msg)
		return
	}

	// A blank key field keeps the stored key; only a new value or the clear
	// box changes it.
	key := p.APIKey
	if f.apiKey != "" {
		key = f.apiKey
	}
	if f.clearKey {
		key = ""
	}

	p.Name, p.Preset, p.BaseURL, p.Model, p.APIKey = f.name, f.preset, f.baseURL, f.model, key
	p.DisableThinking = f.disableThinking
	if err := models.UpdateLLMProvider(h.DB, p); err != nil {
		h.formError(c, p.ID, f, p.APIKey != "", "Failed to save the provider: "+err.Error())
		return
	}
	h.redirectWithFlash(c, providersURL, "Saved “"+f.name+"”.")
}

// ActivateProvider switches the active provider. It is posted from the list
// and from the switcher on the assistant's pages, which send `next` so the
// learner stays in their conversation.
func (h *Handler) ActivateProvider(c *gin.Context) {
	p, ok := h.ownedProvider(c)
	if !ok {
		return
	}
	back := providersURL
	if next := c.PostForm("next"); next != "" {
		back = safeNext(next)
	}
	if err := models.ActivateLLMProvider(h.DB, p.UserID, p.ID); err != nil {
		h.redirectWithFlash(c, back, "Could not switch provider. Try again.")
		return
	}
	h.redirectWithFlash(c, back, "Now using “"+p.Name+"”.")
}

func (h *Handler) DeleteProvider(c *gin.Context) {
	p, ok := h.ownedProvider(c)
	if !ok {
		return
	}
	if err := models.DeleteLLMProvider(h.DB, p.UserID, p.ID); err != nil {
		h.redirectWithFlash(c, providersURL, "Could not delete the provider. Try again.")
		return
	}
	msg := "Deleted “" + p.Name + "”."
	if p.Active {
		msg += " No provider is active now — choose one with “Use this”."
	}
	h.redirectWithFlash(c, providersURL, msg)
}

// TestProvider checks one saved provider — not necessarily the active one,
// so a new configuration can be tried before switching to it.
func (h *Handler) TestProvider(c *gin.Context) {
	p, ok := h.ownedProvider(c)
	if !ok {
		return
	}
	prefix := p.Name + ": "

	provider, err := ai.New(ai.ConfigOf(p))
	if err != nil {
		h.redirectWithFlash(c, providersURL, prefix+"Connection failed: "+err.Error())
		return
	}
	err = provider.Ping(c.Request.Context())

	// The server answered and took the credentials; only the model id looks
	// wrong. Calling that a connection failure sends people to re-check a URL
	// and key that are already fine, and the listing can be a false alarm on
	// servers that accept ids they do not advertise.
	var notListed *ai.ModelNotListedError
	if errors.As(err, &notListed) {
		h.redirectWithFlash(c, providersURL, prefix+"Warning: "+notListed.Error())
		return
	}
	if err != nil {
		h.redirectWithFlash(c, providersURL, prefix+"Connection failed: "+err.Error())
		return
	}
	h.redirectWithFlash(c, providersURL, fmt.Sprintf("%sConnection OK — %s", prefix, p.Model))
}

// presetDefaults fills a blank form from the selected provider's preset, so a
// first-time user sees a working base URL and model rather than two empty
// boxes. Stored values always win — a preset never overwrites a real choice.
func presetDefaults(provider, baseURL, model string) (string, string, string) {
	if provider == "" {
		provider = ai.Presets()[0].ID
	}

	preset, ok := ai.PresetByID(provider)
	if !ok {
		return provider, baseURL, model
	}

	if baseURL == "" {
		baseURL = preset.BaseURL
	}
	if model == "" {
		model = preset.DefaultModel
	}
	return provider, baseURL, model
}
