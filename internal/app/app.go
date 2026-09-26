package app

import (
	"database/sql"
	"html/template"
	"strings"

	"memolang/internal/handlers"
	"memolang/internal/middleware"

	"github.com/gin-gonic/gin"
)

func NewRouter(database *sql.DB) *gin.Engine {
	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		// hasPrefix lets the profile side menu mark its section active on the
		// POST paths below it too, where a rejected form re-renders.
		"hasPrefix": strings.HasPrefix,
	})
	r.LoadHTMLGlob("templates/*.html")
	r.Static("/static", "./static")

	h := handlers.New(database, r.HTMLRender)

	// Public routes.
	r.GET("/login", h.LoginForm)
	r.POST("/login", h.Login)
	r.GET("/register", h.RegisterForm)
	r.POST("/register", h.Register)
	r.POST("/logout", h.Logout)
	// Public: the login and register pages carry the theme switcher too.
	r.POST("/theme", h.SetTheme)

	// Everything else requires a session.
	protected := r.Group("/")
	protected.Use(middleware.RequireAuth(database))
	{
		protected.GET("/", h.Dashboard)

		protected.GET("/decks/new", h.NewDeckForm)
		protected.POST("/decks/new", h.CreateDeck)
		protected.GET("/decks/new/assistant", h.DeckBuilderPage)
		protected.POST("/decks/new/assistant", h.AskDeckBuilder)
		protected.POST("/decks/new/assistant/reset", h.ResetDeckBuilder)
		protected.POST("/decks/new/assistant/review", h.ReviewDeckBuilder)
		protected.GET("/decks/new/assistant/summary", h.DeckBuilderSummary)
		protected.POST("/decks/new-ai", h.CreateDeckAI)
		protected.GET("/decks/new-ai/processing", h.AIProcessing)
		protected.GET("/decks/new-ai/execute", h.AIExecute)

		protected.GET("/decks/:id", h.DeckDetail)
		protected.GET("/decks/:id/edit", h.EditDeckForm)
		protected.POST("/decks/:id/edit", h.UpdateDeck)
		protected.POST("/decks/:id/delete", h.DeleteDeck)

		protected.GET("/decks/:id/import", h.ImportForm)
		protected.POST("/decks/:id/import", h.ImportSubmit)

		protected.GET("/decks/:id/session", h.StartSession)
		protected.POST("/decks/:id/session/answer", h.SubmitAnswer)
		protected.POST("/decks/:id/session/end", h.EndSessionEarly)
		protected.GET("/decks/:id/session/summary", h.SessionSummary)

		protected.GET("/cards/:id/tutor", h.TutorPage)
		protected.POST("/cards/:id/tutor", h.AskTutor)
		protected.POST("/cards/:id/tutor/reset", h.ResetTutor)

		protected.GET("/cards/:id/edit", h.EditCardForm)
		protected.POST("/cards/:id/edit", h.UpdateCard)
		protected.POST("/cards/:id/delete", h.DeleteCard)

		// Everything about the account lives under /profile, reached from the
		// account menu in the nav.
		protected.GET("/profile", h.ProfilePage)
		protected.POST("/profile", h.UpdateProfile)

		protected.GET("/profile/security", h.SecurityPage)
		protected.POST("/profile/security/password", h.ChangePassword)
		protected.POST("/profile/security/sessions", h.SignOutOtherSessions)
		protected.POST("/profile/security/delete", h.DeleteAccount)

		protected.GET("/profile/ai", h.SettingsPage)
		protected.POST("/profile/ai", h.SaveSettings)
		protected.POST("/profile/ai/test", h.TestLLMConnection)

		protected.GET("/settings", h.LegacySettingsRedirect)
	}

	return r
}
