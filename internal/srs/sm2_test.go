package srs

import (
	"testing"
	"time"
)

func TestSM2FirstReviewGood(t *testing.T) {
	s := CardState{Interval: 1, Ease: 2.5, Repetitions: 0}
	updated, due := Update(s, 2) // Good
	if updated.Repetitions != 1 {
		t.Errorf("expected repetitions=1, got %d", updated.Repetitions)
	}
	if updated.Interval != 1 {
		t.Errorf("expected interval=1, got %d", updated.Interval)
	}
	expectedDue := time.Now().AddDate(0, 0, 1)
	if due.Format("2006-01-02") != expectedDue.Format("2006-01-02") {
		t.Errorf("expected due %s, got %s", expectedDue.Format("2006-01-02"), due.Format("2006-01-02"))
	}
}

func TestSM2AgainResetsInterval(t *testing.T) {
	s := CardState{Interval: 15, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, 0) // Again
	if updated.Repetitions != 0 {
		t.Errorf("expected repetitions=0, got %d", updated.Repetitions)
	}
	if updated.Interval != 1 {
		t.Errorf("expected interval=1, got %d", updated.Interval)
	}
}

func TestSM2EaseFloor(t *testing.T) {
	s := CardState{Interval: 1, Ease: 1.3, Repetitions: 0}
	for range 10 {
		s, _ = Update(s, 0) // Again, again, again...
	}
	if s.Ease < 1.3 {
		t.Errorf("ease fell below floor: %f", s.Ease)
	}
}

func TestSM2EasyIncreasesInterval(t *testing.T) {
	s := CardState{Interval: 6, Ease: 2.5, Repetitions: 2}
	updated, _ := Update(s, 3) // Easy
	if updated.Interval <= 6 {
		t.Errorf("expected interval > 6, got %d", updated.Interval)
	}
}

func TestSM2HardResetsInterval(t *testing.T) {
	s := CardState{Interval: 15, Ease: 2.5, Repetitions: 3}
	updated, _ := Update(s, 1) // Hard
	if updated.Repetitions != 0 {
		t.Errorf("expected repetitions=0, got %d", updated.Repetitions)
	}
	if updated.Interval != 1 {
		t.Errorf("expected interval=1, got %d", updated.Interval)
	}
}

func TestSM2SecondReviewGood(t *testing.T) {
	s := CardState{Interval: 1, Ease: 2.5, Repetitions: 1}
	updated, _ := Update(s, 2) // Good
	if updated.Interval != 6 {
		t.Errorf("expected interval=6, got %d", updated.Interval)
	}
	if updated.Repetitions != 2 {
		t.Errorf("expected repetitions=2, got %d", updated.Repetitions)
	}
}
