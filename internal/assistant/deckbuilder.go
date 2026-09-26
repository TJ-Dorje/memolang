package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/models"
)

// DeckBuilderGreeting opens every deck-builder conversation. It is fixed text
// shown by the page, not a model turn: the first question is always the same,
// so asking a model for it would cost a call and add a wait for nothing.
const DeckBuilderGreeting = "Hi! Let's build a deck together. What language do you want to learn, and what do you want to use it for?"

// DeckBuilderPrompt steers the interview. The model gathers what card
// generation needs, then states the plan in a PlanMarker block, which the
// page shows as a deck card with a Generate button. If it never does, the
// learner can press Create deck, which runs SummarizeDeck instead.
func DeckBuilderPrompt() string {
	return `You are a friendly assistant inside MemoLang, a flashcard app, helping a learner design a new deck of vocabulary flashcards. The app has already greeted them with: "` + DeckBuilderGreeting + `" Their first message answers that.

Find out, one short question at a time:
- the language they are learning;
- why they are learning it, and their level (beginner, intermediate, advanced);
- the topic or situation the cards should cover (e.g. ordering food, business emails, 100 most common verbs);
- roughly how many cards they want (suggest 20 if they have no preference; at most 50);
- how they want to study it: "SRS" (spaced repetition — each session shows only the cards due that day, best for remembering long-term) or "Linear" (straight through the deck, new cards first, always something to study). Suggest SRS if they are unsure.

Ask at most one question per reply, and skip anything they have already told you. Suggest concrete options when they are unsure. Keep every reply under about 60 words, in plain text without Markdown.

When you know all five, reply with one short sentence inviting them to check the plan and press "Generate deck" (or tell you what to change), and end that reply with the plan in exactly this form:

` + PlanMarker + `
{"name": "short deck title, at most 40 characters", "language": "the language, in English", "prompt": "one paragraph telling a flashcard generator what to produce: topic, level, goal and any preferences", "count": 20, "mode": "srs"}
` + "```" + `

The app shows that block to the learner as a card with a Generate button — never mention JSON or the block itself. If they ask for changes afterwards, reply briefly and end with a new, complete block. Never write the block before you know all five. You cannot create the deck yourself; the learner does that with the button.`
}

// DeckSpec is what the interview produces: the fields the review form shows
// before cards are generated.
type DeckSpec struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Prompt   string `json:"prompt"`
	Count    int    `json:"count"`
	// Mode is the deck's study mode, "srs" or "linear".
	Mode string `json:"mode"`
}

const (
	defaultDeckCards = 20
	minDeckCards     = 5
	maxDeckCards     = 50
)

// ErrNothingToSummarize is returned when the learner has not said anything yet.
var ErrNothingToSummarize = errors.New("the conversation is empty")

const summaryPrompt = `You turn a conversation between a learner and a deck-design assistant into a specification for generating vocabulary flashcards.

Output ONLY a JSON object, no other text:
{"name": "...", "language": "...", "prompt": "...", "count": 20, "mode": "srs"}

- name: a short deck title, at most 40 characters, e.g. "Spanish — Ordering Food".
- language: the language being learned, in English, e.g. "Spanish".
- prompt: one paragraph telling a flashcard generator exactly what to produce: the topic or situation, the learner's level and goal, and any preferences they mentioned (formal/informal, regional variant, word types). Do not include the card count here.
- count: the number of cards they asked for, 20 if they did not say, never more than 50.
- mode: "linear" if they chose to study straight through the deck, otherwise "srs".`

