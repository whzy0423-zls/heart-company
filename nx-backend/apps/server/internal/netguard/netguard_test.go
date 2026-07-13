package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestGuardedDialerRejectsHostnameResolvedToPrivateAddress(t *testing.T) {
	t.Parallel()

	resolver := ResolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	dialer := GuardedDialerWithResolver(resolver, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("inner dialer must not be called for a private DNS result")
		return nil, nil
	})

	_, err := dialer(context.Background(), "tcp", "models.example.com:443")
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private DNS result rejection, got %v", err)
	}
}

func TestNewGuardedTransportHasExplicitNetworkTimeouts(t *testing.T) {
	t.Parallel()

	transport := NewGuardedTransport()
	if transport.TLSHandshakeTimeout <= 0 {
		t.Fatal("expected an explicit TLS handshake timeout")
	}
	if transport.ResponseHeaderTimeout <= 0 {
		t.Fatal("expected an explicit response header timeout")
	}

	deadlineSeen := make(chan time.Duration, 1)
	transport = NewGuardedTransportWithOptions(TransportOptions{
		Resolver: ResolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				deadlineSeen <- 0
			} else {
				deadlineSeen <- time.Until(deadline)
			}
			return nil, errors.New("stop")
		},
		ConnectTimeout:        250 * time.Millisecond,
		TLSHandshakeTimeout:   350 * time.Millisecond,
		ResponseHeaderTimeout: 450 * time.Millisecond,
	})
	if transport.TLSHandshakeTimeout != 350*time.Millisecond || transport.ResponseHeaderTimeout != 450*time.Millisecond {
		t.Fatalf("unexpected transport timeouts: TLS=%s header=%s", transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
	}
	_, _ = transport.DialContext(context.Background(), "tcp", "models.example.com:443")
	if got := <-deadlineSeen; got <= 0 || got > 250*time.Millisecond {
		t.Fatalf("expected connect deadline within 250ms, got %s", got)
	}
}

func TestGuardedClientRejectsRedirectToPrivateAddress(t *testing.T) {
	t.Parallel()

	private := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("redirect target must not be reached")
	}))
	defer private.Close()

	client := NewGuardedClient(time.Second)
	req, err := http.NewRequest(http.MethodGet, private.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = client.CheckRedirect(req, nil)
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private redirect rejection, got %v", err)
	}
}
