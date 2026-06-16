package handlers

import (
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// importCache holds parsed CSV rows keyed by a short random token.
// Entries are consumed (deleted) on execute, so memory stays bounded.
var importCache sync.Map

type cachedImport struct {
	DeckID     int64
	Rows       [][]string
	FrontCol   int
	BackCol    int
	ExampleCol int
	TagsCol    int
}

func (h *Handler) ImportForm(c *gin.Context) {
	id, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, id)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	h.render(c, http.StatusOK, "import.html", PageData{
		Title: "Import CSV — " + deck.Name,
		Flash: h.getFlash(c),
		Data:  ImportData{Deck: deck},
	})
}

func (h *Handler) ImportSubmit(c *gin.Context) {
	deckID, err := getInt64(c, "id")
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid deck ID")
		return
	}

	deck, err := models.GetDeckByID(h.DB, deckID)
	if err != nil {
		c.String(http.StatusNotFound, "Deck not found")
		return
	}

	step := c.Query("step")

	frontCol, _ := strconv.Atoi(c.PostForm("front_col"))
	backCol, _ := strconv.Atoi(c.PostForm("back_col"))
	exampleCol, _ := strconv.Atoi(c.PostForm("example_col"))
	tagsCol, _ := strconv.Atoi(c.PostForm("tags_col"))

	if step == "execute" {
		token := c.PostForm("import_token")
		val, ok := importCache.LoadAndDelete(token)
		if !ok {
			h.render(c, http.StatusOK, "import.html", PageData{
				Title: "Import CSV — " + deck.Name,
				Data:  ImportData{Deck: deck, Error: "Import session expired. Please re-upload the file."},
			})
			return
		}
		cached := val.(cachedImport)
		if cached.DeckID != deckID {
			h.render(c, http.StatusOK, "import.html", PageData{
				Title: "Import CSV — " + deck.Name,
				Data:  ImportData{Deck: deck, Error: "Import token does not belong to this deck. Please re-upload the file."},
			})
			return
		}
		h.executeImport(c, deckID, deck, cached.Rows, cached.FrontCol, cached.BackCol, cached.ExampleCol, cached.TagsCol)
		return
	}

	// preview step: parse the uploaded file
	file, _, err := c.Request.FormFile("csv")
	if err != nil {
		h.render(c, http.StatusOK, "import.html", PageData{
			Title: "Import CSV — " + deck.Name,
			Data:  ImportData{Deck: deck, Error: "Please select a CSV file"},
		})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	allRows, err := reader.ReadAll()
	if err != nil {
		h.render(c, http.StatusOK, "import.html", PageData{
			Title: "Import CSV — " + deck.Name,
			Data:  ImportData{Deck: deck, Error: "Failed to parse CSV: " + err.Error()},
		})
		return
	}

	if len(allRows) < 1 {
		h.render(c, http.StatusOK, "import.html", PageData{
			Title: "Import CSV — " + deck.Name,
			Data:  ImportData{Deck: deck, Error: "CSV file is empty"},
		})
		return
	}

	hasHeader := isHeaderRow(allRows[0])
	dataRows := allRows
	if hasHeader {
		dataRows = allRows[1:]
	}

	// store parsed rows in cache under a random token
	token := randomToken()
	importCache.Store(token, cachedImport{
		DeckID:     deckID,
		Rows:       dataRows,
		FrontCol:   frontCol,
		BackCol:    backCol,
		ExampleCol: exampleCol,
		TagsCol:    tagsCol,
	})

	var preview []map[string]string
	limit := min(5, len(dataRows))
	for i := range limit {
		row := dataRows[i]
		preview = append(preview, map[string]string{
			"Front":   safeGet(row, frontCol),
			"Back":    safeGet(row, backCol),
			"Example": safeGet(row, exampleCol),
			"Tags":    safeGet(row, tagsCol),
		})
	}

	h.render(c, http.StatusOK, "import.html", PageData{
		Title: "Import CSV — " + deck.Name,
		Data: ImportData{
			Deck:        deck,
			Preview:     preview,
			TotalRows:   len(dataRows),
			HasHeader:   hasHeader,
			FrontCol:    frontCol,
			BackCol:     backCol,
			ExampleCol:  exampleCol,
			TagsCol:     tagsCol,
			ShowExecute: true,
			ImportToken: token,
		},
	})
}

func (h *Handler) executeImport(c *gin.Context, deckID int64, _ models.Deck, dataRows [][]string, frontCol, backCol, exampleCol, tagsCol int) {
	tx, err := h.DB.Begin()
	if err != nil {
		c.String(http.StatusInternalServerError, "Transaction failed")
		return
	}
	defer tx.Rollback()

	inserted := 0
	for _, row := range dataRows {
		front := safeGet(row, frontCol)
		back := safeGet(row, backCol)
		if front == "" || back == "" {
			continue
		}
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO cards (deck_id, front, back, example, tags) VALUES (?, ?, ?, ?, ?)`,
			deckID, front, back, safeGet(row, exampleCol), safeGet(row, tagsCol),
		)
		if err != nil {
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to commit import")
		return
	}

	h.redirectWithFlash(c, "/decks/"+c.Param("id"), fmt.Sprintf("%d cards imported.", inserted))
}

func isHeaderRow(row []string) bool {
	if len(row) == 0 {
		return false
	}
	switch row[0] {
	case "front", "word", "term", "back", "translation":
		return true
	}
	return false
}

func safeGet(row []string, col int) string {
	if col >= 0 && col < len(row) {
		return row[col]
	}
	return ""
}

func randomToken() string {
	b := make([]byte, 12)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

