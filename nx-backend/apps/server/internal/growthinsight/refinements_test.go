package growthinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func insertCorrection(t *testing.T, db *sql.DB, reportID, userID, cardID int64, note string, at time.Time) Evidence {
	t.Helper()
	var e Evidence
	err := db.QueryRow(`INSERT INTO app_growth_insight_corrections(app_user_id,card_id,report_id,kind,note,created_at)
VALUES($1,$2,$3,'inaccurate',$4,$5)
RETURNING 'correction:'||id,'correction',json_build_object('kind',kind,'note',note)::text,created_at`, userID, cardID, reportID, note, at).Scan(&e.ID, &e.Kind, &e.Text, &e.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func insertOutcome(t *testing.T, db *sql.DB, reportID, userID, cardID int64, outcome, note string, at time.Time) Evidence {
	t.Helper()
	var e Evidence
	err := db.QueryRow(`INSERT INTO app_growth_insight_actions(report_id,app_user_id,card_id,title,detail,status,outcome,note,updated_at)
VALUES($1,$2,$3,'Pause','Take a brief pause','attempted',$4,$5,$6)
RETURNING 'action:'||id,'action_outcome',json_build_object('actionTitle',title,'status',status,'outcome',outcome,'note',note)::text,updated_at`, reportID, userID, cardID, outcome, note, at).Scan(&e.ID, &e.Kind, &e.Text, &e.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func saveEvidenceFixture(t *testing.T, db *sql.DB, reportID int64, evidence []Evidence) {
	t.Helper()
	if evidence == nil {
		evidence = []Evidence{}
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app_growth_insight_reports SET payload=jsonb_set(payload,'{evidence}',$2::jsonb) WHERE id=$1`, reportID, body); err != nil {
		t.Fatal(err)
	}
}

func assertProgress(t *testing.T, view UserView, total, included int) {
	t.Helper()
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		FeedbackProgress *struct {
			TotalCount    int `json:"totalCount"`
			IncludedCount int `json:"includedCount"`
			PendingCount  int `json:"pendingCount"`
		} `json:"feedbackProgress"`
	}
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	p := value.FeedbackProgress
	if p == nil || p.TotalCount != total || p.IncludedCount != included || p.PendingCount != total-included {
		t.Fatalf("feedback progress want total=%d included=%d pending=%d: %s", total, included, total-included, body)
	}
}

func assertProgressOmitted(t *testing.T, view UserView) {
	t.Helper()
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"feedbackProgress"`) {
		t.Fatalf("unavailable state leaked feedback progress: %s", body)
	}
}

func TestCollectorReservesRecentCorrectionsAndNegativeOutcomes(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	var corrections, negatives []Evidence
	for i := 0; i < 12; i++ {
		at := now.Add(-5*24*time.Hour + time.Duration(i)*time.Minute)
		corrections = append(corrections, insertCorrection(t, db, view.ReportID, 1, 1, fmt.Sprintf("Correction %d", i), at))
		outcome := "not_helpful"
		if i%2 == 0 {
			outcome = "worse"
		}
		negatives = append(negatives, insertOutcome(t, db, view.ReportID, 1, 1, outcome, fmt.Sprintf("Negative %d", i), at))
	}
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time)
SELECT 1,'user','Recent chat '||n,$1 FROM generate_series(1,100) n`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	first, err := collect(context.Background(), db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Evidence) != 80 {
		t.Fatalf("selection shape len=%d", len(first.Evidence))
	}
	selected := map[string]bool{}
	for _, e := range first.Evidence {
		selected[e.ID] = true
	}
	if !selected["assessment:1"] {
		t.Fatal("assessment lost priority allocation")
	}
	for i := 1; i < len(first.Evidence); i++ {
		prev, next := first.Evidence[i-1], first.Evidence[i]
		if next.OccurredAt.Before(prev.OccurredAt) || (next.OccurredAt.Equal(prev.OccurredAt) && next.ID < prev.ID) {
			t.Fatal("evidence output is not deterministically chronological")
		}
	}
	for i := 0; i < 12; i++ {
		want := i >= 4
		if selected[corrections[i].ID] != want || selected[negatives[i].ID] != want {
			t.Fatalf("reserved feedback selection i=%d correction=%v negative=%v want=%v", i, selected[corrections[i].ID], selected[negatives[i].ID], want)
		}
	}
	if first.Coverage.EligibleCount != 127 || first.Coverage.OmittedCount != 47 || !first.Coverage.Truncated {
		t.Fatalf("coverage=%+v", first.Coverage)
	}
	second, err := collect(context.Background(), db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatal("equal-timestamp selection or fingerprint is unstable")
	}
}

func TestCollectorPriorityHonorsTimeAndUnicodeBudgets(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	atBoundary := insertCorrection(t, db, view.ReportID, 1, 1, "Boundary correction", now.Add(-30*24*time.Hour))
	tooOld := insertCorrection(t, db, view.ReportID, 1, 1, "Expired correction", now.Add(-30*24*time.Hour-time.Microsecond))
	future := insertOutcome(t, db, view.ReportID, 1, 1, "worse", "Future outcome", now.Add(time.Microsecond))
	negative := insertOutcome(t, db, view.ReportID, 1, 1, "not_helpful", strings.Repeat("压", 500), now.Add(-time.Hour))
	for i := 0; i < 8; i++ {
		insertCorrection(t, db, view.ReportID, 1, 1, fmt.Sprintf("Future correction %d", i), now.Add(time.Hour))
		insertOutcome(t, db, view.ReportID, 1, 1, "worse", fmt.Sprintf("Future negative %d", i), now.Add(time.Hour))
	}
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time)
SELECT 1,'user',repeat('文',3000),$1 FROM generate_series(1,100)`, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	sources, err := collect(context.Background(), db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	characters := 0
	for _, e := range sources.Evidence {
		selected[e.ID] = true
		length := utf8.RuneCountInString(e.Text)
		characters += length
		if length > 2000 {
			t.Fatalf("per-source Unicode budget exceeded: %d", length)
		}
	}
	if !selected[atBoundary.ID] || !selected[negative.ID] || selected[tooOld.ID] || selected[future.ID] {
		t.Fatalf("window priority selection=%v", selected)
	}
	if characters != 24000 || len(sources.Evidence) >= 80 || sources.Coverage.TruncatedCount == 0 {
		t.Fatalf("bounds chars=%d coverage=%+v", characters, sources.Coverage)
	}
}

func TestFeedbackProgressUsesExactCurrentVersionAndFullText(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	stamp := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	unchanged := insertCorrection(t, db, view.ReportID, 1, 1, "Saved correction", stamp)
	edited := insertOutcome(t, db, view.ReportID, 1, 1, "helpful", "Original note", stamp)
	timestampChanged := insertOutcome(t, db, view.ReportID, 1, 1, "unknown", "Same text later", stamp)
	truncated := insertCorrection(t, db, view.ReportID, 1, 1, strings.Repeat("\x01", 500), stamp)
	partial := truncated
	partial.Text = string([]rune(partial.Text)[:2000])
	saveEvidenceFixture(t, db, view.ReportID, []Evidence{unchanged, edited, timestampChanged, partial})
	if _, err := db.Exec(`UPDATE app_growth_insight_actions SET note='Edited note' WHERE 'action:'||id=$1;
`, edited.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE app_growth_insight_actions SET updated_at=updated_at+interval '1 microsecond' WHERE 'action:'||id=$1`, timestampChanged.ID); err != nil {
		t.Fatal(err)
	}
	actual, err := s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 4, 1)
	encoded, _ := json.Marshal(actual)
	for _, forbidden := range []string{unchanged.ID, edited.ID, "Saved correction", "Edited note", "evidenceRefs"} {
		var result map[string]json.RawMessage
		_ = json.Unmarshal(encoded, &result)
		if strings.Contains(string(result["feedbackProgress"]), forbidden) {
			t.Fatalf("progress leaked %s", forbidden)
		}
	}
}

func TestFeedbackProgressCountsAllFeedbackBeyondCollectorCap(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	at := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 100; i++ {
		insertCorrection(t, db, view.ReportID, 1, 1, fmt.Sprintf("Current correction %d", i), at)
	}
	sources, err := collect(context.Background(), db, 1, 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	saveEvidenceFixture(t, db, view.ReportID, sources.Evidence)
	actual, err := s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 100, 79)
}

func TestFeedbackProgressUsesPublishedVersionAndHandlesLegacyEvidence(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	e := insertCorrection(t, db, view.ReportID, 1, 1, "Awaiting next publication", time.Now().UTC().Add(-time.Minute))
	var unpublishedID int64
	if err := db.QueryRow(`INSERT INTO app_growth_insight_reports(app_user_id,card_id,version,fingerprint,generated_at,source_through,evidence_count,payload)
SELECT app_user_id,card_id,version+1,'next',generated_at+interval '1 day',source_through,1,payload FROM app_growth_insight_reports WHERE id=$1 RETURNING id`, view.ReportID).Scan(&unpublishedID); err != nil {
		t.Fatal(err)
	}
	saveEvidenceFixture(t, db, unpublishedID, []Evidence{e})
	if _, err := db.Exec(`UPDATE app_growth_insight_consents SET latest_report_id=$2 WHERE app_user_id=$1`, 1, unpublishedID); err != nil {
		t.Fatal(err)
	}
	actual, err := s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 1, 0)
	if _, err := db.Exec(`UPDATE app_growth_insight_consents SET published_report_id=$2 WHERE app_user_id=$1`, 1, unpublishedID); err != nil {
		t.Fatal(err)
	}
	actual, err = s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 1, 1)
	if _, err := db.Exec(`UPDATE app_growth_insight_reports SET payload=payload-'evidence' WHERE id=$1`, unpublishedID); err != nil {
		t.Fatal(err)
	}
	actual, err = s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 1, 0)
}

func TestFeedbackProgressPrivacyAndExpiredWindow(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	view := enableAndRun(t, s, now)
	current := time.Now().UTC()
	insertCorrection(t, db, view.ReportID, 1, 1, "Current own feedback", current.Add(-time.Hour))
	insertCorrection(t, db, view.ReportID, 1, 1, "Expired feedback", current.Add(-31*24*time.Hour))
	insertCorrection(t, db, view.ReportID, 1, 1, "Future feedback", current.Add(time.Hour))
	insertCorrection(t, db, view.ReportID, 1, 3, "Secondary feedback", current.Add(-time.Hour))
	insertCorrection(t, db, view.ReportID, 2, 2, "Other user's feedback", current.Add(-time.Hour))
	actual, err := s.View(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 1, 0)
	secondary, err := s.View(ctx, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertProgressOmitted(t, secondary)
	other, err := s.View(ctx, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	assertProgressOmitted(t, other)
	if _, err = s.View(ctx, 2, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user view: %v", err)
	}
	if _, err = s.SetConsent(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	disabled, err := s.View(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgressOmitted(t, disabled)
	if _, err = s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	pending, err := s.View(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, pending, 0, 0)
}

func TestFeedbackProgressCountsAllFeedbackBeyondTextBudget(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	current := time.Now().UTC().Add(-time.Minute)
	complete := map[string]Evidence{}
	for i := 0; i < 60; i++ {
		e := insertCorrection(t, db, view.ReportID, 1, 1, fmt.Sprintf("%03d", i)+strings.Repeat("改", 497), current)
		complete[e.ID] = e
	}
	sources, err := collect(context.Background(), db, 1, 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	characters, included := 0, 0
	for _, e := range sources.Evidence {
		characters += utf8.RuneCountInString(e.Text)
		if full, exists := complete[e.ID]; exists && full.Text == e.Text && full.OccurredAt.Equal(e.OccurredAt) {
			included++
		}
	}
	if characters != 24000 || len(sources.Evidence) >= 80 || sources.Coverage.TruncatedCount == 0 || included >= 60 {
		t.Fatalf("text budget fixture did not exhaust selection: chars=%d included=%d coverage=%+v", characters, included, sources.Coverage)
	}
	saveEvidenceFixture(t, db, view.ReportID, sources.Evidence)
	actual, err := s.View(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, actual, 60, included)
}

func TestFeedbackProgressUsesInclusiveRollingWindowAndExcludesPendingActions(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	boundary := insertCorrection(t, db, view.ReportID, 1, 1, "Exact lower boundary", now.Add(-30*24*time.Hour))
	atNow := insertOutcome(t, db, view.ReportID, 1, 1, "unknown", "Exact upper boundary", now)
	insertCorrection(t, db, view.ReportID, 1, 1, "One microsecond too old", now.Add(-30*24*time.Hour-time.Microsecond))
	insertOutcome(t, db, view.ReportID, 1, 1, "worse", "One microsecond in future", now.Add(time.Microsecond))
	progress, err := loadFeedbackProgress(context.Background(), db, 1, 1, view.ReportID, []Evidence{boundary, atNow}, now)
	if err != nil {
		t.Fatal(err)
	}
	if progress == nil || progress.TotalCount != 2 || progress.IncludedCount != 2 || progress.PendingCount != 0 {
		t.Fatalf("rolling window progress=%+v", progress)
	}
}

func TestFeedbackProgressRejectsStalePublicationPointer(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	e := insertCorrection(t, db, view.ReportID, 1, 1, "Current correction", now.Add(-time.Hour))
	progress, err := loadFeedbackProgress(context.Background(), db, 1, 1, view.ReportID+1, []Evidence{e}, now)
	if err != nil || progress != nil {
		t.Fatalf("stale publication progress=%+v err=%v", progress, err)
	}
}

func TestFeedbackProgressAndReportStayInOneReadSnapshot(t *testing.T) {
	s, db, now := fixture(t)
	first := enableAndRun(t, s, now)
	current := time.Now().UTC()
	e := insertCorrection(t, db, first.ReportID, 1, 1, "Publication snapshot", current.Add(-time.Minute))
	var secondID int64
	if err := db.QueryRow(`INSERT INTO app_growth_insight_reports(app_user_id,card_id,version,fingerprint,generated_at,source_through,evidence_count,payload)
SELECT app_user_id,card_id,version+1,'next',generated_at+interval '1 day',source_through,1,payload FROM app_growth_insight_reports WHERE id=$1 RETURNING id`, first.ReportID).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	saveEvidenceFixture(t, db, secondID, []Evidence{e})
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	before, err := view(ctx, tx, 1, 1, current)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, before, 1, 0)
	if _, err = db.Exec(`UPDATE app_growth_insight_consents SET published_report_id=$1,latest_report_id=$1 WHERE app_user_id=1`, secondID); err != nil {
		t.Fatal(err)
	}
	insertCorrection(t, db, secondID, 1, 1, "Saved while reading", current.Add(-time.Second))
	during, err := view(ctx, tx, 1, 1, current)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, during, 1, 0)
	if during.ReportID != first.ReportID {
		t.Fatalf("mixed report snapshot: %+v", during)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	after, err := s.View(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertProgress(t, after, 2, 1)
	if after.ReportID != secondID {
		t.Fatalf("publication did not advance: %+v", after)
	}
}

func TestCollectorFingerprintIncludesFractionalFeedbackTimestamp(t *testing.T) {
	s, db, now := fixture(t)
	view := enableAndRun(t, s, now)
	e := insertOutcome(t, db, view.ReportID, 1, 1, "worse", "Unchanged text", now.Add(-time.Minute))
	before, err := collect(context.Background(), db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE app_growth_insight_actions SET updated_at=updated_at+interval '1 microsecond' WHERE 'action:'||id=$1`, e.ID); err != nil {
		t.Fatal(err)
	}
	after, err := collect(context.Background(), db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == after.Fingerprint {
		t.Fatal("fingerprint lost fractional feedback timestamp")
	}
}

func TestFeedbackProgressOldPayloadIsOptional(t *testing.T) {
	var old UserView
	if err := json.Unmarshal([]byte(`{"enabled":true,"status":"ready","cardId":1,"reportId":1,"actions":[]}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.FeedbackProgress != nil {
		t.Fatalf("old payload invented progress: %+v", old.FeedbackProgress)
	}
	assertProgressOmitted(t, old)
}
