package handlers

import (
	"errors"
	"net/http"
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

func (h *Handler) Register(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	confirm := c.PostForm("password_confirm")

	renderErr := func(msg string) {
		h.render(c, http.StatusOK, "register.html", PageData{
			Title: "Register",
			Data:  AuthFormData{Email: email, Error: msg},
		})
	}

	if email == "" || !strings.Contains(email, "@") {
		renderErr("Please enter a valid email address.")
		return
	}
	if len(password) < minPasswordLen {
		renderErr("Password must be at least 8 characters.")
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		renderErr("Failed to register. Please try again.")
		return
	}

	user, err := models.CreateUser(h.DB, email, string(hash))
	if err != nil {
		if errors.Is(err, models.ErrEmailTaken) {
			renderErr("An account with that email already exists.")
			return
		}
		renderErr("Failed to register. Please try again.")
		return
	}

	if err := h.startSessionCookie(c, user.ID); err != nil {
		renderErr("Failed to start a session. Please try logging in.")
		return
	}

	c.Redirect(http.StatusSeeOther, "/")
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

	user, err := models.GetUserByEmail(h.DB, email)
	if err != nil {
		renderErr(badCreds)
		return
	}
	// An empty hash means an account with no password (reserved for future
	// OAuth-only accounts); it must never be loggable through this form.
	if user == nil || user.PasswordHash == "" {
		renderErr(badCreds)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		renderErr(badCreds)
		return
	}

	if err := h.startSessionCookie(c, user.ID); err != nil {
		renderErr("Failed to start a session. Please try again.")
		return
	}

	c.Redirect(http.StatusSeeOther, safeNext(next))
}

func (h *Handler) Logout(c *gin.Context) {
	if token, err := c.Cookie(middleware.SessionCookie); err == nil {
		models.DeleteUserSession(h.DB, token)
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookie, "", -1, "/", "", secureCookies(), true)
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

// safeNext keeps post-login redirects on this site.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/"
}
