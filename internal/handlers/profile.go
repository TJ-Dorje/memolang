package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"memolang/internal/middleware"
	"memolang/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const maxDisplayNameLen = 50

func (h *Handler) ProfilePage(c *gin.Context) {
	h.renderProfile(c, ProfileData{})
}

func (h *Handler) renderProfile(c *gin.Context, data ProfileData) {
	h.render(c, http.StatusOK, "profile.html", PageData{
		Title: "Profile",
		Flash: h.getFlash(c),
		Data:  data,
	})
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("display_name"))

	if utf8.RuneCountInString(name) > maxDisplayNameLen {
		h.renderProfile(c, ProfileData{
			DisplayName: name,
			Error:       fmt.Sprintf("Display name must be at most %d characters.", maxDisplayNameLen),
		})
		return
	}

	if err := models.UpdateDisplayName(h.DB, currentUserID(c), name); err != nil {
		h.renderProfile(c, ProfileData{DisplayName: name, Error: "Failed to save profile. Please try again."})
		return
	}

	h.redirectWithFlash(c, "/profile", "Profile saved.")
}

func (h *Handler) SecurityPage(c *gin.Context) {
	h.renderSecurity(c, SecurityData{})
}

// renderSecurity fills in the session count itself, so every error path shows
// the same page as a clean load.
func (h *Handler) renderSecurity(c *gin.Context, data SecurityData) {
	n, err := models.CountUserSessions(h.DB, currentUserID(c))
	if err != nil {
		log.Printf("SecurityPage: CountUserSessions: %v", err)
	}
	data.Sessions = n

	h.render(c, http.StatusOK, "security.html", PageData{
		Title: "Security",
		Flash: h.getFlash(c),
		Data:  data,
	})
}

// ChangePassword requires the current password even though the user is logged
// in: a borrowed or stolen session alone must not be enough to lock the owner
// out. Every other session is ended afterwards for the same reason.
func (h *Handler) ChangePassword(c *gin.Context) {
	user := userFromContext(c)
	current := c.PostForm("current_password")
	password := c.PostForm("new_password")
	confirm := c.PostForm("new_password_confirm")

	if !passwordMatches(user, current) {
		h.renderSecurity(c, SecurityData{PasswordError: "Current password is incorrect."})
		return
	}
	if msg := newPasswordProblem(password, confirm); msg != "" {
		h.renderSecurity(c, SecurityData{PasswordError: msg})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		h.renderSecurity(c, SecurityData{PasswordError: "Failed to change password. Please try again."})
		return
	}
	if err := models.UpdatePassword(h.DB, user.ID, string(hash)); err != nil {
		h.renderSecurity(c, SecurityData{PasswordError: "Failed to change password. Please try again."})
		return
	}

	token, _ := c.Cookie(middleware.SessionCookie)
	if _, err := models.DeleteOtherUserSessions(h.DB, user.ID, token); err != nil {
		log.Printf("ChangePassword: DeleteOtherUserSessions: %v", err)
		h.redirectWithFlash(c, "/profile/security",
			"Password changed, but signing out your other sessions failed. Use “Sign out everywhere else”.")
		return
	}

	h.redirectWithFlash(c, "/profile/security", "Password changed. Your other sessions were signed out.")
}

func (h *Handler) SignOutOtherSessions(c *gin.Context) {
	token, _ := c.Cookie(middleware.SessionCookie)

	ended, err := models.DeleteOtherUserSessions(h.DB, currentUserID(c), token)
	if err != nil {
		h.redirectWithFlash(c, "/profile/security", "Failed to sign out other sessions. Please try again.")
		return
	}

	h.redirectWithFlash(c, "/profile/security", fmt.Sprintf("Signed out of %d other session(s).", ended))
}

// DeleteAccount also asks for the password, since it is the one action here
// that cannot be undone.
func (h *Handler) DeleteAccount(c *gin.Context) {
	user := userFromContext(c)

	if !passwordMatches(user, c.PostForm("password")) {
		h.renderSecurity(c, SecurityData{DeleteError: "Password is incorrect. Your account was not deleted."})
		return
	}

	if err := models.DeleteUser(h.DB, user.ID); err != nil {
		h.renderSecurity(c, SecurityData{DeleteError: "Failed to delete account. Please try again."})
		return
	}

	clearSessionCookie(c)
	h.redirectWithFlash(c, "/login", "Your account and all its decks have been deleted.")
}

// LegacySettingsRedirect keeps bookmarks to the old /settings page working.
func (h *Handler) LegacySettingsRedirect(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/profile/ai")
}
