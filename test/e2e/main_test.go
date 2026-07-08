package e2e_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"memolang/internal/app"
	"memolang/internal/db"
	"memolang/test/e2e/cases"
	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"

	"github.com/gin-gonic/gin"
	playwright "github.com/playwright-community/playwright-go"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	helpers.ChdirRoot()

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
	configuration.DB = database

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	configuration.BaseURL = fmt.Sprintf("http://localhost:%d", port)

	router := app.NewRouter(database)
	go http.Serve(ln, router)

	pw, err := playwright.Run()
	if err != nil {
		panic(fmt.Sprintf("playwright.Run: %v", err))
	}

	configuration.Browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(!configuration.IsHeaded()),
	})
	if err != nil {
		panic(fmt.Sprintf("launch chromium: %v", err))
	}

	code := m.Run()

	configuration.Browser.Close()
	pw.Stop()
	os.Exit(code)
}

// TestE2E is the single suite entry point. Every exported func in the cases
// package must be registered here — unregistered cases silently never run.
// Filter with: go test ./test/e2e -run 'TestE2E/Settings'
func TestE2E(t *testing.T) {
	t.Run("Deck", func(t *testing.T) {
		t.Run("CreateDeck", cases.CreateDeck)
		t.Run("CreateDeckNameRequired", cases.CreateDeckNameRequired)
		t.Run("DeleteDeck", cases.DeleteDeck)
		t.Run("DeckCardClickable", cases.DeckCardClickable)
		t.Run("StudyDropdownOffersBothModes", cases.StudyDropdownOffersBothModes)
		t.Run("DashboardLayout", cases.DashboardLayout)
		t.Run("EditDeck", cases.EditDeck)
	})
	t.Run("Import", func(t *testing.T) {
		t.Run("CSVImportPreviewThenExecute", cases.CSVImportPreviewThenExecute)
		t.Run("CSVImportEmptyFileError", cases.CSVImportEmptyFileError)
		t.Run("CSVImportTokenBelongsToDeck", cases.CSVImportTokenBelongsToDeck)
	})
	t.Run("Session", func(t *testing.T) {
		t.Run("FlashcardReveal", cases.FlashcardReveal)
		t.Run("FlashcardRatingAdvances", cases.FlashcardRatingAdvances)
		t.Run("MultipleChoiceFeedbackCorrect", cases.MultipleChoiceFeedbackCorrect)
		t.Run("MultipleChoiceFeedbackWrong", cases.MultipleChoiceFeedbackWrong)
		t.Run("RunningScore", cases.RunningScore)
		t.Run("LastCardFeedback", cases.LastCardFeedback)
		t.Run("SummaryMissedCards", cases.SummaryMissedCards)
		t.Run("FlashcardModeRegression", cases.FlashcardModeRegression)
		t.Run("EndSessionEarly", cases.EndSessionEarly)
	})
	t.Run("Settings", func(t *testing.T) {
		t.Run("SettingsNavLink", cases.SettingsNavLink)
		t.Run("SaveSettings", cases.SaveSettings)
		t.Run("ReSaveKeepsAPIKey", cases.ReSaveKeepsAPIKey)
		t.Run("AIFormWithoutConfig", cases.AIFormWithoutConfig)
		t.Run("SettingsValidation", cases.SettingsValidation)
	})
}
