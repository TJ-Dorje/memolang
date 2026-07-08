package components

import (
	playwright "github.com/playwright-community/playwright-go"
)

func FillInput(t playwright.Page, selector, value string) error {
	return t.Locator(selector).Fill(value)
}

func ClickButton(t playwright.Page, selector string) error {
	return t.Locator(selector).Click()
}

func ClearInput(t playwright.Page, selector string) error {
	return t.Locator(selector).Clear()
}

func GetButtonCount(t playwright.Page, selector string) (int, error) {
	return t.Locator(selector).Count()
}

func VerifyButtonVisible(t playwright.Page, selector string) (bool, error) {
	return t.Locator(selector).IsVisible()
}

func GetCheckboxChecked(t playwright.Page, selector string) (bool, error) {
	return t.Locator(selector).IsChecked()
}
