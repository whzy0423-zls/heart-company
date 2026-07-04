package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestIsPublicHTTPURLRejectsLocalAndPrivateAddresses(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/a.mp4",
		"http://localhost./a.mp4",
		"http://foo.localhost/a.mp4",
		"http://127.0.0.1/a.mp4",
		"http://10.0.0.2/a.mp4",
		"http://172.16.0.2/a.mp4",
		"http://192.168.1.2/a.mp4",
		"http://169.254.169.254/latest/meta-data",
		"ftp://cdn.example.com/a.mp4",
		"//cdn.example.com/a.mp4",
	} {
		if IsPublicHTTPURL(raw) {
			t.Fatalf("expected %s to be rejected as non-public", raw)
		}
	}
}

func TestIsPublicHTTPURLAllowsPublicHTTPAndHTTPS(t *testing.T) {
	for _, raw := range []string{
		"http://cdn.example.com/a.mp4",
		"https://cdn.example.com/a.mp4",
	} {
		if !IsPublicHTTPURL(raw) {
			t.Fatalf("expected %s to be accepted", raw)
		}
	}
}

func TestDialContextRejectsPrivateResolvedAddress(t *testing.T) {
	dialer := GuardedDialer(func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("inner dialer must not be called for private addresses")
		return nil, nil
	})

	_, err := dialer(context.Background(), "tcp", "127.0.0.1:8080")
	if err == nil {
		t.Fatal("expected private address to be rejected")
	}
	if !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private/local error, got %v", err)
	}
}

func TestDialContextDelegatesPublicAddress(t *testing.T) {
	expectedErr := errors.New("dial stopped")
	called := false
	dialer := GuardedDialer(func(_ context.Context, network string, address string) (net.Conn, error) {
		called = true
		if network != "tcp" || address != "93.184.216.34:443" {
			t.Fatalf("unexpected dial target network=%s address=%s", network, address)
		}
		return nil, expectedErr
	})

	_, err := dialer(context.Background(), "tcp", "93.184.216.34:443")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected delegated error, got %v", err)
	}
	if !called {
		t.Fatal("expected inner dialer to be called")
	}
}

func TestNewGuardedTransportDoesNotUseEnvironmentProxy(t *testing.T) {
	transport := NewGuardedTransport()
	request, err := http.NewRequest(http.MethodGet, "https://example.com/video.mp4", nil)
	if err != nil {
		t.Fatal(err)
	}
	if transport.Proxy != nil {
		proxy, err := transport.Proxy(request)
		if err != nil {
			t.Fatal(err)
		}
		if proxy != nil {
			t.Fatalf("expected guarded transport to avoid environment proxy, got %s", proxy)
		}
	}
}
