package totp

import (
	"strings"
	"testing"
	"time"
)

func TestMatchAcceptsNeighbouringSteps(t *testing.T) {
	setup, err := NewSetup("anna@example.com")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)

	for _, offset := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		code, _ := Code(setup.Secret, now.Add(offset))
		step, ok := Match(setup.Secret, code, now)
		if !ok {
			t.Errorf("code from %v away rejected", offset)
		}
		if want := now.Add(offset).Unix() / period; step != want {
			t.Errorf("step = %d, want %d", step, want)
		}
	}

	old, _ := Code(setup.Secret, now.Add(-2*time.Minute))
	if _, ok := Match(setup.Secret, old, now); ok {
		t.Error("a two-minute-old code was accepted")
	}
}

func TestSetupForKeepsSecretAndMakesQR(t *testing.T) {
	first, err := NewSetup("anna@example.com")
	if err != nil {
		t.Fatal(err)
	}
	again, err := SetupFor("anna@example.com", first.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if again.Secret != first.Secret {
		t.Error("SetupFor changed the secret")
	}
	if !strings.HasPrefix(again.QR, "data:image/png;base64,") {
		t.Errorf("QR = %.40q…, want a PNG data URI", again.QR)
	}
}
