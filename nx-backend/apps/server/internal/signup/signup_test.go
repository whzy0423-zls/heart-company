package signup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	got, ok := normalizePhone(" 138-0000 0000 ")
	if !ok {
		t.Fatal("expected phone to be valid")
	}
	if got != "13800000000" {
		t.Fatalf("expected normalized phone, got %q", got)
	}
}

func TestNormalizePhoneRejectsInvalidPhone(t *testing.T) {
	for _, input := range []string{"", "123456", "23800000000", "1380000000a"} {
		if got, ok := normalizePhone(input); ok {
			t.Fatalf("expected %q to be invalid, got %q", input, got)
		}
	}
}

func TestNormalizeContactAllowsWechatWithoutPhoneValidation(t *testing.T) {
	contactType, contact, err := normalizeContact("wechat", "  wx_11111  ")
	if err != nil {
		t.Fatalf("expected wechat to be valid, got %v", err)
	}
	if contactType != ContactTypeWechat {
		t.Fatalf("expected contact type %q, got %q", ContactTypeWechat, contactType)
	}
	if contact != "wx_11111" {
		t.Fatalf("expected trimmed wechat id, got %q", contact)
	}
}

func TestNormalizeContactValidatesPhoneOnlyForPhoneType(t *testing.T) {
	_, _, err := normalizeContact("phone", "11111")
	if err == nil || err.Error() != "请输入正确的手机号" {
		t.Fatalf("expected phone validation error, got %v", err)
	}
}

func TestNormalizeAttributionKeepsUsefulSourceFields(t *testing.T) {
	input := AttributionInput{
		VisitorID:   " visitor-1 ",
		SourcePath:  "/game?from=share#signup",
		LandingPage: "https://example.com/?utm_source=douyin",
		Referrer:    "https://referrer.example.com/page",
		UTMSource:   " douyin ",
		UTMMedium:   " video ",
		UTMCampaign: " summer ",
		UTMContent:  " card ",
		UTMTerm:     " enneagram ",
	}

	got := normalizeAttribution(input)

	if got.VisitorID != "visitor-1" {
		t.Fatalf("expected visitor id to be trimmed, got %q", got.VisitorID)
	}
	if got.SourcePath != "/game?from=share#signup" || got.UTMSource != "douyin" {
		t.Fatalf("expected source fields to be preserved, got %+v", got)
	}
}

func TestNormalizeAttributionDefaultsSourcePath(t *testing.T) {
	got := normalizeAttribution(AttributionInput{VisitorID: "v1"})
	if got.SourcePath != "/" {
		t.Fatalf("expected empty source path to default to '/', got %q", got.SourcePath)
	}
}

func TestLeadInputIgnoresClientSourcePlatform(t *testing.T) {
	var input LeadInput
	if err := json.Unmarshal([]byte(`{"name":"张三","sourcePlatform":"miniapp"}`), &input); err != nil {
		t.Fatalf("decode lead input: %v", err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode lead input: %v", err)
	}
	if string(encoded) == "" || json.Valid(encoded) == false {
		t.Fatalf("unexpected encoded input: %s", encoded)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode encoded input: %v", err)
	}
	if _, ok := fields["sourcePlatform"]; ok {
		t.Fatal("LeadInput must not expose sourcePlatform as a client-writable field")
	}
}

func TestSignupSourceCreateWithDBTXWritesTrustedPlatform(t *testing.T) {
	var gotQuery string
	var gotArgs []driver.NamedValue
	database := openSignupScriptDB(t, func(query string, args []driver.NamedValue) (driver.Rows, error) {
		gotQuery = query
		gotArgs = append([]driver.NamedValue(nil), args...)
		return &signupRows{
			columns: []string{"id", "create_time"},
			values:  [][]driver.Value{{int64(42), time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)}},
		}, nil
	})
	request := httptest.NewRequest("POST", "/api/public/signups", nil)

	lead, err := NewStore(database).CreateWithDBTX(context.Background(), database, LeadInput{
		Name: "张三", ContactType: ContactTypePhone, Contact: "13812345678",
	}, request, "website")

	if err != nil {
		t.Fatalf("CreateWithDBTX() error = %v", err)
	}
	if lead.SourcePlatform != "website" {
		t.Fatalf("lead source platform = %q, want website", lead.SourcePlatform)
	}
	if !strings.Contains(gotQuery, "source_platform") {
		t.Fatalf("insert must explicitly write source_platform: %s", gotQuery)
	}
	if len(gotArgs) != 18 || gotArgs[17].Value != "website" {
		t.Fatalf("insert args = %+v, want final website source", gotArgs)
	}
}

