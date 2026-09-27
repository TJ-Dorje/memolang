package assistant

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"memolang/internal/ai"
	"memolang/internal/models"
)

// Deck generation runs in the background like a chat reply: GenerateDeck
// creates the deck and returns at once, a goroutine streams the model's
// reply, saves each card the moment its JSON object is complete, and
// publishes an event per card for the progress page. The database is the
// record — a reload follows the stream while it lasts and simply opens the
// deck afterwards — and a reply cut short still keeps every complete card.

// Card is one generated flashcard.
type Card struct {
	Front   string `json:"front"`
	Back    string `json:"back"`
	Example string `json:"example"`
}

// generationMaxTokens caps a generation reply. It must leave room for a
// thinking model's reasoning, which counts against it on most servers, and
// stay within what hosted APIs accept for their smaller models.
const generationMaxTokens = 8192

// CardGenerationPrompt is the system prompt for generating cards.
func CardGenerationPrompt(language string) string {
	return `You are a flashcard generator for language learning. Given a target language and a topic description, generate a set of flashcards.

Output ONLY valid JSON — an array of objects with these keys:
- "front": the word/phrase in ` + language + `
- "back": the translation in English
- "example": a short example sentence in ` + language + ` using the word/phrase, followed by its English translation in brackets. Format: "Target sentence (English translation)"

Do not include any other text, markdown, or explanation. Just the JSON array.`
}

func cardUserPrompt(language, prompt string) string {
	return fmt.Sprintf("Target language: %s\nTopic: %s", language, prompt)
}

// cardScanner finds the complete objects of a JSON array while it streams
// in, so each card can be saved and shown as soon as it is finished, and a
// reply cut off mid-card still yields every card before it. It skips
// anything before the first "[" (a fence, a preamble) and tracks object
// depth outside strings, honouring escapes, so a brace inside an example
// sentence is not structure. It works on bytes: every character it acts on
// is ASCII, and bytes of multi-byte UTF-8 characters are all >= 0x80.
type cardScanner struct {
	started  bool
	depth    int
	inString bool
	escaped  bool
	object   strings.Builder
}

// Write consumes the next chunk and returns the cards it completed.
func (s *cardScanner) Write(chunk string) []Card {
	var cards []Card
	for i := 0; i < len(chunk); i++ {
		if card, ok := s.step(chunk[i]); ok {
			cards = append(cards, card)
		}
	}
	return cards
}

func (s *cardScanner) step(b byte) (Card, bool) {
	if !s.started {
		s.started = b == '['
		return Card{}, false
	}
	if s.depth == 0 {
		// Between objects: commas, whitespace, the closing "]".
		if b == '{' {
			s.depth = 1
			s.object.Reset()
			s.object.WriteByte(b)
		}
		return Card{}, false
	}

	s.object.WriteByte(b)
	if s.inString {
		switch {
		case s.escaped:
			s.escaped = false
		case b == '\\':
			s.escaped = true
		case b == '"':
			s.inString = false
		}
		return Card{}, false
	}

	switch b {
	case '"':
		s.inString = true
	case '{':
		s.depth++
	case '}':
		s.depth--
		if s.depth == 0 {
			return decodeCard(s.object.String())
		}
	}
	return Card{}, false
}

// decodeCard reads one completed object; one without a front and back is
// skipped rather than saved as a blank card.
func decodeCard(object string) (Card, bool) {
	var c Card
	if err := json.Unmarshal([]byte(object), &c); err != nil {
		return Card{}, false
	}
	c.Front, c.Back, c.Example = strings.TrimSpace(c.Front), strings.TrimSpace(c.Back), strings.TrimSpace(c.Example)
	return c, c.Front != "" && c.Back != ""
}

// GenerationEvent is one line of a generation's progress stream.
type GenerationEvent struct {
	Type    string `json:"type"` // "thinking", "card", "done" or "failed"
	Card    *Card  `json:"card,omitempty"`
	Saved   int    `json:"saved,omitempty"`
	Message string `json:"message,omitempty"`
}

// GenerationKey names a deck's generation in the broker.
func GenerationKey(deckID int64) string {
	return "deck-generation-" + strconv.FormatInt(deckID, 10)
}

