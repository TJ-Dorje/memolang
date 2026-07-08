package components

import (
	playwright "github.com/playwright-community/playwright-go"
)

func WaitForFeedbackBanner(t playwright.Page, expected string) error {
	return t.Locator(".feedback-banner").WaitFor()
}

func GetFeedbackBannerText(t playwright.Page) (string, error) {
	return t.Locator(".feedback-banner").TextContent()
}

func GetFeedbackGivenText(t playwright.Page) (string, error) {
	return t.Locator(".feedback-given").TextContent()
}

func GetFeedbackAnswerText(t playwright.Page) (string, error) {
	return t.Locator(".feedback-answer").TextContent()
}

func ClickNextButton(t playwright.Page) error {
	return t.Locator(".btn-primary").First().Click()
}

func VerifyFeedbackCorrect(t playwright.Page) (bool, error) {
	return t.Locator(".feedback-correct").IsVisible()
}

func VerifyFeedbackWrong(t playwright.Page) (bool, error) {
	return t.Locator(".feedback-wrong").IsVisible()
}

func GetScoreCorrect(t playwright.Page) (string, error) {
	return t.Locator(".score-right").TextContent()
}

func GetScoreWrong(t playwright.Page) (string, error) {
	return t.Locator(".score-wrong").TextContent()
}
