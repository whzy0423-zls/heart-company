package server

import "testing"

func TestNormalizeAppEmail(t *testing.T) {
	if got, want := normalizeAppEmail("  User@Example.COM "), "user@example.com"; got != want {
		t.Fatalf("normalizeAppEmail() = %q, want %q", got, want)
	}
	if validateAppEmail("not-an-email") == nil {
		t.Fatal("invalid email should be rejected")
	}
}

func TestDefaultAppEmailConfigUsesSMTPDefaults(t *testing.T) {
	cfg := defaultAppEmailConfig()
	if cfg.Port != 587 || cfg.Enabled {
		t.Fatalf("default email config = %+v", cfg)
	}
}
