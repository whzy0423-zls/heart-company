package profilecalibration

import (
	"os"
	"strings"
	"testing"
)

func TestDailyQuizSetCreationIsInsertOnlyToAvoidConcurrentOverwrite(t *testing.T) {
	source := readDailyQuizSetsSource(t)

	if count := strings.Count(source, "ON CONFLICT (quiz_date) DO NOTHING"); count < 2 {
		t.Fatalf("expected both AI and fallback daily quiz set creation paths to use DO NOTHING, got %d occurrences", count)
	}
	if strings.Contains(source, "ON CONFLICT (quiz_date) DO UPDATE") {
		t.Fatal("daily quiz set generation must not overwrite an existing retained set")
	}
}

func TestReplaceDailyQuizQuestionLocksWholeDateAfterAnyAnswer(t *testing.T) {
	source := readDailyQuizSetsSource(t)

	for _, want := range []string{
		"FROM app_daily_quiz_batches b",
		"b.quiz_date=$1::date",
		"b.answered_count > 0",
		"EXISTS (SELECT 1 FROM app_daily_quiz_answers a WHERE a.batch_id=b.id)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("expected replacement guard to lock the whole quiz date after any answer with %q", want)
		}
	}
	if strings.Contains(source, "WHERE b.quiz_date=$1::date\n\t\t  AND a.question_id=$2") {
		t.Fatal("replacement guard must not only check the selected question id")
	}
}

func readDailyQuizSetsSource(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("daily_quiz_sets.go")
	if err != nil {
		t.Fatalf("read daily_quiz_sets.go: %v", err)
	}
	return string(content)
}
