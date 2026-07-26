package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type ResolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f ResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

type TransportOptions struct {
	Resolver              Resolver
	DialContext           DialContextFunc
	ConnectTimeout        time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
}

const (
	defaultConnectTimeout        = 10 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second
)

func IsPublicHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	host := normalizeHost(u.Hostname())
	if isLocalhost(host) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return IsPublicIP(ip)
	}
	return true
}

func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !(ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified())
}

func GuardedDialer(next DialContextFunc) DialContextFunc {
	return GuardedDialerWithResolver(net.DefaultResolver, next)
}

func GuardedDialerWithResolver(resolver Resolver, next DialContextFunc) DialContextFunc {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if next == nil {
		d := (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
		next = d.DialContext
	}
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		host = normalizeHost(host)
		if isLocalhost(host) {
			return nil, fmt.Errorf("blocked private or local address: %s", address)
		}
		if ip := net.ParseIP(host); ip != nil && !IsPublicIP(ip) {
			return nil, fmt.Errorf("blocked private or local address: %s", address)
		}
		if net.ParseIP(host) == nil {
			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no addresses found for %s", host)
			}
			for _, item := range ips {
				if !IsPublicIP(item.IP) {
					return nil, fmt.Errorf("blocked private or local address: %s", address)
				}
			}
			var lastErr error
			for _, item := range ips {
				conn, err := next(ctx, network, net.JoinHostPort(item.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		}
		return next(ctx, network, net.JoinHostPort(host, port))
	}
}

func NewGuardedTransport() *http.Transport {
	return NewGuardedTransportWithOptions(TransportOptions{})
}

func NewGuardedTransportWithOptions(options TransportOptions) *http.Transport {
	connectTimeout := options.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}
	tlsTimeout := options.TLSHandshakeTimeout
	if tlsTimeout <= 0 {
		tlsTimeout = defaultTLSHandshakeTimeout
	}
	headerTimeout := options.ResponseHeaderTimeout
	if headerTimeout <= 0 {
		headerTimeout = defaultResponseHeaderTimeout
	}
	dial := GuardedDialerWithResolver(options.Resolver, options.DialContext)
	return &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			return dial(ctx, network, address)
		},
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: headerTimeout,
	}
}

func NewGuardedClient(timeout time.Duration) *http.Client {
	return NewGuardedClientWithOptions(timeout, TransportOptions{})
}

func NewGuardedClientWithOptions(timeout time.Duration, options TransportOptions) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewGuardedTransportWithOptions(options),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if req == nil || req.URL == nil || !IsPublicHTTPURL(req.URL.String()) {
				return fmt.Errorf("blocked private or local redirect address")
			}
			return nil
		},
	}
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func isLocalhost(host string) bool {
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}