// SummarizeDeck asks the model to turn the user's deck-builder conversation
// into a DeckSpec. It is one ordinary (non-streamed) call; the page shows a
// waiting screen meanwhile.
func (s *Service) SummarizeDeck(ctx context.Context, userID, conversationID int64) (DeckSpec, error) {
	provider, err := s.Provider(s.DB, userID)
	if err != nil {
		return DeckSpec{}, err
	}
	msgs, err := models.GetConversationMessages(s.DB, userID, conversationID)
	if err != nil {
		return DeckSpec{}, err
	}

	transcript := transcriptOf(msgs)
	if transcript == "" {
		return DeckSpec{}, ErrNothingToSummarize
	}

	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	raw, err := provider.Chat(ctx, ai.ChatRequest{
		System: summaryPrompt,
		// The transcript is sent as one user turn rather than replayed as
		// turns: this call is a different task from the interview, and the
		// model should read the conversation, not continue it.
		Messages: []ai.ChatMessage{{Role: "user", Content: "Conversation:\n\n" + transcript}},
	}, func(string) error { return nil })
	if err != nil {
		return DeckSpec{}, err
	}
	return parseDeckSpec(raw)
}

// transcriptOf renders the finished turns as "Assistant:/Learner:" lines,
// starting with the fixed greeting the learner was answering.
func transcriptOf(msgs []models.ConversationMessage) string {
	var b strings.Builder
	learnerSpoke := false
	for _, m := range msgs {
		if m.Status != "done" || m.Content == "" {
			continue
		}
		speaker := "Assistant"
		if m.Role == "user" {
			speaker = "Learner"
			learnerSpoke = true
		}
		fmt.Fprintf(&b, "%s: %s\n\n", speaker, m.Content)
	}
	if !learnerSpoke {
		return ""
	}
	return "Assistant: " + DeckBuilderGreeting + "\n\n" + b.String()
}

// parseDeckSpec reads the model's JSON, tolerating a Markdown fence or text
// around the object, and fills in or clamps what is missing or out of range.
// The result is only ever a pre-filled form, so defaults beat failing.
func parseDeckSpec(raw string) (DeckSpec, error) {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return DeckSpec{}, fmt.Errorf("no JSON object in the summary (got %.200q)", raw)
	}

	// Decoded loosely, then coerced field by field: models write "count": "20"
	// or leave a trailing comma, and a strict decode into DeckSpec turned
	// either into no plan at all.
	object := trailingComma.ReplaceAllString(raw[start:end+1], "$1")
	var fields map[string]any
	if err := json.Unmarshal([]byte(object), &fields); err != nil {
		return DeckSpec{}, fmt.Errorf("parse summary: %w (got %.200q)", err, raw)
	}

	spec := DeckSpec{
		Name:     looseString(fields["name"]),
		Language: looseString(fields["language"]),
		Prompt:   looseString(fields["prompt"]),
		Count:    looseInt(fields["count"]),
		Mode:     strings.ToLower(looseString(fields["mode"])),
	}

	if spec.Count == 0 {
		spec.Count = defaultDeckCards
	}
	spec.Count = min(max(spec.Count, minDeckCards), maxDeckCards)
	if spec.Name == "" && spec.Language != "" {
		spec.Name = spec.Language + " deck"
	}
	if spec.Mode != "linear" {
		spec.Mode = "srs"
	}
	return spec, nil
}

// GenerationPrompt is the text handed to card generation, which has no
// separate count parameter.
func (spec DeckSpec) GenerationPrompt() string {
	return fmt.Sprintf("%s\n\nGenerate %d cards.", spec.Prompt, spec.Count)
}

// trailingComma matches a comma before a closing brace or bracket, which
// JSON forbids and models often write.
var trailingComma = regexp.MustCompile(`,\s*([}\]])`)

// looseString reads a JSON value as text: strings as-is, numbers and
// booleans formatted, anything else empty.
func looseString(v any) string {
	switch v := v.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64, bool:
		return fmt.Sprint(v)
	}
	return ""
}

// looseInt reads a JSON number, or a string starting with one ("20",
// "20 cards"); 0 when there is none, which parseDeckSpec turns into the
// default.
func looseInt(v any) int {
	switch v := v.(type) {
	case float64:
		return int(v)
	case string:
		digits := strings.TrimSpace(v)
		end := 0
		for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
			end++
		}
		n, _ := strconv.Atoi(digits[:end])
		return n
	}
	return 0
}
