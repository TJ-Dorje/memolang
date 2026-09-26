package configuration

const (
	DefaultCSVPath = "testdata/spanish_verbs.csv"
	// SingleCardCSVPath is for cases that must finish a whole deck in one
	// session: spanish_verbs.csv has more cards than a linear session takes.
	SingleCardCSVPath = "testdata/single_card.csv"
	MaxClickLoop      = 55
)
