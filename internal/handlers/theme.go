package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ThemeCookie holds an explicit light/dark choice. It is absent when the
// viewer follows their OS setting, which is the default: the stylesheet falls
// back to prefers-color-scheme whenever no data-theme is stamped on <html>.
//
// A cookie rather than a settings row, deliberately. The theme has to be known
// before the first byte of HTML is written or the page paints in the wrong
// palette, and it has to work on /login and /register, where there is no user
// to look a setting up for.
const ThemeCookie = "theme"

const themeCookieMaxAge = 365 * 24 * 60 * 60 // one year

// currentTheme returns "dark", "light", or "" for follow-the-OS. An
// unrecognised cookie value is treated as unset rather than echoed into the
// page, so the attribute can only ever hold one of two known strings.
func currentTheme(c *gin.Context) string {
	v, err := c.Cookie(ThemeCookie)
	if err != nil {
		return ""
	}
	switch v {
	case "dark", "light":
		return v
	default:
		return ""
	}
}

// SetTheme records the choice and returns the viewer to the page they were on.
//
// This is a public route: the login and register pages carry the switcher too,
// and it reads nothing and writes nothing but its own cookie. The return path
// comes from the form (rendered from the request URI), not from Referer, and
// still goes through safeNext so it cannot be pointed off-site.
func (h *Handler) SetTheme(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	switch theme := c.PostForm("theme"); theme {
	case "dark", "light":
		c.SetCookie(ThemeCookie, theme, themeCookieMaxAge, "/", "", secureCookies(), true)
	default:
		// "system", and anything unrecognised, clears the override.
		c.SetCookie(ThemeCookie, "", -1, "/", "", secureCookies(), true)
	}
	c.Redirect(http.StatusSeeOther, safeNext(c.PostForm("next")))
}
