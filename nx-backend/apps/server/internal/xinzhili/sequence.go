package xinzhili

import (
	"errors"
	"fmt"
)

var (
	ErrControlSequenceGap = errors.New("control_sequence_gap")
	ErrGenerationMismatch = errors.New("generation_mismatch")
	ErrTurnKeyCollision   = errors.New("turn_key_collision")
)

type SequenceDisposition uint8

const (
	SequenceAccept SequenceDisposition = iota
	SequenceDrop
)

type sequenceCursor struct {
	seen bool
	last uint64
}

func (cursor sequenceCursor) classify(sequence uint64) (SequenceDisposition, error) {
	if !cursor.seen {
		if sequence != 0 {
			return SequenceDrop, ErrControlSequenceGap
		}
		return SequenceAccept, nil
	}
	if sequence <= cursor.last {
		return SequenceDrop, nil
	}
	if sequence != cursor.last+1 {
		return SequenceDrop, ErrControlSequenceGap
	}
	return SequenceAccept, nil
}

func (cursor *sequenceCursor) advance(sequence uint64) {
	cursor.seen = true
	cursor.last = sequence
}

type turnSequenceKey struct {
	direction Direction
	turnID    string
}

type SequenceGuard struct {
	generation    uint32
	session       map[Direction]sequenceCursor
	turns         map[turnSequenceKey]sequenceCursor
	terminalTurns map[string]struct{}
	activeByID    map[string]uint64
	activeByKey   map[uint64]string
}

func NewSequenceGuard(generation uint32) *SequenceGuard {
	guard := &SequenceGuard{}
	guard.reset(generation)
	return guard
}

func (guard *SequenceGuard) Observe(direction Direction, envelope Envelope) (SequenceDisposition, error) {
	if direction != DirectionClient && direction != DirectionServer {
		return SequenceDrop, fmt.Errorf("unknown direction %q", direction)
	}
	if envelope.Generation < guard.generation {
		return SequenceDrop, nil
	}
	if envelope.Generation > guard.generation {
		return SequenceDrop, ErrGenerationMismatch
	}
	if envelope.SessionSeq == nil {
		return SequenceDrop, errors.New("sessionSeq is required")
	}

	sessionCursor := guard.session[direction]
	disposition, err := sessionCursor.classify(*envelope.SessionSeq)
	if err != nil || disposition == SequenceDrop {
		return disposition, err
	}

	var turnKey turnSequenceKey
	var turnCursor sequenceCursor
	if envelope.TurnID != nil || envelope.TurnSeq != nil {
		if envelope.TurnID == nil || *envelope.TurnID == "" || envelope.TurnSeq == nil {
			return SequenceDrop, errors.New("turn sequence requires turnId and turnSeq")
		}
		turnKey = turnSequenceKey{direction: direction, turnID: *envelope.TurnID}
		turnCursor = guard.turns[turnKey]
		disposition, err = turnCursor.classify(*envelope.TurnSeq)
		if err != nil {
			return disposition, err
		}
		if disposition == SequenceDrop {
			sessionCursor.advance(*envelope.SessionSeq)
			guard.session[direction] = sessionCursor
			return SequenceDrop, nil
		}
	}

	sessionCursor.advance(*envelope.SessionSeq)
	guard.session[direction] = sessionCursor
	if envelope.TurnID != nil {
		turnCursor.advance(*envelope.TurnSeq)
		guard.turns[turnKey] = turnCursor
	}
	return SequenceAccept, nil
}

func (guard *SequenceGuard) AdvanceGeneration(generation uint32) error {
	if guard.generation == ^uint32(0) || generation != guard.generation+1 {
		return ErrGenerationMismatch
	}
	guard.reset(generation)
	return nil
}

func (guard *SequenceGuard) MarkTerminal(turnID string) SequenceDisposition {
	if _, exists := guard.terminalTurns[turnID]; exists {
		return SequenceDrop
	}
	guard.terminalTurns[turnID] = struct{}{}
	guard.ReleaseActiveTurn(turnID)
	return SequenceAccept
}

func (guard *SequenceGuard) RegisterActiveTurn(turnID string, turnKey uint64) error {
	if existingKey, exists := guard.activeByID[turnID]; exists {
		if existingKey == turnKey {
			return nil
		}
		return ErrTurnKeyCollision
	}
	if existingTurnID, exists := guard.activeByKey[turnKey]; exists && existingTurnID != turnID {
		return ErrTurnKeyCollision
	}
	guard.activeByID[turnID] = turnKey
	guard.activeByKey[turnKey] = turnID
	return nil
}

func (guard *SequenceGuard) ReleaseActiveTurn(turnID string) {
	turnKey, exists := guard.activeByID[turnID]
	if !exists {
		return
	}
	delete(guard.activeByID, turnID)
	delete(guard.activeByKey, turnKey)
}

func (guard *SequenceGuard) reset(generation uint32) {
	guard.generation = generation
	guard.session = make(map[Direction]sequenceCursor, 2)
	guard.turns = make(map[turnSequenceKey]sequenceCursor)
	guard.terminalTurns = make(map[string]struct{})
	guard.activeByID = make(map[string]uint64)
	guard.activeByKey = make(map[uint64]string)
}
