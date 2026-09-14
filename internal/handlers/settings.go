package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) SettingsPage(c *gin.Context) {
	userID := currentUserID(c)

	cfg, err := ai.LoadConfig(h.DB, userID)
	if err != nil {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Flash: h.getFlash(c),
			Data:  SettingsData{Error: "Failed to load settings: " + err.Error()},
		})
		return
	}

	// cfg.APIKey is the *effective* key, which may come from LLM_API_KEY. The
	// form has to describe what is stored, or the "Clear saved key" box would
	// appear for a key that is not in the database at all.
	storedKey, _ := models.GetSetting(h.DB, userID, "llm.api_key")

	provider, baseURL, model := presetDefaults(cfg.Provider, cfg.BaseURL, cfg.Model)

	h.render(c, http.StatusOK, "settings.html", PageData{
		Title: "Settings",
		Flash: h.getFlash(c),
		Data: SettingsData{
			Provider:  provider,
			BaseURL:   baseURL,
			Model:     model,
			HasAPIKey: storedKey != "",
			EnvKey:    ai.APIKeyFromEnv() != "",
			Presets:   ai.Presets(),
		},
	})
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

func (h *Handler) SaveSettings(c *gin.Context) {
	userID := currentUserID(c)

	provider := c.PostForm("provider")
	// Trimmed before storing, not just before validating: a trailing space in
	// a base URL survives into every request and fails opaquely.
	baseURL := strings.TrimSpace(c.PostForm("base_url"))
	model := strings.TrimSpace(c.PostForm("model"))
	apiKey := c.PostForm("api_key")
	clearKey := c.PostForm("clear_api_key") == "1"

	preset, known := ai.PresetByID(provider)
	if !known {
		h.settingsError(c, userID, provider, baseURL, model, "Invalid provider. Choose one from the list.")
		return
	}

	// Fall back to the preset's defaults so a user who never touched these
	// fields — or who has JavaScript off and never saw them prefilled — still
	// ends up with a working configuration. Custom has no defaults, so it
	// still fails the required check below, which is the point of it.
	if baseURL == "" {
		baseURL = preset.BaseURL
	}
	if model == "" {
		model = preset.DefaultModel
	}

	if baseURL == "" || model == "" {
		h.settingsError(c, userID, provider, baseURL, model, "Base URL and Model are required.")
		return
	}

	// Contradictory input. Previously the key was written and then deleted, so
	// a key the user had just typed vanished without a word.
	if clearKey && apiKey != "" {
		h.settingsError(c, userID, provider, baseURL, model, "Enter a new API key or tick “Clear saved key”, not both.")
		return
	}

	for _, s := range []struct{ key, value string }{
		{"llm.provider", provider},
		{"llm.base_url", baseURL},
		{"llm.model", model},
	} {
		if err := models.SetSetting(h.DB, userID, s.key, s.value); err != nil {
			h.settingsError(c, userID, provider, baseURL, model, "Failed to save settings: "+err.Error())
			return
		}
	}

	if apiKey != "" {
		if err := models.SetSetting(h.DB, userID, "llm.api_key", apiKey); err != nil {
			h.settingsError(c, userID, provider, baseURL, model, "Failed to save API key: "+err.Error())
			return
		}
	}

	// Clearing a credential must not report success when it failed.
	if clearKey {
		if err := models.DeleteSetting(h.DB, userID, "llm.api_key"); err != nil {
			h.settingsError(c, userID, provider, baseURL, model, "Failed to clear API key: "+err.Error())
			return
		}
	}

	h.redirectWithFlash(c, "/settings", "Settings saved.")
}

// settingsError re-renders the form with a message. It reads back whether a
// key is actually stored rather than trusting what was typed this time round:
// reporting HasAPIKey from the submitted field made a failed save look as
// though an existing key had disappeared, hiding the "Clear saved key" box.
func (h *Handler) settingsError(c *gin.Context, userID int64, provider, baseURL, model, msg string) {
	savedKey, _ := models.GetSetting(h.DB, userID, "llm.api_key")
	h.render(c, http.StatusOK, "settings.html", PageData{
		Title: "Settings",
		Data: SettingsData{
			Provider:  provider,
			BaseURL:   baseURL,
			Model:     model,
			HasAPIKey: savedKey != "",
			EnvKey:    ai.APIKeyFromEnv() != "",
			Presets:   ai.Presets(),
			Error:     msg,
		},
	})
}

func (h *Handler) TestLLMConnection(c *gin.Context) {
	userID := currentUserID(c)

	cfg, err := ai.LoadConfig(h.DB, userID)
	if err != nil {
		h.redirectWithFlash(c, "/settings", "Connection failed: "+err.Error())
		return
	}

	provider, err := ai.New(cfg)
	if err != nil {
		h.redirectWithFlash(c, "/settings", "Connection failed: "+err.Error())
		return
	}

	err = provider.Ping(c.Request.Context())

	// The server answered and took the credentials; only the model id looks
	// wrong. Calling that a connection failure sends people to re-check a URL
	// and key that are already fine, and the listing can be a false alarm on
	// servers that accept ids they do not advertise.
	var notListed *ai.ModelNotListedError
	if errors.As(err, &notListed) {
		h.redirectWithFlash(c, "/settings", "Warning: "+notListed.Error())
		return
	}

	if err != nil {
		h.redirectWithFlash(c, "/settings", "Connection failed: "+err.Error())
		return
	}

	h.redirectWithFlash(c, "/settings", fmt.Sprintf("Connection OK — %s / %s", cfg.Provider, cfg.Model))
}
