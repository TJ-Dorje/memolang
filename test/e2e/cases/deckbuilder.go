// New case funcs MUST be registered in test/e2e/main_test.go, or they will
// silently never run.
package cases

import (
	"fmt"
	"strings"
	"testing"

	"memolang/test/e2e/configuration"
	"memolang/test/e2e/helpers"
	"memolang/test/e2e/helpers/components"

	playwright "github.com/playwright-community/playwright-go"
)

func sendToBuilder(t *testing.T, page playwright.Page, text string) {
	t.Helper()
	fill(t, page, "textarea[name=question]", text)
	click(t, page, ".chat-form button:has-text('Send')")
	waitAttached(t, page, ".chat-done")
}

// generateAndCheckDeck presses Generate Deck on the current plan card and
// checks the deck that comes out, returning its page.
func generateAndCheckDeck(t *testing.T, page playwright.Page, wantMode string) {
	t.Helper()
	click(t, page, ".plan-card button:has-text('Generate Deck')")
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
	if mode := textOf(t, page, ".deck-mode"); mode != wantMode {
		t.Errorf("deck mode = %q, want %q", mode, wantMode)
	}
}

// DeckBuilderPlanInChat walks the main flow: interview → the assistant's
// plan appears as a card in the chat (its JSON never shown, even while
// streaming) → Generate Deck → the deck, with the plan's study mode; the
// finished interview is cleared.
func DeckBuilderPlanInChat(t *testing.T) {
	page, email := freshUser(t, "builder")
	useFakeLLM(t, email)

	components.NavigateTo(t, page, "/decks/new")
	click(t, page, "a:has-text('Create with AI assistant')")
	components.WaitForURL(t, page, "**/decks/new/assistant")

	// The fixed greeting opens the conversation without a model call.
	if greeting := textOf(t, page, ".chat-assistant .chat-text"); !strings.Contains(greeting, "What language") {
		t.Errorf("greeting = %q", greeting)
	}

	sendToBuilder(t, page, "Spanish, for a trip to Mexico")
	if n := components.CountLocators(t, page, ".plan-card"); n != 0 {
		t.Fatal("plan card shown before the assistant proposed one")
	}
	if n := components.CountLocators(t, page, "button:has-text('Create Deck Plan')"); n != 1 {
		t.Error("expected the Create Deck Plan fallback while there is no plan")
	}

	sendToBuilder(t, page, "Sounds good, I'm "+helpers.FakePlanTrigger)
	reply := textOf(t, page, ".chat-streaming .chat-text")
	if strings.Contains(reply, "```") || strings.Contains(reply, "{") {
		t.Errorf("plan block leaked into the streamed text: %q", reply)
	}
	if !strings.Contains(reply, "check the plan") {
		t.Errorf("streamed text = %q, want the sentence before the plan", reply)
	}
	if name := textOf(t, page, ".chat-streaming .plan-card .plan-name"); name != helpers.FakeDeckName {
		t.Errorf("plan card name = %q", name)
	}
	if n := components.CountLocators(t, page, "button:has-text('Create Deck Plan')"); n != 0 {
		t.Error("fallback still offered although a plan exists")
	}

	// After a reload the card comes from the stored message, text still clean.
	components.NavigateTo(t, page, "/decks/new/assistant")
	stored := textOf(t, page, ".chat-msg:last-child .chat-text")
	if strings.Contains(stored, "```") || strings.Contains(stored, "{") {
		t.Errorf("plan block shown after reload: %q", stored)
	}
	facts := textOf(t, page, ".plan-card .plan-facts")
	for _, want := range []string{helpers.FakeDeckLanguage, fmt.Sprint(helpers.FakeDeckCount), "Linear", helpers.FakeDeckPrompt} {
		if !strings.Contains(facts, want) {
			t.Errorf("plan card lacks %q: %q", want, facts)
		}
	}

	generateAndCheckDeck(t, page, "Linear")

	// The interview that made this deck is over.
	components.NavigateTo(t, page, "/decks/new/assistant")
	if n := components.CountLocators(t, page, ".chat-user"); n != 0 {
		t.Errorf("finished interview still shows %d learner messages", n)
	}
}

