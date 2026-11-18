package handlers

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"

	"memolang/internal/models"
	"memolang/internal/totp"

	"github.com/gin-gonic/gin"
)

const mfaSetupURL = "/profile/security/mfa"

// MFASetupData backs mfa_setup.html.
type MFASetupData struct {
	// QR is a data: URI. html/template blocks data: URLs in src by default
	// (they could carry anything); this one is safe to mark trusted because
	// totp builds it entirely from base64 of a PNG we render ourselves.
	QR template.URL
	// Secret is shown in groups of four for typing in by hand.
	Secret string
	Error  string
}

// RecoveryCodesData backs mfa_codes.html, the one time codes are shown.
type RecoveryCodesData struct {
	Codes []string
	// First is true right after turning MFA on, false after regenerating.
	First bool
}

// StartMFASetup creates a new pending secret and shows it. Starting again
// (say, after scanning with the wrong app) simply makes a new one.
func (h *Handler) StartMFASetup(c *gin.Context) {
	user := userFromContext(c)
	setup, err := totp.NewSetup(user.Email)
	if err != nil {
		log.Printf("StartMFASetup: %v", err)
		h.redirectWithFlash(c, "/profile/security", "Failed to start two-factor setup. Please try again.")
		return
	}
	err = models.StartTOTPSetup(h.DB, user.ID, setup.Secret)
	if errors.Is(err, models.ErrMFAEnabled) {
		h.redirectWithFlash(c, "/profile/security", "Two-factor login is already on.")
		return
	}
	if err != nil {
		log.Printf("StartMFASetup: %v", err)
		h.redirectWithFlash(c, "/profile/security", "Failed to start two-factor setup. Please try again.")
		return
	}
	c.Redirect(http.StatusSeeOther, mfaSetupURL)
}

// MFASetupPage shows the pending secret as a QR code. It reads the secret
// from the database rather than making one, so reloading the page doesn't
// change it under a half-finished scan.
func (h *Handler) MFASetupPage(c *gin.Context) {
	h.renderMFASetup(c, "")
}

func (h *Handler) renderMFASetup(c *gin.Context, errMsg string) {
	user := userFromContext(c)
	secret, enabled, err := models.TOTPSecret(h.DB, user.ID)
	if err != nil || enabled || secret == "" {
		c.Redirect(http.StatusSeeOther, "/profile/security")
		return
	}
	setup, err := totp.SetupFor(user.Email, secret)
	if err != nil {
		log.Printf("MFASetupPage: %v", err)
		h.redirectWithFlash(c, "/profile/security", "Failed to show two-factor setup. Please start again.")
		return
	}
	noStore(c)
	h.render(c, http.StatusOK, "mfa_setup.html", PageData{
		Title: "Set up two-factor login",
		Flash: h.getFlash(c),
		Data:  MFASetupData{QR: template.URL(setup.QR), Secret: groupsOf4(secret), Error: errMsg},
	})
}

// ConfirmMFASetup turns MFA on once the user proves their app works, and
// shows the recovery codes. This page is rendered on the POST rather than
// after a redirect: the codes exist in plain text only now, and must not be
// stored to survive a redirect. Reloading re-posts, which is refused
// harmlessly (MFA is already on).
func (h *Handler) ConfirmMFASetup(c *gin.Context) {
	user := userFromContext(c)
	codes, err := models.ConfirmTOTPSetup(h.DB, user.ID, strings.TrimSpace(c.PostForm("code")))
	if errors.Is(err, models.ErrBadMFACode) {
		h.renderMFASetup(c, "That code isn't right. Check the time on your phone and try the newest code.")
		return
	}
	if errors.Is(err, models.ErrMFAEnabled) {
		h.redirectWithFlash(c, "/profile/security", "Two-factor login is already on.")
		return
	}
	if err != nil {
		log.Printf("ConfirmMFASetup: %v", err)
		h.renderMFASetup(c, "Something went wrong. Please try again.")
		return
	}
	h.send(c, mfaEnabledMail(user.Email))
	h.renderRecoveryCodes(c, codes, true)
}

