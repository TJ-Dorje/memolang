package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ResetData backs forgot.html and reset.html.
type ResetData struct {
	Email string
	Error string
}

func resetURL(email string) string {
	return "/reset?email=" + url.QueryEscape(email)
}

func (h *Handler) ForgotForm(c *gin.Context) {
	h.render(c, http.StatusOK, "forgot.html", PageData{
		Title: "Forgot password",
		Flash: h.getFlash(c),
		Data:  ResetData{Email: models.NormalizeEmail(c.Query("email"))},
	})
}

// Forgot mails a reset code if the account exists and always answers the
// same, so it can't be used to test which addresses have accounts. It also
// serves "send a new code" on the reset page.
func (h *Handler) Forgot(c *gin.Context) {
	email := models.NormalizeEmail(c.PostForm("email"))
	if !validEmail(email) {
		h.render(c, http.StatusOK, "forgot.html", PageData{
			Title: "Forgot password",
			Data:  ResetData{Email: email, Error: "Please enter a valid email address."},
		})
		return
	}

	if !h.takeMailSlot(c, email) {
		h.render(c, http.StatusOK, "forgot.html", PageData{
			Title: "Forgot password",
			Data:  ResetData{Email: email, Error: tooManyMails},
		})
		return
	}

	user, err := models.GetUserByEmail(h.DB, email)
	if err == nil && user != nil {
		h.sendResetCode(c, user)
	}
	h.redirectWithFlash(c, resetURL(email), "If that account exists, we sent it a code. It works for 15 minutes.")
}

func (h *Handler) sendResetCode(c *gin.Context, user *models.User) {
	code, err := models.IssueCode(h.DB, user.ID, models.CodeReset, "")
	if err != nil {
		log.Printf("Forgot: IssueCode: %v", err)
		return
	}
	h.send(c, resetCodeMail(user.Email, code))
}

func (h *Handler) ResetForm(c *gin.Context) {
	email := models.NormalizeEmail(c.Query("email"))
	if email == "" {
		c.Redirect(http.StatusSeeOther, "/forgot")
		return
	}
	h.renderReset(c, ResetData{Email: email})
}

func (h *Handler) renderReset(c *gin.Context, data ResetData) {
	h.render(c, http.StatusOK, "reset.html", PageData{
		Title: "Choose a new password",
		Flash: h.getFlash(c),
		Data:  data,
	})
}

// Reset takes the code and the new password in one form. The password is
// checked first, so a mistyped confirmation doesn't spend a code try. On
// success every old session has ended and the user is logged in fresh.
func (h *Handler) Reset(c *gin.Context) {
	email := models.NormalizeEmail(c.PostForm("email"))
	code := strings.TrimSpace(c.PostForm("code"))
	password := c.PostForm("password")
	fail := func(msg string) { h.renderReset(c, ResetData{Email: email, Error: msg}) }

	if msg := newPasswordProblem(password, c.PostForm("password_confirm")); msg != "" {
		fail(msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail("Something went wrong. Please try again.")
		return
	}

	const gone = "That code has expired or was used up. Send a new one below."
	user, err := models.GetUserByEmail(h.DB, email)
	if err != nil || user == nil {
		fail(gone)
		return
	}

	err = models.ResetPassword(h.DB, user.ID, code, string(hash))
	if errors.Is(err, models.ErrCodeWrong) {
		fail("That code is not right. Check the latest email and try again.")
		return
	}
	if errors.Is(err, models.ErrCodeGone) {
		fail(gone)
		return
	}
	if err != nil {
		log.Printf("Reset: %v", err)
		fail("Something went wrong. Please try again.")
		return
	}

	h.send(c, passwordChangedMail(user.Email))
	// A reset proves the inbox, not the phone: with MFA on, the code is
	// still asked for.
	flash := "Password changed. You're logged in, and every other session was signed out."
	if user.MFAEnabled() {
		flash = "Password changed. Enter your authenticator code to finish logging in."
	}
	h.finishPasswordStep(c, user, "/", flash)
}
