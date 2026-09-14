package middleware

import (
	"database/sql"
	"net/http"
	"net/url"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// SessionCookie is the name of the login session cookie.
const SessionCookie = "session"

// RequireAuth rejects requests without a valid session cookie, bouncing them
// to the login page with ?next= set so login can return them where they were.
func RequireAuth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookie)
		if err != nil {
			redirectToLogin(c)
			return
		}

		user, err := models.GetUserByToken(db, token)
		if err != nil || user == nil {
			redirectToLogin(c)
			return
		}

		c.Set("user", user)
		c.Set("userID", user.ID)
		c.Next()
	}
}

func redirectToLogin(c *gin.Context) {
	c.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(c.Request.URL.Path))
	c.Abort()
}
