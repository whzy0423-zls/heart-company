package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type failingTestRecordMessageWriter struct{ err error }

func (f failingTestRecordMessageWriter) Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error) {
	return false, f.err
}

func TestMiniappTestRecordServicePostgresIsAtomic(t *testing.T) {
	database := openMiniappTestRecordPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("success commits record main type and message", func(t *testing.T) {
		var userID int64
		if err := database.QueryRowContext(ctx, `INSERT INTO wx_users (openid,nickname,main_type) VALUES ('quiz-success','小芯',2) RETURNING id`).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		service := NewService(dbtx.SQLBeginner{DB: database}, NewStore(database), businessmessage.Store{})
		record, err := service.SaveTestRecord(ctx, userID, TestRecordInput{ResultType: 9, SecondType: 1})
		if err != nil {
			t.Fatalf("SaveTestRecord() error = %v", err)
		}

		var recordCount, mainType, messageCount int
		var title, content, platform, eventKey, businessID, businessType, targetPath string
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM test_records WHERE wx_user_id=$1 AND id=$2`, userID, record.ID).Scan(&recordCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT main_type FROM wx_users WHERE id=$1`, userID).Scan(&mainType); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `
			SELECT count(*),min(title),min(content),min(platform),min(event_key),min(business_id),min(business_type),min(target_path)
			FROM messages WHERE event_key='miniapp.quiz.submitted' AND business_id=$1`, record.ID).Scan(
			&messageCount, &title, &content, &platform, &eventKey, &businessID, &businessType, &targetPath,
		); err != nil {
			t.Fatal(err)
		}
		if recordCount != 1 || mainType != 9 || messageCount != 1 {
			t.Fatalf("record/main/message = %d/%d/%d, want 1/9/1", recordCount, mainType, messageCount)
		}
		if title != "新的小程序测评" || platform != "miniapp" || eventKey != "miniapp.quiz.submitted" || businessID != record.ID || businessType != "miniapp-test-record" {
			t.Fatalf("unexpected message identity: %q %q %q %q %q", title, platform, eventKey, businessType, businessID)
		}
		wantTarget := "/customer/miniapp-users?userId=" + fmt.Sprint(userID) + "&testRecordId=" + record.ID + "&open=test"
		if targetPath != wantTarget {
			t.Fatalf("target = %q, want %q", targetPath, wantTarget)
		}
		if strings.Contains(content, "quiz-success") {
			t.Fatalf("message leaked openid: %q", content)
		}
		if !strings.Contains(content, "微信用户"+fmt.Sprint(userID)) || !strings.Contains(content, "提交时间："+record.CreateTime) {
			t.Fatalf("message content missing safe identity or submit time: %q", content)
		}
	})

	t.Run("message failure rolls back record and main type", func(t *testing.T) {
		var userID int64
		if err := database.QueryRowContext(ctx, `INSERT INTO wx_users (openid,nickname,main_type) VALUES ('quiz-rollback','回滚用户',3) RETURNING id`).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		wantErr := errors.New("message unavailable")
		service := NewService(dbtx.SQLBeginner{DB: database}, NewStore(database), failingTestRecordMessageWriter{err: wantErr})
		_, err := service.SaveTestRecord(ctx, userID, TestRecordInput{ResultType: 8})
		if !errors.Is(err, wantErr) {
			t.Fatalf("SaveTestRecord() error = %v, want message error", err)
		}
		var recordCount, mainType, messageCount int
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM test_records WHERE wx_user_id=$1`, userID).Scan(&recordCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT main_type FROM wx_users WHERE id=$1`, userID).Scan(&mainType); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE event_key='miniapp.quiz.submitted' AND target_path LIKE $1`, "%userId="+fmt.Sprint(userID)+"%").Scan(&messageCount); err != nil {
			t.Fatal(err)
		}
		if recordCount != 0 || mainType != 3 || messageCount != 0 {
			t.Fatalf("rollback record/main/message = %d/%d/%d, want 0/3/0", recordCount, mainType, messageCount)
		}
	})
}

func openMiniappTestRecordPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run miniapp test record PostgreSQL integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("miniapp_test_record_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create isolated schema: %v", err)
	}
	var database *sql.DB
	t.Cleanup(func() {
		if database != nil {
			_ = database.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
		_ = adminDB.Close()
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	database, err = sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE wx_users (
		  id BIGSERIAL PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT NOT NULL DEFAULT '',
		  nickname TEXT NOT NULL DEFAULT '', avatar TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
		  gender TEXT NOT NULL DEFAULT '', main_type INT NOT NULL DEFAULT 0, member_level INT NOT NULL DEFAULT 0,
		  channel TEXT NOT NULL DEFAULT '', scene TEXT NOT NULL DEFAULT '', create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
		  last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE test_records (
		  id BIGSERIAL PRIMARY KEY, wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
		  gender TEXT NOT NULL DEFAULT '', result_type INT NOT NULL, second_type INT NOT NULL DEFAULT 0,
		  scores JSONB NOT NULL DEFAULT '{}'::jsonb, centers JSONB NOT NULL DEFAULT '[]'::jsonb,
		  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE messages (
		  id BIGSERIAL PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL,
		  platform TEXT NOT NULL, event_key TEXT NOT NULL, business_id TEXT NOT NULL,
		  business_type TEXT NOT NULL, target_path TEXT NOT NULL
		);
		CREATE UNIQUE INDEX uq_messages_event_business ON messages(event_key,business_type,business_id);
	`); err != nil {
		t.Fatalf("create miniapp test record fixtures: %v", err)
	}
	return database
}
