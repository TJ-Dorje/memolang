// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"fmt"
	"strings"
	"testing"

	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"
)

// DeckBuilderInterviewToDeck walks the whole flow: New deck → assistant →
// answer (streamed reply) → Create Deck → review form pre-filled from the
// summary → Generate → a deck with the generated cards; the finished
// interview is cleared.
func DeckBuilderInterviewToDeck(t *testing.T) {
	page, email := freshUser(t, "builder")
	useFakeLLM(t, email)

	components.NavigateTo(t, page, "/decks/new")
	click(t, page, "a:has-text('Create with AI assistant')")
	components.WaitForURL(t, page, "**/decks/new/assistant")

	// The fixed greeting opens the conversation without a model call.
	if greeting := textOf(t, page, ".chat-assistant .chat-text"); !strings.Contains(greeting, "What language") {
		t.Errorf("greeting = %q", greeting)
	}
	if n := components.CountLocators(t, page, "button:has-text('Create Deck')"); n != 0 {
		t.Error("Create Deck offered before the learner said anything")
	}

	fill(t, page, "textarea[name=question]", "Spanish, for a trip to Mexico")
	click(t, page, ".chat-form button:has-text('Send')")
	waitAttached(t, page, ".chat-done")
	if reply := textOf(t, page, ".chat-streaming .chat-text"); !strings.Contains(reply, "You asked: Spanish, for a trip to Mexico") {
		t.Errorf("interview reply = %q", reply)
	}

	click(t, page, "button:has-text('Create Deck')")
	components.WaitForURL(t, page, "**/decks/new?ai_mode=true&token=*")

	if heading := components.GetHeading(t, page); heading != "Review Your Deck" {
		t.Errorf("heading = %q, want the review step", heading)
	}
	if n := components.CountLocators(t, page, ".mode-option .hint"); n != 2 {
		t.Errorf("review form explains %d study modes, want 2", n)
	}
	for selector, want := range map[string]string{
		"input[name=name]":     helpers.FakeDeckName,
		"input[name=language]": helpers.FakeDeckLanguage,
	} {
		if got := components.GetInputValue(t, page, selector); got != want {
			t.Errorf("%s = %q, want %q", selector, got, want)
		}
	}
	prompt := components.GetInputValue(t, page, "textarea[name=prompt]")
	if !strings.HasPrefix(prompt, helpers.FakeDeckPrompt) || !strings.HasSuffix(prompt, fmt.Sprintf("Generate %d cards.", helpers.FakeDeckCount)) {
		t.Errorf("prompt = %q, want the summary plus the card count", prompt)
	}

	click(t, page, "button:has-text('Generate Cards')")
	components.WaitForURL(t, page, "**/decks/*")
	if flash := components.GetFlash(t, page); flash != fmt.Sprintf("Generated %d cards.", len(helpers.FakeCards)) {
		t.Errorf("flash = %q", flash)
	}
	if heading := components.GetHeading(t, page); heading != helpers.FakeDeckName {
		t.Errorf("deck heading = %q, want %q", heading, helpers.FakeDeckName)
	}
	if n := components.CountLocators(t, page, ".card-row"); n != len(helpers.FakeCards) {
		t.Errorf("deck has %d cards, want %d", n, len(helpers.FakeCards))
	}

	// The interview that made this deck is over.
	components.NavigateTo(t, page, "/decks/new/assistant")
	if n := components.CountLocators(t, page, ".chat-user"); n != 0 {
		t.Errorf("finished interview still shows %d learner messages", n)
	}
}

func DeckBuilderNeedsProvider(t *testing.T) {
	page, _ := freshUser(t, "buildernocfg")
	components.NavigateTo(t, page, "/decks/new/assistant")

	if n := components.CountLocators(t, page, ".form-error a[href='/profile/ai']"); n != 1 {
		t.Error("expected a pointer to AI Provider settings")
	}
	if n := components.CountLocators(t, page, ".chat-form"); n != 0 {
		t.Error("answer form shown with no provider configured")
	}
}

// OldAIFormLeadsToAssistant: the old form is now only the review step, so
// opening it with nothing to review starts the assistant instead.
func OldAIFormLeadsToAssistant(t *testing.T) {
	page := components.NewPage(t)
	components.NavigateTo(t, page, "/decks/new?ai_mode=true")
	components.WaitForURL(t, page, "**/decks/new/assistant")
}

func DeckBuilderStartOver(t *testing.T) {
	page, email := freshUser(t, "builderreset")
	useFakeLLM(t, email)
	components.NavigateTo(t, page, "/decks/new/assistant")

	fill(t, page, "textarea[name=question]", "German")
	click(t, page, ".chat-form button:has-text('Send')")
	waitAttached(t, page, ".chat-done")

	components.NavigateTo(t, page, "/decks/new/assistant")
	click(t, page, "button:has-text('Start Over')") // confirm() is auto-accepted
	components.WaitForURL(t, page, "**/decks/new/assistant")
	if n := components.CountLocators(t, page, ".chat-user"); n != 0 {
		t.Errorf("%d learner messages left after Start Over", n)
	}
	// The greeting is page text, so it is always there.
	if n := components.CountLocators(t, page, ".chat-assistant"); n != 1 {
		t.Errorf("assistant turns after Start Over = %d, want just the greeting", n)
	}
}
