package lifestory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExtractFactOrganizationsUsesStructuredCompletion(t *testing.T) {
	completer := &fakeCompleter{raw: `{"organizations":[{"name":"北京大学"},{"name":"腾讯公司"}]}`}
	materials := []Material{{
		Text:       "旧的语音占位",
		Transcript: "我在北京大学读书，毕业后加入腾讯公司。",
	}}

	organizations := ExtractFactOrganizations(context.Background(), completer, materials)

	if completer.calls != 1 {
		t.Fatalf("structured completer calls=%d want=1", completer.calls)
	}
	if !strings.Contains(completer.system, "严格 JSON") || !strings.Contains(completer.user, "北京大学") {
		t.Fatalf("unexpected extraction prompt: system=%q user=%q", completer.system, completer.user)
	}
	if len(organizations) != 2 {
		t.Fatalf("organizations=%+v want two extracted entries", organizations)
	}
	for index, wantName := range []string{"北京大学", "腾讯公司"} {
		got := organizations[index]
		if got.Name != wantName || got.RedactionMode != "blurred" || got.ID == "" {
			t.Fatalf("organization[%d]=%+v", index, got)
		}
	}
}

func TestExtractFactOrganizationsRejectsNamesAbsentFromMaterials(t *testing.T) {
	completer := &fakeCompleter{raw: `{"organizations":[{"name":"北京大学"},{"name":"虚构公司"}]}`}

	organizations := ExtractFactOrganizations(context.Background(), completer, []Material{{Text: "我曾在北京大学学习。"}})

	if len(organizations) != 1 || organizations[0].Name != "北京大学" {
		t.Fatalf("organizations=%+v want only source-backed name", organizations)
	}
}

func TestMergeFactOrganizationsPreservesUserEditsAndAppendsNewEntries(t *testing.T) {
	existing := []FactOrganization{{
		ID: "organization-1", Name: "北京大学", Alias: "母校", RedactionMode: "pseudonym",
	}}
	extracted := []FactOrganization{
		{ID: "organization-1", Name: "北京大学", RedactionMode: "blurred"},
		{ID: "organization-2", Name: "腾讯公司", RedactionMode: "blurred"},
	}

	merged := MergeFactOrganizations(existing, extracted)

	if len(merged) != 2 {
		t.Fatalf("merged=%+v want two entries", merged)
	}
	if merged[0] != existing[0] {
		t.Fatalf("existing user edit changed: got=%+v want=%+v", merged[0], existing[0])
	}
	if merged[1].Name != "腾讯公司" || merged[1].RedactionMode != "blurred" || merged[1].ID == "organization-1" {
		t.Fatalf("new organization=%+v", merged[1])
	}
}

func TestExtractFactOrganizationsFallsBackToNoChanges(t *testing.T) {
	for name, completer := range map[string]*fakeCompleter{
		"provider error": {err: errors.New("provider unavailable")},
		"invalid schema": {raw: `{"organizations":[{"name":"北京大学","alias":"母校"}]}`},
	} {
		t.Run(name, func(t *testing.T) {
			organizations := ExtractFactOrganizations(context.Background(), completer, []Material{{Text: "我在北京大学读书。"}})
			if len(organizations) != 0 {
				t.Fatalf("organizations=%+v want best-effort empty result", organizations)
			}
		})
	}
}
