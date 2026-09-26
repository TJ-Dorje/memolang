// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"strings"
	"testing"

	"memolang/internal/models"
	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/actions"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

// tutorUser registers a fresh user with a one-card deck and returns the page
// and the card's tutor URL. With configured set, the user's AI provider
// points at the fake LLM server TestMain runs.
func tutorUser(t *testing.T, prefix string, configured bool) (playwright.Page, string) {
	t.Helper()
	page, email := freshUser(t, prefix)

	deckURL := actions.CreateDeck(t, page, "Tutor Deck")
	actions.ImportCSV(t, page, deckURL, configuration.SingleCardCSVPath)
	href, err := page.Locator(".card-row a:has-text('Tutor')").GetAttribute("href")
	if err != nil {
		t.Fatal(err)
	}

	if configured {
		useFakeLLM(t, email)
	}
	return page, href
}

// useFakeLLM points the user's AI provider at the fake LLM server TestMain
// runs.
func useFakeLLM(t *testing.T, email string) {
	t.Helper()
	useFakeLLMAt(t, email, configuration.FakeLLMURL)
}

// useFakeLLMAt points the user's AI provider at baseURL on the fake server.
func useFakeLLMAt(t *testing.T, email, baseURL string) {
	t.Helper()
	u, err := models.GetUserByEmail(configuration.DB, email)
	if err != nil || u == nil {
		t.Fatalf("look up %s: %v", email, err)
	}
	for key, value := range map[string]string{
		"llm.provider": "custom",
		"llm.base_url": baseURL,
		"llm.model":    "fake-model",
	} {
		if err := models.SetSetting(configuration.DB, u.ID, key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func waitAttached(t *testing.T, page playwright.Page, selector string) {
	t.Helper()
	err := page.Locator(selector).WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateAttached,
	})
	if err != nil {
		t.Fatalf("waiting for %s: %v", selector, err)
	}
}

// TutorStreamsReply is the no-JavaScript streaming check. The fake LLM sends
// its first chunk, then pauses: while it pauses, the first chunk must already
// be on the page and the end-of-reply marker must not. Then the reply must
// complete, survive a reload, and show model markup as text.
func TutorStreamsReply(t *testing.T) {
	page, tutorURL := tutorUser(t, "tutor", true)
	components.NavigateTo(t, page, tutorURL)

	if heading := components.GetHeading(t, page); heading != "Tutor" {
		t.Fatalf("heading = %q", heading)
	}
	if front := textOf(t, page, ".tutor-card-front"); front != "hola" {
		t.Errorf("card shown = %q, want hola", front)
	}

	click(t, page, ".tutor-presets button:has-text('Explain')")

	// Mid-stream: first chunk shown, reply not finished.
	streaming := page.Locator(".chat-streaming .chat-text")
	if err := streaming.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".chat-streaming .chat-text:has-text('Hola!')").WaitFor(); err != nil {
		t.Fatalf("first chunk never appeared: %v", err)
	}
	if n := components.CountLocators(t, page, ".chat-done"); n != 0 {
		t.Fatal("reply already complete: the page was not streamed")
	}

	waitAttached(t, page, ".chat-done")
	assertNoEdgeWhitespace(t, page, ".chat-streaming .chat-text")
	reply := textOf(t, page, ".chat-streaming .chat-text")
	if !strings.Contains(reply, "You asked: Explain this word") {
		t.Errorf("reply = %q, want the full streamed text", reply)
	}

	// After a reload the reply comes from the database, finished.
	components.NavigateTo(t, page, tutorURL)
	if n := components.CountLocators(t, page, ".chat-streaming"); n != 0 {
		t.Error("reply still marked as streaming after it finished")
	}
	assertNoEdgeWhitespace(t, page, ".chat-assistant .chat-text")
	saved := textOf(t, page, ".chat-assistant .chat-text")
	if !strings.Contains(saved, "<b>bold</b> stays text.") {
		t.Errorf("saved reply = %q, want the model's markup shown as text", saved)
	}
	if n := components.CountLocators(t, page, ".chat-text b"); n != 0 {
		t.Error("model output was rendered as HTML")
	}
	if q := textOf(t, page, ".chat-user .chat-text"); !strings.HasPrefix(q, "Explain this word") {
		t.Errorf("question = %q, want the Explain preset", q)
	}
}

