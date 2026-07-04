package server

import "testing"

func TestTryAcquireVideoAsyncSlotRejectsWhenFull(t *testing.T) {
	slot := make(chan struct{}, 1)
	if !tryAcquireVideoAsyncSlot(slot) {
		t.Fatal("expected first slot acquisition to succeed")
	}
	if tryAcquireVideoAsyncSlot(slot) {
		t.Fatal("expected full slot channel to reject new task")
	}
	releaseVideoAsyncSlot(slot)
	if !tryAcquireVideoAsyncSlot(slot) {
		t.Fatal("expected acquisition to succeed after release")
	}
}