func (h *Handler) renderRecoveryCodes(c *gin.Context, codes []string, first bool) {
	noStore(c)
	h.render(c, http.StatusOK, "mfa_codes.html", PageData{
		Title: "Recovery codes",
		Data:  RecoveryCodesData{Codes: codes, First: first},
	})
}

// recoveryCodeShape is what newRecoveryCode produces.
var recoveryCodeShape = regexp.MustCompile(`^[a-z2-9]{5}-[a-z2-9]{5}$`)

// DownloadRecoveryCodes returns the codes on the page as a text file. The
// server never kept them in plain text, so the page posts them back; they
// are checked against the code format and sent as an attachment, so nothing
// else can be smuggled through.
func (h *Handler) DownloadRecoveryCodes(c *gin.Context) {
	codes := c.PostFormArray("code")
	if len(codes) == 0 || len(codes) > 10 {
		c.Status(http.StatusBadRequest)
		return
	}
	var b strings.Builder
	b.WriteString("MemoLang recovery codes for " + userFromContext(c).Email + "\n")
	b.WriteString("Each code logs you in once if you lose your authenticator app.\n\n")
	for _, code := range codes {
		if !recoveryCodeShape.MatchString(code) {
			c.Status(http.StatusBadRequest)
			return
		}
		b.WriteString(code + "\n")
	}
	noStore(c)
	c.Header("Content-Disposition", `attachment; filename="memolang-recovery-codes.txt"`)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b.String()))
}

// RegenerateRecoveryCodes replaces every recovery code. It asks for the
// password and a code, like turning MFA off: a stolen session must not be
// able to mint itself a way in.
func (h *Handler) RegenerateRecoveryCodes(c *gin.Context) {
	user := userFromContext(c)
	if msg := h.mfaReauthProblem(c, user); msg != "" {
		h.renderSecurity(c, SecurityData{MFAError: msg})
		return
	}
	codes, err := models.RegenerateRecoveryCodes(h.DB, user.ID)
	if err != nil {
		log.Printf("RegenerateRecoveryCodes: %v", err)
		h.renderSecurity(c, SecurityData{MFAError: "Failed to make new codes. Please try again."})
		return
	}
	h.send(c, recoveryCodesRegeneratedMail(user.Email))
	h.renderRecoveryCodes(c, codes, false)
}

func (h *Handler) DisableMFA(c *gin.Context) {
	user := userFromContext(c)
	if msg := h.mfaReauthProblem(c, user); msg != "" {
		h.renderSecurity(c, SecurityData{MFAError: msg})
		return
	}
	if err := models.DisableMFA(h.DB, user.ID); err != nil {
		log.Printf("DisableMFA: %v", err)
		h.renderSecurity(c, SecurityData{MFAError: "Failed to turn off two-factor login. Please try again."})
		return
	}
	h.send(c, mfaDisabledMail(user.Email))
	h.redirectWithFlash(c, "/profile/security", "Two-factor login is off.")
}

// mfaReauthProblem checks the password and a current MFA code (or recovery
// code) posted with a sensitive MFA change; "" means both are right.
func (h *Handler) mfaReauthProblem(c *gin.Context, user *models.User) string {
	if !user.MFAEnabled() {
		return "Two-factor login is not on."
	}
	if !passwordMatches(user, c.PostForm("password")) {
		return "Password is incorrect."
	}
	err := models.CheckMFA(h.DB, user.ID, c.PostForm("code"))
	if errors.Is(err, models.ErrBadMFACode) {
		return "That code isn't right."
	}
	if err != nil {
		log.Printf("mfa reauth: %v", err)
		return "Something went wrong. Please try again."
	}
	return ""
}

// noStore keeps a page with secrets on it out of the browser cache and the
// back button.
func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
}

func groupsOf4(s string) string {
	var parts []string
	for len(s) > 4 {
		parts = append(parts, s[:4])
		s = s[4:]
	}
	return strings.Join(append(parts, s), " ")
}
