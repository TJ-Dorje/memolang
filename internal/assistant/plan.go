package assistant

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PlanMarker opens the block in which the deck builder states its finished
// plan. The block is machine-readable: the page never shows it as text but
// renders it as a deck card with a Generate button.
const PlanMarker = "```deck"

// ExtractDeckPlan splits a stored reply into the text to show and the deck
// plan it carries, if any. Everything from the first PlanMarker on is hidden
// — exactly what PlanFilter hid while the reply streamed, so a reload shows
// the same text — and the plan is read from the last block, running to its
// closing fence or the end of the reply if the model never closed it. plan
// is nil when there is no block or it does not parse; the learner can still
// ask for one with Create deck.
func ExtractDeckPlan(content string) (visible string, plan *DeckSpec) {
	first := strings.Index(content, PlanMarker)
	if first < 0 {
		return content, nil
	}
	visible = strings.TrimRight(content[:first], " \t\n")

	last := strings.LastIndex(content, PlanMarker)
	body := content[last+len(PlanMarker):]
	if end := strings.Index(body, "```"); end >= 0 {
		body = body[:end]
	}
	spec, err := parseDeckSpec(body)
	if err != nil || spec.Language == "" || spec.Prompt == "" {
		return visible, nil
	}
	return visible, &spec
}

// FormatDeckPlan renders a plan as a block ExtractDeckPlan reads back, for
// storing a plan the app produced itself (the Create deck fallback).
func FormatDeckPlan(spec DeckSpec) string {
	b, _ := json.Marshal(spec)
	return fmt.Sprintf("%s\n%s\n```", PlanMarker, b)
}

// PlanFilter hides a reply's plan block while it streams. Text passes
// through until PlanMarker appears, and nothing after it is shown. The
// marker can arrive split across chunks ("``" then "`deck"), so any tail of
// the text so far that could be the start of the marker is held back until
// the next chunk settles it; Flush releases it when the reply ends without
// a marker.
type PlanFilter struct {
	held   string
	hidden bool
}

// Write takes the next chunk and returns the part that is safe to show now.
func (f *PlanFilter) Write(chunk string) string {
	if f.hidden {
		return ""
	}
	text := f.held + chunk
	if i := strings.Index(text, PlanMarker); i >= 0 {
		f.hidden, f.held = true, ""
		return text[:i]
	}

	keep := 0
	for k := min(len(PlanMarker)-1, len(text)); k > 0; k-- {
		if strings.HasSuffix(text, PlanMarker[:k]) {
			keep = k
			break
		}
	}
	// PlanMarker is ASCII, so cutting before a held-back tail of it never
	// splits a multi-byte character.
	f.held = text[len(text)-keep:]
	return text[:len(text)-keep]
}

// Flush returns whatever was held back, once the reply is complete.
func (f *PlanFilter) Flush() string {
	if f.hidden {
		return ""
	}
	held := f.held
	f.held = ""
	return held
}
