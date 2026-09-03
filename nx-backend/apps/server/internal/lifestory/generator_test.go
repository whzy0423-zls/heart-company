package lifestory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/llm"
)

type fakeCompleter struct {
	raw    string
	err    error
	system string
	user   string
	calls  int
}

func (f *fakeCompleter) CompleteJSON(_ context.Context, system, user string, _ int) (string, error) {
	f.calls++
	f.system, f.user = system, user
	return f.raw, f.err
}

func confirmedSnapshot() StorySnapshot {
	chapters := make([]OutlineChapter, 4)
	for i := range chapters {
		chapters[i] = OutlineChapter{Order: i + 1, Title: "章节", Summary: "摘要"}
	}
	return StorySnapshot{
		StoryID:   1,
		Materials: []Material{{Text: "我经历了一段重要的旅程。"}},
		FactCard:  FactCard{Confirmed: true, Characters: []FactCharacter{{Alias: "小林", RealName: "真实姓名"}}},
		Outline:   Outline{Confirmed: true, Perspective: PerspectiveFirst, Tone: ToneWarm, Chapters: chapters},
	}
}

func generatedJSON() string {
	body := strings.Repeat("中", 400)
	return `{"perspective":"first_person","tone":"warm","chapters":[` +
		`{"order":1,"title":"一","body":"` + body + `"},` +
		`{"order":2,"title":"二","body":"` + body + `"},` +
		`{"order":3,"title":"三","body":"` + body + `"},` +
		`{"order":4,"title":"四","body":"` + body + `"}],"reflection":"回望"}`
}

func TestGeneratorProducesValidatedStructuredVersion(t *testing.T) {
	completer := &fakeCompleter{raw: generatedJSON()}
	version, err := NewGenerator(GeneratorConfig{Completer: completer, Model: "MODEL"}).Generate(context.Background(), confirmedSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if version.CharacterCount != 1602 || len(version.Chapters) != 4 || version.Status != VersionPublished {
		t.Fatalf("unexpected version: %+v", version)
	}
	if !strings.Contains(completer.system, "严格 JSON") || strings.Contains(completer.user, "历史聊天") {
		t.Fatal("generator prompt violated source boundary")
	}
	if !strings.Contains(completer.system, "所有 body 合计写到 1400-1800") ||
		!strings.Contains(completer.system, "语言凝练") ||
		!strings.Contains(completer.user, "全部 body 合计为 1400-1800") {
		t.Fatal("generator prompt did not make the validated body-length contract explicit")
	}
}

func TestGeneratorAppliesStoryStyleInstructionsAndServerOwnedStamp(t *testing.T) {
	tests := []struct {
		style       StoryStyle
		instruction string
		symbolic    bool
	}{
		{style: StoryStyleRealistic, instruction: "真实回忆录"},
		{style: StoryStyleNovel, instruction: "小说叙事"},
		{style: StoryStyleFairyTale, instruction: "童话寓言", symbolic: true},
		{style: StoryStyleMyth, instruction: "神话叙事", symbolic: true},
		{style: StoryStyleFolk, instruction: "民间故事"},
	}
	for _, tt := range tests {
		t.Run(string(tt.style), func(t *testing.T) {
			snapshot := confirmedSnapshot()
			snapshot.Outline.StoryStyle = tt.style
			// storyStyle is model-untrusted metadata and must be ignored.
			raw := strings.Replace(generatedJSON(), "{", `{"storyStyle":"myth",`, 1)
			completer := &fakeCompleter{raw: raw}

			version, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if version.StoryStyle != tt.style {
				t.Fatalf("version storyStyle=%q, want server-owned %q", version.StoryStyle, tt.style)
			}
			if !strings.Contains(completer.system, tt.instruction) {
				t.Fatalf("%q prompt missing instruction %q: %s", tt.style, tt.instruction, completer.system)
			}
			if tt.symbolic {
				for _, invariant := range []string{"人物关系", "事件顺序", "核心冲突", "情绪转折", "真实结局", "不得冒充现实"} {
					if !strings.Contains(completer.system, invariant) {
						t.Fatalf("%q prompt missing fidelity invariant %q: %s", tt.style, invariant, completer.system)
					}
				}
			}
		})
	}
}

func TestGeneratorDefaultsLegacySnapshotToRealisticStyle(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Outline.StoryStyle = ""
	completer := &fakeCompleter{raw: generatedJSON()}

	version, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if version.StoryStyle != StoryStyleRealistic {
		t.Fatalf("legacy snapshot storyStyle=%q, want %q", version.StoryStyle, StoryStyleRealistic)
	}
	if !strings.Contains(completer.system, "真实回忆录") {
		t.Fatalf("legacy snapshot did not use realistic prompt: %s", completer.system)
	}
}

func TestGeneratorRejectsUnknownStoryStyleBeforeProvider(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Outline.StoryStyle = StoryStyle("documentary")
	completer := &fakeCompleter{raw: generatedJSON()}

	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot); err == nil {
		t.Fatal("unknown story style must be rejected")
	}
	if completer.calls != 0 {
		t.Fatalf("provider called %d times for invalid story style", completer.calls)
	}
}

