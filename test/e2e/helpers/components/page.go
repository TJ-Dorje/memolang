package components

import (
	"testing"

	"memolang/test/e2e/configuration"

	playwright "github.com/playwright-community/playwright-go"
)

func NewPage(t *testing.T) playwright.Page {
	t.Helper()
	page, err := configuration.Browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.OnDialog(func(d playwright.Dialog) { d.Accept() })
	t.Cleanup(func() { page.Close() })
	return page
}

func GetHeading(t *testing.T, page playwright.Page) string {
	t.Helper()
	text, err := page.Locator("h1").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func GetBody(t *testing.T, page playwright.Page) string {
	t.Helper()
	text, err := page.Locator("body").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func GetFlash(t *testing.T, page playwright.Page) string {
	t.Helper()
	text, err := page.Locator(".flash").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func GetFormError(t *testing.T, page playwright.Page) string {
	t.Helper()
	text, err := page.Locator(".form-error").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func GetInputValue(t *testing.T, page playwright.Page, selector string) string {
	t.Helper()
	val, err := page.Locator(selector).InputValue()
	if err != nil {
		t.Fatal(err)
	}
	return val
}

func GetPlaceholder(t *testing.T, page playwright.Page, selector string) string {
	t.Helper()
	result, err := page.Evaluate("sel => document.querySelector(sel).getAttribute('placeholder')", selector)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result.(string)
	return s
}

func GetMeta(t *testing.T, page playwright.Page) string {
	t.Helper()
	text, err := page.Locator(".meta").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func IsVisible(t *testing.T, page playwright.Page, selector string) bool {
	t.Helper()
	visible, err := page.Locator(selector).IsVisible()
	if err != nil {
		t.Fatal(err)
	}
	return visible
}

func CountLocators(t *testing.T, page playwright.Page, selector string) int {
	t.Helper()
	count, err := page.Locator(selector).Count()
	if err != nil {
		t.Fatal(err)
	}
	return int(count)
}

func GetAttributeValue(t *testing.T, page playwright.Page, selector string) string {
	t.Helper()
	val, err := page.Locator(selector).GetAttribute("value")
	if err != nil {
		t.Fatal(err)
	}
	return val
}

func IsChecked(t *testing.T, page playwright.Page, selector string) bool {
	t.Helper()
	checked, err := page.Locator(selector).IsChecked()
	if err != nil {
		t.Fatal(err)
	}
	return checked
}
