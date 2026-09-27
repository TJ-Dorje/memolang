package e2e_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

	fakeLLM := httptest.NewServer(helpers.FakeLLMHandler())
	defer fakeLLM.Close()
	configuration.FakeLLMURL = fakeLLM.URL

	pw, err := playwright.Run()
	if err != nil {
		panic(fmt.Sprintf("playwright.Run: %v", err))
	}

	configuration.Browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(!configuration.IsHeaded()),
	})
	if err != nil {
		panic(fmt.Sprintf("launch chromium: %v", err))
	}

	configuration.Context, err = helpers.NewLoggedInContext()
	if err != nil {
		panic(fmt.Sprintf("logged-in context: %v", err))
	}

	code := m.Run()

	configuration.Context.Close()
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
	t.Run("DeckBuilder", func(t *testing.T) {
		t.Run("DeckBuilderPlanInChat", cases.DeckBuilderPlanInChat)
		t.Run("DeckBuilderNewerPlanSupersedes", cases.DeckBuilderNewerPlanSupersedes)
		t.Run("DeckBuilderFallbackPlan", cases.DeckBuilderFallbackPlan)
		t.Run("DeckBuilderUnreadablePlan", cases.DeckBuilderUnreadablePlan)
		t.Run("DeckBuilderShowsProviderError", cases.DeckBuilderShowsProviderError)
		t.Run("DeckGenerationShowsThinking", cases.DeckGenerationShowsThinking)
		t.Run("DisableThinkingSkipsReasoning", cases.DisableThinkingSkipsReasoning)
		t.Run("DeckGenerationFailureNotedInChat", cases.DeckGenerationFailureNotedInChat)
		t.Run("GenerationPageWithoutDeckGoesToChat", cases.GenerationPageWithoutDeckGoesToChat)
		t.Run("DeckBuilderNeedsProvider", cases.DeckBuilderNeedsProvider)
		t.Run("OldAIFormLeadsToAssistant", cases.OldAIFormLeadsToAssistant)
		t.Run("DeckBuilderStartOver", cases.DeckBuilderStartOver)
	})
	t.Run("Tutor", func(t *testing.T) {
		t.Run("TutorStreamsReply", cases.TutorStreamsReply)
		t.Run("TutorFreeTextQuestion", cases.TutorFreeTextQuestion)
		t.Run("TutorStartOverClears", cases.TutorStartOverClears)
		t.Run("TutorNeedsProvider", cases.TutorNeedsProvider)
		t.Run("TutorOtherUsersCardIs404", cases.TutorOtherUsersCardIs404)
		t.Run("TutorLinkAfterReveal", cases.TutorLinkAfterReveal)
	})
	t.Run("Rating", func(t *testing.T) {
		t.Run("RatingButtonsShowGaps", cases.RatingButtonsShowGaps)
		t.Run("HardCountsAsCorrect", cases.HardCountsAsCorrect)
		t.Run("AgainBringsTheCardBack", cases.AgainBringsTheCardBack)
		t.Run("AgainStopsAfterThreeAppearances", cases.AgainStopsAfterThreeAppearances)
	})
	t.Run("Mode", func(t *testing.T) {
		t.Run("LinearDeckKeepsStudyingAfterEveryCardIsPassed", cases.LinearDeckKeepsStudyingAfterEveryCardIsPassed)
		t.Run("SRSDeckSaysWhenToComeBack", cases.SRSDeckSaysWhenToComeBack)
		t.Run("ModePickerExplainsBothModes", cases.ModePickerExplainsBothModes)
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
	t.Run("Auth", func(t *testing.T) {
		t.Run("Register", cases.Register)
		t.Run("Logout", cases.Logout)
		t.Run("LoginWrongPassword", cases.LoginWrongPassword)
		t.Run("LoginEmailCaseInsensitive", cases.LoginEmailCaseInsensitive)
		t.Run("ProtectedRouteWithoutSession", cases.ProtectedRouteWithoutSession)
		t.Run("DecksArePrivatePerUser", cases.DecksArePrivatePerUser)
	})
	t.Run("Theme", func(t *testing.T) {
		t.Run("ThemeDefaultsToSystem", cases.ThemeDefaultsToSystem)
		t.Run("ThemeSwitcherIsPublic", cases.ThemeSwitcherIsPublic)
		t.Run("ThemePersistsAcrossNavigation", cases.ThemePersistsAcrossNavigation)
		t.Run("ThemeSystemClearsOverride", cases.ThemeSystemClearsOverride)
		t.Run("ThemeReturnsToOriginPage", cases.ThemeReturnsToOriginPage)
		t.Run("ThemeIgnoresUnknownCookie", cases.ThemeIgnoresUnknownCookie)
		t.Run("ThemeIgnoresUnknownPlainCookie", cases.ThemeIgnoresUnknownPlainCookie)
	})
	t.Run("Profile", func(t *testing.T) {
		t.Run("AccountMenuHidesEntriesUntilOpened", cases.AccountMenuHidesEntriesUntilOpened)
		t.Run("DisplayNameReplacesEmailInNav", cases.DisplayNameReplacesEmailInNav)
		t.Run("ChangePassword", cases.ChangePassword)
		t.Run("ChangePasswordWrongCurrent", cases.ChangePasswordWrongCurrent)
		t.Run("ChangePasswordMismatch", cases.ChangePasswordMismatch)
		t.Run("ChangePasswordEndsOtherSessions", cases.ChangePasswordEndsOtherSessions)
		t.Run("SignOutEverywhereElse", cases.SignOutEverywhereElse)
		t.Run("DeleteAccount", cases.DeleteAccount)
		t.Run("DeleteAccountWrongPassword", cases.DeleteAccountWrongPassword)
	})
	t.Run("Settings", func(t *testing.T) {
		t.Run("ProfileLeadsToAIProvider", cases.ProfileLeadsToAIProvider)
		t.Run("LegacySettingsRedirects", cases.LegacySettingsRedirects)
		t.Run("ProvidersSwitchActive", cases.ProvidersSwitchActive)
		t.Run("ProviderSwitcherInChat", cases.ProviderSwitcherInChat)
		t.Run("DeletingActiveProviderLeavesNone", cases.DeletingActiveProviderLeavesNone)
		t.Run("OtherUsersProviderIs404", cases.OtherUsersProviderIs404)
		t.Run("SaveSettings", cases.SaveSettings)
		t.Run("ReSaveKeepsAPIKey", cases.ReSaveKeepsAPIKey)
		t.Run("ClearAPIKeyRemovesIt", cases.ClearAPIKeyRemovesIt)
		t.Run("ClearAndNewKeyIsRejected", cases.ClearAndNewKeyIsRejected)
		t.Run("SettingsValidation", cases.SettingsValidation)
		t.Run("PresetPrefillsBaseURLAndModel", cases.PresetPrefillsBaseURLAndModel)
		t.Run("PresetSaveNeedsOnlyAKey", cases.PresetSaveNeedsOnlyAKey)
		t.Run("PresetSwitchDoesNotClobberCustom", cases.PresetSwitchDoesNotClobberCustom)
	})
}
