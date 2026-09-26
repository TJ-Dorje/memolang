package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/models"
)

// DeckBuilderGreeting opens every deck-builder conversation. It is fixed text
// shown by the page, not a model turn: the first question is always the same,
// so asking a model for it would cost a call and add a wait for nothing.
const DeckBuilderGreeting = "Hi! Let's build a deck together. What language do you want to learn, and what do you want to use it for?"

// DeckBuilderPrompt steers the interview. The model gathers what card
// generation needs and says when it has enough; the learner decides when to
// stop by pressing Create deck, which runs SummarizeDeck.
func DeckBuilderPrompt() string {
	return `You are a friendly assistant inside MemoLang, a flashcard app, helping a learner design a new deck of vocabulary flashcards. The app has already greeted them with: "` + DeckBuilderGreeting + `" Their first message answers that.

Find out, one short question at a time:
- the language they are learning;
- why they are learning it, and their level (beginner, intermediate, advanced);
- the topic or situation the cards should cover (e.g. ordering food, business emails, 100 most common verbs);
- roughly how many cards they want (suggest 20 if they have no preference; at most 50).

Ask at most one question per reply, and skip anything they have already told you. Suggest concrete options when they are unsure. Once you know the language, the topic and a rough size, say briefly what the deck will contain and tell them they can press "Create deck" to review it — they can also keep refining it with you.

Keep every reply under about 60 words. Plain text, no Markdown. You cannot create the deck yourself; the learner does that with the button.`
}

// DeckSpec is what the interview produces: the fields the review form shows
// before cards are generated.
type DeckSpec struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Prompt   string `json:"prompt"`
	Count    int    `json:"count"`
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
{"name": "...", "language": "...", "prompt": "...", "count": 20}

- name: a short deck title, at most 40 characters, e.g. "Spanish — Ordering Food".
- language: the language being learned, in English, e.g. "Spanish".
- prompt: one paragraph telling a flashcard generator exactly what to produce: the topic or situation, the learner's level and goal, and any preferences they mentioned (formal/informal, regional variant, word types). Do not include the card count here.
- count: the number of cards they asked for, 20 if they did not say, never more than 50.`

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

	var spec DeckSpec
	if err := json.Unmarshal([]byte(raw[start:end+1]), &spec); err != nil {
		return DeckSpec{}, fmt.Errorf("parse summary: %w (got %.200q)", err, raw)
	}

	spec.Name = strings.TrimSpace(spec.Name)
	spec.Language = strings.TrimSpace(spec.Language)
	spec.Prompt = strings.TrimSpace(spec.Prompt)

	if spec.Count == 0 {
		spec.Count = defaultDeckCards
	}
	spec.Count = min(max(spec.Count, minDeckCards), maxDeckCards)
	if spec.Name == "" && spec.Language != "" {
		spec.Name = spec.Language + " deck"
	}
	return spec, nil
}

// GenerationPrompt is the text handed to card generation, which has no
// separate count parameter.
func (spec DeckSpec) GenerationPrompt() string {
	return fmt.Sprintf("%s\n\nGenerate %d cards.", spec.Prompt, spec.Count)
}
