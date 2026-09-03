package lifestory

import (
	"errors"
	"testing"
)

func TestValidatePreparationFactVersionRejectsStaleSnapshot(t *testing.T) {
	if err := validatePreparationFactVersion(FactCard{Version: 4}, FactCard{Version: 4}); err != nil {
		t.Fatalf("matching fact version was rejected: %v", err)
	}
	if err := validatePreparationFactVersion(FactCard{Version: 3}, FactCard{Version: 4}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale fact version error=%v want ErrConflict", err)
	}
}

func TestValidatePreparationSnapshotRejectsStoryOrOutlineChanges(t *testing.T) {
	facts := FactCard{Version: 4}
	outline := Outline{Version: 2}
	if err := validatePreparationSnapshot(facts, facts, outline, outline, 7, 7); err != nil {
		t.Fatalf("matching preparation snapshot was rejected: %v", err)
	}
	if err := validatePreparationSnapshot(facts, facts, outline, outline, 6, 7); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale story revision error=%v want ErrConflict", err)
	}
	if err := validatePreparationSnapshot(facts, facts, Outline{Version: 1}, outline, 7, 7); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale outline version error=%v want ErrConflict", err)
	}
}
