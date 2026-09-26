package srs

import (
	"testing"
	"time"
)

func TestFirstReviewGaps(t *testing.T) {
	s := CardState{Interval: 1, Ease: 2.5, Repetitions: 0}
	want := [4]int{Again: 1, Hard: 1, Good: 2, Easy: 4}
	if got := Preview(s); got != want {
		t.Errorf("Preview(new card) = %v, want %v", got, want)
	}

	updated, due := Update(s, Good)
	if updated.Repetitions != 1 || updated.Interval != 2 {
		t.Errorf("Good on a new card = %+v, want repetitions 1, interval 2", updated)
	}
	if due.Format("2006-01-02") != time.Now().AddDate(0, 0, 2).Format("2006-01-02") {
		t.Errorf("due = %s, want 2 days from now", due.Format("2006-01-02"))
	}
}

func TestSecondReviewGood(t *testing.T) {
	s := CardState{Interval: 2, Ease: 2.5, Repetitions: 1}
	updated, _ := Update(s, Good)
	if updated.Interval != 6 || updated.Repetitions != 2 {
		t.Errorf("Good on second review = %+v, want interval 6, repetitions 2", updated)
	}
}

func TestMatureGoodMultipliesByEase(t *testing.T) {
	s := CardState{Interval: 10, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, Good)
	if updated.Interval != 25 {
		t.Errorf("interval = %d, want 10 × 2.5 = 25", updated.Interval)
	}
	if updated.Ease != 2.5 {
		t.Errorf("Good changed ease to %v", updated.Ease)
	}
}

// Each button must give a strictly longer gap than the one to its left, at
// every stage, or the choice between them stops meaning anything. Low ease
// and short gaps are where rounding would otherwise collapse neighbours.
func TestButtonsAreStrictlyOrdered(t *testing.T) {
	for _, s := range []CardState{
		{Interval: 1, Ease: 2.5, Repetitions: 0},
		{Interval: 1, Ease: 1.3, Repetitions: 1},
		{Interval: 4, Ease: 2.5, Repetitions: 1},
		{Interval: 1, Ease: 1.3, Repetitions: 2},
		{Interval: 2, Ease: 1.3, Repetitions: 5},
		{Interval: 30, Ease: 2.7, Repetitions: 6},
	} {
		g := Preview(s)
		if !(g[Hard] < g[Good] && g[Good] < g[Easy]) {
			t.Errorf("Preview(%+v) = %v, want Hard < Good < Easy", s, g)
		}
	}
}

// Hard is a pass: the streak continues and the gap never shrinks. The
// original SM-2 reset the card exactly like Again.
func TestHardIsAPass(t *testing.T) {
	s := CardState{Interval: 15, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, Hard)
	if updated.Repetitions != 4 {
		t.Errorf("repetitions = %d, want 4 (Hard continues the streak)", updated.Repetitions)
	}
	if updated.Interval != 18 {
		t.Errorf("interval = %d, want 15 × 1.2 = 18", updated.Interval)
	}
	if updated.Ease != 2.35 {
		t.Errorf("ease = %v, want 2.5 - 0.15", updated.Ease)
	}
}

func TestAgainResets(t *testing.T) {
	s := CardState{Interval: 15, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, Again)
	if updated.Repetitions != 0 || updated.Interval != 1 {
		t.Errorf("Again = %+v, want repetitions 0, interval 1", updated)
	}
	if updated.Ease != 2.3 {
		t.Errorf("ease = %v, want 2.5 - 0.2", updated.Ease)
	}
}

func TestEasyStretchesAndRaisesEase(t *testing.T) {
	s := CardState{Interval: 10, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, Easy)
	if updated.Interval != 33 {
		t.Errorf("interval = %d, want 10 × 2.5 × 1.3 ≈ 33", updated.Interval)
	}
	if updated.Ease != 2.65 {
		t.Errorf("ease = %v, want 2.5 + 0.15", updated.Ease)
	}
}

func TestEaseFloor(t *testing.T) {
	s := CardState{Interval: 1, Ease: 1.3, Repetitions: 0}
	for range 10 {
		s, _ = Update(s, Again)
	}
	if s.Ease < minEase {
		t.Errorf("ease fell below floor: %f", s.Ease)
	}
}

// The buttons show Preview; the card is scheduled by Update. They must agree.
func TestPreviewMatchesUpdate(t *testing.T) {
	s := CardState{Interval: 7, Ease: 2.1, Repetitions: 4}
	gaps := Preview(s)
	for rating := Again; rating <= Easy; rating++ {
		updated, _ := Update(s, rating)
		if updated.Interval != gaps[rating] {
			t.Errorf("rating %d: Update interval %d, Preview %d", rating, updated.Interval, gaps[rating])
		}
	}
}

func TestUpdateClampsOutOfRangeRating(t *testing.T) {
	s := CardState{Interval: 1, Ease: 2.5, Repetitions: 0}
	if got, _ := Update(s, 99); got.Interval != Preview(s)[Easy] {
		t.Errorf("rating 99 gave interval %d, want Easy's %d", got.Interval, Preview(s)[Easy])
	}
}