// GenerateDeck creates the deck the plan describes and starts filling it in
// the background, returning its id straight away. The provider is resolved
// first, so an unconfigured user gets an error and no empty deck.
func (s *Service) GenerateDeck(userID int64, spec DeckSpec) (int64, error) {
	provider, err := s.Provider(s.DB, userID)
	if err != nil {
		return 0, err
	}
	deck, err := models.CreateDeck(s.DB, userID, spec.Name, spec.Mode)
	if err != nil {
		return 0, err
	}

	key := GenerationKey(deck.ID)
	s.Broker.Start(key)
	go s.generateCards(provider, userID, deck.ID, spec, key)
	return deck.ID, nil
}

func (s *Service) generateCards(provider ai.Provider, userID, deckID int64, spec DeckSpec, key string) {
	publish := func(ev GenerationEvent) {
		line, _ := json.Marshal(ev)
		s.Broker.Publish(key, string(line)+"\n")
	}

	w := newWatchdog(s.IdleTimeout, s.MaxDuration)
	defer w.Close()

	var scanner cardScanner
	saved, thinking := 0, false
	_, err := provider.Chat(w.ctx, ai.ChatRequest{
		System:    CardGenerationPrompt(spec.Language),
		Messages:  []ai.ChatMessage{{Role: "user", Content: cardUserPrompt(spec.Language, spec.GenerationPrompt())}},
		MaxTokens: generationMaxTokens,
		OnThinking: func(string) {
			w.Alive()
			if !thinking {
				thinking = true
				publish(GenerationEvent{Type: "thinking"})
			}
		},
	}, func(chunk string) error {
		w.Alive()
		for _, card := range scanner.Write(chunk) {
			if _, err := models.CreateCard(s.DB, deckID, card.Front, card.Back, card.Example, ""); err != nil {
				// Most often a duplicate front, which the deck refuses.
				log.Printf("generate: deck %d: skipped %q: %v", deckID, card.Front, err)
				continue
			}
			saved++
			publish(GenerationEvent{Type: "card", Card: &card, Saved: saved})
		}
		return nil
	})
	err = w.explain(err)
	if err != nil {
		log.Printf("generate: deck %d: %d cards saved, then: %v", deckID, saved, err)
	}

	s.finishGeneration(userID, deckID, saved, err, publish)
	s.Broker.Finish(key, nil)
}

// finishGeneration settles the outcome. With cards, the deck stands — cut
// short or not — and the interview that designed it is over. With none, the
// empty deck is removed and the reason goes into the chat, where the plan
// card is still waiting to be generated again.
func (s *Service) finishGeneration(userID, deckID int64, saved int, err error, publish func(GenerationEvent)) {
	if saved > 0 {
		msg := ""
		if err != nil {
			msg = "The model's reply stopped early (" + err.Error() + "), so the deck has fewer cards than planned."
		}
		publish(GenerationEvent{Type: "done", Saved: saved, Message: msg})
		if convID, _ := models.FindDeckBuilderConversation(s.DB, userID); convID != 0 {
			models.DeleteConversation(s.DB, userID, convID)
		}
		return
	}

	reason := "the model's reply contained no complete cards"
	if err != nil {
		reason = err.Error()
	}
	if delErr := models.DeleteDeck(s.DB, userID, deckID); delErr != nil {
		log.Printf("generate: removing empty deck %d: %v", deckID, delErr)
	}
	s.noteInChat(userID, "I couldn't generate the cards: "+reason+". Press Generate deck to try again, or switch provider with the AI switch above.")
	publish(GenerationEvent{Type: "failed", Message: reason})
}

// noteInChat adds an assistant message to the user's deck-builder
// conversation, so a failure is still visible after the progress page.
func (s *Service) noteInChat(userID int64, text string) {
	convID, err := models.FindDeckBuilderConversation(s.DB, userID)
	if err != nil || convID == 0 {
		return
	}
	if _, err := models.AddConversationMessage(s.DB, convID, "assistant", text, "done"); err != nil {
		log.Printf("generate: noting failure in chat: %v", err)
	}
}
