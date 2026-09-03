package lifestory

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTokenMapEncryptionRoundTripPreservesSafeOutputs(t *testing.T) {
	key := tokenKeyBytes("test-secret")
	want := TokenMap{"{{PERSON_1}}": {Token: "{{PERSON_1}}", Value: "张伟", Kind: "person", Output: "小林"}}
	ciphertext, err := encryptTokenMap(want, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decryptTokenMap(ciphertext, key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decrypted token map=%#v want=%#v", got, want)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	if _, err := decryptTokenMap(ciphertext, key); err == nil {
		t.Fatal("tampered token map must fail authentication")
	}
}

func TestTokenizeSnapshotRemovesPIIFromStructuredAndFreeTextFields(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "请联系 13800138000 或 writer@example.com，我在北京大学就读，住在上海市浦东新区张江路88号。"
	snapshot.FactCard.Setting = "北京大学的教室"
	snapshot.FactCard.Questions = []Question{{ID: "q1", Prompt: "你是否联系 13800138000？", Answer: "writer@example.com"}}
	snapshot.FactCard.Events = []FactEvent{{
		Location:    "上海市浦东新区张江路88号",
		Description: "我在北京大学完成转折，联系人是 13800138000",
		People:      []string{"真实姓名"},
	}}
	snapshot.Outline.Chapters = []OutlineChapter{{
		Order: 1, Title: "北京大学", Summary: "住在上海市浦东新区张江路88号",
		Beat: "给 writer@example.com 发邮件",
	}}
	snapshot.RevisionInstruction = "请保留 writer@example.com 的细节"

	safe, tokenMap := TokenizeSnapshot(snapshot)
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, private := range []string{"13800138000", "writer@example.com", "北京大学", "上海市浦东新区张江路88号"} {
		if strings.Contains(text, private) {
			t.Fatalf("tokenized snapshot leaked %q: %s", private, text)
		}
	}
	if len(tokenMap) < 4 {
		t.Fatalf("expected contact, organization, address and person tokens, got %#v", tokenMap)
	}
}

func TestTokenizeSnapshotCharacterPrivacyModeAllowsOnlyExactReal(t *testing.T) {
	tests := []struct {
		name          string
		privacyMode   string
		redactionMode string
		wantAllowed   bool
	}{
		{name: "canonical redaction mode", redactionMode: "real", wantAllowed: true},
		{name: "legacy privacy mode", privacyMode: "real", wantAllowed: true},
		{name: "matching modes", privacyMode: "real", redactionMode: "real", wantAllowed: true},
		{name: "conflicting modes", privacyMode: "real", redactionMode: "pseudonym"},
		{name: "not real", redactionMode: "not_real"},
		{name: "unknown", redactionMode: "public"},
		{name: "empty"},
		{name: "wrong case", redactionMode: "REAL"},
		{name: "padded", redactionMode: " real "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := confirmedSnapshot()
			snapshot.Materials[0].Text = "陈晨讲述了自己的经历。"
			snapshot.FactCard.Characters = []FactCharacter{{
				Name: "陈晨", RealName: "陈晨", Alias: "小陈",
				PrivacyMode: tt.privacyMode, RedactionMode: tt.redactionMode,
			}}

			_, tokens := TokenizeSnapshot(snapshot)
			replacement := tokenReplacementForValue(t, tokens, "陈晨")
			if replacement.Allowed != tt.wantAllowed {
				t.Fatalf("Allowed=%v want=%v replacement=%+v", replacement.Allowed, tt.wantAllowed, replacement)
			}
			if !tt.wantAllowed && replacement.Output == replacement.Value {
				t.Fatalf("fail-closed mode restored the real name: %+v", replacement)
			}
		})
	}
}

func TestTokenizeSnapshotCharacterRedactionOutputsMatchMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantAllowed bool
		wantOutput  string
	}{
		{name: "pseudonym uses chosen alias", mode: "pseudonym", wantOutput: "小陈"},
		{name: "blurred hides alias", mode: "blurred", wantOutput: "某人"},
		{name: "real restores real name", mode: "real", wantAllowed: true, wantOutput: "陈晨"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := confirmedSnapshot()
			snapshot.Materials[0].Text = "陈晨讲述了自己的经历。"
			snapshot.FactCard.Characters = []FactCharacter{{
				Name: "陈晨", RealName: "陈晨", Alias: "小陈", RedactionMode: tt.mode,
			}}

			_, tokens := TokenizeSnapshot(snapshot)
			replacement := tokenReplacementForValue(t, tokens, "陈晨")
			if replacement.Allowed != tt.wantAllowed || replacement.Output != tt.wantOutput {
				t.Fatalf("replacement=%+v want allowed=%v output=%q", replacement, tt.wantAllowed, tt.wantOutput)
			}
		})
	}
}