func TestGeneratorDoesNotAppendUnredactedRevisionInstruction(t *testing.T) {
	const privateEmail = "writer@example.com"
	snapshot := confirmedSnapshot()
	snapshot.RevisionInstruction = "请保留 " + privateEmail + " 的细节"
	completer := &fakeCompleter{raw: generatedJSON()}

	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(completer.user, privateEmail) {
		t.Fatalf("model prompt exposed unredacted revision instruction: %s", completer.user)
	}
	if !strings.Contains(completer.user, "{{EMAIL_1}}") {
		t.Fatalf("model prompt did not retain the redacted instruction: %s", completer.user)
	}
}

func TestGeneratorRestoresModelTokensOnlyToSafeStoryValues(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "张伟在北京大学经历了一次转折。"
	snapshot.FactCard.Characters = []FactCharacter{{
		Name: "张伟", RealName: "张伟", Alias: "小林", PrivacyMode: "pseudonym",
	}}
	snapshot.FactCard.Events = []FactEvent{{
		Location: "北京大学", Description: "张伟在北京大学经历了一次转折。", Confirmed: true,
		RedactionMode: "blurred",
	}}
	safeSnapshot, tokens := TokenizeSnapshot(snapshot)
	var personToken, placeToken string
	for token, replacement := range tokens {
		switch replacement.Kind {
		case "person":
			personToken = token
		case "place":
			placeToken = token
		}
	}
	if personToken == "" || placeToken == "" {
		t.Fatalf("expected person and place tokens, got %#v", tokens)
	}
	raw := strings.Replace(generatedJSON(), `"body":"`, `"body":"`+personToken+"在"+placeToken, 1)
	completer := &fakeCompleter{raw: raw}

	version, err := NewGenerator(GeneratorConfig{Completer: completer}).GenerateTokenized(context.Background(), safeSnapshot, tokens)
	if err != nil {
		t.Fatal(err)
	}
	storyText := versionText(version)
	for _, private := range []string{"张伟", "北京大学", personToken, placeToken} {
		if strings.Contains(storyText, private) {
			t.Fatalf("generated story exposed private value %q: %s", private, storyText)
		}
	}
	for _, safe := range []string{"小林", "某地"} {
		if !strings.Contains(storyText, safe) {
			t.Fatalf("generated story did not restore safe value %q: %s", safe, storyText)
		}
	}
}

func TestGeneratorNeverRestoresConflictingOrNotRealPrivacyModes(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "张伟在月亮湾经历了一次转折。"
	snapshot.FactCard.Characters = []FactCharacter{{
		Name: "张伟", RealName: "张伟", Alias: "小林",
		PrivacyMode: "real", RedactionMode: "pseudonym",
	}}
	snapshot.FactCard.Events = []FactEvent{{
		Location: "月亮湾", Description: "张伟在月亮湾经历了一次转折。", Confirmed: true,
		RedactionMode: "not_real",
	}}
	safeSnapshot, tokens := TokenizeSnapshot(snapshot)
	person := tokenReplacementForValue(t, tokens, "张伟")
	place := tokenReplacementForValue(t, tokens, "月亮湾")
	raw := strings.Replace(generatedJSON(), `"body":"`, `"body":"`+person.Token+"在"+place.Token, 1)

	version, err := NewGenerator(GeneratorConfig{Completer: &fakeCompleter{raw: raw}}).GenerateTokenized(context.Background(), safeSnapshot, tokens)
	if err != nil {
		t.Fatal(err)
	}
	storyText := versionText(version)
	for _, private := range []string{"张伟", "月亮湾"} {
		if strings.Contains(storyText, private) {
			t.Fatalf("generated story restored fail-closed value %q: %s", private, storyText)
		}
	}
	for _, safe := range []string{"小林", "某地"} {
		if !strings.Contains(storyText, safe) {
			t.Fatalf("generated story did not restore safe value %q: %s", safe, storyText)
		}
	}
}

func TestGeneratorRejectsPrivateRealNameAndUnconfirmedMarker(t *testing.T) {
	raw := strings.Replace(generatedJSON(), "回望", "真实姓名回望", 1)
	completer := &fakeCompleter{raw: raw}
	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot()); err == nil {
		t.Fatal("expected private name rejection")
	}
	raw = strings.Replace(generatedJSON(), "回望", "[未确认]回望", 1)
	completer.raw = raw
	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot()); err == nil {
		t.Fatal("expected unconfirmed marker rejection")
	}
}

