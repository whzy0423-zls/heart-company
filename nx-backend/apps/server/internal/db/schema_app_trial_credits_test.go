package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaDefinesAppChatTrialCreditLedger(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	normalized := strings.Join(strings.Fields(schema), " ")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS app_chat_trial_credit_grants",
		"amount INT NOT NULL",
		"remaining INT NOT NULL",
		"reserved INT NOT NULL",
		"idempotency_key TEXT NOT NULL",
		"ALTER TABLE app_chat_quota_reservations ADD COLUMN IF NOT EXISTS source",
		"ALTER TABLE app_chat_quota_reservations ADD COLUMN IF NOT EXISTS trial_grant_id",
		"idx_app_chat_trial_credit_grants_available",
	} {
		if !strings.Contains(normalized, fragment) {
			t.Errorf("schema missing %q", fragment)
		}
	}
}

func TestDefaultMenusIncludeTrialCreditGrantPermission(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.Name != "CustomerAppTrialCreditGrant" {
			continue
		}
		if menu.PID != 502 || menu.AuthCode != "Customer:AppTrialCredit:Grant" || menu.Type != "button" {
			t.Fatalf("unexpected trial credit permission: %+v", menu)
		}
		return
	}
	t.Fatal("expected trial credit grant permission")
}
