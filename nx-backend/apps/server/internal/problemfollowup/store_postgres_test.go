package problemfollowup_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/chat"
	pf "nine-xing/nx-backend/apps/server/internal/problemfollowup"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

type fixture struct {
	db                  *sql.DB
	store               *pf.Store
	user, card, session int64
	now                 time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("PROBLEM_FOLLOWUP_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("PROBLEM_FOLLOWUP_POSTGRES_DSN not set")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if h := u.Hostname(); h != "" && h != "localhost" && h != "127.0.0.1" {
		t.Fatal("only local fixture database allowed")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("pf_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	database, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(12)
	t.Cleanup(func() { database.Close(); admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	_, err = database.Exec(`
CREATE TABLE app_users(id BIGSERIAL PRIMARY KEY,status TEXT NOT NULL DEFAULT 'active',member_level TEXT NOT NULL DEFAULT 'svip',member_expires_at TIMESTAMPTZ);
CREATE TABLE app_plans(code TEXT PRIMARY KEY,feature_flags JSONB NOT NULL DEFAULT '{}',plan_level TEXT NOT NULL DEFAULT 'svip',features JSONB NOT NULL DEFAULT '[]',update_time TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO app_plans(code,feature_flags) VALUES('svip','{"problemFollowup":true}');
CREATE TABLE app_user_cards(id BIGSERIAL PRIMARY KEY,app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,card_type TEXT NOT NULL DEFAULT 'primary',status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE app_chat_sessions(id BIGSERIAL PRIMARY KEY,app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,scene TEXT NOT NULL DEFAULT 'chat',updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_chat_messages(id BIGSERIAL PRIMARY KEY,session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,role TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',sources JSONB NOT NULL DEFAULT '[]',message_type TEXT NOT NULL DEFAULT 'text',audio_asset_id BIGINT,audio_duration_ms INT NOT NULL DEFAULT 0,transcript TEXT NOT NULL DEFAULT '',create_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_notifications(id BIGSERIAL PRIMARY KEY,app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,kind TEXT NOT NULL DEFAULT '',title TEXT NOT NULL DEFAULT '',content TEXT NOT NULL DEFAULT '',deep_link TEXT NOT NULL DEFAULT '',source_key TEXT NOT NULL DEFAULT '',create_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE UNIQUE INDEX idx_app_notifications_user_source ON app_notifications(app_user_id,source_key) WHERE source_key<>'';`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	marker := "-- SVIP problem-solving follow-up: durable activity and one-time tasks."
	index := strings.Index(string(raw), marker)
	if index < 0 {
		t.Fatal("missing problem followup schema")
	}
	migration := string(raw[index:])
	if _, err = database.Exec(migration); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(migration); err != nil {
		t.Fatalf("migration not idempotent: %v", err)
	}
	f := &fixture{db: database, store: pf.NewStore(database), now: time.Now().UTC().Truncate(time.Second)}
	if err = database.QueryRow(`INSERT INTO app_users DEFAULT VALUES RETURNING id`).Scan(&f.user); err != nil {
		t.Fatal(err)
	}
	if err = database.QueryRow(`INSERT INTO app_user_cards(app_user_id) VALUES($1) RETURNING id`, f.user).Scan(&f.card); err != nil {
		t.Fatal(err)
	}
	if err = database.QueryRow(`INSERT INTO app_chat_sessions(app_user_id,card_id) VALUES($1,$2) RETURNING id`, f.user, f.card).Scan(&f.session); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) begin(t *testing.T) pf.Turn {
	t.Helper()
	turn, err := f.store.BeginTurn(context.Background(), f.user, f.session, f.now)
	if err != nil || turn.Revision == 0 {
		t.Fatalf("turn=%+v err=%v", turn, err)
	}
	return turn
}
func (f *fixture) enqueue(t *testing.T, turn pf.Turn) int64 {
	t.Helper()
	ctx := pf.WithTurn(context.Background(), turn)
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = pf.LockTurnTx(ctx, tx, f.session); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,'user','和同事有争执，怎么办',$2)`, f.session, f.now); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = tx.QueryRow(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,'assistant','可以先冷静地沟通你的想法。',$2) RETURNING id`, f.session, f.now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err = pf.EnqueueTx(ctx, tx, f.session, id, f.now); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *fixture) classify(t *testing.T) *pf.Job {
	t.Helper()
	job, err := f.store.Claim(context.Background(), f.now)
	if err != nil || job == nil || job.Status != "classifying" {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	input, err := f.store.LoadContext(context.Background(), *job)
	if err != nil || input.Question == "" || input.Answer == "" {
		t.Fatalf("input=%+v err=%v", input, err)
	}
	if err = f.store.SaveDecision(context.Background(), *job, pf.Decision{ShouldFollowUp: true, ProblemSummary: "同事沟通", Message: "之前和同事沟通的困扰，现在有缓解一些吗？"}, f.now); err != nil {
		t.Fatal(err)
	}
	return job
}
func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostgresFollowupWaitsThirtyMinutesAndDeliversOnce(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	f.classify(t)
	ctx := context.Background()
	if job, err := f.store.Claim(ctx, f.now.Add(pf.Delay-time.Second)); err != nil || job != nil {
		t.Fatalf("early job=%+v err=%v", job, err)
	}
	job, err := f.store.Claim(ctx, f.now.Add(pf.Delay))
	if err != nil || job == nil || job.Status != "delivering" {
		t.Fatalf("due job=%+v err=%v", job, err)
	}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			delivery, err := f.store.Deliver(ctx, *job, f.now.Add(pf.Delay))
			if err != nil && !errors.Is(err, pf.ErrStale) {
				t.Errorf("deliver: %v", err)
			}
			if delivery != nil {
				count.Add(1)
				if delivery.DeepLink != fmt.Sprintf("/chat/%d", f.session) {
					t.Errorf("deep link=%s", delivery.DeepLink)
				}
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 || f.count(t, "app_notifications") != 1 || f.count(t, "app_chat_messages") != 3 || f.count(t, "app_problem_followup_jobs") != 1 {
		t.Fatalf("delivery count=%d", count.Load())
	}
	// New worker instance after a restart sees no repeat reminder.
	if next, err := pf.NewStore(f.db).Claim(ctx, f.now.Add(24*time.Hour)); err != nil || next != nil {
		t.Fatalf("repeated job=%+v err=%v", next, err)
	}
}

func TestPostgresReplyCancelsOldTaskAndLateAnswerCannotRequeue(t *testing.T) {
	f := newFixture(t)
	old := f.begin(t)
	f.enqueue(t, old)
	claimed, err := f.store.Claim(context.Background(), f.now)
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	current := f.begin(t)
	if err = f.store.SaveDecision(context.Background(), *claimed, pf.Decision{ShouldFollowUp: true, ProblemSummary: "同事沟通", Message: "之前和同事沟通的困扰，现在有缓解一些吗？"}, f.now); !errors.Is(err, pf.ErrStale) {
		t.Fatalf("old classifier accepted: %v", err)
	}
	f.enqueue(t, old)
	if f.count(t, "app_problem_followup_jobs") != 1 {
		t.Fatal("late answer requeued")
	}
	f.enqueue(t, current)
	f.classify(t)
	job, err := f.store.Claim(context.Background(), f.now.Add(pf.Delay))
	if err != nil || job == nil {
		t.Fatal(err)
	}
	// Another primary session still cancels this user's earlier follow-up.
	var other int64
	if err = f.db.QueryRow(`INSERT INTO app_chat_sessions(app_user_id,card_id) VALUES($1,$2) RETURNING id`, f.user, f.card).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.BeginTurn(context.Background(), f.user, other, f.now); err != nil {
		t.Fatal(err)
	}
	if delivery, err := f.store.Deliver(context.Background(), *job, f.now.Add(pf.Delay)); err != nil && !errors.Is(err, pf.ErrStale) || delivery != nil {
		t.Fatalf("reply not cancelled: %+v %v", delivery, err)
	}
	if f.count(t, "app_notifications") != 0 {
		t.Fatal("notification survived cancellation")
	}
}

func TestPostgresMembershipAndMainSceneBoundaries(t *testing.T) {
	for _, level := range []string{"free", "vip", "vip_year", "svip_month", "svip_unknown"} {
		t.Run(level, func(t *testing.T) {
			f := newFixture(t)
			f.db.Exec(`UPDATE app_users SET member_level=$1`, level)
			f.enqueue(t, f.begin(t))
			if n := f.count(t, "app_problem_followup_jobs"); n != 0 {
				t.Fatalf("ineligible %s jobs=%d", level, n)
			}
		})
	}
	for _, change := range []string{`UPDATE app_users SET member_expires_at=now()-interval '1 second'`, `UPDATE app_plans SET feature_flags='{"problemFollowup":false}'`} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			f.db.Exec(change)
			f.enqueue(t, f.begin(t))
			if f.count(t, "app_problem_followup_jobs") != 0 {
				t.Fatal("ineligible task queued")
			}
		})
	}
	for _, change := range []string{`UPDATE app_user_cards SET card_type='secondary'`, `UPDATE app_user_cards SET status='deleted'`, `UPDATE app_chat_sessions SET scene='enneagram_1'`, `UPDATE app_chat_sessions SET scene='xinzhili_voice'`, `UPDATE app_users SET status='disabled'`} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			f.db.Exec(change)
			turn, err := f.store.BeginTurn(context.Background(), f.user, f.session, f.now)
			if err != nil || turn.Revision != 0 {
				t.Fatalf("non-main got turn=%+v err=%v", turn, err)
			}
		})
	}
	for _, level := range []string{"svip", "svip_month", "svip_quarter", "svip_year"} {
		t.Run("active_"+level, func(t *testing.T) {
			f := newFixture(t)
			f.db.Exec(`UPDATE app_users SET member_level=$1,member_expires_at=now()+interval '1 day'`, level)
			f.enqueue(t, f.begin(t))
			if f.count(t, "app_problem_followup_jobs") != 1 {
				t.Fatal("valid svip not queued")
			}
		})
	}
}

