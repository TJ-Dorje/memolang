package app

import (
	"database/sql"
	"html/template"

	"memolang/internal/handlers"

	"github.com/gin-gonic/gin"
)

func NewRouter(database *sql.DB) *gin.Engine {
	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	})
	r.LoadHTMLGlob("templates/*.html")
	r.Static("/static", "./static")

	h := handlers.New(database)

	r.GET("/", h.Dashboard)

	r.GET("/decks/new", h.NewDeckForm)
	r.POST("/decks/new", h.CreateDeck)
	r.POST("/decks/new-ai", h.CreateDeckAI)
	r.GET("/decks/new-ai/processing", h.AIProcessing)
	r.GET("/decks/new-ai/execute", h.AIExecute)

	r.GET("/decks/:id", h.DeckDetail)
	r.GET("/decks/:id/edit", h.EditDeckForm)
	r.POST("/decks/:id/edit", h.UpdateDeck)
	r.POST("/decks/:id/delete", h.DeleteDeck)

	r.GET("/decks/:id/import", h.ImportForm)
	r.POST("/decks/:id/import", h.ImportSubmit)

	r.GET("/decks/:id/session", h.StartSession)
	r.POST("/decks/:id/session/answer", h.SubmitAnswer)
	r.POST("/decks/:id/session/end", h.EndSessionEarly)
	r.GET("/decks/:id/session/summary", h.SessionSummary)

	r.GET("/cards/:id/edit", h.EditCardForm)
	r.POST("/cards/:id/edit", h.UpdateCard)
	r.POST("/cards/:id/delete", h.DeleteCard)

	return r
}
