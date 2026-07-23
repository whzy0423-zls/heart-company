package xinzhili

import (
	"errors"
	"testing"
)

func sessionEnvelope(generation uint32, seq uint64) Envelope {
	e := validEnvelope(EventSessionPing)
	e.Generation = generation
	e.SessionSeq = uint64ptr(seq)
	return e
}

func turnEnvelope(generation uint32, sessionSeq, turnSeq uint64, turnID string) Envelope {
	e := validEnvelope(EventASRActivity)
	e.Generation = generation
	e.SessionSeq = uint64ptr(sessionSeq)
	e.TurnID = &turnID
	e.TurnSeq = uint64ptr(turnSeq)
	return e
}

func TestSequenceSessionSequenceStartsAtZeroAndAdvancesPerDirection(t *testing.T) {
	guard := NewSequenceGuard(3)
	steps := []struct {
		name      string
		direction Direction
		seq       uint64
		want      SequenceDisposition
		wantErr   error
	}{
		{"client zero", DirectionClient, 0, SequenceAccept, nil},
		{"client duplicate", DirectionClient, 0, SequenceDrop, nil},
		{"client one", DirectionClient, 1, SequenceAccept, nil},
		{"client older", DirectionClient, 0, SequenceDrop, nil},
		{"server independent zero", DirectionServer, 0, SequenceAccept, nil},
		{"client gap", DirectionClient, 3, SequenceDrop, ErrControlSequenceGap},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			got, err := guard.Observe(step.direction, sessionEnvelope(3, step.seq))
			if got != step.want || !errors.Is(err, step.wantErr) {
				t.Fatalf("got=(%v,%v) want=(%v,%v)", got, err, step.want, step.wantErr)
			}
		})
	}
}

func TestSequenceTurnSequenceIsIndependentByTurnAndDirection(t *testing.T) {
	guard := NewSequenceGuard(5)
	steps := []struct {
		name       string
		direction  Direction
		sessionSeq uint64
		turnSeq    uint64
		turnID     string
		want       SequenceDisposition
		wantErr    error
	}{
		{"a client zero", DirectionClient, 0, 0, "a", SequenceAccept, nil},
		{"a client duplicate", DirectionClient, 0, 0, "a", SequenceDrop, nil},
		{"b client zero", DirectionClient, 1, 0, "b", SequenceAccept, nil},
		{"a client one", DirectionClient, 2, 1, "a", SequenceAccept, nil},
		{"a server zero", DirectionServer, 0, 0, "a", SequenceAccept, nil},
		{"a client gap", DirectionClient, 3, 3, "a", SequenceDrop, ErrControlSequenceGap},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			got, err := guard.Observe(step.direction, turnEnvelope(5, step.sessionSeq, step.turnSeq, step.turnID))
			if got != step.want || !errors.Is(err, step.wantErr) {
				t.Fatalf("got=(%v,%v) want=(%v,%v)", got, err, step.want, step.wantErr)
			}
		})
	}
}

func TestSequenceTurnDuplicateStillConsumesNewSessionSequence(t *testing.T) {
	guard := NewSequenceGuard(5)
	if got, err := guard.Observe(DirectionClient, turnEnvelope(5, 0, 0, "a")); got != SequenceAccept || err != nil {
		t.Fatalf("initial got=(%v,%v)", got, err)
	}
	if got, err := guard.Observe(DirectionClient, turnEnvelope(5, 1, 0, "a")); got != SequenceDrop || err != nil {
		t.Fatalf("turn duplicate got=(%v,%v)", got, err)
	}
	if got, err := guard.Observe(DirectionClient, turnEnvelope(5, 2, 1, "a")); got != SequenceAccept || err != nil {
		t.Fatalf("next event got=(%v,%v)", got, err)
	}
}

func TestSequenceRejectsWrongGenerationAndRequiresIncrementOnReconnect(t *testing.T) {
	guard := NewSequenceGuard(8)
	if got, err := guard.Observe(DirectionClient, sessionEnvelope(7, 0)); got != SequenceDrop || err != nil {
		t.Fatalf("old generation got=(%v,%v)", got, err)
	}
	if got, err := guard.Observe(DirectionClient, sessionEnvelope(9, 0)); got != SequenceDrop || !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("future generation got=(%v,%v)", got, err)
	}
	if err := guard.AdvanceGeneration(10); !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("skipped generation err=%v", err)
	}
	if err := guard.AdvanceGeneration(9); err != nil {
		t.Fatal(err)
	}
	if got, err := guard.Observe(DirectionClient, sessionEnvelope(9, 0)); err != nil || got != SequenceAccept {
		t.Fatalf("new generation got=(%v,%v)", got, err)
	}
}

func TestSequenceSessionGapDoesNotAdvanceState(t *testing.T) {
	guard := NewSequenceGuard(1)
	if got, err := guard.Observe(DirectionClient, sessionEnvelope(1, 1)); got != SequenceDrop || !errors.Is(err, ErrControlSequenceGap) {
		t.Fatalf("gap got=(%v,%v)", got, err)
	}
	if got, err := guard.Observe(DirectionClient, sessionEnvelope(1, 0)); got != SequenceAccept || err != nil {
		t.Fatalf("recovery got=(%v,%v)", got, err)
	}
}

func TestSequenceTerminalEventsAreIdempotent(t *testing.T) {
	guard := NewSequenceGuard(1)
	if got := guard.MarkTerminal("turn-1"); got != SequenceAccept {
		t.Fatalf("first terminal=%v", got)
	}
	if got := guard.MarkTerminal("turn-1"); got != SequenceDrop {
		t.Fatalf("duplicate terminal=%v", got)
	}
	if got := guard.MarkTerminal("turn-2"); got != SequenceAccept {
		t.Fatalf("other terminal=%v", got)
	}
}

func TestSequenceActiveTurnKeyCollision(t *testing.T) {
	guard := NewSequenceGuard(1)
	if err := guard.RegisterActiveTurn("turn-a", 42); err != nil {
		t.Fatal(err)
	}
	if err := guard.RegisterActiveTurn("turn-a", 42); err != nil {
		t.Fatalf("same turn registration should be idempotent: %v", err)
	}
	if err := guard.RegisterActiveTurn("turn-b", 42); !errors.Is(err, ErrTurnKeyCollision) {
		t.Fatalf("collision err=%v", err)
	}
	guard.ReleaseActiveTurn("turn-a")
	if err := guard.RegisterActiveTurn("turn-b", 42); err != nil {
		t.Fatalf("released key should be reusable: %v", err)
	}
}
