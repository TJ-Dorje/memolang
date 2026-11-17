package mail

import (
	"bufio"
	"context"
	"io"
	"mime/quotedprintable"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server: enough of the protocol for net/smtp's
// client to deliver one plain (no TLS, no auth) message, which it records.
type fakeSMTP struct {
	addr     string
	received chan string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	f := &fakeSMTP{addr: ln.Addr().String(), received: make(chan string, 1)}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		f.serve(conn)
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn) {
	r := bufio.NewReader(conn)
	reply := func(s string) { io.WriteString(conn, s+"\r\n") }

	reply("220 fake ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 fake")
		case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go ahead")
			f.received <- readData(r)
			reply("250 queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

// readData reads the message up to the lone "." line that ends DATA.
func readData(r *bufio.Reader) string {
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil || line == ".\r\n" {
			return b.String()
		}
		b.WriteString(line)
	}
}

func newTestSMTP(t *testing.T, addr string) *SMTP {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr)
	s, err := NewSMTP(SMTPConfig{Host: host, Port: port, TLS: TLSNone, From: "MemoLang <no-reply@example.com>"})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	return s
}

func TestSMTPDeliversMessage(t *testing.T) {
	server := startFakeSMTP(t)
	s := newTestSMTP(t, server.addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.Send(ctx, Message{To: "anna@example.com", Subject: "Tu código: 123456", Body: "Your code is 123456.\n"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	raw := <-server.received
	headers, body, _ := strings.Cut(raw, "\r\n\r\n")
	for _, want := range []string{
		`From: "MemoLang" <no-reply@example.com>`,
		"To: <anna@example.com>",
		"Subject: =?utf-8?q?Tu_c=C3=B3digo:_123456?=",
		"Content-Type: text/plain; charset=utf-8",
		"Message-ID: <",
	} {
		if !strings.Contains(headers, want) {
			t.Errorf("headers missing %q:\n%s", want, headers)
		}
	}
	decoded, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if !strings.Contains(string(decoded), "Your code is 123456.") {
		t.Errorf("body = %q, want the code text", decoded)
	}
}

// A server that accepts the connection and never answers must fail the
// send when the context ends, not hang the user's request.
func TestSMTPHonoursDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			defer conn.Close()
			time.Sleep(5 * time.Second)
		}
	}()
	s := newTestSMTP(t, ln.Addr().String())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Send(ctx, Message{To: "anna@example.com", Subject: "x", Body: "x"}); err == nil {
		t.Fatal("Send to a silent server succeeded")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Send took %v, want it to stop at the deadline", elapsed)
	}
}

func TestSubjectWithLineBreakIsRejected(t *testing.T) {
	s := newTestSMTP(t, "127.0.0.1:1")
	_, err := s.compose(nil, Message{Subject: "hi\r\nBcc: victim@example.com"})
	if err == nil {
		t.Fatal("compose accepted a subject with CRLF")
	}
}

func TestNewSMTPValidatesConfig(t *testing.T) {
	if _, err := NewSMTP(SMTPConfig{Host: "h", Port: "25", TLS: TLSNone, From: "not an address"}); err == nil {
		t.Error("bad MAIL_FROM accepted")
	}
	if _, err := NewSMTP(SMTPConfig{Host: "h", Port: "25", TLS: "maybe", From: "a@example.com"}); err == nil {
		t.Error("bad SMTP_TLS accepted")
	}
}

func TestMemoryLast(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	m.Send(ctx, Message{To: "a@example.com", Body: "first"})
	m.Send(ctx, Message{To: "b@example.com", Body: "other"})
	m.Send(ctx, Message{To: "a@example.com", Body: "second"})

	got, ok := m.Last("A@example.com")
	if !ok || got.Body != "second" {
		t.Fatalf("Last = %+v, %v; want the second mail to a@", got, ok)
	}
}