func TestSignupSourceCreateWithDBTXRejectsInvalidPlatformAndNilTarget(t *testing.T) {
	store := &Store{}
	request := httptest.NewRequest("POST", "/api/public/signups", nil)
	if _, err := store.CreateWithDBTX(context.Background(), nil, LeadInput{}, request, "website"); err == nil {
		t.Fatal("expected nil query target error")
	}
	database := openSignupScriptDB(t, func(string, []driver.NamedValue) (driver.Rows, error) {
		return nil, errors.New("query must not run")
	})
	if _, err := store.CreateWithDBTX(context.Background(), database, LeadInput{}, request, "client-value"); err == nil || !strings.Contains(err.Error(), "invalid source platform") {
		t.Fatalf("expected invalid platform error, got %v", err)
	}
}

func TestSignupSourceListScansSourcePlatform(t *testing.T) {
	database := openSignupScriptDB(t, signupReadScript("website"))

	result, err := NewStore(database).List(context.Background(), map[string]string{})

	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].SourcePlatform != "website" {
		t.Fatalf("List() items = %+v, want website source", result.Items)
	}
}

func TestSignupSourceDetailScansSourcePlatform(t *testing.T) {
	database := openSignupScriptDB(t, signupReadScript("miniapp"))

	result, err := NewStore(database).Detail(context.Background(), "42")

	if err != nil {
		t.Fatalf("Detail() error = %v", err)
	}
	if result.Lead.SourcePlatform != "miniapp" {
		t.Fatalf("Detail() source platform = %q, want miniapp", result.Lead.SourcePlatform)
	}
}

func TestSignupSourceFollowScansSourcePlatform(t *testing.T) {
	database := openSignupScriptDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "SELECT follow_status"):
			return &signupRows{columns: []string{"follow_status"}, values: [][]driver.Value{{"pending"}}}, nil
		case strings.Contains(query, "UPDATE signups"):
			return &signupRows{columns: signupLeadColumns(), values: [][]driver.Value{signupLeadValues("miniapp")}}, nil
		case strings.Contains(query, "INSERT INTO signup_followups"):
			return &signupRows{}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	})

	lead, err := NewStore(database).Follow(context.Background(), "42", FollowInput{Status: "contacted"}, "admin")

	if err != nil {
		t.Fatalf("Follow() error = %v", err)
	}
	if lead.SourcePlatform != "miniapp" {
		t.Fatalf("Follow() source platform = %q, want miniapp", lead.SourcePlatform)
	}
}

var signupScriptDriverID atomic.Int64

type signupScript func(string, []driver.NamedValue) (driver.Rows, error)

type signupDriver struct{ script signupScript }

func (d signupDriver) Open(string) (driver.Conn, error) { return signupConn{script: d.script}, nil }

type signupConn struct{ script signupScript }

func (c signupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (c signupConn) Close() error              { return nil }
func (c signupConn) Begin() (driver.Tx, error) { return signupTx{}, nil }
func (c signupConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.script(query, args)
}

func (c signupConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	rows, err := c.script(query, args)
	if err != nil {
		return nil, err
	}
	if rows != nil {
		_ = rows.Close()
	}
	return driver.RowsAffected(1), nil
}

type signupTx struct{}

func (signupTx) Commit() error   { return nil }
func (signupTx) Rollback() error { return nil }

type signupRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *signupRows) Columns() []string { return r.columns }
func (r *signupRows) Close() error      { return nil }
func (r *signupRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func openSignupScriptDB(t *testing.T, script signupScript) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("signup-script-%d", signupScriptDriverID.Add(1))
	sql.Register(name, signupDriver{script: script})
	database, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open scripted database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func signupReadScript(sourcePlatform string) signupScript {
	return func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "SELECT count(*)"):
			return &signupRows{columns: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
		case strings.Contains(query, "FROM signups WHERE"):
			return &signupRows{
				columns: signupLeadColumns(),
				values:  [][]driver.Value{signupLeadValues(sourcePlatform)},
			}, nil
		case strings.Contains(query, "FROM signup_followups"):
			return &signupRows{columns: []string{"status", "owner", "content", "next_follow_time", "operator", "create_time"}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	}
}

func signupLeadColumns() []string {
	return []string{
		"id", "name", "contact_type", "contact", "interest", "message", "follow_status", "owner", "follow_note", "next_follow_time",
		"visitor_id", "source_path", "source_platform", "landing_page", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term",
		"game_result_id", "ip", "user_agent", "create_time",
	}
}

func signupLeadValues(sourcePlatform string) []driver.Value {
	return []driver.Value{
		int64(42), "张三", "phone", "13812345678", "咨询", "留言", "pending", "", "", nil,
		"", "/", sourcePlatform, "", "", "", "", "", "", "", "", "127.0.0.1", "test-agent",
		time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC),
	}
}
