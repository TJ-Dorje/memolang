package srs

import "time"

// CardState holds the SM-2 scheduling fields for a card.
type CardState struct {
	Interval    int
	Ease        float64
	Repetitions int
}

// Rating: 0=Again, 1=Hard, 2=Good, 3=Easy

// Update applies one SM-2 review iteration and returns the updated state
// plus the next due date.
func Update(s CardState, rating int) (CardState, time.Time) {
	quality := map[int]float64{0: 0, 1: 2, 2: 4, 3: 5}[rating]

	if quality < 3 {
		s.Repetitions = 0
		s.Interval = 1
	} else {
		switch s.Repetitions {
		case 0:
			s.Interval = 1
		case 1:
			s.Interval = 6
		default:
			s.Interval = int(float64(s.Interval)*s.Ease + 0.5)
		}
		s.Repetitions++
	}

	s.Ease = s.Ease + 0.1 - (5-quality)*(0.08+(5-quality)*0.02)
	if s.Ease < 1.3 {
		s.Ease = 1.3
	}

	due := time.Now().AddDate(0, 0, s.Interval)
	return s, due
}