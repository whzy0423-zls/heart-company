package wxpay

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewClientSupportsWechatPayPublicKeyVerification(t *testing.T) {
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	wechatPayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	privateKeyPath := filepath.Join(dir, "apiclient_key.pem")
	publicKeyPath := filepath.Join(dir, "pub_key.pem")
	privateDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	if err != nil {
		t.Fatal(err)
	}
	writeTestPEM(t, privateKeyPath, "PRIVATE KEY", privateDER)
	publicDER, err := x509.MarshalPKIXPublicKey(&wechatPayKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	writeTestPEM(t, publicKeyPath, "PUBLIC KEY", publicDER)

	client, err := NewClient(Config{
		MchID: "merchant", AppID: "wx-app", APIv3Key: strings.Repeat("k", 32), SerialNo: "merchant-serial",
		PrivateKeyPath: privateKeyPath, PublicKeyPath: publicKeyPath, PublicKeyID: "PUB_KEY_ID_123", NotifyURL: "https://example.com/notify",
	})
	if err != nil {
		t.Fatalf("expected public-key mode to initialize: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	body := []byte(`{"id":"notification"}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := "nonce"
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, body)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, wechatPayKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{
		"Wechatpay-Timestamp": []string{timestamp},
		"Wechatpay-Nonce":     []string{nonce},
		"Wechatpay-Signature": []string{base64.StdEncoding.EncodeToString(signature)},
		"Wechatpay-Serial":    []string{"PUB_KEY_ID_123"},
	}
	if err := client.verifyCallbackSignature(headers, body, now); err != nil {
		t.Fatalf("expected valid public-key signature: %v", err)
	}
	headers.Set("Wechatpay-Serial", "PUB_KEY_ID_OTHER")
	if err := client.verifyCallbackSignature(headers, body, now); err == nil {
		t.Fatal("expected mismatched Wechatpay-Serial to be rejected")
	}
}

func writeTestPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

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