func TestTokenizeSnapshotBlurredCharacterHidesLegacyAliasOnlyRecord(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "小陈讲述了自己的经历。"
	snapshot.FactCard.Characters = []FactCharacter{{
		Name: "小陈", Alias: "小陈", RedactionMode: "blurred",
	}}

	safe, tokens := TokenizeSnapshot(snapshot)
	replacement := tokenReplacementForValue(t, tokens, "小陈")
	if replacement.Allowed || replacement.Output != "某人" {
		t.Fatalf("blurred alias replacement=%+v", replacement)
	}
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "小陈") {
		t.Fatalf("blurred alias remained in model snapshot: %s", raw)
	}
}

func TestTokenizeSnapshotOrganizationRedactionOutputsMatchMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantAllowed bool
		wantOutput  string
	}{
		{name: "pseudonym uses chosen alias", mode: "pseudonym", wantOutput: "那所学校"},
		{name: "blurred hides organization", mode: "blurred", wantOutput: "某机构"},
		{name: "real restores organization", mode: "real", wantAllowed: true, wantOutput: "北京大学"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := confirmedSnapshot()
			snapshot.Materials[0].Text = "我在北京大学经历了一次转折。"
			snapshot.FactCard.Organizations = []FactOrganization{{
				Name: "北京大学", Alias: "那所学校", RedactionMode: tt.mode,
			}}

			safe, tokens := TokenizeSnapshot(snapshot)
			replacement := tokenReplacementForValue(t, tokens, "北京大学")
			if replacement.Allowed != tt.wantAllowed || replacement.Output != tt.wantOutput {
				t.Fatalf("replacement=%+v want allowed=%v output=%q", replacement, tt.wantAllowed, tt.wantOutput)
			}
			raw, err := json.Marshal(safe)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "北京大学") {
				t.Fatalf("organization remained in model snapshot: %s", raw)
			}
		})
	}
}

func TestTokenizeSnapshotEventPrivacyModeAllowsOnlyExactReal(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantAllowed bool
	}{
		{name: "real", mode: "real", wantAllowed: true},
		{name: "not real", mode: "not_real"},
		{name: "unknown", mode: "public"},
		{name: "empty"},
		{name: "wrong case", mode: "REAL"},
		{name: "padded", mode: " real "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := confirmedSnapshot()
			snapshot.Materials[0].Text = "故事发生在月亮湾。"
			snapshot.FactCard.Events = []FactEvent{{
				Location: "月亮湾", Description: "故事发生在月亮湾。", Confirmed: true,
				RedactionMode: tt.mode,
			}}

			_, tokens := TokenizeSnapshot(snapshot)
			replacement := tokenReplacementForValue(t, tokens, "月亮湾")
			if replacement.Allowed != tt.wantAllowed {
				t.Fatalf("Allowed=%v want=%v replacement=%+v", replacement.Allowed, tt.wantAllowed, replacement)
			}
			if !tt.wantAllowed && replacement.Output == replacement.Value {
				t.Fatalf("fail-closed mode restored the real location: %+v", replacement)
			}
		})
	}
}

func TestTokenizeSnapshotConflictingDuplicatePrivacyModesFailClosed(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "陈晨在月亮湾经历了一次转折。"
	snapshot.FactCard.Characters = []FactCharacter{
		{Name: "陈晨", RealName: "陈晨", Alias: "小陈", RedactionMode: "real"},
		{Name: "陈晨", RealName: "陈晨", Alias: "某位朋友", RedactionMode: "pseudonym"},
	}
	snapshot.FactCard.Events = []FactEvent{
		{Location: "月亮湾", Description: "第一次提到月亮湾。", Confirmed: true, RedactionMode: "real"},
		{Location: "月亮湾", Description: "第二次提到月亮湾。", Confirmed: true, RedactionMode: "not_real"},
	}

	_, tokens := TokenizeSnapshot(snapshot)
	for _, private := range []string{"陈晨", "月亮湾"} {
		replacement := tokenReplacementForValue(t, tokens, private)
		if replacement.Allowed || replacement.Output == private {
			t.Fatalf("conflicting decisions for %q did not fail closed: %+v", private, replacement)
		}
	}
}

func tokenReplacementForValue(t *testing.T, tokens TokenMap, value string) TokenReplacement {
	t.Helper()
	for _, replacement := range tokens {
		if replacement.Value == value {
			return replacement
		}
	}
	t.Fatalf("token map has no replacement for %q: %#v", value, tokens)
	return TokenReplacement{}
}

func TestContainsSensitiveTokenRejectsPersistedOpaqueTokensAndDirectContacts(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "13800138000"
	safe, _ := TokenizeSnapshot(snapshot)
	version := Version{Chapters: []Chapter{{Title: "一", Body: "{{PHONE_1}}"}}, Reflection: "回望"}
	if !ContainsSensitiveToken(version, safe, nil) {
		t.Fatal("expected persisted opaque token to be rejected")
	}
	version.Chapters[0].Body = "联系 writer@example.com"
	if !ContainsSensitiveToken(version, snapshot, nil) {
		t.Fatal("expected direct email to be rejected")
	}
}
