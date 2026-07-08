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
	cfg, err := ai.LoadConfig(h.DB)
	if err != nil {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Flash: h.getFlash(c),
			Data:  SettingsData{Error: "Failed to load settings: " + err.Error()},
		})
		return
	}

	h.render(c, http.StatusOK, "settings.html", PageData{
		Title: "Settings",
		Flash: h.getFlash(c),
		Data: SettingsData{
			Provider:  cfg.Provider,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			HasAPIKey: cfg.APIKey != "",
		},
	})
}

func (h *Handler) SaveSettings(c *gin.Context) {
	provider := c.PostForm("provider")
	baseURL := c.PostForm("base_url")
	apiKey := c.PostForm("api_key")
	model := c.PostForm("model")
	clearKey := c.PostForm("clear_api_key")

	validProviders := map[string]bool{
		"ollama":   true,
		"openai":   true,
		"anthropic": true,
		"custom":   true,
	}

	if !validProviders[provider] {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: apiKey != "", Error: "Invalid provider. Must be one of: ollama, openai, anthropic, custom."},
		})
		return
	}

	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: apiKey != "", Error: "Base URL and Model are required."},
		})
		return
	}

	if err := models.SetSetting(h.DB, "llm.provider", provider); err != nil {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: apiKey != "", Error: "Failed to save provider: " + err.Error()},
		})
		return
	}

	if err := models.SetSetting(h.DB, "llm.base_url", baseURL); err != nil {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: apiKey != "", Error: "Failed to save base URL: " + err.Error()},
		})
		return
	}

	if err := models.SetSetting(h.DB, "llm.model", model); err != nil {
		h.render(c, http.StatusOK, "settings.html", PageData{
			Title: "Settings",
			Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: apiKey != "", Error: "Failed to save model: " + err.Error()},
		})
		return
	}

	if apiKey != "" {
		if err := models.SetSetting(h.DB, "llm.api_key", apiKey); err != nil {
			h.render(c, http.StatusOK, "settings.html", PageData{
				Title: "Settings",
				Data:  SettingsData{Provider: provider, BaseURL: baseURL, Model: model, HasAPIKey: true, Error: "Failed to save API key: " + err.Error()},
			})
			return
		}
	}

	if clearKey == "1" {
		models.DeleteSetting(h.DB, "llm.api_key")
	}

	h.redirectWithFlash(c, "/settings", "Settings saved.")
}

func (h *Handler) TestLLMConnection(c *gin.Context) {
	cfg, err := ai.LoadConfig(h.DB)
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