func TestPostgresDeliveryRechecksEligibilityAndLatestMessage(t *testing.T) {
	for _, change := range []string{`UPDATE app_users SET member_level='vip'`, `UPDATE app_users SET member_expires_at=now()-interval '1 second'`, `UPDATE app_users SET status='disabled'`, `UPDATE app_plans SET feature_flags='{"problemFollowup":false}'`, `UPDATE app_user_cards SET status='deleted'`, `DELETE FROM app_chat_sessions`, `INSERT INTO app_chat_messages(session_id,role,content) SELECT id,'user','我有新问题' FROM app_chat_sessions`} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			f.enqueue(t, f.begin(t))
			f.classify(t)
			job, err := f.store.Claim(context.Background(), f.now.Add(pf.Delay))
			if err != nil || job == nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(change); err != nil {
				t.Fatal(err)
			}
			if delivery, err := f.store.Deliver(context.Background(), *job, f.now.Add(pf.Delay)); err != nil && !errors.Is(err, pf.ErrStale) || delivery != nil {
				t.Fatalf("invalid deliver=%+v err=%v", delivery, err)
			}
			if f.count(t, "app_notifications") != 0 {
				t.Fatal("ineligible notification delivered")
			}
		})
	}
}

func TestPostgresFailedAndNegativeClassificationNeverReminds(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	ctx := context.Background()
	job, err := f.store.Claim(ctx, f.now)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err = f.store.SaveDecision(ctx, *job, pf.Decision{}, f.now); err != nil {
		t.Fatal(err)
	}
	if next, err := f.store.Claim(ctx, f.now.Add(time.Hour)); err != nil || next != nil {
		t.Fatalf("negative repeated: %+v %v", next, err)
	}
	f.enqueue(t, f.begin(t))
	for i := 0; i < pf.MaxAttempts; i++ {
		now := f.now.Add(time.Duration(i) * 10 * time.Minute)
		job, err = f.store.Claim(ctx, now)
		if err != nil || job == nil {
			t.Fatalf("retry %d: %+v %v", i, job, err)
		}
		if err = f.store.Fail(ctx, *job, now); err != nil {
			t.Fatal(err)
		}
	}
	if next, err := f.store.Claim(ctx, f.now.Add(24*time.Hour)); err != nil || next != nil {
		t.Fatalf("unbounded retry: %+v %v", next, err)
	}
	if f.count(t, "app_notifications") != 0 {
		t.Fatal("failed classifier notified")
	}
}

