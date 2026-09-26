package handlers

import (
	"time"

	"memolang/internal/ai"
	"memolang/internal/models"
)

// DashboardData is the data payload for the dashboard page.
type DashboardData struct {
	Decks []models.Deck
}

// DeckDetailData is the data payload for the deck detail page.
type DeckDetailData struct {
	Deck   models.Deck
	Cards  []models.Card
	Filter string
	Search string
}

// DeckFormData is the data payload for the create/edit deck form.
type DeckFormData struct {
	Deck  *models.Deck
	Error string
}

// CardEditData is the data payload for the edit card form.
type CardEditData struct {
	Card  *models.Card
	Error string
}

// ImportData is the data payload for the CSV import page.
type ImportData struct {
	Deck        models.Deck
	Error       string
	Preview     []map[string]string
	TotalRows   int
	HasHeader   bool
	FrontCol    int
	BackCol     int
	ExampleCol  int
	TagsCol     int
	ShowExecute bool
	ImportToken string
}

// SessionData is the data payload for the study session page.
type SessionData struct {
	Deck     models.Deck
	Session  models.StudySession
	Card     models.Card
	Progress float64
	Empty    bool
	// NextDue is when an empty SRS deck's first card falls due; zero when
	// the deck has no cards.
	NextDue   time.Time
	MCOptions []string
	// Gaps labels each rating button (indexed by srs rating) with when the
	// card would come back, e.g. "2d".
	Gaps       [4]string
	Feedback   bool
	Answer     models.SessionAnswer
	AnswerCard models.Card
}

// AIFormData is the data payload for the AI card generation form.
type AIFormData struct {
	Name     string
	Language string
	Prompt   string
	Mode     string
	Error    string
}

// SettingsData is the data payload for the settings page.
type SettingsData struct {
	Provider  string
	BaseURL   string
	Model     string
	HasAPIKey bool
	// EnvKey reports that LLM_API_KEY is set and therefore overrides whatever
	// is stored, so the form can say so instead of looking out of date.
	EnvKey bool
	// Presets drives the provider dropdown and the prefill behaviour.
	Presets []ai.Preset
	Error   string
}

// ProfileData is the data payload for the profile (account) page. DisplayName
// is only set when re-rendering a rejected submission; otherwise the template
// reads the saved value from PageData.User.
type ProfileData struct {
	DisplayName string
	Error       string
}

// SecurityData is the data payload for the security page. Each form has its
// own error so the message appears next to the form that caused it.
type SecurityData struct {
	Sessions      int
	PasswordError string
	DeleteError   string
}

// ChatData is what every assistant chat page shows: the finished messages,
// the reply still streaming (if any), and whether a provider is set up.
type ChatData struct {
	Messages []models.ConversationMessage
	// Streaming is the reply still being generated, written into the page as
	// it arrives; nil when nothing is in progress.
	Streaming  *models.ConversationMessage
	Configured bool
	// AssistantName labels the assistant's turns ("Tutor", "Assistant").
	AssistantName string
	// Greeting, when set, is shown as the assistant's opening turn. It is
	// page text, not a stored message.
	Greeting string
}

// ChatState exposes the embedded ChatData to the streaming helper.
func (d ChatData) ChatState() ChatData { return d }

// TutorData is the data payload for the tutor page.
type TutorData struct {
	ChatData
	Card    models.Card
	Deck    models.Deck
	Presets []TutorPreset
}

// DeckBuilderData is the data payload for the deck-builder interview.
type DeckBuilderData struct {
	ChatData
}

// WaitingData is the data payload for the spinner page that hands over to a
// slow GET.
type WaitingData struct {
	Message string
	Sub     string
	Next    string
}

// TutorPreset is a one-click question button.
type TutorPreset struct {
	Key   string
	Label string
}

// tutorPresetList orders the preset buttons; the question text for each key
// is in tutorPresets.
var tutorPresetList = []TutorPreset{
	{"explain", "Explain"},
	{"examples", "More examples"},
	{"mnemonic", "Memory trick"},
	{"quiz", "Quiz me"},
}

// SessionSummaryData is the data payload for the session summary page.
type SessionSummaryData struct {
	Deck        models.Deck
	Session     models.StudySession
	Accuracy    int
	DueTomorrow int
	Wrong       int
	MissedCards []models.SessionAnswer
}

// AuthFormData is the data payload for the login and register forms.
type AuthFormData struct {
	Email string
	Next  string
	Error string
}
