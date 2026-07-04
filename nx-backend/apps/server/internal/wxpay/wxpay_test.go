package wxpay

import (
	"strings"
	"testing"
)

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

func TestDevModeDoesNotAcceptDirectPayloadOnRealCallbackParser(t *testing.T) {
	client, err := NewClient(Config{Dev: true})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ParseCallbackWithHeaders(nil, []byte(`{"out_trade_no":"rpt1"}`))
	if err == nil {
		t.Fatal("expected dev direct payload to be rejected by real callback parser")
	}
	if !strings.Contains(err.Error(), "simulation") {
		t.Fatalf("expected error to point to simulation endpoint, got %v", err)
	}
}

func TestParseDevCallbackAcceptsExplicitSimulationPayload(t *testing.T) {
	client, err := NewClient(Config{Dev: true})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ParseDevCallback([]byte(`{"out_trade_no":"rpt1","transaction_id":"dev-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.OutTradeNo != "rpt1" || result.TransactionID != "dev-1" {
		t.Fatalf("unexpected dev callback result: %+v", result)
	}
}
