package configuration

import (
	"database/sql"
	"os"

	playwright "github.com/playwright-community/playwright-go"
)

var (
	BaseURL string
	Browser playwright.Browser
	DB      *sql.DB
)

func IsHeaded() bool {
	return os.Getenv("HEADED") == "true"
}
