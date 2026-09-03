package lifestory

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLifeStoryModelsRoundTrip(t *testing.T) {
	input := Story{
		ID: 7, AppUserID: 42, Title: "那年夏天", Status: StatusDraft, Stage: StageOutline,
		Materials:      []Material{{ID: 9, SourceType: MaterialText, Sequence: 1, Text: "我第一次离开家。", Transcript: "我第一次离开家。", ASRStatus: ASRNotApplicable}},
		FactCard:       FactCard{Characters: []FactCharacter{{Name: "小林", Relation: "自己"}}, Events: []FactEvent{{Time: "2012", Description: "离开家"}}, Ending: "后来我回来了。"},
		Outline:        Outline{Perspective: PerspectiveFirst, Tone: ToneWarm, StoryStyle: StoryStyleFairyTale, Chapters: []OutlineChapter{{Order: 1, Title: "出发", Summary: "离开熟悉的地方"}}},
		CurrentVersion: &Version{ID: 3, Number: 1, StoryStyle: StoryStyleFairyTale, Chapters: []Chapter{{Order: 1, Title: "出发", Body: "那天我背起行囊。"}}, Reflection: "我学会了面对变化。", CharacterCount: 14},
		LatestJob:      &Job{ID: 5, RequestKey: "req-1", Status: JobQueued, Attempt: 1},
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var got Story
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != input.ID || got.Status != StatusDraft || got.Stage != StageOutline || len(got.Materials) != 1 || got.CurrentVersion == nil || got.LatestJob == nil || got.Outline.StoryStyle != StoryStyleFairyTale || got.CurrentVersion.StoryStyle != StoryStyleFairyTale {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestStoryStyleNormalizationDefaultsMissingAndRejectsUnknown(t *testing.T) {
	tests := []struct {
		name  string
		input StoryStyle
		want  StoryStyle
	}{
		{name: "missing", want: StoryStyleRealistic},
		{name: "realistic", input: StoryStyleRealistic, want: StoryStyleRealistic},
		{name: "novel", input: StoryStyleNovel, want: StoryStyleNovel},
		{name: "fairy tale", input: StoryStyleFairyTale, want: StoryStyleFairyTale},
		{name: "myth", input: StoryStyleMyth, want: StoryStyleMyth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeStoryStyle(tt.input)
			if err != nil || got != tt.want {
				t.Fatalf("NormalizeStoryStyle(%q)=(%q,%v), want (%q,nil)", tt.input, got, err, tt.want)
			}
			if !got.Valid() {
				t.Fatalf("normalized story style %q is not valid", got)
			}
		})
	}

	if _, err := NormalizeStoryStyle(StoryStyle("documentary")); err == nil {
		t.Fatal("unknown non-empty story style must be rejected")
	}
}

func TestStoryStyleBelongsOnlyToOutlineAndVersion(t *testing.T) {
	raw, err := json.Marshal(struct {
		Facts   FactCard `json:"facts"`
		Outline Outline  `json:"outline"`
		Version Version  `json:"version"`
	}{
		Outline: Outline{StoryStyle: StoryStyleMyth},
		Version: Version{StoryStyle: StoryStyleMyth},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["facts"]["storyStyle"]; ok {
		t.Fatalf("fact card unexpectedly owns storyStyle: %s", raw)
	}
	if decoded["outline"]["storyStyle"] != "myth" || decoded["version"]["storyStyle"] != "myth" {
		t.Fatalf("outline/version storyStyle was not serialized: %s", raw)
	}
}

func TestOutlineStoryStyleSelectionPresenceRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		style    StoryStyle
		selected bool
	}{
		{name: "legacy missing style", raw: `{}`, style: StoryStyleRealistic, selected: false},
		{name: "legacy explicit realistic", raw: `{"storyStyle":"realistic"}`, style: StoryStyleRealistic, selected: true},
		{name: "legacy explicit myth", raw: `{"storyStyle":"myth"}`, style: StoryStyleMyth, selected: true},
		{name: "server default is explicitly unselected", raw: `{"storyStyle":"realistic","storyStyleSelected":false}`, style: StoryStyleRealistic, selected: false},
		{name: "server selection is explicit", raw: `{"storyStyle":"realistic","storyStyleSelected":true}`, style: StoryStyleRealistic, selected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outline Outline
			if err := json.Unmarshal([]byte(tt.raw), &outline); err != nil {
				t.Fatal(err)
			}
			if err := normalizeOutlineStoryStyle(&outline); err != nil {
				t.Fatal(err)
			}
			if outline.StoryStyle != tt.style || outline.StoryStyleSelected != tt.selected {
				t.Fatalf("outline style=%q selected=%v, want style=%q selected=%v", outline.StoryStyle, outline.StoryStyleSelected, tt.style, tt.selected)
			}
			roundTrip, err := json.Marshal(outline)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(roundTrip, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["storyStyle"] != string(tt.style) || payload["storyStyleSelected"] != tt.selected {
				t.Fatalf("serialized outline=%s, want effective style and selection presence", roundTrip)
			}
		})
	}
}

func TestResolveOutlineStoryStyleForWritePreservesExistingSelection(t *testing.T) {
	existing := Outline{StoryStyle: StoryStyleMyth, StoryStyleSelected: true}
	for _, incoming := range []Outline{
		{},
		{StoryStyle: StoryStyleRealistic, StoryStyleSelected: false},
	} {
		resolved, err := resolveOutlineStoryStyleForWrite(incoming, existing)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.StoryStyle != StoryStyleMyth || !resolved.StoryStyleSelected {
			t.Fatalf("unselected write erased existing selection: %+v", resolved)
		}
	}

	selected, err := resolveOutlineStoryStyleForWrite(
		Outline{StoryStyle: StoryStyleNovel, StoryStyleSelected: true},
		existing,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.StoryStyle != StoryStyleNovel || !selected.StoryStyleSelected {
		t.Fatalf("explicit selection did not win: %+v", selected)
	}
}

func TestResolveStoryStyleForWritePreservesExistingSelection(t *testing.T) {
	tests := []struct {
		name     string
		incoming StoryStyle
		existing StoryStyle
		want     StoryStyle
	}{
		{name: "old client preserves existing", existing: StoryStyleMyth, want: StoryStyleMyth},
		{name: "missing old data defaults", want: StoryStyleRealistic},
		{name: "new selection wins", incoming: StoryStyleNovel, existing: StoryStyleMyth, want: StoryStyleNovel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStoryStyleForWrite(tt.incoming, tt.existing)
			if err != nil || got != tt.want {
				t.Fatalf("resolveStoryStyleForWrite(%q,%q)=(%q,%v), want (%q,nil)", tt.incoming, tt.existing, got, err, tt.want)
			}
		})
	}
}

func TestLifeStoryEditorialFieldsRoundTrip(t *testing.T) {
	input := FactCard{
		QuestionSetID: "set-1",
		Questions: []Question{{
			ID: "q1", Prompt: "后来发生了什么？", Sequence: 2, Required: true,
		}},
	}
	outline := Outline{Chapters: []OutlineChapter{{
		Order: 1, Title: "转折", Beat: "收到录取通知", KeyBeats: []string{"决定离家"},
	}}}

	raw, err := json.Marshal(struct {
		Facts   FactCard `json:"facts"`
		Outline Outline  `json:"outline"`
	}{Facts: input, Outline: outline})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Facts   FactCard `json:"facts"`
		Outline Outline  `json:"outline"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Facts.QuestionSetID != "set-1" || len(got.Facts.Questions) != 1 || got.Facts.Questions[0].Sequence != 2 || !got.Facts.Questions[0].Required {
		t.Fatalf("question contract fields were not preserved: %+v", got.Facts)
	}
	if len(got.Outline.Chapters) != 1 || got.Outline.Chapters[0].Beat != "收到录取通知" {
		t.Fatalf("outline beat was not preserved: %+v", got.Outline)
	}
}

func TestLifeStoryFactOrganizationsRoundTrip(t *testing.T) {
	raw := []byte(`{"organizations":[{"id":"organization-1","name":"北京大学","alias":"那所学校","redactionMode":"pseudonym"}]}`)
	var facts FactCard
	if err := json.Unmarshal(raw, &facts); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"organizations", "北京大学", "那所学校", "pseudonym"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("organization field %q was not preserved: %s", want, encoded)
		}
	}
}

func TestLifeStoryValidationUsesUnicodeCodePointsAndBounds(t *testing.T) {
	chapters := make([]Chapter, 4)
	for i := range chapters {
		chapters[i] = Chapter{Order: i + 1, Title: "章", Body: strings.Repeat("中", 650)}
	}
	version := Version{Chapters: chapters, Reflection: "回望"}
	if err := ValidateVersion(version); err != nil {
		t.Fatalf("expected valid version: %v", err)
	}
	if got := version.CharacterCountValue(); got != 2602 {
		t.Fatalf("code point count=%d, want 2602", got)
	}
	chapters[0].Body = ""
	if err := ValidateVersion(Version{Chapters: chapters, Reflection: "回望"}); err == nil {
		t.Fatal("expected empty chapter rejection")
	}
}

func TestValidateOutlineRequiresConfirmableChapterFields(t *testing.T) {
	outline := Outline{
		Perspective: PerspectiveFirst,
		Tone:        ToneWarm,
		Chapters: []OutlineChapter{
			{Order: 1, Title: "一"},
			{Order: 2, Title: "二"},
			{Order: 3, Title: "三"},
			{Order: 4, Title: "四"},
		},
	}
	if err := ValidateOutline(outline); err != nil {
		t.Fatalf("expected valid outline: %v", err)
	}
	invalidStyle := outline
	invalidStyle.StoryStyle = StoryStyle("documentary")
	if err := ValidateOutline(invalidStyle); err == nil {
		t.Fatal("expected unknown story style rejection")
	}
	outline.Chapters[2].Title = ""
	if err := ValidateOutline(outline); err == nil {
		t.Fatal("expected blank chapter title rejection")
	}
	outline.Chapters = outline.Chapters[:3]
	if err := ValidateOutline(outline); err == nil {
		t.Fatal("expected chapter count rejection")
	}
}

func TestChapterValidationRequiresStrictOneBasedOrder(t *testing.T) {
	outline := Outline{
		Perspective: PerspectiveFirst,
		Tone:        ToneWarm,
		Chapters: []OutlineChapter{
			{Order: 1, Title: "一"},
			{Order: 2, Title: "二"},
			{Order: 2, Title: "三"},
			{Order: 4, Title: "四"},
		},
	}
	if err := ValidateOutline(outline); err == nil {
		t.Fatal("expected duplicate outline chapter order rejection")
	}

	chapters := make([]Chapter, 4)
	for i := range chapters {
		chapters[i] = Chapter{Order: i + 1, Title: "章", Body: strings.Repeat("中", 650)}
	}
	chapters[1].Order, chapters[2].Order = 3, 2
	if err := ValidateVersion(Version{Chapters: chapters, Reflection: "回望"}); err == nil {
		t.Fatal("expected out-of-order generated chapters rejection")
	}
}
