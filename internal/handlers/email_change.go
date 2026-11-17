package handlers

import (
	"errors"
	"log"
	"strings"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// RequestEmailChange starts moving the account to a new address. It needs
// the current password (a borrowed session alone must not be enough to take
// the account's address), sends a code to the new address and a notice to
// the old one. The address changes only when the code is typed.
//
// A new address that already belongs to another account gets a notice
// instead of a code, and the page reads the same, so this can't be used to
// find out which addresses have accounts.
func (h *Handler) RequestEmailChange(c *gin.Context) {
	user := userFromContext(c)
	newEmail := models.NormalizeEmail(c.PostForm("new_email"))
	fail := func(msg string) { h.renderSecurity(c, SecurityData{EmailError: msg, NewEmail: newEmail}) }

	if !passwordMatches(user, c.PostForm("password")) {
		fail("Password is incorrect.")
		return
	}
	if !validEmail(newEmail) {
		fail("Please enter a valid email address.")
		return
	}
	if newEmail == user.Email {
		fail("That is already your email address.")
		return
	}

	code, err := models.IssueCode(h.DB, user.ID, models.CodeChangeEmail, newEmail)
	if err != nil {
		log.Printf("RequestEmailChange: IssueCode: %v", err)
		fail("Something went wrong. Please try again.")
		return
	}

	owner, err := models.GetUserByEmail(h.DB, newEmail)
	switch {
	case err != nil:
		log.Printf("RequestEmailChange: GetUserByEmail: %v", err)
	case owner != nil:
		h.send(c, addressInUseMail(newEmail))
	default:
		h.send(c, emailChangeCodeMail(newEmail, code))
	}
	h.send(c, emailChangeRequestedMail(user.Email, newEmail))

	h.redirectWithFlash(c, "/profile/security", "We sent a code to "+newEmail+". Enter it below to finish.")
}

func (h *Handler) ConfirmEmailChange(c *gin.Context) {
	user := userFromContext(c)
	code := strings.TrimSpace(c.PostForm("code"))
	fail := func(msg string) { h.renderSecurity(c, SecurityData{EmailError: msg}) }

	newEmail, err := models.ChangeEmail(h.DB, user.ID, code)
	if errors.Is(err, models.ErrCodeWrong) {
		fail("That code is not right. Check the latest email and try again.")
		return
	}
	if errors.Is(err, models.ErrCodeGone) {
		fail("That code has expired or was used up. Start the change again.")
		return
	}
	if errors.Is(err, models.ErrEmailTaken) {
		fail("That address is already in use by another account.")
		return
	}
	if err != nil {
		log.Printf("ConfirmEmailChange: %v", err)
		fail("Something went wrong. Please try again.")
		return
	}

	h.send(c, emailChangedMail(user.Email, newEmail))
	h.redirectWithFlash(c, "/profile/security", "Your email is now "+newEmail+".")
}

func (h *Handler) CancelEmailChange(c *gin.Context) {
	if err := models.CancelCode(h.DB, currentUserID(c), models.CodeChangeEmail); err != nil {
		h.redirectWithFlash(c, "/profile/security", "Failed to cancel. Please try again.")
		return
	}
	h.redirectWithFlash(c, "/profile/security", "Email change cancelled.")
}
