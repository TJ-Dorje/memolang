package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// pendingCookie carries the "password OK, waiting for the second factor"
// token. It is scoped to /login/mfa, so no other request sends it.
const pendingCookie = "mfa_pending"

// finishPasswordStep is where every password-proving flow ends (login,
// password reset): users without MFA get their session now; users with MFA
// get a short-lived pending login and the code page. flash is shown on
// whichever page comes next.
func (h *Handler) finishPasswordStep(c *gin.Context, user *models.User, next, flash string) {
	if !user.MFAEnabled() {
		models.ClearAttempts(h.DB, loginFromHere.kind, loginCounters(c, user.Email)[0].key)
		if err := h.startSessionCookie(c, user.ID); err != nil {
			h.redirectWithFlash(c, "/login", "Failed to start a session. Please try again.")
			return
		}
		h.redirectOptionalFlash(c, safeNext(next), flash)
		return
	}

	token, err := models.CreatePendingLogin(h.DB, user.ID, safeNext(next))
	if err != nil {
		log.Printf("CreatePendingLogin: %v", err)
		h.redirectWithFlash(c, "/login", "Failed to log in. Please try again.")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(pendingCookie, token, int(models.PendingLoginTTL.Seconds()), "/login/mfa", "", secureCookies(), true)
	h.redirectOptionalFlash(c, "/login/mfa", flash)
}

func (h *Handler) redirectOptionalFlash(c *gin.Context, location, flash string) {
	if flash == "" {
		c.Redirect(http.StatusSeeOther, location)
		return
	}
	h.redirectWithFlash(c, location, flash)
}

func clearPendingCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(pendingCookie, "", -1, "/login/mfa", "", secureCookies(), true)
}

// MFAData backs login_mfa.html.
type MFAData struct {
	Error string
}

func (h *Handler) LoginMFAForm(c *gin.Context) {
	token, _ := c.Cookie(pendingCookie)
	live, err := models.LookupPendingLogin(h.DB, token)
	if err != nil || !live {
		clearPendingCookie(c)
		h.redirectWithFlash(c, "/login", "Please log in again.")
		return
	}
	h.renderLoginMFA(c, MFAData{})
}

func (h *Handler) renderLoginMFA(c *gin.Context, data MFAData) {
	h.render(c, http.StatusOK, "login_mfa.html", PageData{
		Title: "Two-factor login",
		Flash: h.getFlash(c),
		Data:  data,
	})
}

// LoginMFA takes an authenticator code or a recovery code. Every try is
// spent before the code is checked (5 per password login), and a wrong code
// also counts as a failed login, so logging in again with the password
// doesn't buy unlimited guesses: after 5 the address is locked from here
// like any other failed login.
func (h *Handler) LoginMFA(c *gin.Context) {
	token, _ := c.Cookie(pendingCookie)
	code := strings.TrimSpace(c.PostForm("code"))

	pending, err := models.TryPendingLogin(h.DB, token)
	if errors.Is(err, models.ErrPendingLoginGone) {
		clearPendingCookie(c)
		h.redirectWithFlash(c, "/login", "That took too long or had too many tries. Please log in again.")
		return
	}
	if err != nil {
		log.Printf("LoginMFA: %v", err)
		h.renderLoginMFA(c, MFAData{Error: "Something went wrong. Please try again."})
		return
	}
	user, err := models.GetUserByID(h.DB, pending.UserID)
	if err != nil || user == nil {
		clearPendingCookie(c)
		h.redirectWithFlash(c, "/login", "Please log in again.")
		return
	}

	err = models.CheckMFA(h.DB, user.ID, code)
	if errors.Is(err, models.ErrBadMFACode) {
		h.wrongMFACode(c, user, token, pending.TriesLeft)
		return
	}
	if err != nil {
		log.Printf("LoginMFA: CheckMFA: %v", err)
		h.renderLoginMFA(c, MFAData{Error: "Something went wrong. Please try again."})
		return
	}

	models.DeletePendingLogin(h.DB, token)
	clearPendingCookie(c)
	models.ClearAttempts(h.DB, loginFromHere.kind, loginCounters(c, user.Email)[0].key)
	if err := h.startSessionCookie(c, user.ID); err != nil {
		h.redirectWithFlash(c, "/login", "Failed to start a session. Please try again.")
		return
	}
	h.redirectOptionalFlash(c, pending.Next, h.recoveryCodeNotice(user.ID, code))
}

func (h *Handler) wrongMFACode(c *gin.Context, user *models.User, token string, triesLeft int) {
	h.record(loginCounters(c, user.Email)...)
	if triesLeft == 0 {
		models.DeletePendingLogin(h.DB, token)
		clearPendingCookie(c)
		h.redirectWithFlash(c, "/login", "Too many wrong codes. Please log in again.")
		return
	}
	h.renderLoginMFA(c, MFAData{Error: fmt.Sprintf("That code isn't right. %d %s left.", triesLeft, plural(triesLeft, "try", "tries"))})
}

// recoveryCodeNotice tells someone who just spent a recovery code how many
// are left: they probably lost their phone.
func (h *Handler) recoveryCodeNotice(userID int64, code string) string {
	if len(code) == 6 {
		return ""
	}
	n, err := models.CountRecoveryCodes(h.DB, userID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("You used a recovery code: %d left. Get new ones under Profile → Security.", n)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
