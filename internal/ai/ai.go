package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

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

	// Chat continues a conversation, streaming the reply to onToken as it is
	// generated, and returns the full reply. It sets no deadline of its own:
	// the caller decides how long a reply may take through ctx.
	Chat(ctx context.Context, req ChatRequest, onToken TokenFunc) (string, error)
}

// ErrNotConfigured is returned by New when no provider is set.
var ErrNotConfigured = errors.New("llm provider not configured")

// ModelNotListedError reports that the server answered and accepted the
// credentials, but does not advertise the configured model. Connectivity and
// auth are fine; the model id is most likely wrong or out of date.
//
// It is deliberately a distinct type rather than a plain error: callers should
// present it as a warning, not as "connection failed". Some servers also list
// models under ids that differ from the ones they accept, so this can be a
// false alarm and must never block a configuration the user insists on.
type ModelNotListedError struct {
	Model     string
	Available []string
}

func (e *ModelNotListedError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("connected, but the server does not list model %q", e.Model)
	}

	shown := e.Available
	suffix := ""
	if len(shown) > 5 {
		suffix = fmt.Sprintf(" (+%d more)", len(shown)-5)
		shown = shown[:5]
	}
	return fmt.Sprintf("connected, but the server does not list model %q. Available: %s%s",
		e.Model, strings.Join(shown, ", "), suffix)
}

// APIKeyEnvVar overrides the stored key when set. It exists so a key with real
// billing attached never has to be written to the settings table, which holds
// values in plaintext.
const APIKeyEnvVar = "LLM_API_KEY"

// APIKeyFromEnv returns the override, or "" when unset. Callers use it to tell
// the user which key is actually in effect.
func APIKeyFromEnv() string {
	return os.Getenv(APIKeyEnvVar)
}

// LoadConfig returns the user's active provider as a Config; an empty Config
// (which New rejects with ErrNotConfigured) when none is active.
func LoadConfig(db *sql.DB, userID int64) (Config, error) {
	p, err := models.GetActiveLLMProvider(db, userID)
	if err != nil {
		return Config{}, err
	}
	if p == nil {
		return Config{}, nil
	}
	return ConfigOf(*p), nil
}

// ConfigOf turns a saved provider into a Config, with LLM_API_KEY taking
// precedence over its stored key. The override is process wide and
// deliberately not per-user: it is a single-operator escape hatch for keeping
// a key out of the database, not a way to configure accounts.
func ConfigOf(p models.LLMProvider) Config {
	apiKey := p.APIKey
	if env := APIKeyFromEnv(); env != "" {
		apiKey = env
	}
	return Config{
		Provider: p.Preset,
		BaseURL:  p.BaseURL,
		APIKey:   apiKey,
		Model:    p.Model,
	}
}

// New builds a Provider from config. Which client to use, and whether a key is
// required, both come from the provider's preset rather than from a switch
// that has to be kept in step with the list by hand.
func New(cfg Config) (Provider, error) {
	if cfg.Provider == "" {
		return nil, ErrNotConfigured
	}

	preset, ok := PresetByID(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}

	// Anthropic supplies its own base URL when none is stored.
	if cfg.BaseURL == "" && preset.Transport != "anthropic" {
		return nil, fmt.Errorf("BaseURL and Model are required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("BaseURL and Model are required")
	}

	if preset.NeedsKey && cfg.APIKey == "" {
		return nil, fmt.Errorf("APIKey is required for provider %q", cfg.Provider)
	}

	if preset.Transport == "anthropic" {
		return newAnthropic(cfg), nil
	}
	return newOpenAICompat(cfg), nil
}
