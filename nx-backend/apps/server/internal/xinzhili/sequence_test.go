package xinzhili

import (
	"errors"
	"fmt"
	"sync"
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

func TestSequenceMarkTerminalDeletesBothDirectionTurnCursors(t *testing.T) {
	guard := NewSequenceGuard(1)
	if got, err := guard.Observe(DirectionClient, turnEnvelope(1, 0, 0, "turn-1")); got != SequenceAccept || err != nil {
		t.Fatalf("client event got=(%v,%v)", got, err)
	}
	if got, err := guard.Observe(DirectionServer, turnEnvelope(1, 0, 0, "turn-1")); got != SequenceAccept || err != nil {
		t.Fatalf("server event got=(%v,%v)", got, err)
	}
	guard.MarkTerminal("turn-1")
	for _, direction := range []Direction{DirectionClient, DirectionServer} {
		if _, exists := guard.turns[turnSequenceKey{direction: direction, turnID: "turn-1"}]; exists {
			t.Fatalf("%s turn cursor retained after terminal", direction)
		}
	}
}

func TestSequenceDropsLateTurnEventsAfterTerminalWithoutAdvancingTurnSequence(t *testing.T) {
	guard := NewSequenceGuard(1)
	if got, err := guard.Observe(DirectionClient, turnEnvelope(1, 0, 0, "turn-1")); got != SequenceAccept || err != nil {
		t.Fatalf("initial client event got=(%v,%v)", got, err)
	}
	guard.MarkTerminal("turn-1")

	lateEvents := []struct {
		name       string
		direction  Direction
		sessionSeq uint64
		turnSeq    uint64
	}{
		{"client", DirectionClient, 1, 1},
		{"server", DirectionServer, 0, 0},
	}
	for _, event := range lateEvents {
		t.Run(event.name, func(t *testing.T) {
			got, err := guard.Observe(event.direction, turnEnvelope(1, event.sessionSeq, event.turnSeq, "turn-1"))
			if got != SequenceDrop || err != nil {
				t.Fatalf("late event got=(%v,%v)", got, err)
			}
		})
	}

	clientCursor := guard.turns[turnSequenceKey{direction: DirectionClient, turnID: "turn-1"}]
	if clientCursor.seen {
		t.Fatalf("client turn cursor retained after terminal: %+v", clientCursor)
	}
	if _, exists := guard.turns[turnSequenceKey{direction: DirectionServer, turnID: "turn-1"}]; exists {
		t.Fatal("server turn cursor created for terminal event")
	}

	clientSession := sessionEnvelope(1, 2)
	if got, err := guard.Observe(DirectionClient, clientSession); got != SequenceAccept || err != nil {
		t.Fatalf("client session event got=(%v,%v)", got, err)
	}
	serverSession := sessionEnvelope(1, 1)
	if got, err := guard.Observe(DirectionServer, serverSession); got != SequenceAccept || err != nil {
		t.Fatalf("server session event got=(%v,%v)", got, err)
	}
}

func TestSequenceTerminalWindowIsBoundedAndKeepsRecentTurns(t *testing.T) {
	guard := NewSequenceGuard(1)
	for i := 0; i <= TerminalTurnWindowSize; i++ {
		guard.MarkTerminal(fmt.Sprintf("turn-%d", i))
	}
	if len(guard.terminalTurns) != TerminalTurnWindowSize {
		t.Fatalf("terminalTurns=%d want=%d", len(guard.terminalTurns), TerminalTurnWindowSize)
	}
	if _, exists := guard.terminalTurns["turn-0"]; exists {
		t.Fatal("oldest terminal turn was not evicted")
	}
	if _, exists := guard.terminalTurns[fmt.Sprintf("turn-%d", TerminalTurnWindowSize)]; !exists {
		t.Fatal("most recent terminal turn was evicted")
	}

	recent := fmt.Sprintf("turn-%d", TerminalTurnWindowSize)
	if got, err := guard.Observe(DirectionClient, turnEnvelope(1, 0, 0, recent)); got != SequenceDrop || err != nil {
		t.Fatalf("recent terminal event got=(%v,%v)", got, err)
	}
}

func TestSequenceGuardConcurrentAccess(t *testing.T) {
	guard := NewSequenceGuard(1)
	var wait sync.WaitGroup
	for i := 0; i < 64; i++ {
		i := i
		wait.Add(4)
		go func() {
			defer wait.Done()
			guard.Observe(DirectionClient, sessionEnvelope(1, 0))
		}()
		go func() {
			defer wait.Done()
			turnID := fmt.Sprintf("terminal-%d", i)
			guard.MarkTerminal(turnID)
		}()
		go func() {
			defer wait.Done()
			turnID := fmt.Sprintf("active-%d", i)
			guard.RegisterActiveTurn(turnID, uint64(i+1))
			guard.ReleaseActiveTurn(turnID)
		}()
		go func() {
			defer wait.Done()
			guard.AdvanceGeneration(2)
		}()
	}
	wait.Wait()
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
