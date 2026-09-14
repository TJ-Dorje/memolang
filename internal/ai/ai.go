package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"memolang/internal/models"
)

// CardData is unchanged from today.
type CardData struct {
	Front   string `json:"front"`
	Back    string `json:"back"`
	Example string `json:"example"`
}

// Config holds LLM connection settings loaded from the DB.
type Config struct {
	Provider string // "ollama" | "openai" | "anthropic" | "custom"
	BaseURL  string
	APIKey   string
	Model    string
}

// Provider generates flashcards from a language + topic prompt.
type Provider interface {
	// GenerateCards returns the generated cards. Implementations must apply
	// a 3-minute timeout via context.WithTimeout.
	GenerateCards(ctx context.Context, language, promptText string) ([]CardData, error)

	// Ping performs a minimal request to verify connectivity and auth
	// (5-second timeout). Used by the settings "Test connection" button.
	Ping(ctx context.Context) error
}

// ErrNotConfigured is returned by New when no provider is set.
var ErrNotConfigured = errors.New("llm provider not configured")

// LoadConfig reads one user's llm.* keys via models.GetSettings.
func LoadConfig(db *sql.DB, userID int64) (Config, error) {
	settings, err := models.GetSettings(db, userID, "llm.")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Provider: settings["llm.provider"],
		BaseURL:  settings["llm.base_url"],
		APIKey:   settings["llm.api_key"],
		Model:    settings["llm.model"],
	}, nil
}

// New builds a Provider from config.
func New(cfg Config) (Provider, error) {
	if cfg.Provider == "" {
		return nil, ErrNotConfigured
	}

	// Validation: BaseURL and Model must be non-empty
	if cfg.BaseURL == "" && cfg.Provider != "anthropic" {
		return nil, fmt.Errorf("BaseURL and Model are required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("BaseURL and Model are required")
	}

	// APIKey must be non-empty for "openai" and "anthropic"
	if (cfg.Provider == "openai" || cfg.Provider == "anthropic") && cfg.APIKey == "" {
		return nil, fmt.Errorf("APIKey is required for provider %q", cfg.Provider)
	}

	switch cfg.Provider {
	case "anthropic":
		return newAnthropic(cfg), nil
	case "ollama", "openai", "custom":
		return newOpenAICompat(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}
