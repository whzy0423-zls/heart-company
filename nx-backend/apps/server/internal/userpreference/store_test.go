package userpreference

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestPreferenceStoreRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	store := NewStore(nil)
	valid := Preference{
		Category:    "length",
		Slot:        "length.detail_level",
		Instruction: "回答简短一些",
		SourceText:  "以后回答短一点",
	}

	tests := []struct {
		name      string
		userID    int64
		mutations []Mutation
	}{
		{name: "zero user", mutations: []Mutation{{Upsert: &valid}}},
		{name: "negative user", userID: -1, mutations: []Mutation{{Upsert: &valid}}},
		{name: "empty mutation", userID: 1, mutations: []Mutation{{}}},
		{name: "two operations", userID: 1, mutations: []Mutation{{Upsert: &valid, DeleteSlot: valid.Slot}}},
		{name: "unknown category", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "Category", "identity")}}},
		{name: "unknown slot", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "Slot", "length.unknown")}}},
		{name: "category slot mismatch", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "Category", "tone")}}},
		{name: "blank instruction", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "Instruction", "  ")}}},
		{name: "instruction too long", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "Instruction", strings.Repeat("答", MaxInstructionRunes+1))}}},
		{name: "source too long", userID: 1, mutations: []Mutation{{Upsert: preferenceWith(valid, "SourceText", strings.Repeat("说", MaxSourceTextRunes+1))}}},
		{name: "invalid delete slot", userID: 1, mutations: []Mutation{{DeleteSlot: "length.unknown"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Apply(context.Background(), tt.userID, tt.mutations)
			if !errors.Is(err, ErrInvalidPreference) {
				t.Fatalf("expected ErrInvalidPreference, got %v", err)
			}
		})
	}

	if _, err := store.List(context.Background(), 0); !errors.Is(err, ErrInvalidPreference) {
		t.Fatalf("List zero user: expected ErrInvalidPreference, got %v", err)
	}
}

func TestPreferenceStorePostgresTransactions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run preference transaction integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	suffix := time.Now().UnixNano()
	createUser := func(phone string) int64 {
		t.Helper()
		var id int64
		if err := database.QueryRowContext(ctx,
			`INSERT INTO app_users (phone) VALUES ($1) RETURNING id`,
			fmt.Sprintf("%s-%d", phone, suffix),
		).Scan(&id); err != nil {
			t.Fatalf("create app user: %v", err)
		}
		t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id = $1`, id) })
		return id
	}

	userA := createUser("pref-a")
	userB := createUser("pref-b")
	store := NewStore(database)

	initial := []Mutation{
		{Upsert: &Preference{Category: "tone", Slot: "tone.direct", Instruction: "表达直接一点", SourceText: "少说教，直接一点"}},
		{Upsert: &Preference{Category: "addressing", Slot: "addressing.preferred_name", Instruction: "称呼我为小林", SourceText: "以后叫我小林"}},
		{Upsert: &Preference{Category: "length", Slot: "length.detail_level", Instruction: "回答简短一些", SourceText: "以后回答短一点"}},
	}
	if err := store.Apply(ctx, userA, initial); err != nil {
		t.Fatalf("apply initial preferences: %v", err)
	}
	if err := store.Apply(ctx, userB, []Mutation{{Upsert: &Preference{
		Category: "format", Slot: "format.conclusion_first", Instruction: "先给结论", SourceText: "以后先给结论",
	}}}); err != nil {
		t.Fatalf("apply other user's preference: %v", err)
	}

	gotA, err := store.List(ctx, userA)
	if err != nil {
		t.Fatalf("list user A: %v", err)
	}
	wantSlots := []string{"addressing.preferred_name", "length.detail_level", "tone.direct"}
	if len(gotA) != len(wantSlots) {
		t.Fatalf("user A: expected %d preferences, got %+v", len(wantSlots), gotA)
	}
	for i, want := range wantSlots {
		if gotA[i].Slot != want {
			t.Fatalf("stable order at %d: want %q, got %+v", i, want, gotA)
		}
	}
	for _, preference := range gotA {
		if preference.Slot == "format.conclusion_first" {
			t.Fatalf("user A leaked user B preference: %+v", preference)
		}
	}

	if err := store.Apply(ctx, userA, []Mutation{
		{DeleteSlot: "length.detail_level"},
		{Upsert: &Preference{Category: "length", Slot: "length.detail_level", Instruction: "需要时回答详细一些", SourceText: "以后详细一点"}},
		{DeleteSlot: "tone.direct"},
	}); err != nil {
		t.Fatalf("apply mixed conflicting mutations: %v", err)
	}
	gotA, err = store.List(ctx, userA)
	if err != nil {
		t.Fatalf("list user A after mutation: %v", err)
	}
	if len(gotA) != 2 {
		t.Fatalf("expected replacement plus one retained preference, got %+v", gotA)
	}
	if gotA[1].Slot != "length.detail_level" || gotA[1].Instruction != "需要时回答详细一些" || gotA[1].SourceText != "以后详细一点" {
		t.Fatalf("slot was not replaced: %+v", gotA[1])
	}

	tooLarge := strings.Repeat("详", MaxInstructionRunes)
	err = store.Apply(ctx, userA, []Mutation{
		{Upsert: &Preference{Category: "tone", Slot: "tone.formality", Instruction: tooLarge}},
		{Upsert: &Preference{Category: "tone", Slot: "tone.warmth", Instruction: tooLarge}},
		{Upsert: &Preference{Category: "format", Slot: "format.no_lists", Instruction: tooLarge}},
		{Upsert: &Preference{Category: "interaction", Slot: "interaction.no_followup", Instruction: tooLarge}},
	})
	if !errors.Is(err, ErrPreferenceLimit) {
		t.Fatalf("expected ErrPreferenceLimit, got %v", err)
	}
	gotAfterRollback, err := store.List(ctx, userA)
	if err != nil {
		t.Fatalf("list after rejected transaction: %v", err)
	}
	if len(gotAfterRollback) != len(gotA) {
		t.Fatalf("limit failure partially committed: before=%+v after=%+v", gotA, gotAfterRollback)
	}
	for i := range gotA {
		if gotAfterRollback[i] != gotA[i] {
			t.Fatalf("limit failure changed preference %d: before=%+v after=%+v", i, gotA[i], gotAfterRollback[i])
		}
	}
}

func preferenceWith(base Preference, field, value string) *Preference {
	switch field {
	case "Category":
		base.Category = value
	case "Slot":
		base.Slot = value
	case "Instruction":
		base.Instruction = value
	case "SourceText":
		base.SourceText = value
	}
	return &base
}
