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

// RegisterUser creates a verified account directly in the DB and returns it.
func RegisterUser(email, password string) (models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return models.User{}, err
	}
	u, err := models.CreateUser(configuration.DB, email, string(hash))
	if err != nil {
		return models.User{}, err
	}
	code, err := models.IssueCode(configuration.DB, u.ID, models.CodeVerify, "")
	if err != nil {
		return models.User{}, err
	}
	return u, models.VerifyEmail(configuration.DB, u.ID, code)
}

// FreshCode stands in for reading the inbox: it replaces the user's live
// code for purpose with a new one and returns it. Codes are stored hashed,
// so the one actually mailed can't be read back — but the flow under test
// (type the code, get in) is the same. Works against both the in-process
// app and the integration containers, which share configuration.DB.
func FreshCode(t *testing.T, email string, purpose models.CodePurpose) string {
	t.Helper()
	u, err := models.GetUserByEmail(configuration.DB, email)
	if err != nil || u == nil {
		t.Fatalf("FreshCode: no user %q: %v", email, err)
	}
	// An email change's code carries the address it confirms; keep it.
	newEmail := ""
	if purpose == models.CodeChangeEmail {
		newEmail, err = models.PendingEmailChange(configuration.DB, u.ID)
		if err != nil || newEmail == "" {
			t.Fatalf("FreshCode: no pending email change for %q: %v", email, err)
		}
	}
	code, err := models.IssueCode(configuration.DB, u.ID, purpose, newEmail)
	if err != nil {
		t.Fatalf("FreshCode: %v", err)
	}
	return code
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
