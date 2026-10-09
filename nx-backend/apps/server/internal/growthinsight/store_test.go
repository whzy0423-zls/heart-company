package growthinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func isolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GROWTH_INSIGHT_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GROWTH_INSIGHT_POSTGRES_DSN not set")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*cfg)
	schema := fmt.Sprintf("growth_fixture_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { db.Close(); admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	return db
}

func fixture(t *testing.T) (*Store, *sql.DB, time.Time) {
	t.Helper()
	db := isolatedDB(t)
	_, err := db.Exec(`CREATE TABLE app_users(id BIGINT PRIMARY KEY,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE app_quiz_submissions(id BIGINT PRIMARY KEY, app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE, result JSONB NOT NULL DEFAULT '{}',primary_type INT NOT NULL DEFAULT 4,wing_type INT NOT NULL DEFAULT 0,answers JSONB NOT NULL DEFAULT '[]',create_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_user_cards(id BIGINT PRIMARY KEY,app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE,card_type TEXT NOT NULL DEFAULT 'primary',status TEXT NOT NULL DEFAULT 'active',submission_id BIGINT REFERENCES app_quiz_submissions(id) ON DELETE SET NULL,enneagram INT NOT NULL DEFAULT 0,profile JSONB NOT NULL DEFAULT '{}',revision BIGINT NOT NULL DEFAULT 1,update_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_chat_sessions(id BIGINT PRIMARY KEY,app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE,card_id BIGINT REFERENCES app_user_cards(id) ON DELETE CASCADE,scene TEXT NOT NULL DEFAULT 'chat',skill_version_id BIGINT);
CREATE TABLE app_chat_messages(id BIGSERIAL PRIMARY KEY,session_id BIGINT REFERENCES app_chat_sessions(id) ON DELETE CASCADE,role TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',transcript TEXT NOT NULL DEFAULT '',create_time TIMESTAMPTZ NOT NULL DEFAULT now());`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../db/growth_insight_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(b)); err != nil {
		t.Fatalf("schema is not idempotent: %v", err)
	}
	now := time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	_, err = db.Exec(`INSERT INTO app_users(id) VALUES(1),(2);
INSERT INTO app_quiz_submissions(id,app_user_id,result,create_time) VALUES(1,1,'{"primaryType":4,"templateTrait":"GENERATED_BASELINE_SECRET"}','2026-10-07T01:00:00Z');
INSERT INTO app_user_cards(id,app_user_id,submission_id) VALUES(1,1,1),(2,2,NULL);
INSERT INTO app_user_cards(id,app_user_id,card_type) VALUES(3,1,'secondary');
INSERT INTO app_chat_sessions(id,app_user_id,card_id) VALUES(1,1,1),(2,2,2),(3,1,3);
INSERT INTO app_chat_sessions(id,app_user_id,scene,skill_version_id) VALUES(4,1,'skill_chat',1);`)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		session    int
		role, text string
	}{{1, "user", "I paused before replying."}, {1, "user", "I asked for a short break."}, {1, "assistant", "ASSISTANT_SECRET"}, {2, "user", "OTHER_USER_SECRET"}, {3, "user", "SECONDARY_SECRET"}, {4, "user", "PRIVATE_SKILL_SECRET"}} {
		if _, err = db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,$2,$3,$4)`, v.session, v.role, v.text, now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(db), db, now
}

func fixtureComplete(t *testing.T, calls *int) CompleteFunc {
	t.Helper()
	return func(ctx context.Context, prompt string) (string, error) {
		*calls++
		for _, secret := range []string{"ASSISTANT_SECRET", "OTHER_USER_SECRET", "SECONDARY_SECRET", "PRIVATE_SKILL_SECRET", "GENERATED_BASELINE_SECRET"} {
			if strings.Contains(prompt, secret) {
				t.Fatalf("source leak: %s", secret)
			}
		}
		if !strings.Contains(prompt, "message:1") || !strings.Contains(prompt, "assessment:1") {
			t.Fatal("missing original sources")
		}
		b, _ := json.Marshal(validAnalysis())
		return string(b), nil
	}
}

func enableAndRun(t *testing.T, s *Store, now time.Time) UserView {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	// Consent itself is not new behavioral evidence, so historic eligible input may run immediately.
	var calls int
	if err := s.Tick(ctx, fixtureComplete(t, &calls), now); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	v, err := s.View(ctx, 1, 1)
	if err != nil || v.Status != "ready" || len(v.Actions) != 1 {
		t.Fatalf("view=%+v err=%v", v, err)
	}
	return v
}

func TestPersistentConsentSourceIsolationAndFeedback(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	v := enableAndRun(t, s, now)
	if _, err := s.View(ctx, 2, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross owner view: %v", err)
	}
	v2, err := s.View(ctx, 1, 3)
	if err != nil || v2.ReportID != 0 {
		t.Fatalf("secondary projection leaked: %+v %v", v2, err)
	}
	f := Feedback{Status: "attempted", Outcome: "helpful", Note: "It helped me pause."}
	a, err := s.FeedbackAction(ctx, 1, v.Actions[0].ID, f)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.FeedbackAction(ctx, 1, v.Actions[0].ID, f)
	if err != nil || a.UpdatedAt != a2.UpdatedAt {
		t.Fatalf("idempotency: %+v %+v %v", a, a2, err)
	}
	if _, err = s.FeedbackAction(ctx, 2, a.ID, f); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross owner feedback: %v", err)
	}
	if err = s.Correct(ctx, 1, v.ReportID, "inaccurate", "This was an exception."); err != nil {
		t.Fatal(err)
	}
	if err = s.Correct(ctx, 2, v.ReportID, "inaccurate", "not mine"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross owner correction: %v", err)
	}
	reloaded, err := NewStore(db).View(ctx, 1, 1)
	if err != nil || reloaded.Actions[0].Note != f.Note {
		t.Fatal("feedback did not persist")
	}
	meta, err := s.ConsentMetadata(ctx, 1)
	if err != nil || meta.ReportID != v.ReportID || meta.DisclosureVersion != DisclosureVersion {
		t.Fatalf("meta=%+v %v", meta, err)
	}
	if _, err = s.SetConsent(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	list, err := s.AdminList(ctx, 1)
	if err != nil || list.Enabled || len(list.Reports) != 0 {
		t.Fatalf("optout retains data: %+v %v", list, err)
	}
	if err = s.Enqueue(ctx, 1); !errors.Is(err, ErrDisabled) {
		t.Fatalf("admin overrode optout: %v", err)
	}
}

func TestCadencePublicationAndUnchangedEvidence(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	first := enableAndRun(t, s, now)
	var calls int
	complete := fixtureComplete(t, &calls)
	if err := s.Tick(ctx, complete, now.Add(48*time.Hour)); err != nil || calls != 0 {
		t.Fatalf("unchanged input called model: %d %v", calls, err)
	}
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(1,'user','New activity',$1)`, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []time.Duration{time.Hour + 10*time.Minute, 2 * time.Hour} {
		if err := s.Tick(ctx, complete, now.Add(offset)); err != nil || calls != 0 {
			t.Fatalf("cadence/debounce failed: %d %v", calls, err)
		}
	}
	if err := s.Tick(ctx, complete, now.Add(25*time.Hour)); err != nil || calls != 1 {
		t.Fatalf("daily refresh: %d %v", calls, err)
	}
	v, err := s.View(ctx, 1, 1)
	if err != nil || v.ReportID != first.ReportID {
		t.Fatal("publication changed before seven days")
	}
	if err := s.Tick(ctx, complete, now.Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	v, err = s.View(ctx, 1, 1)
	if err != nil || v.ReportID == first.ReportID || calls != 1 {
		t.Fatalf("mature report not published without extra model call: %+v calls=%d err=%v", v, calls, err)
	}
	if v.NextUpdateAt != stamp(now.Add(15*24*time.Hour)) {
		t.Fatalf("publication cooldown uses generation date: %s", v.NextUpdateAt)
	}
}

