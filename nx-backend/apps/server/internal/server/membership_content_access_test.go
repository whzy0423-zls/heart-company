package server

import (
	"reflect"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/compatibility"
	"nine-xing/nx-backend/apps/server/internal/lifestory"
)

func TestMembershipContentLockedStates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access membershipResourceMetadata
		locked bool
	}{
		{name: "active", access: membershipResourceMetadata{State: resourceAccessActive}, locked: false},
		{name: "read only", access: membershipResourceMetadata{State: resourceAccessReadOnlyOverLimit}, locked: true},
		{name: "upgrade lock", access: membershipResourceMetadata{State: resourceAccessLockedUpgrade}, locked: true},
		{name: "upgrade flag", access: membershipResourceMetadata{State: resourceAccessActive, UpgradeRequired: true}, locked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := membershipContentLocked(tc.access); got != tc.locked {
				t.Fatalf("membershipContentLocked(%+v) = %v, want %v", tc.access, got, tc.locked)
			}
		})
	}
}

func TestMembershipMetadataForCardKeepsFreeBaseCardReadable(t *testing.T) {
	withoutLedger := membershipMetadataForCard("free", resourceAccessState{}, false, "历史记忆")
	if withoutLedger.State != resourceAccessActive || withoutLedger.RequiredPlanLevel != "free" || withoutLedger.UpgradeRequired {
		t.Fatalf("free card without ledger = %+v, want active free access", withoutLedger)
	}

	primary := membershipMetadataForCard("free", resourceAccessState{
		State:             resourceAccessActive,
		RequiredPlanLevel: "free",
	}, true, "历史画像")
	if primary.State != resourceAccessActive || primary.RequiredPlanLevel != "free" || primary.UpgradeRequired {
		t.Fatalf("free primary card = %+v, want active free access", primary)
	}
}

func TestMembershipMetadataForCardHonorsOverflowAndUpgradeStates(t *testing.T) {
	overflow := membershipMetadataForCard("free", resourceAccessState{
		State:             resourceAccessReadOnlyOverLimit,
		RequiredPlanLevel: "svip",
		Reason:            "超出额度",
	}, true, "历史趋势")
	if overflow.State != resourceAccessReadOnlyOverLimit || overflow.RequiredPlanLevel != "svip" || !overflow.UpgradeRequired || overflow.Reason != "超出额度" {
		t.Fatalf("overflow card = %+v, want retained S VIP gate", overflow)
	}

	// A stale active row must not bypass a downgrade; conversely, an upgraded
	// plan should immediately restore a row whose required level is satisfied.
	staleAfterDowngrade := membershipMetadataForCard("free", resourceAccessState{
		State:             resourceAccessActive,
		RequiredPlanLevel: "vip",
	}, true, "")
	if staleAfterDowngrade.State != resourceAccessReadOnlyOverLimit || !staleAfterDowngrade.UpgradeRequired {
		t.Fatalf("stale active row after downgrade = %+v, want read-only gate", staleAfterDowngrade)
	}
	restoredAfterUpgrade := membershipMetadataForCard("vip", resourceAccessState{
		State:             resourceAccessReadOnlyOverLimit,
		RequiredPlanLevel: "vip",
		Reason:            "previously over limit",
	}, true, "")
	if restoredAfterUpgrade.State != resourceAccessActive || restoredAfterUpgrade.UpgradeRequired {
		t.Fatalf("row after upgrade = %+v, want active access", restoredAfterUpgrade)
	}
}

func TestLockedContentRedactionPreservesHistoryMetadata(t *testing.T) {
	access := membershipResourceMetadata{
		State:             resourceAccessReadOnlyOverLimit,
		RequiredPlanLevel: "svip",
		UpgradeRequired:   true,
	}

	memory := appMemoryItem{ID: 7, CardID: 3, Content: "private memory", Status: "active"}
	redactAppMemoryContent(&memory, access)
	if memory.Content != "" || memory.ID != 7 || memory.CardID != 3 {
		t.Fatalf("memory redaction = %+v", memory)
	}

	trend := appTrendSeries{Label: "压力", Dimension: "stress", Points: []appTrendPoint{{Date: "2026-09-21", Value: 80}}}
	redactAppTrendSeries(&trend, access)
	if trend.Label != "压力" || trend.Dimension != "stress" || trend.Points == nil || len(trend.Points) != 0 {
		t.Fatalf("trend redaction = %+v", trend)
	}

	portrait := portraitResp{
		HasEnoughData: true, Summary: "private portrait", StateLabel: "成长区",
		Strengths: []string{"strength"}, MainType: 4, UpdatedAt: "2026/09/21 10:00:00",
	}
	redactPortraitContent(&portrait, access)
	if portrait.HasEnoughData || portrait.Summary != "" || portrait.StateLabel != "" || len(portrait.Strengths) != 0 || portrait.MainType != 4 || portrait.UpdatedAt == "" {
		t.Fatalf("portrait redaction = %+v", portrait)
	}

	report := appCompatibilityReport{
		ID: 9, CardAName: "本人", CardBName: "朋友", Summary: "short summary",
		Dynamics: "full dynamics", Strengths: "full strengths", Advice: "full advice",
		IsFull: true, Highlights: []string{"h"}, ConflictPoints: []string{"c"},
		Suggestions: []string{"s"}, Scores: compatibility.Scores{Resonance: 80},
		ExplainTags: []string{"tag"}, Evidence: []compatibility.Evidence{{Title: "e"}},
	}
	applyAppCompatibilityAccess(&report, access)
	if report.Summary != "short summary" || report.IsFull || report.IsFullSnake || report.Dynamics != "" || report.Strengths != "" || report.Advice != "" || len(report.Highlights) != 0 || len(report.ConflictPoints) != 0 || len(report.Suggestions) != 0 || report.Scores != (compatibility.Scores{}) || len(report.ExplainTags) != 0 || len(report.Evidence) != 0 {
		t.Fatalf("compatibility redaction = %+v", report)
	}
}

