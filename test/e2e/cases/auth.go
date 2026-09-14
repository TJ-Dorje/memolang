// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// uniqueEmail keeps each auth case independent of the others' accounts.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@memolang.test", prefix, time.Now().UnixNano())
}

func registerThrough(t *testing.T, page playwright.Page, email, password string) {
	t.Helper()
	components.NavigateTo(t, page, "/register")
	if err := components.FillInput(page, "input[name=email]", email); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=password]", password); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=password_confirm]", password); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
}

func loginThrough(t *testing.T, page playwright.Page, email, password string) {
	t.Helper()
	components.NavigateTo(t, page, "/login")
	if err := components.FillInput(page, "input[name=email]", email); err != nil {
		t.Fatal(err)
	}
	if err := components.FillInput(page, "input[name=password]", password); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "button[type=submit]"); err != nil {
		t.Fatal(err)
	}
}

func Register(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("register")

	registerThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	body := components.GetBody(t, page)
	if !strings.Contains(body, email) {
		t.Errorf("expected the nav to show %q after registering, got %q", email, body)
	}
}

func Logout(t *testing.T) {
	page := helpers.NewAnonymousPage(t)

	registerThrough(t, page, uniqueEmail("logout"), "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	if err := components.ClickButton(page, "nav button:has-text('Log Out')"); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, "**/login")

	// The session is gone, so a protected route bounces back to login.
	components.NavigateTo(t, page, "/")
	components.WaitForURL(t, page, "**/login**")
}

func LoginWrongPassword(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("wrongpw")

	registerThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
	if err := components.ClickButton(page, "nav button:has-text('Log Out')"); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, "**/login")

	loginThrough(t, page, email, "not-the-password")

	formErr := components.GetFormError(t, page)
	if formErr != "Invalid email or password." {
		t.Errorf("expected the generic credential error, got %q", formErr)
	}
	if !strings.Contains(page.URL(), "/login") {
		t.Errorf("expected to stay on /login, got %q", page.URL())
	}
}

func LoginEmailCaseInsensitive(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("Case")

	registerThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
	if err := components.ClickButton(page, "nav button:has-text('Log Out')"); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, "**/login")

	loginThrough(t, page, strings.ToUpper(email), "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}

func ProtectedRouteWithoutSession(t *testing.T) {
	page := helpers.NewAnonymousPage(t)

	components.NavigateTo(t, page, "/")
	components.WaitForURL(t, page, "**/login**")

	heading := components.GetHeading(t, page)
	if heading != "Log In" {
		t.Errorf("expected the login page, got heading %q", heading)
	}
}

// DecksArePrivatePerUser is the core IDOR check: one user's deck must be
// invisible, and un-addressable, to another.
func DecksArePrivatePerUser(t *testing.T) {
	owner := helpers.NewAnonymousPage(t)
	registerThrough(t, owner, uniqueEmail("owner"), "supersecret")
	components.WaitForURL(t, owner, configuration.BaseURL+"/")
	deckURL := helpers.CreateDeck(t, owner, "Private Deck")

	intruder := helpers.NewAnonymousPage(t)
	registerThrough(t, intruder, uniqueEmail("intruder"), "supersecret")
	components.WaitForURL(t, intruder, configuration.BaseURL+"/")

	dashboard := components.GetBody(t, intruder)
	if strings.Contains(dashboard, "Private Deck") {
		t.Error("the other user's deck is listed on this user's dashboard")
	}

	resp, err := intruder.Goto(deckURL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status() != 404 {
		t.Errorf("GET another user's deck = %d, want 404", resp.Status())
	}
}
