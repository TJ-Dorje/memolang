package helpers

import (
	"fmt"
	"testing"
	"time"

	"memolang/internal/models"
	"memolang/test/e2e/configuration"

	playwright "github.com/playwright-community/playwright-go"
	"golang.org/x/crypto/bcrypt"
)

// e2eSessionTTL only has to outlive one test run.
const e2eSessionTTL = time.Hour

// RegisterUser creates an account directly in the DB and returns it.
func RegisterUser(email, password string) (models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return models.User{}, err
	}
	return models.CreateUser(configuration.DB, email, string(hash))
}

// NewLoggedInContext registers the shared test user and returns a browser
// context holding its session cookie. Called once from TestMain so the cases
// that predate auth keep working untouched.
func NewLoggedInContext() (playwright.BrowserContext, error) {
	user, err := RegisterUser(configuration.TestUserEmail, configuration.TestUserPassword)
	if err != nil {
		return nil, fmt.Errorf("register e2e user: %w", err)
	}

	token, err := models.CreateUserSession(configuration.DB, user.ID, e2eSessionTTL)
	if err != nil {
		return nil, fmt.Errorf("create e2e session: %w", err)
	}

	ctx, err := configuration.Browser.NewContext()
	if err != nil {
		return nil, fmt.Errorf("new browser context: %w", err)
	}
	if err := ctx.AddCookies([]playwright.OptionalCookie{{
		Name:  "session",
		Value: token,
		URL:   playwright.String(configuration.BaseURL),
	}}); err != nil {
		return nil, fmt.Errorf("set session cookie: %w", err)
	}
	return ctx, nil
}

// NewAnonymousPage returns a page in a fresh context with no session cookie,
// for cases that exercise the logged-out state.
func NewAnonymousPage(t *testing.T) playwright.Page {
	t.Helper()
	ctx, err := configuration.Browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.OnDialog(func(d playwright.Dialog) { d.Accept() })
	t.Cleanup(func() { ctx.Close() })
	return page
}
