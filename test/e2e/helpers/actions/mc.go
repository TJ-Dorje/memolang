package actions

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"

	"memolang/test/e2e/configuration"

	playwright "github.com/playwright-community/playwright-go"
)

// LoadCSVAnswers parses a card CSV fixture into a front → back map, so MC
// tests can pick the correct (or a deliberately wrong) option.
func LoadCSVAnswers(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	answers := make(map[string]string, len(rows))
	for _, row := range rows {
		if len(row) < 2 || strings.EqualFold(row[0], "front") {
			continue
		}
		answers[row[0]] = row[1]
	}
	return answers
}

// AnswerMC clicks an MC option on the current question view: the correct one
// when correct is true, otherwise the first wrong one. Returns the clicked
// option's text.
func AnswerMC(t *testing.T, page playwright.Page, answers map[string]string, correct bool) string {
	t.Helper()

	front, err := page.Locator(".front-text").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	front = strings.TrimSpace(front)
	expected, ok := answers[front]
	if !ok {
		t.Fatalf("front %q not in CSV answer map", front)
	}

	options := page.Locator(".mc-option")
	n, err := options.Count()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		text, err := options.Nth(i).TextContent()
		if err != nil {
			t.Fatal(err)
		}
		text = strings.TrimSpace(text)
		if (text == expected) == correct {
			if err := options.Nth(i).Click(); err != nil {
				t.Fatal(err)
			}
			return text
		}
	}
	t.Fatalf("no MC option with correct=%v for front %q (expected back %q, %d options)", correct, front, expected, n)
	return ""
}

// ResetLLMSettings wipes all llm.* keys so a settings test starts from a
// clean, order-independent state.
func ResetLLMSettings(t *testing.T) {
	t.Helper()
	if _, err := configuration.DB.Exec("DELETE FROM settings WHERE key LIKE 'llm.%'"); err != nil {
		t.Fatal(err)
	}
}
