package e2e_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"memolang/internal/app"
	"memolang/internal/db"

	"github.com/gin-gonic/gin"
	playwright "github.com/playwright-community/playwright-go"
)

var (
	baseURL string
	pw      *playwright.Playwright
	browser playwright.Browser
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	if err := os.Chdir(findRoot()); err != nil {
		panic(err)
	}

	tmpDB, err := os.CreateTemp("", "memolang-e2e-*.db")
	if err != nil {
		panic(err)
	}
	tmpDB.Close()
	dbPath := tmpDB.Name()
	defer os.Remove(dbPath)

	database, err := db.Open(dbPath)
	if err != nil {
		panic(err)
	}
	defer database.Close()

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	baseURL = fmt.Sprintf("http://localhost:%d", port)

	router := app.NewRouter(database)
	go http.Serve(ln, router)

	pw, err = playwright.Run()
	if err != nil {
		panic(fmt.Sprintf("playwright.Run: %v", err))
	}
	browser, err = pw.Chromium.Launch()
	if err != nil {
		panic(fmt.Sprintf("launch chromium: %v", err))
	}

	code := m.Run()

	browser.Close()
	pw.Stop()
	os.Exit(code)
}

// newPage opens a fresh browser page that auto-accepts confirm() dialogs.
func newPage(t *testing.T) playwright.Page {
	t.Helper()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.OnDialog(func(d playwright.Dialog) { d.Accept() })
	t.Cleanup(func() { page.Close() })
	return page
}

// createDeck navigates to the new-deck form, fills it, submits, and returns
// the deck detail URL (e.g. http://localhost:PORT/decks/3).
func createDeck(t *testing.T, page playwright.Page, name string) string {
	t.Helper()
	if _, err := page.Goto(baseURL + "/decks/new"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=name]").Fill(name); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/decks/**"); err != nil {
		t.Fatal(err)
	}
	return page.URL()
}

// deleteDeck navigates to the deck page and clicks the deck-level Delete button.
// Uses a scoped selector so it doesn't match per-card delete buttons.
func deleteDeck(t *testing.T, page playwright.Page, deckURL string) {
	t.Helper()
	if _, err := page.Goto(deckURL); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".page-header button:has-text('Delete')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(baseURL + "/"); err != nil {
		t.Fatal(err)
	}
}

// importCards uploads a CSV file and executes the two-step import flow.
func importCards(t *testing.T, page playwright.Page, deckURL, csvPath string) {
	t.Helper()
	if _, err := page.Goto(deckURL + "/import"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("input[name=csv]").SetInputFiles(csvPath); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Preview')").Click(); err != nil {
		t.Fatal(err)
	}
	// Wait for the execute form to appear
	if err := page.Locator("button:has-text('Import')").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button:has-text('Import')").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(deckURL); err != nil {
		t.Fatal(err)
	}
}

func findRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}
