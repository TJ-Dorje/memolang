// Package totp is the authenticator-app second factor (RFC 6238: 6 digits,
// 30-second steps, SHA-1 — what every authenticator app supports). It wraps
// github.com/pquerna/otp for the code math and QR image, and adds what that
// library leaves to the caller: which time step a code matched, so the
// caller can refuse a code that was already used.
package totp

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"image/png"
	"net/url"
	"time"

	"github.com/pquerna/otp"
	pqtotp "github.com/pquerna/otp/totp"
)

const (
	issuer = "MemoLang"
	period = 30 // seconds per step
	// skew accepts the step before and after the current one: phone clocks
	// drift, and people type slowly.
	skew = 1
)

var opts = pqtotp.ValidateOpts{Period: period, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// Setup is a new secret, ready to show to the user.
type Setup struct {
	Secret string // base32, for typing in by hand
	// QR is a data: URI of a PNG holding the otpauth:// URI, so the page can
	// show it in a plain <img> — no JavaScript.
	QR string
}

// NewSetup generates a secret for the account named email.
func NewSetup(email string) (Setup, error) {
	key, err := pqtotp.Generate(pqtotp.GenerateOpts{
		Issuer:      issuer,
		AccountName: email,
		Period:      period,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return Setup{}, fmt.Errorf("totp: generate: %w", err)
	}
	qr, err := qrDataURI(key)
	if err != nil {
		return Setup{}, err
	}
	return Setup{Secret: key.Secret(), QR: qr}, nil
}

// SetupFor rebuilds the page data for a secret already stored, so the setup
// page can be reloaded without changing the secret under the user.
func SetupFor(email, secret string) (Setup, error) {
	key, err := otp.NewKeyFromURL(fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&period=%d&digits=6&algorithm=SHA1",
		issuer, url.PathEscape(email), url.QueryEscape(secret), issuer, period,
	))
	if err != nil {
		return Setup{}, fmt.Errorf("totp: key: %w", err)
	}
	qr, err := qrDataURI(key)
	if err != nil {
		return Setup{}, err
	}
	return Setup{Secret: secret, QR: qr}, nil
}

func qrDataURI(key *otp.Key) (string, error) {
	img, err := key.Image(220, 220)
	if err != nil {
		return "", fmt.Errorf("totp: qr: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("totp: png: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Match reports whether code is valid for secret at now, and for which time
// step. The caller stores the step and refuses any later code whose step is
// not newer, so a code seen over someone's shoulder can't be replayed within
// its 90-second window.
func Match(secret, code string, now time.Time) (step int64, ok bool) {
	current := now.Unix() / period
	for offset := int64(-skew); offset <= skew; offset++ {
		s := current + offset
		want, err := pqtotp.GenerateCodeCustom(secret, time.Unix(s*period, 0), opts)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// Code returns the current code for secret. Tests use it to stand in for
// the user's phone.
func Code(secret string, now time.Time) (string, error) {
	return pqtotp.GenerateCodeCustom(secret, now, opts)
}
