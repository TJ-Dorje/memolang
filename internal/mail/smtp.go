package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTP_TLS values.
const (
	TLSStartTLS = "starttls" // plain connect, then upgrade; refused if the server can't
	TLSImplicit = "tls"      // TLS from the first byte (port 465)
	TLSNone     = "none"     // no encryption: only for a local catcher like Mailpit
)

type SMTPConfig struct {
	Host, Port         string
	Username, Password string
	TLS                string
	From               string
}

// SMTP sends through a mail provider (Postmark, Resend, SES…) or a local
// catcher. It opens one connection per message: the app sends a handful of
// mails an hour, so pooling would be complexity for nothing.
type SMTP struct {
	cfg  SMTPConfig
	from *netmail.Address
}

func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	from, err := netmail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: MAIL_FROM=%q: %w", cfg.From, err)
	}
	switch cfg.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return nil, fmt.Errorf("mail: SMTP_TLS=%q: want starttls, tls or none", cfg.TLS)
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := netmail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail: recipient %q: %w", m.To, err)
	}
	body, err := s.compose(to, m)
	if err != nil {
		return err
	}

	client, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := s.secure(client); err != nil {
		return err
	}
	if err := s.auth(client); err != nil {
		return err
	}
	return deliver(client, s.from.Address, to.Address, body)
}

// dial connects and greets the server. net/smtp takes no context, so the
// context's deadline is put on the connection itself: a hung server then
// fails the send instead of hanging the user's request.
func (s *SMTP) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mail: connect %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	if s.cfg.TLS == TLSImplicit {
		conn = tls.Client(conn, &tls.Config{ServerName: s.cfg.Host})
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mail: greet %s: %w", addr, err)
	}
	return client, nil
}

// secure upgrades to TLS when configured. STARTTLS is required, not
// opportunistic: a server (or someone in the middle) that doesn't offer it
// must not get the password and the codes in clear text.
func (s *SMTP) secure(client *smtp.Client) error {
	if s.cfg.TLS != TLSStartTLS {
		return nil
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("mail: %s does not offer STARTTLS (set SMTP_TLS=tls for port 465)", s.cfg.Host)
	}
	if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
		return fmt.Errorf("mail: STARTTLS: %w", err)
	}
	return nil
}

// auth logs in when a user is configured; a local catcher needs none.
// smtp.PlainAuth itself refuses to send the password unencrypted, except
// to localhost.
func (s *SMTP) auth(client *smtp.Client) error {
	if s.cfg.Username == "" {
		return nil
	}
	if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return fmt.Errorf("mail: auth: %w", err)
	}
	return nil
}

func deliver(client *smtp.Client, from, to string, body []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return client.Quit()
}

// compose builds the RFC 5322 message: headers, a blank line, the body.
// The subject is Q-encoded and the body quoted-printable, so non-ASCII text
// (a Spanish word in a subject, say) survives any relay.
func (s *SMTP) compose(to *netmail.Address, m Message) ([]byte, error) {
	if err := checkHeader("subject", m.Subject); err != nil {
		return nil, err
	}

	var b bytes.Buffer
	header := func(name, value string) { fmt.Fprintf(&b, "%s: %s\r\n", name, value) }
	header("From", s.from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", s.messageID())
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(m.Body)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// messageID is unique per mail and on the sender's domain; spam filters
// score mail without one lower.
func (s *SMTP) messageID() string {
	b := make([]byte, 16)
	rand.Read(b)
	domain := s.from.Address[strings.LastIndex(s.from.Address, "@")+1:]
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}
