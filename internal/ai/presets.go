package ai

// Preset describes one selectable provider: what to call, what model to start
// from, and whether a key is needed. It exists so the settings form can ask a
// first-time user for one thing (a key) instead of four, while leaving every
// field editable.
//
// Presets are a starting point, not a lock. Base URLs and model ids drift, and
// this project ships no mechanism to push updated defaults to a running
// install, so a stale default must always be something the user can correct in
// the form rather than something that traps them.
type Preset struct {
	ID    string
	Label string

	// BaseURL and DefaultModel prefill the form. Empty means "the user must
	// supply this", which is only true for Custom.
	BaseURL      string
	DefaultModel string

	// NeedsKey drives validation and whether the key field is emphasised.
	NeedsKey bool

	// KeyURL is where to go to get a key, shown as a link next to the field.
	KeyURL string

	// Note is a short line of guidance under the provider select.
	Note string

	// Transport selects the client implementation. Anthropic has its own
	// message format; everything else speaks the OpenAI chat API.
	Transport string // "openai" | "anthropic"

	// Editable marks the provider whose URL and model the user is expected to
	// fill in themselves.
	Editable bool
}

// presets is ordered deliberately: the options that cost nothing and need the
// least setup come first, because that is the path a first-time user should
// fall into.
var presets = []Preset{
	{
		ID:           "gemini",
		Label:        "Google Gemini (free tier)",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai/",
		DefaultModel: "gemini-3.6-flash",
		NeedsKey:     true,
		KeyURL:       "https://aistudio.google.com/apikey",
		Note:         "Free tier, no card required. Google may use free-tier input to improve its models.",
		Transport:    "openai",
	},
	{
		ID:           "groq",
		Label:        "Groq (free tier)",
		BaseURL:      "https://api.groq.com/openai/v1",
		DefaultModel: "llama-3.3-70b-versatile",
		NeedsKey:     true,
		KeyURL:       "https://console.groq.com/keys",
		Note:         "Free tier with rate limits. Fast.",
		Transport:    "openai",
	},
	{
		ID:           "lmstudio",
		Label:        "LM Studio (local)",
		BaseURL:      "http://localhost:1234/v1",
		DefaultModel: "",
		NeedsKey:     false,
		Note:         "Start the local server in LM Studio, then set Model to the loaded model's id. Change the port here if you moved it.",
		Transport:    "openai",
	},
	{
		ID:           "ollama",
		Label:        "Ollama (local)",
		BaseURL:      "http://localhost:11434/v1",
		DefaultModel: "llama3.2",
		NeedsKey:     false,
		Note:         "Runs entirely on this machine. No key, no cost.",
		Transport:    "openai",
	},
	{
		ID:           "openai",
		Label:        "OpenAI (paid)",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini",
		NeedsKey:     true,
		KeyURL:       "https://platform.openai.com/api-keys",
		Note:         "Requires prepaid credit — there is no free tier for the API.",
		Transport:    "openai",
	},
	{
		ID:           "anthropic",
		Label:        "Anthropic (paid)",
		BaseURL:      "https://api.anthropic.com",
		DefaultModel: "claude-sonnet-5",
		NeedsKey:     true,
		KeyURL:       "https://console.anthropic.com/settings/keys",
		Note:         "Requires prepaid credit.",
		Transport:    "anthropic",
	},
	{
		ID:        "custom",
		Label:     "Custom (OpenAI-compatible)",
		NeedsKey:  false,
		Note:      "Any server speaking the OpenAI chat API. Supply the base URL and model yourself.",
		Transport: "openai",
		Editable:  true,
	},
}

// Presets returns the selectable providers in display order.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// PresetByID looks up a provider. The bool reports whether the id is one this
// build knows about, which is also how the settings handler validates input.
func PresetByID(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
