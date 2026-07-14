package userpreference

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractDeterministicCommunicationPreferences(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		directive   string
		category    string
		slot        string
		instruction string
	}{
		{
			name:        "avoid dear",
			message:     "不要叫我亲爱的",
			directive:   "不要使用“亲爱的”等亲昵称呼",
			category:    "addressing",
			slot:        "addressing.avoid_dear",
			instruction: "不要使用“亲爱的”等亲昵称呼",
		},
		{
			name:        "preferred name",
			message:     "以后叫我小林",
			directive:   "称呼用户为小林",
			category:    "addressing",
			slot:        "addressing.preferred_name",
			instruction: "称呼用户为小林",
		},
		{
			name:        "concise",
			message:     "回答短一点，不要长篇大论",
			directive:   "回答简短，避免长篇大论",
			category:    "length",
			slot:        "length.detail_level",
			instruction: "回答简短，避免长篇大论",
		},
		{
			name:        "detailed",
			message:     "以后详细一点",
			directive:   "回答更详细",
			category:    "length",
			slot:        "length.detail_level",
			instruction: "回答更详细",
		},
		{
			name:        "direct tone",
			message:     "少说教，直接一点",
			directive:   "表达直接，少说教",
			category:    "tone",
			slot:        "tone.direct",
			instruction: "表达直接，少说教",
		},
		{
			name:        "no lists chinese",
			message:     "不要列表",
			directive:   "不要使用列表",
			category:    "format",
			slot:        "format.no_lists",
			instruction: "不要使用列表",
		},
		{
			name:        "no lists english",
			message:     "Please use no lists in future replies",
			directive:   "不要使用列表",
			category:    "format",
			slot:        "format.no_lists",
			instruction: "不要使用列表",
		},
		{
			name:        "conclusion first",
			message:     "先给结论",
			directive:   "先给结论",
			category:    "format",
			slot:        "format.conclusion_first",
			instruction: "先给结论",
		},
		{
			name:        "no followup",
			message:     "不要反问我，也少追问",
			directive:   "不要反问或追问",
			category:    "interaction",
			slot:        "interaction.no_followup",
			instruction: "不要反问或追问",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Extract(tt.message)
			if len(got.CurrentDirectives) != 1 || got.CurrentDirectives[0] != tt.directive {
				t.Fatalf("directives: want %q, got %+v", tt.directive, got.CurrentDirectives)
			}
			if len(got.Mutations) != 1 || got.Mutations[0].Upsert == nil {
				t.Fatalf("expected one upsert, got %+v", got.Mutations)
			}
			preference := *got.Mutations[0].Upsert
			if preference.Category != tt.category || preference.Slot != tt.slot || preference.Instruction != tt.instruction {
				t.Fatalf("unexpected preference: %+v", preference)
			}
			if preference.SourceText != tt.message {
				t.Fatalf("source text: want %q, got %q", tt.message, preference.SourceText)
			}
		})
	}
}

func TestExtractCurrentOnlyInstructionsAreImmediateAndNotPersisted(t *testing.T) {
	tests := []struct {
		message   string
		directive string
	}{
		{message: "这次只给结论", directive: "只给结论"},
		{message: "这次详细说", directive: "回答更详细"},
		{message: "这次回答短一点", directive: "回答简短，避免长篇大论"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			got := Extract(tt.message)
			if len(got.CurrentDirectives) != 1 || got.CurrentDirectives[0] != tt.directive {
				t.Fatalf("want current directive %q, got %+v", tt.directive, got.CurrentDirectives)
			}
			if len(got.Mutations) != 0 {
				t.Fatalf("current-only instruction must not persist: %+v", got.Mutations)
			}
		})
	}
}

func TestExtractUsesLastInstructionForConflictingSlot(t *testing.T) {
	got := Extract("回答短一点，不过以后还是详细一点")
	if len(got.CurrentDirectives) != 1 || got.CurrentDirectives[0] != "回答更详细" {
		t.Fatalf("last directive must win: %+v", got.CurrentDirectives)
	}
	if len(got.Mutations) != 1 || got.Mutations[0].Upsert == nil {
		t.Fatalf("expected one final upsert, got %+v", got.Mutations)
	}
	if preference := got.Mutations[0].Upsert; preference.Slot != "length.detail_level" || preference.Instruction != "回答更详细" {
		t.Fatalf("last durable instruction must replace the slot: %+v", preference)
	}
}

