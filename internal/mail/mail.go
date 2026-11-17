// Package mail sends the app's transactional email: verification and reset
// codes and security notices. Everything above it sees one interface, so
// development (log), tests (memory) and production (SMTP) differ only in
// which Mailer is plugged in.
package mail

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Message is one plain-text email to one recipient.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Mailer delivers a Message. Send must honour ctx's deadline: handlers send
// while the user waits for the page.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// FromEnv picks the mailer from the environment: SMTP when SMTP_HOST is set,
// otherwise the log. A malformed SMTP setting is an error, so a typo stops
// startup instead of silently dropping every mail.
//
//	SMTP_HOST, SMTP_PORT (default 587), SMTP_USER, SMTP_PASSWORD,
//	SMTP_TLS = starttls (default) | tls (implicit, usually port 465) | none,
//	MAIL_FROM, e.g. "MemoLang <no-reply@mail.example.com>"
func FromEnv() (Mailer, error) {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return LogMailer{}, nil
	}
	return NewSMTP(SMTPConfig{
		Host:     host,
		Port:     envOr("SMTP_PORT", "587"),
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		TLS:      envOr("SMTP_TLS", TLSStartTLS),
		From:     os.Getenv("MAIL_FROM"),
	})
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// LogMailer writes each mail to the log instead of sending it: the
// development default, so codes can be read in the terminal.
type LogMailer struct{}

func (LogMailer) Send(_ context.Context, m Message) error {
	log.Printf("mail (not sent, SMTP_HOST unset)\nTo: %s\nSubject: %s\n\n%s", m.To, m.Subject, m.Body)
	return nil
}

// Memory keeps every mail for tests to read back, e.g. to type a code that
// was "sent". Safe for concurrent use.
type Memory struct {
	mu   sync.Mutex
	sent []Message
}

func NewMemory() *Memory { return &Memory{} }

func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// Sent returns a copy of every mail so far, oldest first.
func (m *Memory) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}

// Last returns the newest mail to the given address.
func (m *Memory) Last(to string) (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if strings.EqualFold(m.sent[i].To, to) {
			return m.sent[i], true
		}
	}
	return Message{}, false
}

// checkHeader rejects a value that would end the header line early. A
// subject or address containing CR/LF could otherwise inject extra headers
// (Bcc:) or a whole second message.
func checkHeader(name, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("mail: %s contains a line break", name)
	}
	return nil
}
