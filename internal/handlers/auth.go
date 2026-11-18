package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	netmail "net/mail"
	"os"
	"strings"
	"time"

	"memolang/internal/middleware"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// sessionTTL is how long a login lasts, for both the DB row and the cookie.
const sessionTTL = 30 * 24 * time.Hour

const minPasswordLen = 8

func (h *Handler) RegisterForm(c *gin.Context) {
	h.render(c, http.StatusOK, "register.html", PageData{
		Title: "Register",
		Flash: h.getFlash(c),
		Data:  AuthFormData{},
	})
}

// Register always ends on the same "we sent you a code" page, whatever the
// address's state, so the form can't be used to find out who has an
// account:
//
//   - new address: create the account (unverified) and send a code;
//   - unverified account: nobody has proved owning it, so take it over with
//     the new password and send a fresh code — a typo or squatter can't
//     block the real owner;
//   - verified account: send the owner a "someone tried to register" notice
//     instead of a code.
//
// The password is hashed on every path, so all three take about as long.
func (h *Handler) Register(c *gin.Context) {
	email := models.NormalizeEmail(c.PostForm("email"))
	password := c.PostForm("password")
	confirm := c.PostForm("password_confirm")

	renderErr := func(msg string) {
		h.render(c, http.StatusOK, "register.html", PageData{
			Title: "Register",
			Data:  AuthFormData{Email: email, Error: msg},
		})
	}

	if !validEmail(email) {
		renderErr("Please enter a valid email address.")
		return
	}
	if msg := newPasswordProblem(password, confirm); msg != "" {
		renderErr(msg)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		renderErr("Failed to register. Please try again.")
		return
	}

	if !h.takeMailSlot(c, email) {
		renderErr(tooManyMails)
		return
	}
	if err := h.registerAddress(c, email, string(hash)); err != nil {
		log.Printf("Register: %v", err)
		renderErr("Failed to register. Please try again.")
		return
	}
	c.Redirect(http.StatusSeeOther, verifyURL(email))
}

// registerAddress applies the three cases described on Register.
func (h *Handler) registerAddress(c *gin.Context, email, hash string) error {
	existing, err := models.GetUserByEmail(h.DB, email)
	if err != nil {
		return err
	}
	if existing == nil {
		return h.createAndSendCode(c, email, hash)
	}
	if existing.Verified() {
		h.send(c, accountExistsMail(email))
		return nil
	}
	if err := models.ReplaceUnverifiedPassword(h.DB, existing.ID, hash); err != nil {
		return err
	}
	return h.sendVerifyCode(c, existing)
}

func (h *Handler) createAndSendCode(c *gin.Context, email, hash string) error {
	user, err := models.CreateUser(h.DB, email, hash)
	if errors.Is(err, models.ErrEmailTaken) {
		// Lost a race with a concurrent registration of the same address;
		// that one sends the code.
		return nil
	}
	if err != nil {
		return err
	}
	return h.sendVerifyCode(c, &user)
}

// validEmail is a sanity check, not proof — the emailed code is the proof.
// It rejects what can't be an address (no @, a display name, spaces).
func validEmail(email string) bool {
	addr, err := netmail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	_, domain, _ := strings.Cut(email, "@")
	return strings.Contains(domain, ".")
}

// loginUnverified sends a fresh code (if the mail limit allows) and shows
// the code page: the right password for an unconfirmed address.
func (h *Handler) loginUnverified(c *gin.Context, user *models.User) {
	if !h.takeMailSlot(c, user.Email) {
		h.redirectWithFlash(c, verifyURL(user.Email), "Confirm your email address first. "+tooManyMails)
		return
	}
	if err := h.sendVerifyCode(c, user); err != nil {
		log.Printf("Login: send verify code: %v", err)
	}
	h.redirectWithFlash(c, verifyURL(user.Email), "Confirm your email address first: we sent you a new code.")
}

func (h *Handler) LoginForm(c *gin.Context) {
	h.render(c, http.StatusOK, "login.html", PageData{
		Title: "Log In",
		Flash: h.getFlash(c),
		Data:  AuthFormData{Next: c.Query("next")},
	})
}

func (h *Handler) Login(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	next := c.PostForm("next")

	renderErr := func(msg string) {
		h.render(c, http.StatusOK, "login.html", PageData{
			Title: "Log In",
			Data:  AuthFormData{Email: email, Next: next, Error: msg},
		})
	}

	// One message for every failure: never leak whether the account exists.
	const badCreds = "Invalid email or password."

	// Checked before the password, so a locked-out guesser learns nothing
	// and costs no bcrypt. Unknown addresses count too, keyed the same.
	counters := loginCounters(c, models.NormalizeEmail(email))
	if !h.allowed(counters...) {
		renderErr(tooManyLogins)
		return
	}

	user, err := models.GetUserByEmail(h.DB, email)
	if err != nil {
		renderErr(badCreds)
		return
	}
	if !passwordMatches(user, password) {
		h.record(counters...)
		renderErr(badCreds)
		return
	}
	// Only after the right password do we say the address needs
	// confirming, so this can't be used to probe which emails exist.
	if !user.Verified() {
		h.loginUnverified(c, user)
		return
	}

	// The failures from here are cleared once the login completes,
	// including its second factor (finishPasswordStep / LoginMFA).
	h.finishPasswordStep(c, user, next, "")
}

func (h *Handler) Logout(c *gin.Context) {
	if token, err := c.Cookie(middleware.SessionCookie); err == nil {
		models.DeleteUserSession(h.DB, token)
	}
	clearSessionCookie(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

// startSessionCookie issues a session row and the matching cookie.
func (h *Handler) startSessionCookie(c *gin.Context, userID int64) error {
	token, err := models.CreateUserSession(h.DB, userID, sessionTTL)
	if err != nil {
		return err
	}
	// SameSite=Lax is the CSRF mitigation for this stage: every mutating route
	// in the app is a POST, and Lax withholds the cookie on cross-site POSTs.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookie, token, int(sessionTTL.Seconds()), "/", "", secureCookies(), true)
	return nil
}

// secureCookies is opt-in so local `task dev` over plain HTTP still works,
// while a TLS deployment sets SECURE_COOKIES=1.
func secureCookies() bool {
	return os.Getenv("SECURE_COOKIES") == "1"
}

// newPasswordProblem applies the rules for choosing a password, shared by
// registration and password change. It returns "" when the password is fine.
func newPasswordProblem(password, confirm string) string {
	if len(password) < minPasswordLen {
		return fmt.Sprintf("Password must be at least %d characters.", minPasswordLen)
	}
	if password != confirm {
		return "Passwords do not match."
	}
	return ""
}

// passwordMatches reports whether password is the user's current one. An
// empty hash (reserved for OAuth-only accounts) never matches.
func passwordMatches(u *models.User, password string) bool {
	if u == nil || u.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

// clearSessionCookie is the cookie half of logging out.
func clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookie, "", -1, "/", "", secureCookies(), true)
}

// safeNext keeps post-login redirects on this site.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/"
}