func TestExtractAppliesCancellationInMessageOrder(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		wantDelete  bool
		instruction string
	}{
		{
			name:       "cancel after upsert",
			message:    "以后回答简短一点，后来取消回答简短的要求",
			wantDelete: true,
		},
		{
			name:        "upsert after cancel",
			message:     "取消之前回答简短的要求，以后回答详细一点",
			instruction: "回答更详细",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Extract(tt.message)
			if len(got.Mutations) != 1 {
				t.Fatalf("expected one final mutation, got %+v", got.Mutations)
			}
			mutation := got.Mutations[0]
			if tt.wantDelete {
				if mutation.Upsert != nil || mutation.DeleteSlot != "length.detail_level" {
					t.Fatalf("expected final delete, got %+v", mutation)
				}
				return
			}
			if mutation.Upsert == nil || mutation.DeleteSlot != "" || mutation.Upsert.Instruction != tt.instruction {
				t.Fatalf("expected final upsert %q, got %+v", tt.instruction, mutation)
			}
		})
	}
}

func TestExtractCancellationDeletesMatchingSlotOrCategory(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantSlots []string
	}{
		{name: "concise slot", message: "取消之前回答简短的要求", wantSlots: []string{"length.detail_level"}},
		{name: "list slot", message: "忘掉不要列表的要求", wantSlots: []string{"format.no_lists"}},
		{name: "tone slot", message: "取消少说教的要求", wantSlots: []string{"tone.direct"}},
		{name: "addressing category", message: "忘掉之前所有称呼方面的要求", wantSlots: []string{"addressing.avoid_dear", "addressing.preferred_name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Extract(tt.message)
			if len(got.CurrentDirectives) != 0 {
				t.Fatalf("cancellation must not inject the canceled directive: %+v", got.CurrentDirectives)
			}
			if len(got.Mutations) != len(tt.wantSlots) {
				t.Fatalf("want delete slots %+v, got %+v", tt.wantSlots, got.Mutations)
			}
			for i, wantSlot := range tt.wantSlots {
				if got.Mutations[i].Upsert != nil || got.Mutations[i].DeleteSlot != wantSlot {
					t.Fatalf("mutation %d: want delete %q, got %+v", i, wantSlot, got.Mutations[i])
				}
			}
		})
	}
}

func TestExtractAvoidsQuotedOtherPersonAndOrdinaryFalsePositives(t *testing.T) {
	messages := []string{
		`他说“不要叫我亲爱的”，这句话是什么意思？`,
		"她不喜欢别人叫她亲爱的，我该怎么办？",
		"亲爱的是一种什么称呼？",
		"小林以后叫他的同事亲爱的。",
		"长篇大论通常指多长？",
		"怎么让另一个人回答短一点？",
		"我今天直接去了公司。",
		"请帮我写一个有列表的项目计划",
	}
	for _, message := range messages {
		t.Run(message, func(t *testing.T) {
			got := Extract(message)
			if len(got.CurrentDirectives) != 0 || len(got.Mutations) != 0 {
				t.Fatalf("false positive for %q: %+v", message, got)
			}
		})
	}
}

func TestExtractBoundsStoredSourceText(t *testing.T) {
	message := "以后回答短一点" + strings.Repeat("很", MaxSourceTextRunes+100)
	got := Extract(message)
	if len(got.Mutations) != 1 || got.Mutations[0].Upsert == nil {
		t.Fatalf("expected concise preference, got %+v", got.Mutations)
	}
	source := got.Mutations[0].Upsert.SourceText
	if utf8.RuneCountInString(source) > MaxSourceTextRunes {
		t.Fatalf("source text exceeded storage bound: %d", utf8.RuneCountInString(source))
	}
}
