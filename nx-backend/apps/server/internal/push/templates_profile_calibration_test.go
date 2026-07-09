package push

import "testing"

func TestDailyQuizReminderUsesDailyQuizDeepLink(t *testing.T) {
	msg := DailyQuizReminder()

	if msg.Title == "" || msg.Content == "" {
		t.Fatalf("expected daily quiz reminder title/content, got %+v", msg)
	}
	if msg.DeepLink != "/daily-quiz" {
		t.Fatalf("expected /daily-quiz deep link, got %q", msg.DeepLink)
	}
}

func TestReassessmentReadyUsesReportDeepLink(t *testing.T) {
	msg := ReassessmentReady(42)

	if msg.Title == "" || msg.Content == "" {
		t.Fatalf("expected reassessment title/content, got %+v", msg)
	}
	if msg.DeepLink != "/reassessment/42" {
		t.Fatalf("expected report deep link, got %q", msg.DeepLink)
	}
}
