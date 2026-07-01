package wxpay

import "testing"

func TestNewClientDoesNotImplicitlyEnableDevMode(t *testing.T) {
	client, err := NewClient(Config{})
	if err == nil {
		t.Fatalf("expected incomplete production config to fail, got client devMode=%v", client.DevMode())
	}
}

func TestNewClientAllowsExplicitDevMode(t *testing.T) {
	client, err := NewClient(Config{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if !client.DevMode() {
		t.Fatal("expected explicit dev config to enable dev mode")
	}
}
