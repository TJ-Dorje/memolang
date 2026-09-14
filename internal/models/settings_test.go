package models_test

import (
	"testing"

	"memolang/internal/models"
)

// TestSetSettingUpserts is the only guard on the ON CONFLICT target: under the
// composite (user_id, key) primary key, a stale ON CONFLICT(key) compiles fine
// and fails at runtime on the second save.
func TestSetSettingUpserts(t *testing.T) {
	database := testDB(t)

	user, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := models.SetSetting(database, user.ID, "llm.model", "first"); err != nil {
		t.Fatalf("SetSetting (insert): %v", err)
	}
	if err := models.SetSetting(database, user.ID, "llm.model", "second"); err != nil {
		t.Fatalf("SetSetting (update): %v", err)
	}

	got, err := models.GetSetting(database, user.ID, "llm.model")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "second" {
		t.Fatalf("GetSetting = %q, want %q", got, "second")
	}
}

func TestSettingsArePerUser(t *testing.T) {
	database := testDB(t)

	alice, err := models.CreateUser(database, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser(alice): %v", err)
	}
	bob, err := models.CreateUser(database, "bob@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser(bob): %v", err)
	}

	if err := models.SetSetting(database, alice.ID, "llm.api_key", "alice-key"); err != nil {
		t.Fatalf("SetSetting(alice): %v", err)
	}
	if err := models.SetSetting(database, bob.ID, "llm.api_key", "bob-key"); err != nil {
		t.Fatalf("SetSetting(bob): %v", err)
	}

	bobSettings, err := models.GetSettings(database, bob.ID, "llm.")
	if err != nil {
		t.Fatalf("GetSettings(bob): %v", err)
	}
	if bobSettings["llm.api_key"] != "bob-key" {
		t.Fatalf("bob sees %q, want bob-key", bobSettings["llm.api_key"])
	}

	if err := models.DeleteSetting(database, bob.ID, "llm.api_key"); err != nil {
		t.Fatalf("DeleteSetting(bob): %v", err)
	}

	aliceKey, err := models.GetSetting(database, alice.ID, "llm.api_key")
	if err != nil {
		t.Fatalf("GetSetting(alice): %v", err)
	}
	if aliceKey != "alice-key" {
		t.Fatalf("alice's key = %q after bob deleted his, want alice-key", aliceKey)
	}
}
