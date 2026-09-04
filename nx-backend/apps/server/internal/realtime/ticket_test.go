package realtime

import "testing"

func TestTicketHashIsDeterministicAndOneWayShape(t *testing.T) {
	a := HashTicket("ticket-value")
	if a == "" || a != HashTicket("ticket-value") {
		t.Fatalf("hash is not deterministic: %q", a)
	}
	if a == "ticket-value" {
		t.Fatal("raw ticket must not be stored as hash")
	}
}

func TestRawTicketHasSufficientEntropy(t *testing.T) {
	a, err := NewRawTicket()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRawTicket()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 32 {
		t.Fatalf("unexpected tickets: %q %q", a, b)
	}
}
