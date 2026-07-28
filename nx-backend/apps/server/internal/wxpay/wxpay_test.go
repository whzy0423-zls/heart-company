package wxpay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloseOrderDevModeValidatesMerchantOrderNumber(t *testing.T) {
	client, err := NewClient(Config{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CloseOrder(context.Background(), "classroom-order"); err != nil {
		t.Fatalf("dev close should emulate success: %v", err)
	}
	if err := client.CloseOrder(context.Background(), " "); err == nil {
		t.Fatal("blank out_trade_no must be rejected")
	}
}

func TestCloseOrderCallsWeChatMerchantOrderEndpoint(t *testing.T) {
	var gotPath, gotMchID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotMchID = body["mchid"]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{cfg: Config{MchID: "merchant", SerialNo: "serial"}, privateKey: key, http: upstream.Client(), baseURL: upstream.URL}
	if err = client.CloseOrder(context.Background(), "classroom-order"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v3/pay/transactions/out-trade-no/classroom-order/close" || gotMchID != "merchant" {
		t.Fatalf("unexpected close request path=%q mchid=%q", gotPath, gotMchID)
	}
}

func TestCloseOrderTreatsAlreadyClosedAsIdempotentButSurfacesPaidRace(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		code    string
		wantErr bool
	}{
		{code: "ORDER_CLOSED"},
		{code: "ORDERPAID", wantErr: true},
	} {
		t.Run(tt.code, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":"` + tt.code + `"}`))
			}))
			defer upstream.Close()
			client := &Client{cfg: Config{MchID: "merchant", SerialNo: "serial"}, privateKey: key, http: upstream.Client(), baseURL: upstream.URL}
			err := client.CloseOrder(context.Background(), "classroom-order")
			if (err != nil) != tt.wantErr {
				t.Fatalf("CloseOrder error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

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
