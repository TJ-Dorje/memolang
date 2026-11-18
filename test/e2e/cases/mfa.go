// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"os"
	"strings"
	"testing"
	"time"

	"memolang/internal/models"
	"memolang/internal/totp"
	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// The phone in these tests is totp.Code on the secret read off the setup
// page. A code is accepted once per 30-second step, so each later step of a
// test uses the code for a newer step (the server accepts one step ahead).

// enableMFAThrough registers a user, turns MFA on through the UI and
// returns the secret, the recovery codes and the time the setup code was
// made for.
func enableMFAThrough(t *testing.T, page playwright.Page, email string) (secret string, codes []string) {
	t.Helper()
	registerThrough(t, page, email, "supersecret")
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	components.NavigateTo(t, page, "/profile/security")
	click(t, page, "form.mfa-setup-form button[type=submit]")
	components.WaitForURL(t, page, "**/profile/security/mfa")

	text, err := page.Locator(".mfa-secret").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	secret = strings.ReplaceAll(text, " ", "")
	if src, _ := page.Locator(".mfa-qr img").GetAttribute("src"); !strings.HasPrefix(src, "data:image/png") {
		t.Errorf("QR src = %.30q…, want a PNG data URI", src)
	}

	fill(t, page, "input[name=code]", phoneCode(t, secret, 0))
	click(t, page, "form[action='/profile/security/mfa'] button[type=submit]")

	codes, err = page.Locator(".recovery-codes code").AllTextContents()
	if err != nil || len(codes) != 10 {
		t.Fatalf("recovery codes on the page: %v, %v; want 10", codes, err)
	}
	return secret, codes
}

// phoneCode is the authenticator's code stepsAhead 30-second steps from now.
func phoneCode(t *testing.T, secret string, stepsAhead int) string {
	t.Helper()
	code, err := totp.Code(secret, time.Now().Add(time.Duration(stepsAhead)*30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func enterLoginCode(t *testing.T, page playwright.Page, code string) {
	t.Helper()
	fill(t, page, "input[name=code]", code)
	click(t, page, "form[action='/login/mfa'] button[type=submit]")
}

func MFALoginNeedsCode(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mfa")
	secret, _ := enableMFAThrough(t, page, email)

	fresh := helpers.NewAnonymousPage(t)
	loginThrough(t, fresh, email, "supersecret")
	components.WaitForURL(t, fresh, "**/login/mfa")

	// Without the code there is no session yet.
	components.NavigateTo(t, fresh, "/")
	components.WaitForURL(t, fresh, "**/login**")

	loginThrough(t, fresh, email, "supersecret")
	components.WaitForURL(t, fresh, "**/login/mfa")
	enterLoginCode(t, fresh, phoneCode(t, secret, 1))
	components.WaitForURL(t, fresh, configuration.BaseURL+"/")
}

func MFARecoveryCodeWorksOnce(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mfa-recovery")
	_, codes := enableMFAThrough(t, page, email)

	first := helpers.NewAnonymousPage(t)
	loginThrough(t, first, email, "supersecret")
	enterLoginCode(t, first, strings.ToUpper(codes[0]))
	components.WaitForURL(t, first, configuration.BaseURL+"/")
	if flash := components.GetFlash(t, first); !strings.Contains(flash, "9 left") {
		t.Errorf("flash after a recovery code = %q, want the count left", flash)
	}

	again := helpers.NewAnonymousPage(t)
	loginThrough(t, again, email, "supersecret")
	enterLoginCode(t, again, codes[0])
	if got := components.GetFormError(t, again); !strings.Contains(got, "isn't right") {
		t.Errorf("reused recovery code: error %q", got)
	}
}

func MFAWrongCodesEndTheLogin(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mfa-wrong")
	secret, _ := enableMFAThrough(t, page, email)
	wrong := "000000"
	if wrong == phoneCode(t, secret, 0) || wrong == phoneCode(t, secret, 1) || wrong == phoneCode(t, secret, -1) {
		wrong = "111111"
	}

	fresh := helpers.NewAnonymousPage(t)
	loginThrough(t, fresh, email, "supersecret")
	for range 4 {
		enterLoginCode(t, fresh, wrong)
	}
	if got := components.GetFormError(t, fresh); !strings.Contains(got, "1 try left") {
		t.Errorf("after 4 wrong codes: error %q", got)
	}
	enterLoginCode(t, fresh, wrong)
	components.WaitForURL(t, fresh, "**/login")
	if flash := components.GetFlash(t, fresh); !strings.Contains(flash, "Too many wrong codes") {
		t.Errorf("after 5 wrong codes: flash %q", flash)
	}
}

func MFADisable(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mfa-off")
	_, codes := enableMFAThrough(t, page, email)
	click(t, page, "a[href='/profile/security']")

	fill(t, page, "#mfa_password", "supersecret")
	fill(t, page, "#mfa_code", codes[0])
	click(t, page, "button[formaction='/profile/security/mfa/disable']")
	components.WaitForURL(t, page, "**/profile/security")
	if flash := components.GetFlash(t, page); !strings.Contains(flash, "Two-factor login is off") {
		t.Errorf("flash = %q", flash)
	}

	fresh := helpers.NewAnonymousPage(t)
	loginThrough(t, fresh, email, "supersecret")
	components.WaitForURL(t, fresh, configuration.BaseURL+"/")
}

func MFARecoveryCodesDownload(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	_, codes := enableMFAThrough(t, page, uniqueEmail("mfa-download"))

	download, err := page.ExpectDownload(func() error {
		return components.ClickButton(page, "form.codes-download-form button[type=submit]")
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := download.Path()
	if err != nil {
		t.Fatal(err)
	}
	body := readFile(t, path)
	for _, code := range codes {
		if !strings.Contains(body, code) {
			t.Errorf("downloaded file is missing %s:\n%s", code, body)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A reset proves the inbox, not the phone: it must not skip the second
// factor.
func MFAResetStillAsksCode(t *testing.T) {
	page := helpers.NewAnonymousPage(t)
	email := uniqueEmail("mfa-reset")
	secret, _ := enableMFAThrough(t, page, email)

	fresh := helpers.NewAnonymousPage(t)
	requestReset(t, fresh, email)
	fill(t, fresh, "input[name=code]", helpers.FreshCode(t, email, models.CodeReset))
	fill(t, fresh, "input[name=password]", "new-password")
	fill(t, fresh, "input[name=password_confirm]", "new-password")
	click(t, fresh, "form[action='/reset'] button[type=submit]")
	components.WaitForURL(t, fresh, "**/login/mfa")

	enterLoginCode(t, fresh, phoneCode(t, secret, 1))
	components.WaitForURL(t, fresh, configuration.BaseURL+"/")
}
