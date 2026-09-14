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
	Data  any
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

// render fills in the logged-in user for the nav so individual handlers don't
// each have to, then renders the page.
func (h *Handler) render(c *gin.Context, status int, template string, pd PageData) {
	if pd.User == nil {
		if v, ok := c.Get("user"); ok {
			if u, ok := v.(*models.User); ok {
				pd.User = u
			}
		}
	}
	c.HTML(status, template, pd)
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