func TutorFreeTextQuestion(t *testing.T) {
	page, tutorURL := tutorUser(t, "tutorq", true)
	components.NavigateTo(t, page, tutorURL)

	fill(t, page, "textarea[name=question]", "Is it formal?")
	click(t, page, ".chat-form button:has-text('Ask')")
	waitAttached(t, page, ".chat-done")

	if reply := textOf(t, page, ".chat-streaming .chat-text"); !strings.Contains(reply, "You asked: Is it formal?") {
		t.Errorf("reply = %q", reply)
	}
}

func TutorStartOverClears(t *testing.T) {
	page, tutorURL := tutorUser(t, "tutorreset", true)
	components.NavigateTo(t, page, tutorURL)
	click(t, page, ".tutor-presets button:has-text('Quiz me')")
	waitAttached(t, page, ".chat-done")

	components.NavigateTo(t, page, tutorURL)
	click(t, page, "button:has-text('Start Over')") // confirm() is auto-accepted
	components.WaitForURL(t, page, "**"+tutorURL)

	if n := components.CountLocators(t, page, ".chat-msg"); n != 0 {
		t.Errorf("%d messages left after Start Over", n)
	}
}

func TutorNeedsProvider(t *testing.T) {
	page, tutorURL := tutorUser(t, "tutornocfg", false)
	components.NavigateTo(t, page, tutorURL)

	if n := components.CountLocators(t, page, ".form-error a[href='/profile/ai']"); n != 1 {
		t.Error("expected a pointer to AI Provider settings")
	}
	if n := components.CountLocators(t, page, ".chat-form"); n != 0 {
		t.Error("question form shown with no provider configured")
	}
}

func TutorOtherUsersCardIs404(t *testing.T) {
	_, tutorURL := tutorUser(t, "tutorowner", false)

	intruder := helpers.NewAnonymousPage(t)
	registerThrough(t, intruder, uniqueEmail("tutorintruder"), "supersecret")
	components.WaitForURL(t, intruder, configuration.BaseURL+"/")

	resp, err := intruder.Goto(configuration.BaseURL + tutorURL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status() != 404 {
		t.Errorf("GET another user's tutor page = %d, want 404", resp.Status())
	}
}

// TutorLinkAfterReveal: the link appears only once the answer is shown.
func TutorLinkAfterReveal(t *testing.T) {
	page := components.NewPage(t)
	deckURL := actions.CreateDeck(t, page, "Tutor Link Deck")
	defer actions.DeleteDeck(t, page, deckURL)
	actions.ImportCSV(t, page, deckURL, configuration.SingleCardCSVPath)

	actions.StartFlashcardSession(t, page, deckURL)
	if components.IsVisible(t, page, ".tutor-link") {
		t.Error("tutor link visible before the answer is revealed")
	}
	click(t, page, "#reveal-btn")
	if !components.IsVisible(t, page, ".tutor-link") {
		t.Fatal("tutor link not visible after reveal")
	}
	click(t, page, ".tutor-link")
	components.WaitForURL(t, page, "**/tutor")
}

// assertNoEdgeWhitespace checks a reply's raw text has no leading or
// trailing whitespace. Replies render with white-space: pre-wrap, so any
// there shows as empty lines — from the model (Qwen3 starts with blank
// lines) or from template whitespace inside the element. textOf trims, so
// it cannot see this.
func assertNoEdgeWhitespace(t *testing.T, page playwright.Page, selector string) {
	t.Helper()
	raw, err := page.Locator(selector).First().TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if raw != strings.TrimSpace(raw) {
		t.Errorf("%s has edge whitespace, which pre-wrap renders as blank lines: %q", selector, raw)
	}
}
