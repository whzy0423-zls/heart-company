package miniapp

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMiniappUserServicePostgresConcurrentFirstLoginIsAtomicAndIdempotent(t *testing.T) {
	database := openMiniappUserServicePostgres(t)
	service := NewService(dbtx.SQLBeginner{DB: database}, NewStore(database), businessmessage.Store{})
	const workers = 8
	ids := make(chan int64, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id, err := service.UpsertUser(context.Background(), " concurrent-openid ", " concurrent-unionid ", " launch ", " 1001 ")
			ids <- id
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent UpsertUser() error = %v", err)
		}
	}
	var wantID int64
	for id := range ids {
		if wantID == 0 {
			wantID = id
		}
		if id != wantID {
			t.Fatalf("concurrent ids differ: got %d want %d", id, wantID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var userCount, messageCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM wx_users WHERE openid='concurrent-openid'`).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	var title, content, platform, eventKey, businessID, businessType, targetPath string
	if err := database.QueryRowContext(ctx, `
		SELECT count(*),min(title),min(content),min(platform),min(event_key),min(business_id),min(business_type),min(target_path)
		FROM messages WHERE event_key='miniapp.user.created'`).Scan(
		&messageCount, &title, &content, &platform, &eventKey, &businessID, &businessType, &targetPath,
	); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 || messageCount != 1 {
		t.Fatalf("counts = users:%d messages:%d, want one each", userCount, messageCount)
	}
	if title != "新的小程序用户" || platform != "miniapp" || eventKey != "miniapp.user.created" || businessID != fmt.Sprint(wantID) || businessType != "miniapp-user" {
		t.Fatalf("unexpected event identity: title=%q platform=%q key=%q business=%q/%q", title, platform, eventKey, businessType, businessID)
	}
	wantTarget := "/customer/miniapp-users?userId=" + fmt.Sprint(wantID) + "&open=detail"
	if targetPath != wantTarget {
		t.Fatalf("target path = %q, want %q", targetPath, wantTarget)
	}
	if strings.Contains(content, "concurrent-openid") || strings.Contains(content, "concurrent-unionid") {
		t.Fatalf("message content leaked identity: %q", content)
	}
}

func TestMiniappUserStorePostgresRepeatLoginUpdatesSafeSourceFields(t *testing.T) {
	database := openMiniappUserServicePostgres(t)
	store := NewStore(database)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	id, created, err := store.UpsertByOpenIDWithDBTX(ctx, database, " padded-openid ", " first-unionid ", " first-channel ", " first-scene ")
	if err != nil || !created {
		t.Fatalf("first upsert = id:%d created:%v err:%v", id, created, err)
	}
	oldLogin := time.Unix(100, 0).UTC()
	if _, err := database.ExecContext(ctx, `UPDATE wx_users SET last_login_at=$1 WHERE id=$2`, oldLogin, id); err != nil {
		t.Fatal(err)
	}

	repeatID, created, err := store.UpsertByOpenIDWithDBTX(ctx, database, "padded-openid", " updated-unionid ", "", " updated-scene ")
	if err != nil || created || repeatID != id {
		t.Fatalf("repeat upsert = id:%d created:%v err:%v, want id:%d false nil", repeatID, created, err, id)
	}
	var openid, unionid, channel, scene string
	var lastLogin time.Time
	if err := database.QueryRowContext(ctx, `SELECT openid,unionid,channel,scene,last_login_at FROM wx_users WHERE id=$1`, id).Scan(&openid, &unionid, &channel, &scene, &lastLogin); err != nil {
		t.Fatal(err)
	}
	if openid != "padded-openid" || unionid != "updated-unionid" || channel != "first-channel" || scene != "updated-scene" {
		t.Fatalf("stored source = %q/%q/%q/%q", openid, unionid, channel, scene)
	}
	if !lastLogin.After(oldLogin) {
		t.Fatalf("last_login_at = %v, want after %v", lastLogin, oldLogin)
	}
	user, err := store.GetUser(ctx, id)
	if err != nil || user.ID != fmt.Sprint(id) {
		t.Fatalf("GetUser() = %+v, %v", user, err)
	}
}

func openMiniappUserServicePostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run miniapp user PostgreSQL integration tests")
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
	schemaName := fmt.Sprintf("miniapp_user_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create isolated schema: %v", err)
	}
	var database *sql.DB
	t.Cleanup(func() {
		if database != nil {
			_ = database.Close()
		}
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
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
		  id BIGSERIAL PRIMARY KEY,
		  openid TEXT NOT NULL UNIQUE,
		  unionid TEXT NOT NULL DEFAULT '',
		  nickname TEXT NOT NULL DEFAULT '',
		  avatar TEXT NOT NULL DEFAULT '',
		  phone TEXT NOT NULL DEFAULT '',
		  gender TEXT NOT NULL DEFAULT '',
		  main_type INT NOT NULL DEFAULT 0,
		  member_level INT NOT NULL DEFAULT 0,
		  channel TEXT NOT NULL DEFAULT '',
		  scene TEXT NOT NULL DEFAULT '',
		  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
		  last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE messages (
		  id BIGSERIAL PRIMARY KEY,
		  type TEXT NOT NULL,
		  title TEXT NOT NULL,
		  content TEXT NOT NULL,
		  platform TEXT NOT NULL,
		  event_key TEXT NOT NULL,
		  business_id TEXT NOT NULL,
		  business_type TEXT NOT NULL,
		  target_path TEXT NOT NULL
		);
		CREATE UNIQUE INDEX uq_messages_event_business
		  ON messages(event_key,business_type,business_id);
	`); err != nil {
		t.Fatalf("create miniapp user fixtures: %v", err)
	}
	return database
}

func miniappUserDSNWithSearchPath(dsn, schemaName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
