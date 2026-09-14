package handlers

import (
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

	h.render(c, http.StatusOK, "settings.html", PageData{
		Title: "Settings",
		Flash: h.getFlash(c),
		Data: SettingsData{
			Provider:  cfg.Provider,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			HasAPIKey: storedKey != "",
			EnvKey:    ai.APIKeyFromEnv() != "",
		},
	})
}

var validProviders = map[string]bool{
	"ollama":    true,
	"openai":    true,
	"anthropic": true,
	"custom":    true,
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

	if !validProviders[provider] {
		h.settingsError(c, userID, provider, baseURL, model, "Invalid provider. Must be one of: ollama, openai, anthropic, custom.")
		return
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

	if err := provider.Ping(c.Request.Context()); err != nil {
		h.redirectWithFlash(c, "/settings", "Connection failed: "+err.Error())
		return
	}

	h.redirectWithFlash(c, "/settings", fmt.Sprintf("Connection OK — %s / %s", cfg.Provider, cfg.Model))
}