func TestLockedLifeStoryRedactionPreservesSummaryAndStatus(t *testing.T) {
	story := lifestory.Story{
		ID: 12, Title: "我的故事", Status: lifestory.StatusCompleted, Summary: "历史摘要",
		MaterialCount: 4, Materials: []lifestory.Material{{Text: "private"}},
		FactCard:         lifestory.FactCard{Setting: "private setting"},
		Outline:          lifestory.Outline{Chapters: []lifestory.OutlineChapter{{Title: "private chapter"}}},
		CurrentVersionID: 3, CurrentVersion: &lifestory.Version{ID: 3},
		Versions: []lifestory.Version{{ID: 3}}, LatestJob: &lifestory.Job{ID: 4},
		Jobs: []lifestory.Job{{ID: 4}}, Progress: &lifestory.ReadingProgress{StoryID: 12},
		AccessState: resourceAccessReadOnlyOverLimit, RequiredPlanLevel: "svip", AccessReason: "upgrade",
	}
	redactLockedLifeStoryContent(&story)
	if story.ID != 12 || story.Title != "我的故事" || story.Status != lifestory.StatusCompleted || story.Summary != "历史摘要" || story.MaterialCount != 4 {
		t.Fatalf("history metadata was lost: %+v", story)
	}
	if story.Materials != nil || !reflect.DeepEqual(story.FactCard, lifestory.FactCard{}) || !reflect.DeepEqual(story.Outline, lifestory.Outline{}) || story.CurrentVersionID != 0 || story.CurrentVersion != nil || story.Versions != nil || story.LatestJob != nil || story.Jobs != nil || story.Progress != nil || story.DraftVersion != 0 {
		t.Fatalf("locked story content was not redacted: %+v", story)
	}
}

func TestLockedLifeStorySubresourceRedactionKeepsPollingShape(t *testing.T) {
	access := membershipResourceMetadata{State: resourceAccessReadOnlyOverLimit, UpgradeRequired: true}
	version := lifestory.Version{
		ID: 3, Chapters: []lifestory.Chapter{{Order: 1, Body: "private body"}},
		Reflection: "private reflection", CharacterCount: 100, WordCount: 50,
		Model: "private-model", GenerationConfig: []byte(`{"temperature":0.2}`),
	}
	redactLockedLifeStoryVersion(&version, access)
	if version.ID != 3 || version.Chapters != nil || version.Reflection != "" || version.CharacterCount != 0 || version.WordCount != 0 || version.Model != "" || version.GenerationConfig != nil {
		t.Fatalf("version redaction = %+v", version)
	}

	job := lifestory.Job{ID: 4, Status: lifestory.JobSucceeded, Progress: 100, SourceVersionID: 2, VersionID: 3, ErrorMessage: "private failure"}
	redactLockedLifeStoryJob(&job, access)
	if job.ID != 4 || job.Status != lifestory.JobSucceeded || job.Progress != 100 || job.SourceVersionID != 0 || job.VersionID != 0 || job.ErrorMessage != "" {
		t.Fatalf("job redaction = %+v", job)
	}

	progress := lifestory.ReadingProgress{StoryID: 12, VersionID: 3, ChapterIndex: 2, ChapterOrder: 3, CharacterOffset: 80, Completed: true}
	redactLockedLifeStoryProgress(&progress, access)
	if progress.StoryID != 12 || progress.VersionID != 0 || progress.ChapterIndex != 0 || progress.ChapterOrder != 0 || progress.CharacterOffset != 0 || progress.Completed {
		t.Fatalf("progress redaction = %+v", progress)
	}
}
