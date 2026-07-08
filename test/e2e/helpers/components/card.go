package components

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

func ClickDeckCard(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator(".deck-card .deck-link").Click(); err != nil {
		t.Fatal(err)
	}
}

func VerifyDeckCardCount(t *testing.T, page playwright.Page) int {
	t.Helper()
	count, err := page.Locator(".deck-card").Count()
	if err != nil {
		t.Fatal(err)
	}
	return int(count)
}

func ClickStudyOption(t *testing.T, page playwright.Page, optionText string) {
	t.Helper()
	if err := page.Locator(".study-menu summary").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".study-options a:has-text('" + optionText + "')").Click(); err != nil {
		t.Fatal(err)
	}
}

func VerifyStudyOptionsVisible(t *testing.T, page playwright.Page) {
	t.Helper()
	flashcard := page.Locator(".study-options a:has-text('Flashcards')")
	mc := page.Locator(".study-options a:has-text('Multiple Choice')")
	v1, _ := flashcard.IsVisible()
	v2, _ := mc.IsVisible()
	if !v1 {
		t.Error("Flashcards option not visible after clicking Study")
	}
	if !v2 {
		t.Error("Multiple Choice option not visible after clicking Study")
	}
}

func ClickEditButton(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator("a:has-text('Edit')").Click(); err != nil {
		t.Fatal(err)
	}
}

func ClickDeleteButton(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator(".page-header button:has-text('Delete')").Click(); err != nil {
		t.Fatal(err)
	}
}
