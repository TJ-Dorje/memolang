package assistant

import (
	"strings"
	"testing"
)

const samplePlan = "Here's your deck — check it below.\n\n```deck\n" +
	`{"name":"Spanish — Travel","language":"Spanish","prompt":"Travel phrases, beginner.","count":15,"mode":"linear"}` +
	"\n```"

func TestExtractDeckPlan(t *testing.T) {
	visible, plan, err := ExtractDeckPlan(samplePlan)
	if err != nil {
		t.Fatalf("ExtractDeckPlan: %v", err)
	}
	if visible != "Here's your deck — check it below." {
		t.Errorf("visible = %q", visible)
	}
	want := DeckSpec{Name: "Spanish — Travel", Language: "Spanish", Prompt: "Travel phrases, beginner.", Count: 15, Mode: "linear"}
	if plan == nil || *plan != want {
		t.Fatalf("plan = %+v, want %+v", plan, want)
	}
}

func TestExtractDeckPlanCases(t *testing.T) {
	for _, tc := range []struct {
		name, content, visible string
		hasPlan, unreadable    bool
	}{
		{"no block", "Just a question?", "Just a question?", false, false},
		{"text is trimmed", "\n\nHola\n\n", "Hola", false, false},
		{"unclosed block", "Plan:\n```deck\n{\"language\":\"German\",\"prompt\":\"verbs\"}", "Plan:", true, false},
		{"malformed block hides text, reports it", "Plan:\n```deck\n{not json\n```", "Plan:", false, true},
		{"missing language is reported", "Plan:\n```deck\n{\"prompt\":\"verbs\"}\n```", "Plan:", false, true},
		{"hidden from the first block, plan from the last", "old\n```deck\n{\"language\":\"A\",\"prompt\":\"a\"}\n```\nnew\n```deck\n{\"language\":\"B\",\"prompt\":\"b\"}\n```", "old", true, false},
	} {
		visible, plan, err := ExtractDeckPlan(tc.content)
		if (err != nil) != tc.unreadable {
			t.Errorf("%s: err = %v, want unreadable=%v", tc.name, err, tc.unreadable)
		}
		if visible != tc.visible {
			t.Errorf("%s: visible = %q, want %q", tc.name, visible, tc.visible)
		}
		if (plan != nil) != tc.hasPlan {
			t.Errorf("%s: plan = %+v, want present=%v", tc.name, plan, tc.hasPlan)
		}
	}
}

func TestPlanDefaultsModeToSRS(t *testing.T) {
	_, plan, _ := ExtractDeckPlan("```deck\n{\"language\":\"L\",\"prompt\":\"p\",\"mode\":\"weird\"}\n```")
	if plan == nil || plan.Mode != "srs" {
		t.Errorf("plan = %+v, want mode srs", plan)
	}
}

func TestFormatDeckPlanRoundTrips(t *testing.T) {
	spec := DeckSpec{Name: "N", Language: "L", Prompt: "P", Count: 12, Mode: "srs"}
	_, got, _ := ExtractDeckPlan("Plan:\n" + FormatDeckPlan(spec))
	if got == nil || *got != spec {
		t.Errorf("round trip = %+v, want %+v", got, spec)
	}
}

// streamThrough feeds chunks through a filter and returns what the page would show.
func streamThrough(chunks ...string) string {
	var f PlanFilter
	var out strings.Builder
	for _, c := range chunks {
		out.WriteString(f.Write(c))
	}
	out.WriteString(f.Flush())
	return out.String()
}

func TestPlanFilter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunks []string
		want   string
	}{
		{"no marker passes through", []string{"Hola, ", "amigo"}, "Hola, amigo"},
		{"marker in one chunk", []string{"Plan ready.\n```deck\n{\"a\":1}\n```"}, "Plan ready.\n"},
		{"marker split across chunks", []string{"Plan ready.\n``", "`de", "ck\n{\"a\":1}", "\n```"}, "Plan ready.\n"},
		{"lookalike released at the end", []string{"Use ``", "`code`"}, "Use ```code`"},
		{"lookalike released by next chunk", []string{"x ``", "`dec", "ade"}, "x ```decade"},
		{"backticks at the very end flushed", []string{"end ``"}, "end ``"},
		{"multibyte text before marker", []string{"¿Qué tal? ", "```deck{}"}, "¿Qué tal? "},
	} {
		if got := streamThrough(tc.chunks...); got != tc.want {
			t.Errorf("%s: shown %q, want %q", tc.name, got, tc.want)
		}
	}
}

// What streams must equal what the stored reply shows after a reload.
func TestPlanFilterMatchesExtract(t *testing.T) {
	var chunks []string
	for i := 0; i < len(samplePlan); i += 3 {
		chunks = append(chunks, samplePlan[i:min(i+3, len(samplePlan))])
	}
	streamed := streamThrough(chunks...)
	visible, _, _ := ExtractDeckPlan(samplePlan)
	if strings.TrimSpace(streamed) != visible {
		t.Errorf("streamed %q, stored view %q", streamed, visible)
	}
}

// Real models write plans a strict decoder rejects; those used to produce no
// card at all.
func TestPlanParsingIsForgiving(t *testing.T) {
	for name, block := range map[string]string{
		"count as string":  `{"name":"G","language":"German","prompt":"Zurich","count":"20","mode":"SRS"}`,
		"count with words": `{"name":"G","language":"German","prompt":"Zurich","count":"20 cards","mode":"srs"}`,
		"trailing comma":   `{"name":"G","language":"German","prompt":"Zurich","count":20,"mode":"srs",}`,
	} {
		_, plan, err := ExtractDeckPlan("Plan.\n```deck\n" + block + "\n```")
		if err != nil || plan == nil {
			t.Errorf("%s: plan = %v, err = %v", name, plan, err)
			continue
		}
		if plan.Count != 20 || plan.Mode != "srs" || plan.Language != "German" {
			t.Errorf("%s: plan = %+v", name, *plan)
		}
	}
}

func trimThrough(chunks ...string) string {
	var f TrimFilter
	var out strings.Builder
	for _, c := range chunks {
		out.WriteString(f.Write(c))
	}
	return out.String()
}

func TestTrimFilter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunks []string
		want   string
	}{
		{"leading blank lines dropped", []string{"\n\n", "\n Hola"}, "Hola"},
		{"trailing blank lines dropped", []string{"Hola", "\n\n", " \n"}, "Hola"},
		{"inner blank lines kept", []string{"Hola", "\n\n", "amigo"}, "Hola\n\namigo"},
		{"all whitespace", []string{" \n", "\t"}, ""},
	} {
		if got := trimThrough(tc.chunks...); got != tc.want {
			t.Errorf("%s: shown %q, want %q", tc.name, got, tc.want)
		}
	}
}
