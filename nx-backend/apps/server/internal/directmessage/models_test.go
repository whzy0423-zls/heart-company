package directmessage

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeHistoryCursorRejectsBothDirections(t *testing.T) {
	if _, err := NormalizeHistoryCursor(10, 20); !errors.Is(err, ErrCursorConflict) {
		t.Fatalf("expected cursor conflict, got %v", err)
	}
	if got, err := NormalizeHistoryCursor(0, 20); err != nil || got.After != 20 {
		t.Fatalf("unexpected after cursor: %+v, %v", got, err)
	}
}

func TestPayloadHashIsStableAndIdempotencyDetectsConflict(t *testing.T) {
	a := PayloadHash("text", "hello", 0)
	b := PayloadHash("text", "hello", 0)
	if a == "" || a != b {
		t.Fatalf("payload hash is not stable: %q %q", a, b)
	}
	if a == PayloadHash("text", "different", 0) {
		t.Fatal("different payloads must hash differently")
	}
	if !SamePayload(a, a) || SamePayload(a, PayloadHash("text", "different", 0)) {
		t.Fatal("payload comparison is incorrect")
	}
}

func TestRecallWindow(t *testing.T) {
	if !CanRecall(2*time.Minute - time.Second) {
		t.Fatal("message inside recall window should be allowed")
	}
	if CanRecall(2 * time.Minute) {
		t.Fatal("message at recall boundary should be rejected")
	}
}