func TestRevocationAndDeletionDuringGenerationDiscardOutput(t *testing.T) {
	for _, mode := range []string{"optout", "message_delete", "session_delete", "card_replace", "account_disable", "assessment_delete", "account_delete", "message_edit"} {
		t.Run(mode, func(t *testing.T) {
			s, db, now := fixture(t)
			ctx := context.Background()
			if _, err := s.SetConsent(ctx, 1, true); err != nil {
				t.Fatal(err)
			}
			complete := func(context.Context, string) (string, error) {
				var err error
				switch mode {
				case "optout":
					_, err = s.SetConsent(ctx, 1, false)
				case "message_delete":
					_, err = db.Exec(`DELETE FROM app_chat_messages WHERE id=1`)
				case "session_delete":
					_, err = db.Exec(`DELETE FROM app_chat_sessions WHERE id=1`)
				case "card_replace":
					_, err = db.Exec(`UPDATE app_user_cards SET status='deleted' WHERE id=1`)
				case "account_disable":
					_, err = db.Exec(`UPDATE app_users SET status='disabled' WHERE id=1`)
				case "assessment_delete":
					_, err = db.Exec(`DELETE FROM app_quiz_submissions WHERE id=1`)
				case "account_delete":
					_, err = db.Exec(`DELETE FROM app_users WHERE id=1`)
				case "message_edit":
					_, err = db.Exec(`UPDATE app_chat_messages SET content='Actually I did not pause' WHERE id=1`)
				}
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(validAnalysis())
				return string(b), nil
			}
			if err := s.Tick(ctx, complete, now); err != nil && !errors.Is(err, ErrStale) {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_reports`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("stale report persisted count=%d err=%v", count, err)
			}
		})
	}
}

func TestPublishedReportImmediatelyInvalidatedBySourceDeletion(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	enableAndRun(t, s, now)
	if _, err := db.Exec(`DELETE FROM app_chat_messages WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	v, err := s.View(ctx, 1, 1)
	if err != nil || v.ReportID != 0 {
		t.Fatalf("deleted evidence still published %+v %v", v, err)
	}
}

func TestOptinCannotRaceAccountDeactivation(t *testing.T) {
	s, db, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE app_users SET status='disabled' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := s.SetConsent(ctx, 1, true); result <- err }()
	select {
	case err := <-result:
		t.Fatalf("optin bypassed in-flight deactivation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrNotFound) {
		t.Fatalf("deactivated user opted in: %v", err)
	}
	meta, err := s.ConsentMetadata(ctx, 1)
	if err != nil || meta.Enabled {
		t.Fatalf("disabled consent: %+v %v", meta, err)
	}
}

func TestRetriesAreBoundedAndDoNotReplaceLastGoodPublication(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	first := enableAndRun(t, s, now)
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(1,'user','A new difficulty',$1)`, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	boom := errors.New("fixture outage")
	complete := func(context.Context, string) (string, error) { calls++; return "", boom }
	for _, offset := range []time.Duration{24 * time.Hour, 24*time.Hour + 15*time.Minute, 24*time.Hour + 45*time.Minute} {
		if err := s.Tick(ctx, complete, now.Add(offset)); !errors.Is(err, boom) {
			t.Fatalf("expected model error: %v", err)
		}
	}
	if err := s.Tick(ctx, complete, now.Add(48*time.Hour)); err != nil || calls != 3 {
		t.Fatalf("retry bound calls=%d err=%v", calls, err)
	}
	v, err := s.View(ctx, 1, 1)
	if err != nil || v.ReportID != first.ReportID || v.Status != "ready" {
		t.Fatalf("failed refresh replaced publication %+v %v", v, err)
	}
}

func TestConcurrentTicksAndExpiredClaimToken(t *testing.T) {
	s, _, now := fixture(t)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueue(ctx, 1, now, false); err != nil {
		t.Fatal(err)
	}
	first, err := s.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.claim(ctx, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("concurrent claim: %v", err)
	}
	second, err := s.claim(ctx, now.Add(11*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := collect(ctx, s.db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(ctx, first, sources, validAnalysis(), now, false); !errors.Is(err, ErrStale) {
		t.Fatalf("old token published: %v", err)
	}
	if err = s.finish(ctx, second, sources, validAnalysis(), now.Add(11*time.Minute), false); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseValidationIncludesElapsedGenerationTime(t *testing.T) {
	s, _, now := fixture(t)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueue(ctx, 1, now, false); err != nil {
		t.Fatal(err)
	}
	j, err := s.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	j.claimedAt = time.Now().Add(-11 * time.Minute)
	sources, err := collect(ctx, s.db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(ctx, j, sources, validAnalysis(), now, false); !errors.Is(err, ErrStale) {
		t.Fatalf("expired wall clock lease published: %v", err)
	}
}

func TestFinalExpiredLeaseBecomesFailed(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueue(ctx, 1, now, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.claim(ctx, now.Add(time.Duration(i)*11*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Tick(ctx, func(context.Context, string) (string, error) { t.Fatal("exhausted lease called model"); return "", nil }, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM app_growth_insight_jobs WHERE app_user_id=1`).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("terminal lease status=%s err=%v", status, err)
	}
}

func TestInsufficientEvidenceAndBoundedCoverage(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`DELETE FROM app_chat_messages WHERE session_id=1 AND role='user'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, func(context.Context, string) (string, error) { t.Fatal("baseline alone called model"); return "", nil }, now); err != nil {
		t.Fatal(err)
	}
	v, err := s.View(ctx, 1, 1)
	if err != nil || v.Status != "insufficient_data" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) SELECT 1,'user',repeat('文',3000),$1 FROM generate_series(1,100)`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	sources, err := collect(ctx, db, 1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if !sources.Coverage.Truncated || sources.Coverage.EligibleCount != 101 || sources.Coverage.IncludedCount >= 80 || sources.Coverage.TruncatedCount == 0 {
		t.Fatalf("coverage=%+v", sources.Coverage)
	}
	runes := 0
	for _, e := range sources.Evidence {
		runes += len([]rune(e.Text))
	}
	if runes > 24000 {
		t.Fatalf("source bound=%d", runes)
	}
}

func TestPrivacyExportIsAllowlistedOwnedAndEmptyAfterOptout(t *testing.T) {
	s, _, now := fixture(t)
	ctx := context.Background()
	view := enableAndRun(t, s, now)
	if _, err := s.FeedbackAction(ctx, 1, view.Actions[0].ID, Feedback{Status: "attempted", Outcome: "not_helpful", Note: "Please keep this explicit feedback"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Correct(ctx, 1, view.ReportID, "inaccurate", "Please retain this correction"); err != nil {
		t.Fatal(err)
	}
	data, err := s.Export(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !data.Consent.Enabled || len(data.Reports) != 1 || len(data.Actions) != 1 || len(data.Corrections) != 1 {
		t.Fatalf("export=%+v", data)
	}
	if data.Actions[0].Note != "Please keep this explicit feedback" || data.Corrections[0].Note != "Please retain this correction" {
		t.Fatal("explicit evidence missing")
	}
	body, _ := json.Marshal(data)
	for _, secret := range []string{"evidenceRefs", "uncertainties", "message:1", "OTHER_USER_SECRET"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("export leaks internal %s", secret)
		}
	}
	other, err := s.Export(ctx, 2)
	if err != nil || len(other.Reports) > 0 || len(other.Actions) > 0 || len(other.Corrections) > 0 {
		t.Fatalf("cross-owner export %+v %v", other, err)
	}
	if _, err = s.SetConsent(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	disabled, err := s.Export(ctx, 1)
	if err != nil || disabled.Consent.Enabled || len(disabled.Reports) > 0 || len(disabled.Actions) > 0 || len(disabled.Corrections) > 0 {
		t.Fatalf("optout export %+v %v", disabled, err)
	}
}

func TestMigrationAgainstFullExistingSchema(t *testing.T) {
	db := isolatedDB(t)
	for _, path := range []string{"../db/schema.sql", "../db/growth_insight_schema.sql", "../db/growth_insight_schema.sql"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO app_users(id,phone) VALUES(1,'fixture-1'); INSERT INTO app_user_cards(id,app_user_id) VALUES(1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(db).SetConsent(context.Background(), 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM app_users WHERE id=1`); err != nil {
		t.Fatalf("full-schema deletion: %v", err)
	}
}

func TestOptedInUsersWithoutPrimaryCannotStarveEligibleUsers(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE app_growth_insight_consents SET last_scanned_at=now() WHERE app_user_id=1;
INSERT INTO app_users(id) SELECT generate_series(10,109);
INSERT INTO app_growth_insight_consents(app_user_id,enabled) SELECT generate_series(10,109),true;`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := s.Tick(ctx, fixtureComplete(t, &calls), now); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("primary user starved behind cardless accounts: calls=%d", calls)
	}
}

func TestSecondarySessionDeletionPreservesPrimaryPublication(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	before := enableAndRun(t, s, now)
	if _, err := db.Exec(`DELETE FROM app_chat_sessions WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	after, err := s.View(ctx, 1, 1)
	if err != nil || after.ReportID != before.ReportID || len(after.Actions) != 1 {
		t.Fatalf("secondary deletion erased primary: %+v %v", after, err)
	}
}

func TestUnpublishedActionAndCorrectionAreNotUserWritable(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	first := enableAndRun(t, s, now)
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(1,'user','New activity',$1)`, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := s.Tick(ctx, fixtureComplete(t, &calls), now.Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var actionID, reportID int64
	if err := db.QueryRow(`SELECT id,report_id FROM app_growth_insight_actions WHERE report_id<>$1`, first.ReportID).Scan(&actionID, &reportID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FeedbackAction(ctx, 1, actionID, Feedback{Status: "attempted", Outcome: "unknown"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpublished action writable: %v", err)
	}
	if err := s.Correct(ctx, 1, reportID, "inaccurate", "Not yet published"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpublished report writable: %v", err)
	}
}

func TestOptoutAndSourceDeletionDoNotResetAnalysisCadence(t *testing.T) {
	for _, mode := range []string{"optout", "source_delete"} {
		t.Run(mode, func(t *testing.T) {
			s, db, now := fixture(t)
			ctx := context.Background()
			enableAndRun(t, s, now)
			if mode == "optout" {
				if _, err := s.SetConsent(ctx, 1, false); err != nil {
					t.Fatal(err)
				}
				if _, err := s.SetConsent(ctx, 1, true); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`DELETE FROM app_chat_messages WHERE id=1; INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(1,'user','Replacement evidence','2026-10-09T03:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			complete := func(context.Context, string) (string, error) {
				calls++
				a := validAnalysis()
				a.Summary.EvidenceRefs = []string{"message:2"}
				a.WeeklyReview = a.Summary
				a.TrendExplanation = a.Summary
				a.Observations = nil
				a.Recommendations = nil
				a.Actions = nil
				b, _ := json.Marshal(a)
				return string(b), nil
			}
			if err := s.Tick(ctx, complete, now.Add(2*time.Hour)); err != nil || calls != 0 {
				t.Fatalf("24h limit bypassed: calls=%d err=%v", calls, err)
			}
			if err := s.Tick(ctx, complete, now.Add(25*time.Hour)); err != nil || calls != 1 {
				t.Fatalf("retained cooldown stuck: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestManualEnqueueReportsNoNewEvidenceWithoutCreatingJob(t *testing.T) {
	s, db, now := fixture(t)
	ctx := context.Background()
	enableAndRun(t, s, now)
	if _, err := db.Exec(`DELETE FROM app_growth_insight_jobs WHERE app_user_id=1`); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueue(ctx, 1, now.Add(time.Hour), true); !errors.Is(err, ErrNoNewEvidence) {
		t.Fatalf("manual unchanged input must report no new evidence: %v", err)
	}
	if err := s.enqueue(ctx, 1, now.Add(time.Hour), false); err != nil {
		t.Fatalf("automatic unchanged input must remain a no-op: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_jobs WHERE app_user_id=1`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unchanged input created job: count=%d err=%v", count, err)
	}
}
