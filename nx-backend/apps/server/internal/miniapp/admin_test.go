package miniapp

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
)

func TestAdminMiniappPaginationValidation(t *testing.T) {
	got, err := NormalizeAdminPagination(0, 0)
	if err != nil || got.Page != 1 || got.PageSize != 20 {
		t.Fatalf("default pagination = %+v, %v", got, err)
	}
	if _, err := NormalizeAdminPagination(-1, 20); err == nil {
		t.Fatal("negative page should be rejected")
	}
	if _, err := NormalizeAdminPagination(1, 101); err == nil {
		t.Fatal("pageSize over 100 should be rejected")
	}
}

func TestAdminMiniappQueriesFilterBeforeMaskingAndReturnPagedDetails(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run miniapp admin integration test")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	for _, statement := range []string{
		`TRUNCATE bookings, test_records, wx_users RESTART IDENTITY CASCADE`,
		`INSERT INTO wx_users (openid,nickname,phone,channel,scene,main_type,member_level) VALUES
		 ('admin-query-a','小芯','13812345678','wechat','qr-home',9,1),
		 ('admin-query-b','访客','13987654321','campaign','poster',2,0)`,
		`INSERT INTO test_records (wx_user_id,gender,result_type,second_type,scores,centers)
		 SELECT id,'female',9,1,'{"9":18}'::jsonb,'[]'::jsonb FROM wx_users WHERE openid='admin-query-a'`,
		`INSERT INTO bookings (wx_user_id,kind,contact_name,phone,status,signup_id)
		 SELECT id,'consult','张三','13812345678','pending',91 FROM wx_users WHERE openid='admin-query-a'`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	store := NewAdminStore(database)
	page, err := store.ListUsers(ctx, AdminListOptions{Page: 1, PageSize: 20, Keyword: "123456", Channel: "wechat"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Phone != "138****5678" {
		t.Fatalf("unexpected filtered page: %+v", page)
	}
	if page.Items[0].OpenID != "" || page.Items[0].UnionID != "" {
		t.Fatalf("wechat identity leaked: %+v", page.Items[0])
	}

	id, err := strconv.ParseInt(page.Items[0].ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetUserDetail(ctx, id, AdminDetailOptions{
		TestPage: 1, TestPageSize: 20, BookingPage: 1, BookingPageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.TestRecords.Total != 1 || detail.Bookings.Total != 1 {
		t.Fatalf("unexpected detail totals: %+v", detail)
	}
	if detail.Bookings.Items[0].SignupID != "91" || detail.Bookings.Items[0].Phone != "138****5678" {
		t.Fatalf("unexpected booking dto: %+v", detail.Bookings.Items[0])
	}
}
