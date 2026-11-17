// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"memolang/internal/models"
	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// uniqueEmail keeps each auth case independent of the others' accounts.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@memolang.test", prefix, time.Now().UnixNano())
}

// registerThrough signs up and types the emailed code, ending logged in.
func registerThrough(t *testing.T, page playwright.Page, email, password string) {
	t.Helper()
	submitRegistration(t, page, email, password)
	enterCode(t, page, helpers.FreshCode(t, email, models.CodeVerify))
}

// submitRegistration fills in the register form and stops on the code page.
func submitRegistration(t *testing.T, page playwright.Page, email, password string) {
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
	components.WaitForURL(t, page, "**/verify?email=**")
}

// enterCode types a code into the page's code form and submits it.
func enterCode(t *testing.T, page playwright.Page, code string) {
	t.Helper()
	if err := components.FillInput(page, "input[name=code]", code); err != nil {
		t.Fatal(err)
	}
	if err := components.ClickButton(page, "form[action='/verify'] button[type=submit]"); err != nil {
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

// logOutThrough uses the real control: Log Out lives in the account menu.
func logOutThrough(t *testing.T, page playwright.Page) {
	t.Helper()
	components.ClickAccountMenuItem(t, page, "Log Out")
	components.WaitForURL(t, page, "**/login")
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

	logOutThrough(t, page)
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
	logOutThrough(t, page)
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
	logOutThrough(t, page)
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

func RegisterWrongCode(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("wrongcode")

	submitRegistration(t, page, email, "supersecret")
	if body := components.GetBody(t, page); !strings.Contains(body, email) {
		t.Errorf("the code page should show the address the code went to, got %q", body)
	}

	code := helpers.FreshCode(t, email, models.CodeVerify)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	enterCode(t, page, wrong)

	if got := components.GetFormError(t, page); !strings.Contains(got, "not right") {
		t.Errorf("wrong code error = %q", got)
	}
	// The right code still works after one miss.
	enterCode(t, page, code)
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}

// An account that never typed its code can't log in; the right password
// leads to the code page instead, and the code finishes the login.
func LoginUnverifiedAsksForCode(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("unverified")
	submitRegistration(t, page, email, "supersecret")

	loginThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, "**/verify?email=**")

	enterCode(t, page, helpers.FreshCode(t, email, models.CodeVerify))
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}

// Registering an address that already has a verified account looks exactly
// like a new registration, and logs nobody in.
func RegisterExistingAccountSameReply(t *testing.T) {
	email := uniqueEmail("existing")
	first := helpers.NewAnonymousPage(t)
	registerThrough(t, first, email, "supersecret")

	page := helpers.NewAnonymousPage(t)
	submitRegistration(t, page, email, "another-password")
	if body := components.GetBody(t, page); !strings.Contains(body, "We sent a 6-digit code") {
		t.Errorf("expected the usual code page, got %q", body)
	}

	// The original password still works; the second one took nothing over.
	loginPage := helpers.NewAnonymousPage(t)
	loginThrough(t, loginPage, email, "another-password")
	if got := components.GetFormError(t, loginPage); got != "Invalid email or password." {
		t.Errorf("login with the would-be hijacker's password: error %q", got)
	}

	if configuration.Mail == nil {
		return
	}
	last, ok := configuration.Mail.Last(email)
	if !ok || !strings.Contains(last.Subject, "Someone tried to register") {
		t.Errorf("owner's last mail = %+v, want the someone-tried notice", last)
	}
}

// Nobody owns an unverified account yet, so registering again takes it over:
// the new password wins and a typo or squatter can't block the address.
func RegisterTakesOverUnverifiedAccount(t *testing.T) {
	email := uniqueEmail("takeover")
	squatter := helpers.NewAnonymousPage(t)
	submitRegistration(t, squatter, email, "squatter-pw")

	owner := helpers.NewAnonymousPage(t)
	registerThrough(t, owner, email, "owner-password")
	components.WaitForURL(t, owner, configuration.BaseURL+"/")

	old := helpers.NewAnonymousPage(t)
	loginThrough(t, old, email, "squatter-pw")
	if got := components.GetFormError(t, old); got != "Invalid email or password." {
		t.Errorf("the replaced password still logs in (error %q)", got)
	}
}

func VerificationCodeIsMailed(t *testing.T) {
	if configuration.Mail == nil {
		t.Skip("mail is only readable in-process")
	}
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mailed")
	submitRegistration(t, page, email, "supersecret")

	msg, ok := configuration.Mail.Last(email)
	if !ok {
		t.Fatal("no mail was sent to the new address")
	}
	// Type the code from the mail itself, not a FreshCode.
	code := strings.TrimPrefix(msg.Subject, "Your MemoLang code: ")
	if len(code) != 6 {
		t.Fatalf("subject %q doesn't carry a 6-digit code", msg.Subject)
	}
	enterCode(t, page, code)
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}
