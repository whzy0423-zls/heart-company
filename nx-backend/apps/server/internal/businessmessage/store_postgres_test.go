package businessmessage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStoreCreatePostgresIsIdempotentNormalizedAndMasked(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run businessmessage PostgreSQL integration test")
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
	schemaName := fmt.Sprintf("businessmessage_%d", time.Now().UnixNano())
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

	scopedDSN, err := businessMessageDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err = sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := database.ExecContext(ctx, `
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
		t.Fatalf("create messages fixture: %v", err)
	}

	event := WebsiteSignupCreated(" 42 ", "张三", "手机", "138-1234-5678")
	event.Type = " signup "
	event.Platform = " website "
	event.EventKey = " signup.created "
	event.BusinessType = " signup "
	event.BusinessID = " 42 "
	event.TargetPath = " /customer/signups?leadId=42&open=detail "
	store := Store{}
	created, err := store.Create(ctx, database, event)
	if err != nil || !created {
		t.Fatalf("first Create() = created:%v error:%v, want true/nil", created, err)
	}
	created, err = store.Create(ctx, database, WebsiteSignupCreated("42", "张三", "手机", "13812345678"))
	if err != nil || created {
		t.Fatalf("duplicate Create() = created:%v error:%v, want false/nil", created, err)
	}

	var count int
	var eventKey, businessType, businessID, content string
	if err := database.QueryRowContext(ctx, `SELECT count(*),min(event_key),min(business_type),min(business_id),min(content) FROM messages`).Scan(&count, &eventKey, &businessType, &businessID, &content); err != nil {
		t.Fatal(err)
	}
	if count != 1 || eventKey != "signup.created" || businessType != "signup" || businessID != "42" {
		t.Fatalf("stored identity = count:%d %q/%q/%q, want one normalized row", count, eventKey, businessType, businessID)
	}
	if content != "张三提交了官网报名，手机：138****5678" {
		t.Fatalf("stored content = %q, want formatted phone masked", content)
	}
}

func businessMessageDSNWithSearchPath(dsn, schemaName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
