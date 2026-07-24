package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type failingBookingMessageWriter struct{ err error }

func (f failingBookingMessageWriter) Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error) {
	return false, f.err
}

func TestMiniappBookingServicePostgresIsAtomicAndEnforcesSignupForeignKey(t *testing.T) {
	database := openMiniappBookingPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := NewStore(database)
	leads := signup.NewStore(database)

	t.Run("success commits signup booking and only booking message", func(t *testing.T) {
		userID := insertBookingTestUser(t, database, "booking-success", "小芯")
		service := NewService(dbtx.SQLBeginner{DB: database}, store, businessmessage.Store{}, WithSignupWriter(leads), WithBookingWriter(store))
		request := httptest.NewRequest(http.MethodPost, "/api/miniapp/bookings", nil)

		result, err := service.CreateBooking(ctx, userID, BookingInput{
			Kind: "course", ContactName: "张三", Phone: "138-1234 5678", Intent: "系统课程", Message: "请周末联系",
		}, request)
		if err != nil {
			t.Fatalf("CreateBooking() error = %v", err)
		}

		var signupCount, bookingCount, bookingMessageCount, signupMessageCount int
		var source, storedSignupID, eventKey, businessID, targetPath, content string
		if err := database.QueryRowContext(ctx, `SELECT count(*),min(source_platform) FROM signups WHERE id=$1`, result.Lead.ID).Scan(&signupCount, &source); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT count(*),min(signup_id)::text FROM bookings WHERE id=$1 AND wx_user_id=$2`, result.Booking.ID, userID).Scan(&bookingCount, &storedSignupID); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `
			SELECT count(*),min(event_key),min(business_id),min(target_path),min(content)
			FROM messages WHERE event_key='miniapp.booking.created' AND business_id=$1`, result.Lead.ID).Scan(&bookingMessageCount, &eventKey, &businessID, &targetPath, &content); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE event_key='signup.created' AND business_id=$1`, result.Lead.ID).Scan(&signupMessageCount); err != nil {
			t.Fatal(err)
		}
		if signupCount != 1 || bookingCount != 1 || bookingMessageCount != 1 || signupMessageCount != 0 || source != "miniapp" || storedSignupID != result.Lead.ID {
			t.Fatalf("counts/source/link = signup:%d booking:%d bookingMsg:%d signupMsg:%d source:%q link:%q", signupCount, bookingCount, bookingMessageCount, signupMessageCount, source, storedSignupID)
		}
		wantTarget := "/customer/signups?leadId=" + result.Lead.ID + "&open=detail"
		if eventKey != "miniapp.booking.created" || businessID != result.Lead.ID || targetPath != wantTarget {
			t.Fatalf("message identity = %q/%q/%q, want %q/%q/%q", eventKey, businessID, targetPath, "miniapp.booking.created", result.Lead.ID, wantTarget)
		}
		if strings.Contains(content, "13812345678") || !strings.Contains(content, "138****5678") {
			t.Fatalf("message phone was not masked: %q", content)
		}
	})

	t.Run("message failure rolls back signup and booking", func(t *testing.T) {
		userID := insertBookingTestUser(t, database, "booking-rollback", "回滚用户")
		wantErr := errors.New("message unavailable")
		service := NewService(dbtx.SQLBeginner{DB: database}, store, failingBookingMessageWriter{err: wantErr}, WithSignupWriter(leads), WithBookingWriter(store))

		_, err := service.CreateBooking(ctx, userID, BookingInput{ContactName: "李四", Phone: "13912345678"}, httptest.NewRequest(http.MethodPost, "/api/miniapp/bookings", nil))
		if !errors.Is(err, wantErr) {
			t.Fatalf("CreateBooking() error = %v, want message error", err)
		}
		var signupCount, bookingCount, messageCount int
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM signups WHERE name='李四' AND contact='13912345678'`).Scan(&signupCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM bookings WHERE wx_user_id=$1`, userID).Scan(&bookingCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE event_key IN ('miniapp.booking.created','signup.created') AND content LIKE '%回滚用户%'`).Scan(&messageCount); err != nil {
			t.Fatal(err)
		}
		if signupCount != 0 || bookingCount != 0 || messageCount != 0 {
			t.Fatalf("rollback counts = signup:%d booking:%d message:%d, want 0/0/0", signupCount, bookingCount, messageCount)
		}
	})

	t.Run("booking rejects missing signup foreign key", func(t *testing.T) {
		userID := insertBookingTestUser(t, database, "booking-fk", "外键用户")
		_, err := store.InsertBooking(ctx, database, userID, BookingInput{ContactName: "王五", Phone: "13712345678"}, 987654321)
		if err == nil {
			t.Fatal("InsertBooking() accepted a missing signup foreign key")
		}
	})
}

func insertBookingTestUser(t *testing.T, database *sql.DB, openid, nickname string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`INSERT INTO wx_users (openid,nickname) VALUES ($1,$2) RETURNING id`, openid, nickname).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func openMiniappBookingPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run miniapp booking PostgreSQL integration tests")
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
	schemaName := fmt.Sprintf("miniapp_booking_%d", time.Now().UnixNano())
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
	scopedDSN, err := miniappUserDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err = sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE wx_users (
		  id BIGSERIAL PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT NOT NULL DEFAULT '',
		  nickname TEXT NOT NULL DEFAULT '', avatar TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
		  gender TEXT NOT NULL DEFAULT '', main_type INT NOT NULL DEFAULT 0, member_level INT NOT NULL DEFAULT 0,
		  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE signups (
		  id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL DEFAULT '', contact_type TEXT NOT NULL DEFAULT 'phone',
		  contact TEXT NOT NULL DEFAULT '', interest TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
		  follow_status TEXT NOT NULL DEFAULT 'pending', visitor_id TEXT NOT NULL DEFAULT '', source_path TEXT NOT NULL DEFAULT '',
		  landing_page TEXT NOT NULL DEFAULT '', referrer TEXT NOT NULL DEFAULT '', utm_source TEXT NOT NULL DEFAULT '',
		  utm_medium TEXT NOT NULL DEFAULT '', utm_campaign TEXT NOT NULL DEFAULT '', utm_content TEXT NOT NULL DEFAULT '',
		  utm_term TEXT NOT NULL DEFAULT '', game_result_id BIGINT, ip TEXT NOT NULL DEFAULT '', user_agent TEXT NOT NULL DEFAULT '',
		  source_platform TEXT NOT NULL CHECK (source_platform IN ('website','miniapp')), create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE bookings (
		  id BIGSERIAL PRIMARY KEY, wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
		  kind TEXT NOT NULL DEFAULT 'consult', contact_name TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
		  intent TEXT NOT NULL DEFAULT '', preferred_time TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
		  status TEXT NOT NULL DEFAULT 'pending', signup_id BIGINT NOT NULL REFERENCES signups(id),
		  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE messages (
		  id BIGSERIAL PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL,
		  platform TEXT NOT NULL, event_key TEXT NOT NULL, business_id TEXT NOT NULL,
		  business_type TEXT NOT NULL, target_path TEXT NOT NULL
		);
		CREATE UNIQUE INDEX uq_messages_event_business ON messages(event_key,business_type,business_id);
	`); err != nil {
		t.Fatalf("create miniapp booking fixtures: %v", err)
	}
	return database
}
