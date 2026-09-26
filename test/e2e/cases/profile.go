// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// Every case that changes credentials or deletes an account registers its own
// user: doing either to the shared e2e account would break every later case.

// freshUser registers a new account on a page of its own and returns both.
func freshUser(t *testing.T, prefix string) (playwright.Page, string) {
	t.Helper()
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail(prefix)
	registerThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
	return page, email
}

func fill(t *testing.T, page playwright.Page, selector, value string) {
	t.Helper()
	if err := components.FillInput(page, selector, value); err != nil {
		t.Fatal(err)
	}
}

func click(t *testing.T, page playwright.Page, selector string) {
	t.Helper()
	if err := components.ClickButton(page, selector); err != nil {
		t.Fatal(err)
	}
}

func changePasswordThrough(t *testing.T, page playwright.Page, current, next, confirm string) {
	t.Helper()
	components.NavigateTo(t, page, "/profile/security")
	fill(t, page, "input[name=current_password]", current)
	fill(t, page, "input[name=new_password]", next)
	fill(t, page, "input[name=new_password_confirm]", confirm)
	click(t, page, "button:has-text('Change Password')")
}

// isLoggedIn visits a protected page and reports whether it stayed there.
func isLoggedIn(t *testing.T, page playwright.Page) bool {
	t.Helper()
	components.NavigateTo(t, page, "/")
	return !strings.Contains(page.URL(), "/login")
}

func AccountMenuHidesEntriesUntilOpened(t *testing.T) {
	page := components.NewPage(t)
	components.NavigateTo(t, page, "/")

	if components.IsVisible(t, page, ".account-menu-list a:has-text('Profile')") {
		t.Fatal("Profile entry visible before the menu was opened")
	}

	components.ClickAccountMenuItem(t, page, "Profile")
	components.WaitForURL(t, page, "**/profile")

	if heading := components.GetHeading(t, page); heading != "Profile" {
		t.Errorf("expected heading 'Profile', got %q", heading)
	}
	if body := components.GetBody(t, page); !strings.Contains(body, configuration.TestUserEmail) {
		t.Error("profile page does not show the account email")
	}
}

func DisplayNameReplacesEmailInNav(t *testing.T) {
	page, email := freshUser(t, "name")

	if label := components.GetAccountMenuLabel(t, page); label != email {
		t.Fatalf("nav label before setting a name = %q, want the email", label)
	}

	components.NavigateTo(t, page, "/profile")
	fill(t, page, "input[name=display_name]", "  Ada  ")
	click(t, page, "main button:has-text('Save')")
	components.WaitForURL(t, page, "**/profile")

	if flash := components.GetFlash(t, page); flash != "Profile saved." {
		t.Errorf("flash = %q, want %q", flash, "Profile saved.")
	}
	if label := components.GetAccountMenuLabel(t, page); label != "Ada" {
		t.Errorf("nav label = %q, want the trimmed display name %q", label, "Ada")
	}

	// Clearing the name falls back to the email.
	components.ClearInput(page, "input[name=display_name]")
	click(t, page, "main button:has-text('Save')")
	components.WaitForURL(t, page, "**/profile")
	if label := components.GetAccountMenuLabel(t, page); label != email {
		t.Errorf("nav label after clearing = %q, want the email", label)
	}
}

func ChangePassword(t *testing.T) {
	page, email := freshUser(t, "chpw")

	changePasswordThrough(t, page, "supersecret", "brand-new-pass", "brand-new-pass")
	components.WaitForURL(t, page, "**/profile/security")
	if flash := components.GetFlash(t, page); !strings.HasPrefix(flash, "Password changed.") {
		t.Fatalf("flash = %q, want a password-changed confirmation", flash)
	}

	logOutThrough(t, page)

	loginThrough(t, page, email, "supersecret")
	if formErr := components.GetFormError(t, page); formErr != "Invalid email or password." {
		t.Errorf("old password: form error = %q, want the credential error", formErr)
	}

	loginThrough(t, page, email, "brand-new-pass")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}