// DeckBuilderNewerPlanSupersedes: asking for changes produces a new card;
// only the newest can be generated.
func DeckBuilderNewerPlanSupersedes(t *testing.T) {
	page, email := freshUser(t, "buildertwice")
	useFakeLLM(t, email)
	components.NavigateTo(t, page, "/decks/new/assistant")

	sendToBuilder(t, page, "Spanish, I'm "+helpers.FakePlanTrigger)
	sendToBuilder(t, page, "Make it more formal, I'm "+helpers.FakePlanTrigger)

	components.NavigateTo(t, page, "/decks/new/assistant")
	if n := components.CountLocators(t, page, ".plan-card"); n != 2 {
		t.Fatalf("%d plan cards, want 2", n)
	}
	if n := components.CountLocators(t, page, ".plan-superseded"); n != 1 {
		t.Errorf("%d superseded cards, want 1", n)
	}
	if n := components.CountLocators(t, page, ".plan-card button:has-text('Generate Deck')"); n != 1 {
		t.Errorf("%d Generate buttons, want only the newest card's", n)
	}
}

// DeckBuilderFallbackPlan: when the model never writes a plan, Create Deck
// Plan summarises the chat into one, shown as the same card.
func DeckBuilderFallbackPlan(t *testing.T) {
	page, email := freshUser(t, "builderfallback")
	useFakeLLM(t, email)
	components.NavigateTo(t, page, "/decks/new/assistant")

	sendToBuilder(t, page, "Spanish, for a trip to Mexico")
	click(t, page, "button:has-text('Create Deck Plan')")
	components.WaitForURL(t, page, "**/decks/new/assistant")

	if name := textOf(t, page, ".plan-card .plan-name"); name != helpers.FakeDeckName {
		t.Fatalf("fallback plan card name = %q", name)
	}
	// The summary fake gives no mode, so the default applies.
	generateAndCheckDeck(t, page, "SRS")
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

// OldAIFormLeadsToAssistant: the old AI form is gone; its URL now starts the
// assistant, for anyone with it bookmarked.
func OldAIFormLeadsToAssistant(t *testing.T) {
	page := components.NewPage(t)
	components.NavigateTo(t, page, "/decks/new?ai_mode=true")
	components.WaitForURL(t, page, "**/decks/new/assistant")
}

func DeckBuilderStartOver(t *testing.T) {
	page, email := freshUser(t, "builderreset")
	useFakeLLM(t, email)
	components.NavigateTo(t, page, "/decks/new/assistant")
	sendToBuilder(t, page, "German")

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

// DeckBuilderUnreadablePlan: a plan block the parser can't use must say so
// and leave the fallback available, not silently show nothing.
func DeckBuilderUnreadablePlan(t *testing.T) {
	page, email := freshUser(t, "builderbadplan")
	useFakeLLM(t, email)
	components.NavigateTo(t, page, "/decks/new/assistant")

	fill(t, page, "textarea[name=question]", "German, "+helpers.FakeBadPlanTrigger)
	click(t, page, ".chat-form button:has-text('Send')")
	components.WaitForURL(t, page, "**/decks/new/assistant")
	components.NavigateTo(t, page, "/decks/new/assistant")

	if note := textOf(t, page, ".plan-note"); !strings.Contains(note, "couldn't be read") {
		t.Errorf("note = %q, want the unreadable-plan note", note)
	}
	if text := textOf(t, page, ".chat-assistant:last-child .chat-text"); strings.Contains(text, "{") {
		t.Errorf("broken plan block shown as text: %q", text)
	}
	if n := components.CountLocators(t, page, "button:has-text('Create Deck Plan')"); n != 1 {
		t.Error("fallback not offered after an unreadable plan")
	}
}

// DeckBuilderShowsProviderError: a server that answers the chat route with
// 200 and a JSON error (LM Studio behind a base URL missing /v1) used to
// produce an empty "successful" reply. It must show as an error pointing at
// the provider settings — and fail Test Connection too.
func DeckBuilderShowsProviderError(t *testing.T) {
	page, email := freshUser(t, "builderbroken")
	useFakeLLMAt(t, email, configuration.FakeLLMURL+helpers.FakeBrokenPrefix)
	components.NavigateTo(t, page, "/decks/new/assistant")

	fill(t, page, "textarea[name=question]", "German")
	click(t, page, ".chat-form button:has-text('Send')")
	components.WaitForURL(t, page, "**/decks/new/assistant")
	// The failure is immediate, so this may be the live stream or the stored
	// reply; both must show the error.
	components.NavigateTo(t, page, "/decks/new/assistant")
	if msg := textOf(t, page, ".chat-error"); !strings.Contains(msg, "AI Provider") {
		t.Errorf("error = %q, want a pointer to the AI Provider settings", msg)
	}

	components.NavigateTo(t, page, "/profile/ai")
	click(t, page, "button:has-text('Test Connection')")
	components.WaitForURL(t, page, "**/profile/ai")
	if flash := components.GetFlash(t, page); !strings.Contains(flash, "Connection failed") {
		t.Errorf("Test Connection flash = %q, want a failure", flash)
	}
}