func TestPostgresLeaseRecoveryAndExpiredOwnerCannotCommit(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	ctx := context.Background()
	old, err := f.store.Claim(ctx, f.now)
	if err != nil || old == nil {
		t.Fatal(err)
	}
	if again, err := f.store.Claim(ctx, f.now.Add(time.Second)); err != nil || again != nil {
		t.Fatalf("double claim: %+v %v", again, err)
	}
	newJob, err := pf.NewStore(f.db).Claim(ctx, f.now.Add(pf.Lease+time.Second))
	if err != nil || newJob == nil || newJob.ClaimToken == old.ClaimToken {
		t.Fatalf("reclaim=%+v %v", newJob, err)
	}
	if err = f.store.SaveDecision(ctx, *old, pf.Decision{}, f.now.Add(pf.Lease+time.Second)); !errors.Is(err, pf.ErrStale) {
		t.Fatalf("old lease saved: %v", err)
	}
	if err = f.store.SaveDecision(ctx, *newJob, pf.Decision{}, f.now.Add(pf.Lease+time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresChatTextVoiceHooksAndRollback(t *testing.T) {
	f := newFixture(t)
	ctx := pf.WithTurn(context.Background(), f.begin(t))
	store := chat.NewStore(f.db)
	if _, err := store.SavePair(ctx, f.session, "我与朋友吵架了怎么办", "试着表达自己的感受。", nil); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "app_problem_followup_jobs") != 1 {
		t.Fatal("text hook missing")
	}
	ctx = pf.WithTurn(context.Background(), f.begin(t))
	if _, _, err := store.SaveVoicePair(ctx, f.session, 1, 1200, "工作压力大怎么办", "可以列出任务的优先级。", nil); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "app_problem_followup_jobs") != 2 {
		t.Fatal("voice hook missing")
	}
	if _, err := store.SavePair(context.Background(), f.session, "普通旧接口", "保持兼容", nil); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "app_problem_followup_jobs") != 2 {
		t.Fatal("no-token call unexpectedly queued")
	}
	turn := f.begin(t)
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ctx = pf.WithTurn(context.Background(), turn)
	if err = pf.LockTurnTx(ctx, tx, f.session); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = tx.QueryRow(`INSERT INTO app_chat_messages(session_id,role,content) VALUES($1,'assistant','回滚回答') RETURNING id`, f.session).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err = pf.EnqueueTx(ctx, tx, f.session, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if f.count(t, "app_problem_followup_jobs") != 2 {
		t.Fatal("rolled-back answer queued")
	}
}

func TestPostgresConcurrentClaimAndTerminalLeaseExhaustion(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := f.store.Claim(context.Background(), f.now)
			if err != nil {
				t.Error(err)
			}
			if job != nil {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("claimed %d jobs", claimed.Load())
	}
	for i := 1; i < pf.MaxAttempts; i++ {
		job, err := f.store.Claim(context.Background(), f.now.Add(time.Duration(i)*(pf.Lease+time.Second)))
		if err != nil || job == nil {
			t.Fatalf("lease reclaim %d: %+v %v", i, job, err)
		}
	}
	if job, err := f.store.Claim(context.Background(), f.now.Add(time.Hour)); err != nil || job != nil {
		t.Fatalf("exhausted job: %+v %v", job, err)
	}
	var status string
	if err := f.db.QueryRow(`SELECT status FROM app_problem_followup_jobs`).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func TestPostgresClassifierRechecksEligibilityAndTerminatesStaleWork(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	job, err := f.store.Claim(context.Background(), f.now)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE app_users SET member_level='vip'`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.LoadContext(context.Background(), *job); !errors.Is(err, pf.ErrStale) {
		t.Fatalf("ineligible context=%v", err)
	}
	if job, err = f.store.Claim(context.Background(), f.now.Add(time.Hour)); err != nil || job != nil {
		t.Fatalf("stale job repeated: %+v %v", job, err)
	}
}

func TestPostgresCanonicalFlagMissingDefaultsAndMalformedDenied(t *testing.T) {
	for _, value := range []string{`{}`, `{"problemFollowup":true}`, `{"problemFollowup":false}`, `{"problemFollowup":null}`, `{"problemFollowup":"true"}`, `{"problemFollowup":1}`, `null`, `[]`, `true`, `1`, `"invalid"`} {
		t.Run(value, func(t *testing.T) {
			f := newFixture(t)
			if _, err := f.db.Exec(`UPDATE app_plans SET feature_flags=$1::jsonb`, value); err != nil {
				t.Fatal(err)
			}
			f.enqueue(t, f.begin(t))
			want := 0
			if value == `{}` || value == `{"problemFollowup":true}` {
				want = 1
			}
			if got := f.count(t, "app_problem_followup_jobs"); got != want {
				t.Fatalf("jobs=%d want=%d", got, want)
			}
		})
	}
}

func TestPostgresClassifierDoesNotAttributeStandaloneAnswerToOlderQuestion(t *testing.T) {
	f := newFixture(t)
	turn := f.begin(t)
	ctx := pf.WithTurn(context.Background(), turn)
	// A previous turn's question must not become the source question of a
	// standalone assistant record, even if it is the closest preceding user.
	if _, err := f.db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,'user','旧问题',$2),($1,'assistant','旧回答',$2)`, f.session, f.now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = pf.LockTurnTx(ctx, tx, f.session); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = tx.QueryRow(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,'assistant','独立消息',$2) RETURNING id`, f.session, f.now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err = pf.EnqueueTx(ctx, tx, f.session, id, f.now); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	job, err := f.store.Claim(context.Background(), f.now)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if input, err := f.store.LoadContext(context.Background(), *job); !errors.Is(err, pf.ErrStale) {
		t.Fatalf("standalone answer used old question: %+v err=%v", input, err)
	}
}

func TestPostgresLateOlderAnswerPreservesLatestUserTurnsFollowup(t *testing.T) {
	for _, oldVoice := range []bool{false, true} {
		t.Run(fmt.Sprint(oldVoice), func(t *testing.T) {
			f := newFixture(t)
			old := f.begin(t)
			current := f.begin(t)
			store := chat.NewStore(f.db)
			if _, err := store.SavePair(pf.WithTurn(context.Background(), current), f.session, "最近工作压力大怎么办", "先把任务分出优先级。", nil); err != nil {
				t.Fatal(err)
			}
			if oldVoice {
				if _, _, err := store.SaveVoicePair(pf.WithTurn(context.Background(), old), f.session, 1, 1000, "旧轮关系困扰", "旧轮建议。", nil); err != nil {
					t.Fatal(err)
				}
			} else if _, err := store.SavePair(pf.WithTurn(context.Background(), old), f.session, "旧轮关系困扰", "旧轮建议。", nil); err != nil {
				t.Fatal(err)
			}
			if n := f.count(t, "app_problem_followup_jobs"); n != 1 {
				t.Fatalf("jobs=%d", n)
			}
			job, err := f.store.Claim(context.Background(), time.Now())
			if err != nil || job == nil {
				t.Fatal(err)
			}
			input, err := f.store.LoadContext(context.Background(), *job)
			if err != nil || input.Question != "最近工作压力大怎么办" {
				t.Fatalf("newest user turn lost: %+v %v", input, err)
			}
			if err = f.store.SaveDecision(context.Background(), *job, pf.Decision{ShouldFollowUp: true, ProblemSummary: "工作压力", Message: "之前工作上的压力，现在有缓解一些吗？"}, time.Now()); err != nil {
				t.Fatal(err)
			}
			due := time.Now().Add(pf.Delay + time.Second)
			job, err = f.store.Claim(context.Background(), due)
			if err != nil || job == nil {
				t.Fatal(err)
			}
			if delivery, err := f.store.Deliver(context.Background(), *job, due); err != nil || delivery == nil {
				t.Fatalf("newest turn not delivered: %+v %v", delivery, err)
			}
		})
	}
}

func TestPostgresDeliverySerializesWithAccountDeletion(t *testing.T) {
	f := newFixture(t)
	f.enqueue(t, f.begin(t))
	f.classify(t)
	job, err := f.store.Claim(context.Background(), f.now.Add(pf.Delay))
	if err != nil || job == nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRow(`SELECT id FROM app_users WHERE id=$1 FOR UPDATE`, f.user).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE app_users SET status='deleted' WHERE id=$1`, f.user); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`DELETE FROM app_chat_sessions WHERE app_user_id=$1`, f.user); err != nil {
		t.Fatal(err)
	}
	type result struct {
		delivery *pf.Delivery
		err      error
	}
	done := make(chan result, 1)
	go func() {
		delivery, err := f.store.Deliver(context.Background(), *job, f.now.Add(pf.Delay))
		done <- result{delivery, err}
	}()
	select {
	case got := <-done:
		t.Fatalf("delivery bypassed deletion lock: %+v", got)
	case <-time.After(30 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.delivery != nil || !errors.Is(got.err, pf.ErrStale) {
			t.Fatalf("post deletion result=%+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delivery/deletion lock deadlock")
	}
	if f.count(t, "app_notifications") != 0 {
		t.Fatal("notification recreated after account deletion")
	}
}
