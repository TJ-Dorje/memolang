package srs

import "time"

// CardState holds the SM-2 scheduling fields for a card.
type CardState struct {
	Interval    int
	Ease        float64
	Repetitions int
}

// Ratings, as posted by the four study buttons.
const (
	Again = 0 // forgot: the streak restarts, and the card returns this session
	Hard  = 1 // remembered with effort: a pass, but the gap grows slowly
	Good  = 2 // remembered: the normal gap
	Easy  = 3 // remembered instantly: a longer gap
)

const (
	minEase = 1.3
	// hardFactor grows the gap on Hard; easyBonus stretches it on Easy.
	hardFactor = 1.2
	easyBonus  = 1.3
)

// easeDelta is how each rating moves the ease factor, which multiplies every
// gap after the second review. Anki's defaults.
var easeDelta = [4]float64{Again: -0.20, Hard: -0.15, Good: 0, Easy: 0.15}

// Update applies one review and returns the new state plus the next due date.
//
// This is SM-2's shape (a fixed first and second gap, then gap × ease) with
// Anki's reading of the buttons: Hard is a pass, not a failure, and each
// button's gap is strictly longer than the one to its left, so the choice
// always makes a difference. The original SM-2 counted Hard as a failure and
// gave Good and Easy identical first two gaps, which made all four buttons
// look alike in use.
func Update(s CardState, rating int) (CardState, time.Time) {
	rating = min(max(rating, Again), Easy)
	gaps := Preview(s)

	if rating == Again {
		s.Repetitions = 0
	} else {
		s.Repetitions++
	}
	s.Interval = gaps[rating]
	s.Ease = max(s.Ease+easeDelta[rating], minEase)

	return s, time.Now().AddDate(0, 0, s.Interval)
}

// Preview returns the gap in days each rating would give, indexed by rating,
// without changing anything. The study screen shows these on the buttons, so
// it must stay the single source of the numbers Update uses.
//
// Again is always 1 day; the session also brings the card back before then.
// Hard, Good and Easy are each at least a day longer than the button to their
// left: rounding short gaps would otherwise make neighbours show the same
// number, which is exactly the "what is the difference?" problem.
func Preview(s CardState) [4]int {
	var hard, good, easy int
	switch s.Repetitions {
	case 0:
		hard, good, easy = 1, 2, 4
	case 1:
		hard = hardGap(s.Interval)
		good = 6
		easy = round(6 * easyBonus)
	default:
		hard = hardGap(s.Interval)
		good = round(float64(s.Interval) * s.Ease)
		easy = round(float64(s.Interval) * s.Ease * easyBonus)
	}

	good = max(good, hard+1)
	easy = max(easy, good+1)
	return [4]int{Again: 1, Hard: hard, Good: good, Easy: easy}
}

// hardGap grows a reviewed card's gap slowly, but never lets Hard shrink it:
// the card was remembered, so it should not come back sooner than last time.
func hardGap(prev int) int {
	return max(round(float64(prev)*hardFactor), prev+1)
}

func round(f float64) int {
	return int(f + 0.5)
}
