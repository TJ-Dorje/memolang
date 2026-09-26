package configuration

import (
	"database/sql"
	"os"

	playwright "github.com/playwright-community/playwright-go"
)

var (
	BaseURL string
	// FakeLLMURL is the base URL of the fake OpenAI-compatible server
	// TestMain starts, for cases that need an AI provider.
	FakeLLMURL string
	Browser playwright.Browser
	DB      *sql.DB

	// Context is a browser context already carrying the shared test user's
	// session cookie, so every case runs logged in without its own login
	// flow. Auth cases build their own logged-out context instead.
	Context playwright.BrowserContext
)

// The account every case shares. Auth cases register their own users.
const (
	TestUserEmail    = "e2e@memolang.test"
	TestUserPassword = "e2e-password"
)

func IsHeaded() bool {
	return os.Getenv("HEADED") == "true"
}