func TestGeneratorRejectsSensitiveChapterSummary(t *testing.T) {
	for name, sensitive := range map[string]string{
		"phone":         "13800138000",
		"email":         "writer@example.com",
		"private name":  "真实姓名",
		"unknown token": "{{PRIVATE_99}}",
	} {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(generatedJSON(), `"body":"`, `"summary":"`+sensitive+`","body":"`, 1)
			completer := &fakeCompleter{raw: raw}
			if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot()); !errors.Is(err, ErrSafetyBlocked) {
				t.Fatalf("sensitive summary %q error=%v, want ErrSafetyBlocked", sensitive, err)
			}
		})
	}
}

func TestGeneratorMapsProviderContentFilterToSafetyBlock(t *testing.T) {
	completer := &fakeCompleter{err: llm.ErrContentFiltered}
	_, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot())
	if !errors.Is(err, ErrSafetyBlocked) {
		t.Fatalf("provider filter error=%v, want ErrSafetyBlocked", err)
	}
	var safetyErr *SafetyError
	if !errors.As(err, &safetyErr) || safetyErr.Phase != "provider" || safetyErr.Code != "content_filtered" {
		t.Fatalf("provider filter was not classified structurally: %#v", err)
	}
}

func TestGeneratorBlocksExplicitHarmInstructionsBeforeCallingProvider(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.RevisionInstruction = "请详细写出如何自杀的具体步骤和方法"
	completer := &fakeCompleter{raw: generatedJSON()}

	_, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot)
	if !errors.Is(err, ErrSafetyBlocked) {
		t.Fatalf("explicit harmful instruction error=%v, want ErrSafetyBlocked", err)
	}
	var safetyErr *SafetyError
	if !errors.As(err, &safetyErr) || safetyErr.Phase != "input" || safetyErr.Code != "harmful_instruction" {
		t.Fatalf("harmful input was not classified structurally: %#v", err)
	}
	if completer.calls != 0 {
		t.Fatalf("provider was called %d times for blocked input", completer.calls)
	}
}

func TestGeneratorBlocksHarmfulGeneratedGuidance(t *testing.T) {
	for name, unsafe := range map[string]string{
		"encouragement": "自杀是唯一的解脱。",
		"instructions":  "自残的操作步骤：第一步准备工具。",
	} {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(generatedJSON(), "回望", unsafe, 1)
			completer := &fakeCompleter{raw: raw}

			_, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot())
			if !errors.Is(err, ErrSafetyBlocked) {
				t.Fatalf("harmful output error=%v, want ErrSafetyBlocked", err)
			}
			var safetyErr *SafetyError
			if !errors.As(err, &safetyErr) || safetyErr.Phase != "output" || safetyErr.Code != "harmful_guidance" {
				t.Fatalf("harmful output was not classified structurally: %#v", err)
			}
		})
	}
}

func TestGeneratorAllowsNonInstructionalTraumaNarrative(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "我曾经有过自伤念头，后来向家人和医生求助。"
	completer := &fakeCompleter{raw: generatedJSON()}

	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), snapshot); err != nil {
		t.Fatalf("non-instructional trauma narrative was blocked: %v", err)
	}
	if completer.calls != 1 {
		t.Fatalf("provider calls=%d, want 1", completer.calls)
	}
}

func TestGeneratorAllowsRecoveryOrientedSafetyLanguage(t *testing.T) {
	raw := strings.Replace(generatedJSON(), "回望", "我找到了不再自伤的方法，是向家人和医生求助。", 1)
	completer := &fakeCompleter{raw: raw}

	if _, err := NewGenerator(GeneratorConfig{Completer: completer}).Generate(context.Background(), confirmedSnapshot()); err != nil {
		t.Fatalf("recovery-oriented language was blocked: %v", err)
	}
}

func TestGeneratorIgnoresUntrustedVersionMetadataAndGenerationConfig(t *testing.T) {
	sensitiveConfig := `"id":99,"storyId":88,"number":7,"status":"superseded",` +
		`"model":"untrusted-model","generationConfig":{"phone":"13800138000",` +
		`"email":"writer@example.com","name":"真实姓名","token":"{{PRIVATE_99}}"},`
	raw := strings.Replace(generatedJSON(), "{", "{"+sensitiveConfig, 1)
	completer := &fakeCompleter{raw: raw}

	version, err := NewGenerator(GeneratorConfig{Completer: completer, Model: "trusted-model"}).Generate(context.Background(), confirmedSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if version.ID != 0 || version.StoryID != 0 || version.Number != 0 {
		t.Fatalf("model-controlled identifiers were retained: %+v", version)
	}
	if version.Status != VersionPublished || version.Model != "trusted-model" {
		t.Fatalf("trusted server metadata was not authoritative: %+v", version)
	}
	if len(version.GenerationConfig) != 0 {
		t.Fatalf("model-controlled generation config was retained: %s", version.GenerationConfig)
	}
}
