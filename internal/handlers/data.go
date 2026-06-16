package handlers

import "memolang/internal/models"

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
	Deck      models.Deck
	Session   models.StudySession
	Card      models.Card
	Progress  float64
	Empty     bool
	MCOptions []string
}

// AIFormData is the data payload for the AI card generation form.
type AIFormData struct {
	Name     string
	Language string
	Prompt   string
	Mode     string
	Error    string
}
	// SessionSummaryData is the data payload for the session summary page.
type SessionSummaryData struct {
	Deck        models.Deck
	Session     models.StudySession
	Accuracy    int
	DueTomorrow int
}