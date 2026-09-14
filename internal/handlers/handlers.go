package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

type PageData struct {
	Title string
	Flash string
	User  *models.User
	// Theme is "dark", "light", or "" to follow the OS. render fills it.
	Theme string
	// Path is the current request URI, so the theme switcher can post a
	// return address instead of trusting Referer. render fills it.
	Path string
	Data any
}

type Handler struct {
	DB      *sql.DB
	pending sync.Map
	token   string
}

func New(db *sql.DB) *Handler {
	return &Handler{DB: db}
}

func (h *Handler) storePending(data any) string {
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)
	h.pending.Store(token, data)
	return token
}

func (h *Handler) loadPending(token string) (any, bool) {
	v, ok := h.pending.LoadAndDelete(token)
	return v, ok
}

// render fills in the logged-in user for the nav, plus the chrome every page
// shares (theme, return path), so individual handlers don't each have to, then
// renders the page. This is the only c.HTML call in the package, which is what
// makes filling these here sufficient.
func (h *Handler) render(c *gin.Context, status int, template string, pd PageData) {
	pd.Theme = currentTheme(c)
	pd.Path = c.Request.URL.RequestURI()

	if pd.User == nil {
		pd.User = userFromContext(c)
	}
	c.HTML(status, template, pd)
}

// userFromContext returns the user RequireAuth stored, or nil on the public
// routes where there is none.
func userFromContext(c *gin.Context) *models.User {
	v, ok := c.Get("user")
	if !ok {
		return nil
	}
	u, ok := v.(*models.User)
	if !ok {
		return nil
	}
	return u
}

func (h *Handler) redirectWithFlash(c *gin.Context, location, flash string) {
	c.SetCookie("flash", flash, 5, "/", "", false, false)
	c.Redirect(http.StatusSeeOther, location)
}

func (h *Handler) getFlash(c *gin.Context) string {
	flash, err := c.Cookie("flash")
	if err != nil {
		return ""
	}
	c.SetCookie("flash", "", -1, "/", "", false, false)
	return flash
}

func getInt64(c *gin.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

// currentUserID returns the id set by middleware.RequireAuth. It is 0 only on
// routes outside the protected group, which never call it.
func currentUserID(c *gin.Context) int64 {
	v, _ := c.Get("userID")
	id, _ := v.(int64)
	return id
}
