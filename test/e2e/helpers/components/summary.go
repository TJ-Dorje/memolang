package components

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const maxClickLoop = 55

func WaitForSummary(t playwright.Page) {
	for range maxClickLoop {
		currentURL := t.URL()
		if contains(currentURL, "summary") {
			break
		}
		option := t.Locator(".mc-option").First()
		visible, _ := option.IsVisible()
		if !visible {
			break
		}
		if err := option.Click(); err != nil {
			break
		}
		t.WaitForLoadState()
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func VerifyMissedCardsTable(t playwright.Page) (bool, error) {
	return t.Locator("table.card-table").IsVisible()
}

func GetMissedCardCount(t playwright.Page) (int, error) {
	return t.Locator("table.card-table tbody tr").Count()
}

func VerifyPerfectSessionMessage(t playwright.Page) (bool, error) {
	return t.Locator("p:has-text('Perfect session')").IsVisible()
}

func VerifyStatTile(t playwright.Page, label string) (bool, error) {
	return t.Locator(".stat:has-text('" + label + "')").IsVisible()
}

func GetBodyContains(t *testing.T, page playwright.Page, substr string) bool {
	t.Helper()
	body, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return contains(body, substr)
}
