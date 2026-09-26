package assistant

import (
	"strings"
	"testing"
)

const samplePlan = "Here's your deck — check it below.\n\n```deck\n" +
	`{"name":"Spanish — Travel","language":"Spanish","prompt":"Travel phrases, beginner.","count":15,"mode":"linear"}` +
	"\n```"

func TestExtractDeckPlan(t *testing.T) {
	visible, plan := ExtractDeckPlan(samplePlan)
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
		hasPlan                bool
	}{
		{"no block", "Just a question?", "Just a question?", false},
		{"unclosed block", "Plan:\n```deck\n{\"language\":\"German\",\"prompt\":\"verbs\"}", "Plan:", true},
		{"malformed block hides text, no card", "Plan:\n```deck\n{not json\n```", "Plan:", false},
		{"missing language is not a plan", "Plan:\n```deck\n{\"prompt\":\"verbs\"}\n```", "Plan:", false},
		{"hidden from the first block, plan from the last", "old\n```deck\n{\"language\":\"A\",\"prompt\":\"a\"}\n```\nnew\n```deck\n{\"language\":\"B\",\"prompt\":\"b\"}\n```", "old", true},
	} {
		visible, plan := ExtractDeckPlan(tc.content)
		if visible != tc.visible {
			t.Errorf("%s: visible = %q, want %q", tc.name, visible, tc.visible)
		}
		if (plan != nil) != tc.hasPlan {
			t.Errorf("%s: plan = %+v, want present=%v", tc.name, plan, tc.hasPlan)
		}
	}
}

func TestPlanDefaultsModeToSRS(t *testing.T) {
	_, plan := ExtractDeckPlan("```deck\n{\"language\":\"L\",\"prompt\":\"p\",\"mode\":\"weird\"}\n```")
	if plan == nil || plan.Mode != "srs" {
		t.Errorf("plan = %+v, want mode srs", plan)
	}
}

func TestFormatDeckPlanRoundTrips(t *testing.T) {
	spec := DeckSpec{Name: "N", Language: "L", Prompt: "P", Count: 12, Mode: "srs"}
	_, got := ExtractDeckPlan("Plan:\n" + FormatDeckPlan(spec))
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
	visible, _ := ExtractDeckPlan(samplePlan)
	if strings.TrimRight(streamed, " \t\n") != visible {
		t.Errorf("streamed %q, stored view %q", streamed, visible)
	}
}
