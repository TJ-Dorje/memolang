// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"
)

func FlashcardReveal(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Flashcard Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)

	backVisible := components.IsVisible(t, page, "#card-back")
	if backVisible {
		t.Error("card back should be hidden before reveal")
	}
	formVisible := components.IsVisible(t, page, "#answer-form")
	if formVisible {
		t.Error("answer form should be hidden before reveal")
	}

	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}

	backVisible = components.IsVisible(t, page, "#card-back")
	if !backVisible {
		t.Error("card back should be visible after reveal")
	}
	formVisible = components.IsVisible(t, page, "#answer-form")
	if !formVisible {
		t.Error("answer form should be visible after reveal")
	}
}

func FlashcardRatingAdvances(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Rating Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)

	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[value='2']").Click(); err != nil {
		t.Fatal(err)
	}

	components.WaitForURL(t, page, "**/session")

	progress, _ := page.Locator(".progress").IsVisible()
	if !progress {
		t.Error("progress bar not visible on next card")
	}
}

func MultipleChoiceFeedbackCorrect(t *testing.T) {
	page := components.NewPage(t)
	answers := actions.LoadCSVAnswers(t, configuration.DefaultCSVPath)

	deckURL := actions.CreateDeck(t, page, "MC Correct Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartMCSession(t, page, deckURL)

	revealVisible := components.IsVisible(t, page, "#reveal-btn")
	if revealVisible {
		t.Error("reveal button should not appear in MC mode")
	}

	count, _ := page.Locator(".mc-option").Count()
	if count < 2 {
		t.Errorf("expected at least 2 MC options, got %d", count)
	}

	actions.AnswerMC(t, page, answers, true)
	components.WaitForURL(t, page, "**/session?feedback=*")

	bannerText, _ := components.GetFeedbackBannerText(page)
	if !strings.Contains(bannerText, "Correct") {
		t.Errorf("expected 'Correct' in banner, got %q", bannerText)
	}

	correct, _ := components.VerifyFeedbackCorrect(page)
	if !correct {
		t.Error("expected feedback-correct class")
	}

	if err := components.ClickNextButton(page); err != nil {
		t.Fatal(err)
	}
}

func MultipleChoiceFeedbackWrong(t *testing.T) {
	page := components.NewPage(t)
	answers := actions.LoadCSVAnswers(t, configuration.DefaultCSVPath)

	deckURL := actions.CreateDeck(t, page, "MC Wrong Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartMCSession(t, page, deckURL)

	clicked := actions.AnswerMC(t, page, answers, false)
	components.WaitForURL(t, page, "**/session?feedback=*")

	bannerText, _ := components.GetFeedbackBannerText(page)
	if !strings.Contains(bannerText, "Wrong") {
		t.Errorf("expected 'Wrong' in banner, got %q", bannerText)
	}

	wrong, _ := components.VerifyFeedbackWrong(page)
	if !wrong {
		t.Error("expected feedback-wrong class")
	}

	givenText, _ := components.GetFeedbackGivenText(page)
	if !strings.Contains(givenText, clicked) {
		t.Errorf("expected 'Your answer' to contain %q, got %q", clicked, givenText)
	}

	// The feedback view shows the same card's front — resolve its real back.
	front, _ := page.Locator(".front-text").TextContent()
	expected := answers[strings.TrimSpace(front)]
	answerText, _ := components.GetFeedbackAnswerText(page)
	if !strings.Contains(answerText, "Correct answer:") || !strings.Contains(answerText, expected) {
		t.Errorf("expected 'Correct answer: %s' in feedback, got %q", expected, answerText)
	}

	if err := components.ClickNextButton(page); err != nil {
		t.Fatal(err)
	}
}

func RunningScore(t *testing.T) {
	page := components.NewPage(t)
	answers := actions.LoadCSVAnswers(t, configuration.DefaultCSVPath)

	deckURL := actions.CreateDeck(t, page, "Score Test Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartMCSession(t, page, deckURL)

	actions.AnswerMC(t, page, answers, true)
	components.WaitForURL(t, page, "**/session?feedback=*")

	scoreCorrect, _ := components.GetScoreCorrect(page)
	scoreWrong, _ := components.GetScoreWrong(page)
	if strings.TrimSpace(scoreCorrect) != "✓ 1" {
		t.Errorf("expected score-right '✓ 1' after correct answer, got %q", scoreCorrect)
	}
	if strings.TrimSpace(scoreWrong) != "✗ 0" {
		t.Errorf("expected score-wrong '✗ 0' after correct answer, got %q", scoreWrong)
	}

	if err := components.ClickNextButton(page); err != nil {
		t.Fatal(err)
	}

	actions.AnswerMC(t, page, answers, false)
	components.WaitForURL(t, page, "**/session?feedback=*")

	scoreCorrect, _ = components.GetScoreCorrect(page)
	scoreWrong, _ = components.GetScoreWrong(page)
	if strings.TrimSpace(scoreCorrect) != "✓ 1" {
		t.Errorf("expected score-right '✓ 1' after wrong answer, got %q", scoreCorrect)
	}
	if strings.TrimSpace(scoreWrong) != "✗ 1" {
		t.Errorf("expected score-wrong '✗ 1' after wrong answer, got %q", scoreWrong)
	}

	if err := components.ClickNextButton(page); err != nil {
		t.Fatal(err)
	}
}

func LastCardFeedback(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Last Card Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, "testdata/single_card.csv")

	actions.StartMCSession(t, page, deckURL)

	// Single-card deck: the first answer is the last. Feedback must still
	// appear before the summary (regression test for the ordering bug).
	if err := page.Locator(".mc-option").First().Click(); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, "**/session?feedback=*")

	bannerText, _ := components.GetFeedbackBannerText(page)
	if bannerText == "" {
		t.Fatal("expected feedback banner on last card, got none")
	}

	if err := components.ClickNextButton(page); err != nil {
		t.Fatal(err)
	}
	components.WaitForURL(t, page, "**/summary")

	body := components.GetBody(t, page)
	if !strings.Contains(body, "Reviewed") {
		t.Error("summary missing 'Reviewed' stat")
	}
}

func SummaryMissedCards(t *testing.T) {
	page := components.NewPage(t)
	answers := actions.LoadCSVAnswers(t, configuration.DefaultCSVPath)

	deckURL := actions.CreateDeck(t, page, "Missed Cards Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartMCSession(t, page, deckURL)

	// Answer every card wrong, clicking through each feedback screen. A miss
	// brings the card back, up to 3 appearances each, so the session runs to
	// three times the deck.
	var firstFront, firstClicked string
	for range len(answers)*3 + 5 {
		if strings.Contains(page.URL(), "summary") {
			break
		}
		front, _ := page.Locator(".front-text").TextContent()
		clicked := actions.AnswerMC(t, page, answers, false)
		if firstFront == "" {
			firstFront, firstClicked = strings.TrimSpace(front), clicked
		}
		components.WaitForURL(t, page, "**/session?feedback=*")
		if err := components.ClickNextButton(page); err != nil {
			t.Fatal(err)
		}
		page.WaitForLoadState()
	}
	components.WaitForURL(t, page, "**/summary")

	body := components.GetBody(t, page)
	for _, want := range []string{"Reviewed", "Correct", "Accuracy", "Missed cards"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary page missing %q", want)
		}
	}
	if !strings.Contains(body, firstFront) {
		t.Errorf("missed-cards table missing front %q", firstFront)
	}
	if !strings.Contains(body, firstClicked) {
		t.Errorf("missed-cards table missing given answer %q", firstClicked)
	}
	// Each card was missed three times; it must still be listed once.
	if n := strings.Count(body, firstFront); n != 1 {
		t.Errorf("missed-cards table lists %q %d times, want once", firstFront, n)
	}
}

func FlashcardModeRegression(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "Flashcard Reg Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)

	if err := page.Locator("#reveal-btn").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("button[value='2']").Click(); err != nil {
		t.Fatal(err)
	}

	components.WaitForURL(t, page, "**/session")

	progress, _ := page.Locator(".progress").IsVisible()
	if !progress {
		t.Error("progress bar not visible after rating")
	}

	scoreCorrect, _ := components.GetScoreCorrect(page)
	if !strings.Contains(scoreCorrect, "✓") {
		t.Errorf("score should be visible in flashcard mode, got %q", scoreCorrect)
	}
}

func EndSessionEarly(t *testing.T) {
	page := components.NewPage(t)

	deckURL := actions.CreateDeck(t, page, "End Early Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.DefaultCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)

	actions.EndSessionEarly(t, page)
	components.WaitForURL(t, page, configuration.BaseURL+"/")

	body := components.GetBody(t, page)
	if !strings.Contains(body, "Session ended") {
		t.Errorf("expected 'Session ended' flash, got body snippet %q", body[:min(300, len(body))])
	}
}
