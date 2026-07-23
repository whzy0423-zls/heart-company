package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPlatformNotificationMigrationContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	ordered := []string{
		"ALTER TABLE signups ADD COLUMN IF NOT EXISTS source_platform TEXT",
		"ALTER TABLE messages ADD COLUMN IF NOT EXISTS platform TEXT",
		"ALTER TABLE messages ADD COLUMN IF NOT EXISTS event_key TEXT",
		"UPDATE signups SET source_platform = 'website'",
		"UPDATE signups s\nSET source_platform = 'miniapp'",
		"UPDATE messages m\nSET platform = CASE",
		"INSERT INTO migration_logs",
		"UPDATE bookings b\nSET signup_id = NULL",
		"DELETE FROM messages duplicate",
		"ADD CONSTRAINT chk_signups_source_platform",
		"ADD CONSTRAINT fk_bookings_signup",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_event_business",
		"ALTER COLUMN source_platform SET NOT NULL",
		"ALTER COLUMN platform SET NOT NULL",
		"ALTER COLUMN event_key SET NOT NULL",
	}
	last := -1
	for _, want := range ordered {
		at := strings.Index(schema, want)
		if at < 0 {
			t.Fatalf("schema is missing %q", want)
		}
		if at <= last {
			t.Fatalf("%q appears out of migration order", want)
		}
		last = at
	}

	bookingsCreate := strings.Index(schema, "CREATE TABLE IF NOT EXISTS bookings")
	migrationStart := strings.Index(schema, ordered[0])
	if bookingsCreate < 0 || migrationStart <= bookingsCreate {
		t.Fatal("platform migration must run after bookings is created")
	}
}

func TestPlatformNotificationMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run platform notification migration integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := database.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset test database: %v", err)
	}
	if _, err := database.ExecContext(ctx, legacyPlatformNotificationSchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	var websiteSignupID, miniappSignupID, orphanSignupID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO signups (name) VALUES ('官网客户') RETURNING id`).Scan(&websiteSignupID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO signups (name) VALUES ('小程序客户') RETURNING id`).Scan(&miniappSignupID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO signups (name) VALUES ('无预约客户') RETURNING id`).Scan(&orphanSignupID); err != nil {
		t.Fatal(err)
	}

	var wxUserID, testRecordID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO wx_users (openid, nickname) VALUES ('migration-openid', '测试用户') RETURNING id`).Scan(&wxUserID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO test_records (wx_user_id, result_type) VALUES ($1, 9) RETURNING id`, wxUserID).Scan(&testRecordID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO bookings (wx_user_id, signup_id, contact_name) VALUES
		  ($1, $2, '有效预约'),
		  ($1, 987654321, '孤儿预约')
	`, wxUserID, miniappSignupID); err != nil {
		t.Fatal(err)
	}

	messageInsert := `INSERT INTO messages (type,title,business_id,business_type,target_path) VALUES ($1,$2,$3,$4,$5)`
	fixtures := [][]any{
		{"signup", "官网报名一", fmt.Sprint(websiteSignupID), "signup", "/message/management?type=signup"},
		{"signup", "官网报名重复", fmt.Sprint(websiteSignupID), "signup", "/message/management?type=signup"},
		{"signup", "小程序预约", fmt.Sprint(miniappSignupID), "signup", ""},
		{"miniapp", "小程序新用户", fmt.Sprint(wxUserID), "miniapp-user", ""},
		{"miniapp", "小程序测评", fmt.Sprint(testRecordID), "miniapp-test-record", ""},
		{"notice", "未知系统消息", "", "", ""},
	}
	for _, fixture := range fixtures {
		if _, err := database.ExecContext(ctx, messageInsert, fixture...); err != nil {
			t.Fatal(err)
		}
	}

	for run := 1; run <= 2; run++ {
		if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
			t.Fatalf("apply schema run %d: %v", run, err)
		}
	}

	assertTextQuery(t, database, `SELECT source_platform FROM signups WHERE id=$1`, "website", websiteSignupID)
	assertTextQuery(t, database, `SELECT source_platform FROM signups WHERE id=$1`, "miniapp", miniappSignupID)
	assertTextQuery(t, database, `SELECT source_platform FROM signups WHERE id=$1`, "website", orphanSignupID)

	assertHistoricalMessage(t, database, "官网报名一", "website", "signup.created", "signup", fmt.Sprint(websiteSignupID), fmt.Sprintf("/customer/signups?leadId=%d&open=detail", websiteSignupID))
	assertHistoricalMessage(t, database, "小程序预约", "miniapp", "miniapp.booking.created", "signup", fmt.Sprint(miniappSignupID), fmt.Sprintf("/customer/signups?leadId=%d&open=detail", miniappSignupID))
	assertHistoricalMessage(t, database, "小程序新用户", "miniapp", "miniapp.user.created", "miniapp-user", fmt.Sprint(wxUserID), fmt.Sprintf("/customer/miniapp-users?userId=%d&open=detail", wxUserID))
	assertHistoricalMessage(t, database, "小程序测评", "miniapp", "miniapp.quiz.submitted", "miniapp-test-record", fmt.Sprint(testRecordID), fmt.Sprintf("/customer/miniapp-users?userId=%d&testRecordId=%d&open=test", wxUserID, testRecordID))

	var unknownID int64
	var unknownPlatform, unknownEventKey, unknownBusinessType, unknownBusinessID string
	if err := database.QueryRowContext(ctx, `SELECT id,platform,event_key,business_type,business_id FROM messages WHERE title='未知系统消息'`).Scan(&unknownID, &unknownPlatform, &unknownEventKey, &unknownBusinessType, &unknownBusinessID); err != nil {
		t.Fatal(err)
	}
	if unknownPlatform != "system" || unknownEventKey != "system.legacy" || unknownBusinessType != "message" || unknownBusinessID != fmt.Sprint(unknownID) {
		t.Fatalf("unknown message backfill = platform:%q event:%q type:%q id:%q, row id %d", unknownPlatform, unknownEventKey, unknownBusinessType, unknownBusinessID, unknownID)
	}

	assertIntQuery(t, database, `SELECT count(*) FROM messages WHERE business_type='signup' AND business_id=$1`, 1, fmt.Sprint(websiteSignupID))
	assertIntQuery(t, database, `SELECT count(*) FROM bookings WHERE contact_name='孤儿预约' AND signup_id IS NULL`, 1)
	assertJSONLogCount(t, database, "platform_notification.orphan_bookings", 1)
	assertJSONLogCount(t, database, "platform_notification.duplicate_messages", 1)

	for _, constraint := range []string{"chk_signups_source_platform", "chk_messages_platform", "fk_bookings_signup"} {
		assertIntQuery(t, database, `SELECT count(*) FROM pg_constraint WHERE conname=$1`, 1, constraint)
	}
	assertTextQuery(t, database, `SELECT confdeltype::text FROM pg_constraint WHERE conname='fk_bookings_signup'`, "n")
	for _, index := range []string{"uq_messages_event_business", "idx_messages_unread_id", "idx_messages_platform_create_time", "idx_signups_source_create_time"} {
		assertIntQuery(t, database, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname=$1`, 1, index)
	}
	assertIntQuery(t, database, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='signups' AND column_name='source_platform' AND is_nullable='NO'`, 1)
	assertIntQuery(t, database, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='messages' AND column_name IN ('platform','event_key') AND is_nullable='NO'`, 2)

	if _, err := database.ExecContext(ctx, `INSERT INTO messages (title) VALUES ('默认系统消息一'), ('默认系统消息二')`); err != nil {
		t.Fatalf("safe message defaults must support legacy insert paths: %v", err)
	}
	assertIntQuery(t, database, `SELECT count(*) FROM messages WHERE title LIKE '默认系统消息%' AND platform='system' AND event_key='system.legacy' AND business_type='message' AND business_id<>''`, 2)

	if _, err := database.ExecContext(ctx, `INSERT INTO signups (source_platform) VALUES ('desktop')`); err == nil {
		t.Fatal("signups source platform CHECK did not reject invalid value")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO messages (platform,event_key,business_type,business_id) VALUES ('desktop','event','type','id')`); err == nil {
		t.Fatal("messages platform CHECK did not reject invalid value")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO bookings (wx_user_id,signup_id) VALUES ($1,987654321)`, wxUserID); err == nil {
		t.Fatal("bookings signup foreign key did not reject missing signup")
	}
}

func assertHistoricalMessage(t *testing.T, database *sql.DB, title, platform, eventKey, businessType, businessID, targetPath string) {
	t.Helper()
	var gotPlatform, gotEventKey, gotBusinessType, gotBusinessID, gotTargetPath string
	if err := database.QueryRow(`SELECT platform,event_key,business_type,business_id,target_path FROM messages WHERE title=$1`, title).Scan(&gotPlatform, &gotEventKey, &gotBusinessType, &gotBusinessID, &gotTargetPath); err != nil {
		t.Fatal(err)
	}
	if gotPlatform != platform || gotEventKey != eventKey || gotBusinessType != businessType || gotBusinessID != businessID || gotTargetPath != targetPath {
		t.Fatalf("message %q = (%q,%q,%q,%q,%q), want (%q,%q,%q,%q,%q)", title, gotPlatform, gotEventKey, gotBusinessType, gotBusinessID, gotTargetPath, platform, eventKey, businessType, businessID, targetPath)
	}
}

func assertTextQuery(t *testing.T, database *sql.DB, query, want string, args ...any) {
	t.Helper()
	var got string
	if err := database.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query result = %q, want %q", got, want)
	}
}

func assertIntQuery(t *testing.T, database *sql.DB, query string, want int, args ...any) {
	t.Helper()
	var got int
	if err := database.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query result = %d, want %d", got, want)
	}
}

func assertJSONLogCount(t *testing.T, database *sql.DB, key string, want int) {
	t.Helper()
	assertIntQuery(t, database, `SELECT (detail->>'count')::int FROM migration_logs WHERE key=$1`, want, key)
}

const legacyPlatformNotificationSchema = `
CREATE TABLE signups (
  id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL DEFAULT '', contact_type TEXT NOT NULL DEFAULT 'phone',
  contact TEXT NOT NULL DEFAULT '', interest TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
  follow_status TEXT NOT NULL DEFAULT 'pending', owner TEXT NOT NULL DEFAULT '', next_follow_time TIMESTAMPTZ,
  follow_note TEXT NOT NULL DEFAULT '', visitor_id TEXT NOT NULL DEFAULT '', source_path TEXT NOT NULL DEFAULT '',
  landing_page TEXT NOT NULL DEFAULT '', referrer TEXT NOT NULL DEFAULT '', utm_source TEXT NOT NULL DEFAULT '',
  utm_medium TEXT NOT NULL DEFAULT '', utm_campaign TEXT NOT NULL DEFAULT '', utm_content TEXT NOT NULL DEFAULT '',
  utm_term TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '', user_agent TEXT NOT NULL DEFAULT '',
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(), create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE messages (
  id BIGSERIAL PRIMARY KEY, type TEXT NOT NULL DEFAULT 'signup', title TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '', business_id TEXT NOT NULL DEFAULT '', business_type TEXT NOT NULL DEFAULT '',
  target_path TEXT NOT NULL DEFAULT '', is_read BOOLEAN NOT NULL DEFAULT false,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE wx_users (
  id BIGSERIAL PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT NOT NULL DEFAULT '', nickname TEXT NOT NULL DEFAULT '',
  avatar TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '', gender TEXT NOT NULL DEFAULT '', main_type INT NOT NULL DEFAULT 0,
  member_level INT NOT NULL DEFAULT 0, channel TEXT NOT NULL DEFAULT '', scene TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(), last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE test_records (
  id BIGSERIAL PRIMARY KEY, wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  gender TEXT NOT NULL DEFAULT '', result_type INT NOT NULL, second_type INT NOT NULL DEFAULT 0,
  scores JSONB NOT NULL DEFAULT '{}'::jsonb, centers JSONB NOT NULL DEFAULT '[]'::jsonb,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE bookings (
  id BIGSERIAL PRIMARY KEY, wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'consult', contact_name TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
  intent TEXT NOT NULL DEFAULT '', preferred_time TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending', signup_id BIGINT, create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
`
