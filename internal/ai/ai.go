package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

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

// APIKeyEnvVar overrides the stored key when set. It exists so a key with real
// billing attached never has to be written to the settings table, which holds
// values in plaintext.
const APIKeyEnvVar = "LLM_API_KEY"

// APIKeyFromEnv returns the override, or "" when unset. Callers use it to tell
// the user which key is actually in effect.
func APIKeyFromEnv() string {
	return os.Getenv(APIKeyEnvVar)
}

// LoadConfig reads one user's llm.* keys via models.GetSettings, with
// LLM_API_KEY taking precedence over the stored key. The override is process
// wide and deliberately not per-user: it is a single-operator escape hatch for
// keeping a key out of the database, not a way to configure accounts.
func LoadConfig(db *sql.DB, userID int64) (Config, error) {
	settings, err := models.GetSettings(db, userID, "llm.")
	if err != nil {
		return Config{}, err
	}

	apiKey := settings["llm.api_key"]
	if env := APIKeyFromEnv(); env != "" {
		apiKey = env
	}

	return Config{
		Provider: settings["llm.provider"],
		BaseURL:  settings["llm.base_url"],
		APIKey:   apiKey,
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
