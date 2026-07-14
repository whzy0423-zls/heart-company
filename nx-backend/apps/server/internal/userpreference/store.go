package userpreference

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxInstructionRunes      = 512
	MaxSourceTextRunes       = 1024
	MaxPreferencesPerUser    = 12
	MaxTotalInstructionRunes = 2048
	maxMutationsPerApply     = 64
)

var (
	ErrInvalidPreference = errors.New("userpreference: invalid preference")
	ErrPreferenceLimit   = errors.New("userpreference: preference limit exceeded")
)

var allowedSlotCategories = map[string]string{
	"addressing.preferred_name":  "addressing",
	"addressing.avoid_dear":      "addressing",
	"length.detail_level":        "length",
	"tone.direct":                "tone",
	"tone.formality":             "tone",
	"tone.warmth":                "tone",
	"format.no_lists":            "format",
	"format.conclusion_first":    "format",
	"interaction.no_followup":    "interaction",
	"custom.communication_style": "custom",
}

type Preference struct {
	Category    string
	Slot        string
	Instruction string
	SourceText  string
}

type Mutation struct {
	Upsert     *Preference
	DeleteSlot string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context, userID int64) ([]Preference, error) {
	if userID <= 0 {
		return nil, invalid("user ID must be positive")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT category, slot, instruction, source_text
		 FROM app_user_preferences
		 WHERE app_user_id = $1
		 ORDER BY category, slot
		 LIMIT $2`,
		userID, MaxPreferencesPerUser+1,
	)
	if err != nil {
		return nil, fmt.Errorf("userpreference: list: %w", err)
	}
	defer rows.Close()

	preferences := make([]Preference, 0)
	totalRunes := 0
	for rows.Next() {
		var preference Preference
		if err := rows.Scan(&preference.Category, &preference.Slot, &preference.Instruction, &preference.SourceText); err != nil {
			return nil, fmt.Errorf("userpreference: scan: %w", err)
		}
		if _, err := normalizePreference(preference); err != nil {
			return nil, fmt.Errorf("userpreference: stored value: %w", err)
		}
		preferences = append(preferences, preference)
		totalRunes += utf8.RuneCountInString(preference.Instruction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("userpreference: rows: %w", err)
	}
	if len(preferences) > MaxPreferencesPerUser || totalRunes > MaxTotalInstructionRunes {
		return nil, ErrPreferenceLimit
	}
	return preferences, nil
}

func (s *Store) Apply(ctx context.Context, userID int64, mutations []Mutation) error {
	if userID <= 0 {
		return invalid("user ID must be positive")
	}
	if len(mutations) > maxMutationsPerApply {
		return invalid("too many mutations")
	}
	normalized := make([]Mutation, len(mutations))
	for i, mutation := range mutations {
		upsertSet := mutation.Upsert != nil
		deleteSlot := strings.TrimSpace(mutation.DeleteSlot)
		deleteSet := deleteSlot != ""
		if upsertSet == deleteSet {
			return invalid("mutation must contain exactly one operation")
		}
		if upsertSet {
			preference, err := normalizePreference(*mutation.Upsert)
			if err != nil {
				return err
			}
			normalized[i].Upsert = &preference
			continue
		}
		if _, ok := allowedSlotCategories[deleteSlot]; !ok {
			return invalid("unsupported delete slot")
		}
		normalized[i].DeleteSlot = deleteSlot
	}
	if len(normalized) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("userpreference: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var lockedUserID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM app_users WHERE id = $1 FOR UPDATE`, userID,
	).Scan(&lockedUserID); err != nil {
		return fmt.Errorf("userpreference: lock user: %w", err)
	}

	for _, mutation := range normalized {
		if mutation.Upsert != nil {
			preference := *mutation.Upsert
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO app_user_preferences (app_user_id, category, slot, instruction, source_text)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (app_user_id, slot) DO UPDATE SET
				   category = EXCLUDED.category,
				   instruction = EXCLUDED.instruction,
				   source_text = EXCLUDED.source_text,
				   update_time = now()`,
				userID, preference.Category, preference.Slot, preference.Instruction, preference.SourceText,
			); err != nil {
				return fmt.Errorf("userpreference: upsert %s: %w", preference.Slot, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM app_user_preferences WHERE app_user_id = $1 AND slot = $2`,
			userID, mutation.DeleteSlot,
		); err != nil {
			return fmt.Errorf("userpreference: delete %s: %w", mutation.DeleteSlot, err)
		}
	}

	var count, totalRunes int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(char_length(instruction)), 0)
		 FROM app_user_preferences
		 WHERE app_user_id = $1`,
		userID,
	).Scan(&count, &totalRunes); err != nil {
		return fmt.Errorf("userpreference: check limits: %w", err)
	}
	if count > MaxPreferencesPerUser || totalRunes > MaxTotalInstructionRunes {
		return ErrPreferenceLimit
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("userpreference: commit: %w", err)
	}
	return nil
}

func normalizePreference(preference Preference) (Preference, error) {
	preference.Category = strings.TrimSpace(preference.Category)
	preference.Slot = strings.TrimSpace(preference.Slot)
	preference.Instruction = strings.TrimSpace(preference.Instruction)
	preference.SourceText = strings.TrimSpace(preference.SourceText)

	wantCategory, ok := allowedSlotCategories[preference.Slot]
	if !ok {
		return Preference{}, invalid("unsupported slot")
	}
	if preference.Category != wantCategory {
		return Preference{}, invalid("category does not match slot")
	}
	instructionRunes := utf8.RuneCountInString(preference.Instruction)
	if instructionRunes == 0 || instructionRunes > MaxInstructionRunes {
		return Preference{}, invalid("instruction length is out of range")
	}
	if utf8.RuneCountInString(preference.SourceText) > MaxSourceTextRunes {
		return Preference{}, invalid("source text is too long")
	}
	return preference, nil
}

func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidPreference, reason)
}
