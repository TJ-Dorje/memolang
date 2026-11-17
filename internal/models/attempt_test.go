package models_test

import (
	"testing"
	"time"

	"memolang/internal/models"
)

func TestCountAttemptsWithinWindow(t *testing.T) {
	database := testDB(t)

	for range 3 {
		if err := models.RecordAttempt(database, "login_email", "a@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	models.RecordAttempt(database, "login_email", "other@example.com")
	models.RecordAttempt(database, "mail_email", "a@example.com")
	// An old event falls outside the window.
	database.Exec("INSERT INTO auth_attempts (kind, key, created_at) VALUES ('login_email', 'a@example.com', now() - interval '1 hour')")

	n, err := models.CountAttempts(database, "login_email", "a@example.com", 15*time.Minute)
	if err != nil || n != 3 {
		t.Fatalf("CountAttempts = %d, %v; want 3", n, err)
	}

	models.ClearAttempts(database, "login_email", "a@example.com")
	if n, _ := models.CountAttempts(database, "login_email", "a@example.com", 15*time.Minute); n != 0 {
		t.Fatalf("after ClearAttempts = %d, want 0", n)
	}
	if n, _ := models.CountAttempts(database, "mail_email", "a@example.com", time.Hour); n != 1 {
		t.Fatalf("other kinds must survive ClearAttempts, got %d", n)
	}
}

func TestSweepAuth(t *testing.T) {
	database := testDB(t)

	stale, _ := models.CreateUser(database, "stale@example.com", "x")
	fresh, _ := models.CreateUser(database, "fresh@example.com", "x")
	verified, _ := models.CreateUser(database, "old-verified@example.com", "x")
	database.Exec("UPDATE users SET created_at = now() - interval '8 days' WHERE id IN ($1, $2)", stale.ID, verified.ID)
	database.Exec("UPDATE users SET email_verified_at = now() WHERE id = $1", verified.ID)

	models.IssueCode(database, fresh.ID, models.CodeVerify, "")
	models.IssueCode(database, verified.ID, models.CodeReset, "")
	database.Exec("UPDATE email_codes SET expires_at = now() - interval '1 minute' WHERE user_id = $1", verified.ID)
	database.Exec("INSERT INTO auth_attempts (kind, key, created_at) VALUES ('k', 'old', now() - interval '2 days'), ('k', 'new', now())")

	if err := models.SweepAuth(database); err != nil {
		t.Fatalf("SweepAuth: %v", err)
	}

	for _, c := range []struct {
		id   int64
		want bool
	}{{stale.ID, false}, {fresh.ID, true}, {verified.ID, true}} {
		u, _ := models.GetUserByID(database, c.id)
		if (u != nil) != c.want {
			t.Errorf("user %d exists = %v, want %v", c.id, u != nil, c.want)
		}
	}

	var codes, attempts int
	database.QueryRow("SELECT COUNT(*) FROM email_codes").Scan(&codes)
	database.QueryRow("SELECT COUNT(*) FROM auth_attempts").Scan(&attempts)
	if codes != 1 || attempts != 1 {
		t.Errorf("after sweep: %d codes, %d attempts; want 1 and 1", codes, attempts)
	}
}
