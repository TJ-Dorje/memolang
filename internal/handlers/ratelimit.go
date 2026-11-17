package handlers

import (
	"log"
	"os"
	"time"

	"memolang/internal/models"

	"github.com/gin-gonic/gin"
)

// limit is one rate limit: at most max events of kind per key in window.
// Counts live in Postgres (models.RecordAttempt), so they hold across pods
// and restarts.
type limit struct {
	kind   string
	max    int
	window time.Duration
	// perIP limits are keyed by client IP alone. AUTH_IP_LIMITS=off turns
	// them off for test suites, where every request comes from one address;
	// per-email limits — the ones that stop guessing — always apply.
	perIP bool
}

var (
	// Failed logins. Counting per (email, IP) first means a stranger
	// guessing wrong on purpose can't lock the owner out from elsewhere;
	// the looser per-email cap still stops guessing spread over many IPs.
	loginFromHere = limit{kind: "login_email_ip", max: 5, window: 15 * time.Minute}
	loginForEmail = limit{kind: "login_email", max: 20, window: 15 * time.Minute}
	loginFromIP   = limit{kind: "login_ip", max: 50, window: 15 * time.Minute, perIP: true}

	// Mails sent on an anonymous request (codes and register notices). The
	// per-email cap also bounds code guessing: every new code resets its
	// 5 tries, so 5 mails an hour allow at most 25 guesses at 10⁶ codes.
	mailToEmail = limit{kind: "mail_email", max: 5, window: time.Hour}
	mailFromIP  = limit{kind: "mail_ip", max: 30, window: time.Hour, perIP: true}
)

const (
	tooManyLogins = "Too many attempts. Try again in 15 minutes, or reset your password."
	tooManyMails  = "Too many codes were sent to this address. Use the latest one, or wait an hour and try again."
)

// IPLimitsEnabled is false only under AUTH_IP_LIMITS=off, for test suites.
func IPLimitsEnabled() bool { return os.Getenv("AUTH_IP_LIMITS") != "off" }

// counter is a limit applied to one key.
type counter struct {
	limit
	key string
}

func active(cs []counter) []counter {
	if IPLimitsEnabled() {
		return cs
	}
	var out []counter
	for _, c := range cs {
		if !c.perIP {
			out = append(out, c)
		}
	}
	return out
}

// allowed reports whether every counter is under its limit. A database error
// counts as over the limit: an auth endpoint fails closed.
func (h *Handler) allowed(cs ...counter) bool {
	for _, c := range active(cs) {
		n, err := models.CountAttempts(h.DB, c.kind, c.key, c.window)
		if err != nil {
			log.Printf("rate limit %s: %v", c.kind, err)
			return false
		}
		if n >= c.max {
			return false
		}
	}
	return true
}

func (h *Handler) record(cs ...counter) {
	for _, c := range active(cs) {
		if err := models.RecordAttempt(h.DB, c.kind, c.key); err != nil {
			log.Printf("rate limit %s: record: %v", c.kind, err)
		}
	}
}

// loginCounters are the limits a login attempt for email from this client
// is checked against.
func loginCounters(c *gin.Context, email string) []counter {
	ip := c.ClientIP()
	return []counter{
		{loginFromHere, email + " " + ip},
		{loginForEmail, email},
		{loginFromIP, ip},
	}
}

// takeMailSlot claims one mail to email from this client, or reports that
// the limit is reached. Handlers call it before looking the address up, so
// it counts — and refuses — the same whether or not an account exists, and
// before issuing a code: a new code resets its tries, so issuing one
// without the slot would undo the guessing cap.
func (h *Handler) takeMailSlot(c *gin.Context, email string) bool {
	cs := []counter{{mailToEmail, email}, {mailFromIP, c.ClientIP()}}
	if !h.allowed(cs...) {
		return false
	}
	h.record(cs...)
	return true
}
