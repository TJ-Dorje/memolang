package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"memolang/internal/mail"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// mailTimeout bounds a send made while the user waits for the page.
const mailTimeout = 10 * time.Second

// VerifyData backs verify.html: the address the code went to, shown so a
// typo is visible, and the form's error.
type VerifyData struct {
	Email string
	Error string
}

func verifyURL(email string) string {
	return "/verify?email=" + url.QueryEscape(email)
}

func (h *Handler) VerifyForm(c *gin.Context) {
	email := models.NormalizeEmail(c.Query("email"))
	if email == "" {
		c.Redirect(http.StatusSeeOther, "/register")
		return
	}
	h.renderVerify(c, VerifyData{Email: email})
}

func (h *Handler) renderVerify(c *gin.Context, data VerifyData) {
	h.render(c, http.StatusOK, "verify.html", PageData{
		Title: "Confirm your email",
		Flash: h.getFlash(c),
		Data:  data,
	})
}

// Verify checks the code and, on success, logs the user in: typing the code
// proved they own the address, and they already gave the password to get
// here. Every failure that isn't "wrong digits" reads the same, whether the
// account exists, is already verified, or the code ran out.
func (h *Handler) Verify(c *gin.Context) {
	email := models.NormalizeEmail(c.PostForm("email"))
	code := strings.TrimSpace(c.PostForm("code"))
	const gone = "That code has expired or was used up. Send a new one below."

	user, err := models.GetUserByEmail(h.DB, email)
	if err != nil || user == nil || user.Verified() {
		h.renderVerify(c, VerifyData{Email: email, Error: gone})
		return
	}

	err = models.VerifyEmail(h.DB, user.ID, code)
	if errors.Is(err, models.ErrCodeWrong) {
		h.renderVerify(c, VerifyData{Email: email, Error: "That code is not right. Check the latest email and try again."})
		return
	}
	if errors.Is(err, models.ErrCodeGone) {
		h.renderVerify(c, VerifyData{Email: email, Error: gone})
		return
	}
	if err != nil {
		log.Printf("Verify: %v", err)
		h.renderVerify(c, VerifyData{Email: email, Error: "Something went wrong. Please try again."})
		return
	}

	if err := h.startSessionCookie(c, user.ID); err != nil {
		h.redirectWithFlash(c, "/login", "Email confirmed. Please log in.")
		return
	}
	h.redirectWithFlash(c, "/", "Email confirmed. Welcome to MemoLang!")
}

// ResendVerify sends a fresh code when the address has an unverified
// account, and nothing otherwise — but always answers the same way.
func (h *Handler) ResendVerify(c *gin.Context) {
	email := models.NormalizeEmail(c.PostForm("email"))
	if !h.takeMailSlot(c, email) {
		h.redirectWithFlash(c, verifyURL(email), tooManyMails)
		return
	}

	user, err := models.GetUserByEmail(h.DB, email)
	if err == nil && user != nil && !user.Verified() {
		if err := h.sendVerifyCode(c, user); err != nil {
			log.Printf("ResendVerify: %v", err)
		}
	}
	h.redirectWithFlash(c, verifyURL(email), "We sent a new code. It works for 15 minutes.")
}

// sendVerifyCode issues a verification code and mails it. A mail failure is
// logged, not returned: the user lands on the code page either way, where
// "send a new code" retries.
func (h *Handler) sendVerifyCode(c *gin.Context, user *models.User) error {
	code, err := models.IssueCode(h.DB, user.ID, models.CodeVerify, "")
	if err != nil {
		return err
	}
	h.send(c, verifyCodeMail(user.Email, code))
	return nil
}

// send delivers a mail within mailTimeout, logging a failure. It never logs
// the body: that is where the codes are.
func (h *Handler) send(c *gin.Context, m mail.Message) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), mailTimeout)
	defer cancel()
	if err := h.Mail.Send(ctx, m); err != nil {
		log.Printf("mail to %s (%q) failed: %v", m.To, m.Subject, err)
	}
}