func ChangePasswordWrongCurrent(t *testing.T) {
	page, email := freshUser(t, "chpwbad")

	changePasswordThrough(t, page, "not-my-password", "brand-new-pass", "brand-new-pass")

	if formErr := components.GetFormError(t, page); formErr != "Current password is incorrect." {
		t.Errorf("form error = %q, want the wrong-current error", formErr)
	}

	// Nothing changed: the original password still logs in.
	logOutThrough(t, page)
	loginThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")
}

func ChangePasswordMismatch(t *testing.T) {
	page, _ := freshUser(t, "chpwmis")

	changePasswordThrough(t, page, "supersecret", "brand-new-pass", "different-pass")

	if formErr := components.GetFormError(t, page); formErr != "Passwords do not match." {
		t.Errorf("form error = %q, want the mismatch error", formErr)
	}
}

// ChangePasswordEndsOtherSessions: a session opened with the old password
// must not survive the change, while the one that made it stays logged in.
func ChangePasswordEndsOtherSessions(t *testing.T) {
	page, email := freshUser(t, "chpwsess")
	other := helpers.NewAnonymousPage(t)
	loginThrough(t, other, email, "supersecret")
	components.WaitForURL(t, other, configuration.BaseURL+"/")

	changePasswordThrough(t, page, "supersecret", "brand-new-pass", "brand-new-pass")
	components.WaitForURL(t, page, "**/profile/security")

	if isLoggedIn(t, other) {
		t.Error("the other session survived the password change")
	}
	if !isLoggedIn(t, page) {
		t.Error("the session that changed the password was logged out")
	}
}

func SignOutEverywhereElse(t *testing.T) {
	page, email := freshUser(t, "signout")
	other := helpers.NewAnonymousPage(t)
	loginThrough(t, other, email, "supersecret")
	components.WaitForURL(t, other, configuration.BaseURL+"/")

	components.NavigateTo(t, page, "/profile/security")
	count, err := page.Locator(".session-count strong").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if count != "2" {
		t.Errorf("session count = %q, want 2", count)
	}

	click(t, page, "button:has-text('Sign Out Everywhere Else')")
	components.WaitForURL(t, page, "**/profile/security")

	if flash := components.GetFlash(t, page); flash != "Signed out of 1 other session(s)." {
		t.Errorf("flash = %q", flash)
	}
	if isLoggedIn(t, other) {
		t.Error("the other session is still logged in")
	}
	if !isLoggedIn(t, page) {
		t.Error("signing out everywhere else logged out this session too")
	}
}

func DeleteAccount(t *testing.T) {
	page, email := freshUser(t, "delete")
	helpers.CreateDeck(t, page, "Soon Gone")

	// NewAnonymousPage accepts the confirm() dialog.
	components.NavigateTo(t, page, "/profile/security")
	fill(t, page, "input#delete_password", "supersecret")
	click(t, page, "button:has-text('Delete Account')")
	components.WaitForURL(t, page, "**/login")

	if flash := components.GetFlash(t, page); !strings.Contains(flash, "deleted") {
		t.Errorf("flash = %q, want an account-deleted notice", flash)
	}

	loginThrough(t, page, email, "supersecret")
	if formErr := components.GetFormError(t, page); formErr != "Invalid email or password." {
		t.Errorf("deleted account could still log in (form error %q)", formErr)
	}
}

func DeleteAccountWrongPassword(t *testing.T) {
	page, _ := freshUser(t, "deletebad")

	components.NavigateTo(t, page, "/profile/security")
	fill(t, page, "input#delete_password", "not-my-password")
	click(t, page, "button:has-text('Delete Account')")

	if formErr := components.GetFormError(t, page); !strings.Contains(formErr, "not deleted") {
		t.Errorf("form error = %q, want the not-deleted error", formErr)
	}
	if !isLoggedIn(t, page) {
		t.Error("a rejected delete logged the user out")
	}
}
