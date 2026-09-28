package netguard

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

type fixtureHTTPTimeout struct{}

func (fixtureHTTPTimeout) Error() string   { return "client timeout" }
func (fixtureHTTPTimeout) Timeout() bool   { return true }
func (fixtureHTTPTimeout) Temporary() bool { return true }

func TestNormalizeHTTPErrorPreservesTransportTimeoutAndDeadlineIdentity(t *testing.T) {
	original := &url.Error{Op: "Post", URL: "http://localhost", Err: fixtureHTTPTimeout{}}
	err := NormalizeHTTPError(context.Background(), original)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, original) {
		t.Fatalf("normalized timeout=%v must retain deadline and original", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr != original {
		t.Fatalf("normalized timeout lost original request error: %v", err)
	}
}

func TestNormalizeHTTPErrorPreservesCancellationAndOtherErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := NormalizeHTTPError(ctx, fixtureHTTPTimeout{}); got != context.Canceled {
		t.Fatalf("cancellation=%v", got)
	}
	original := errors.New("transport failed")
	if got := NormalizeHTTPError(context.Background(), original); got != original {
		t.Fatalf("non-timeout error changed: %v", got)
	}
	if got := NormalizeHTTPError(context.Background(), nil); got != nil {
		t.Fatalf("success became error: %v", got)
	}
}
