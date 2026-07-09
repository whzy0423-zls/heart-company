package profilecalibration

import (
	"strings"
	"testing"
)

func TestBuildReportIncludesExtractedPrivateEvidence(t *testing.T) {
	extracted := []EvidenceSummary{
		{Kind: "chat", Label: "聊天", Text: "多次提到担心关系不稳定，会反复确认对方态度。"},
		{Kind: "voice_text", Label: "语音", Text: "语音转文字中反复出现担心、害怕出错等表达。"},
		{Kind: "behavior", Label: "行为", Text: "连续完成校准题，互动节奏稳定。"},
	}

	report := buildReport(66, 123, "generated", 4, 6, 100, map[int]int{6: 70}, extracted, 0.82)

	if len(report.Evidence) < 4 {
		t.Fatalf("expected daily evidence plus extracted evidence, got %+v", report.Evidence)
	}
	if report.Evidence[0].Kind != "daily_quiz" {
		t.Fatalf("daily quiz should remain the primary evidence first, got %+v", report.Evidence)
	}
	joined := evidenceTexts(report.Evidence)
	for _, want := range []string{"关系不稳定", "语音转文字", "连续完成校准题"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected report evidence to include %q, evidence=%+v", want, report.Evidence)
		}
	}
	if !strings.Contains(report.Profile.PrivateSignalSummary, "聊天") || !strings.Contains(report.Profile.PrivateSignalSummary, "语音") || !strings.Contains(report.Profile.PrivateSignalSummary, "行为") {
		t.Fatalf("expected private signal summary to mention private evidence sources, got %q", report.Profile.PrivateSignalSummary)
	}
}

func TestSuggestTypeCombinesDailyAnswersAndPrivateEvidence(t *testing.T) {
	dailyCounts := map[int]int{4: 60, 6: 45}
	extracted := []EvidenceSummary{
		{Kind: "chat", Label: "聊天", Text: "多次提到担心出错、害怕关系不稳定、需要反复确认。"},
		{Kind: "voice_text", Label: "语音", Text: "语音转文字里出现担心、风险、确认等安全感表达。"},
	}

	suggested, confidence := combinedSuggestedType(4, dailyCounts, extracted)

	if suggested != 6 {
		t.Fatalf("expected private evidence to move suggestion toward type 6, got %d", suggested)
	}
	if confidence <= 0.76 {
		t.Fatalf("expected private evidence to raise confidence above baseline, got %.2f", confidence)
	}
}

func TestReassessmentTriggerReadsProfileEvidenceWindow(t *testing.T) {
	source := readStoreSource(t)
	for _, want := range []string{
		"FROM app_profile_evidence",
		"source_type",
		"status='active'",
		"chat_evidence_count",
		"voice_evidence_count",
		"behavior_evidence_count",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("expected reassessment trigger to use profile evidence with %q", want)
		}
	}
}

func evidenceTexts(items []EvidenceSummary) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, "\n")
}
