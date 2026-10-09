package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/growthinsight"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

var growthProviderSmokeCanaries = []string{
	"SMOKE_ASSISTANT_EXCLUDED", "SMOKE_OTHER_USER_EXCLUDED",
	"SMOKE_SECONDARY_EXCLUDED", "SMOKE_SKILL_EXCLUDED",
	"SMOKE_ASSESSMENT_TEMPLATE_EXCLUDED", "SMOKE_PRIOR_REPORT_EXCLUDED",
}

func TestGrowthProviderSmokeErrorCategoriesDoNotExposeDetails(t *testing.T) {
	const secret = "PRIVATE_RESPONSE_OR_CREDENTIAL"
	var malformed any
	jsonErr := json.Unmarshal([]byte(secret), &malformed)
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"http_400", errors.New("管理端大模型请求失败(400): " + secret), "provider_http_400"},
		{"http_429", fmt.Errorf("request wrapper: %w", errors.New("管理端大模型请求失败(429): "+secret)), "provider_http_429"},
		{"http_502", errors.New("管理端大模型请求失败(502): " + secret), "provider_http_502"},
		{"invalid_http", errors.New("管理端大模型请求失败(999): " + secret), "provider_request_failed"},
		{"timeout", &url.Error{Op: "Post", URL: "https://" + secret, Err: context.DeadlineExceeded}, "provider_timeout"},
		{"cancelled", fmt.Errorf("%s: %w", secret, context.Canceled), "provider_cancelled"},
		{"dns", &net.DNSError{Err: secret, Name: secret}, "provider_dns_failed"},
		{"dns_timeout", &net.DNSError{Err: secret, Name: secret, IsTimeout: true}, "provider_dns_timeout"},
		{"empty_openai", errors.New("OpenAI 兼容模型未返回文本"), "provider_empty_text"},
		{"empty_anthropic", errors.New("Anthropic 模型未返回文本"), "provider_empty_text"},
		{"empty_minimax", errors.New("MiniMax 模型未返回文本"), "provider_empty_text"},
		{"json", jsonErr, "provider_invalid_json"},
		{"unknown", errors.New(secret), "provider_request_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := growthProviderSmokeErrorCategory(tc.err); got != tc.want || strings.Contains(got, secret) {
				t.Fatal("provider_error_classification_invalid")
			}
		})
	}
}

var growthProviderSmokeHTTPStatus = regexp.MustCompile(`^管理端大模型请求失败\(([345][0-9]{2})\):`)

// Return only fixed labels and a three-digit HTTP status, never error text,
// request URLs, response bodies or configuration values.
func growthProviderSmokeErrorCategory(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return "provider_dns_timeout"
		}
		return "provider_dns_failed"
	}
	var networkErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()) {
		return "provider_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "provider_cancelled"
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return "provider_invalid_json"
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		text := current.Error()
		if match := growthProviderSmokeHTTPStatus.FindStringSubmatch(text); len(match) == 2 {
			return "provider_http_" + match[1]
		}
		switch text {
		case "OpenAI 兼容模型未返回文本", "Anthropic 模型未返回文本", "MiniMax 模型未返回文本":
			return "provider_empty_text"
		}
	}
	return "provider_request_failed"
}

func TestGrowthLoopRealProviderSmoke(t *testing.T) {
	rawConfig := strings.TrimSpace(os.Getenv("GROWTH_PROVIDER_SMOKE_CONFIG"))
	if rawConfig == "" {
		t.Skip("provider_smoke_not_enabled")
	}
	var cfg modelconfig.AdminModelConfig
	decoder := json.NewDecoder(strings.NewReader(rawConfig))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		t.Fatal("provider_config_invalid")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || strings.TrimSpace(cfg.APIKey) == "" {
		t.Fatal("provider_config_invalid")
	}

	database := growthProviderSmokeDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	store := growthinsight.NewStore(database)
	growthProviderSmokeSeed(t, ctx, database, store, now)

	calls := 0
	category := "generation_failed"
	err := store.Tick(ctx, func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls != 1 {
			category = "provider_call_count_invalid"
			return "", errors.New(category)
		}
		if growthProviderSmokeContainsCanary(prompt) {
			category = "prompt_source_isolation_failed"
			return "", errors.New(category)
		}
		_, evidenceJSON, ok := strings.Cut(prompt, "EVIDENCE_JSON:\n")
		var input struct {
			Evidence []growthinsight.Evidence `json:"evidence"`
			Coverage growthinsight.Coverage   `json:"coverage"`
		}
		if !ok || json.Unmarshal([]byte(evidenceJSON), &input) != nil || !growthProviderSmokeEvidenceValid(input.Evidence, input.Coverage) {
			category = "prompt_evidence_invalid"
			return "", errors.New(category)
		}
		raw, err := callGrowthModelJSON(ctx, cfg, growthInsightSystemPrompt, prompt, 4096)
		if err != nil {
			category = growthProviderSmokeErrorCategory(err)
			return "", errors.New(category)
		}
		if growthProviderSmokeContainsCanary(raw) {
			category = "provider_output_isolation_failed"
			return "", errors.New(category)
		}
		return raw, nil
	}, now)
	if err != nil {
		t.Fatal(category)
	}
	if calls != 1 {
		t.Fatal("provider_call_count_invalid")
	}
	view, err := store.View(ctx, 1, 1)
	if err != nil || !view.Enabled || view.Status != "ready" || view.ReportID == 0 || view.Version != 1 {
		t.Fatal("user_view_not_ready")
	}
	report, err := store.Report(ctx, view.ReportID)
	if err != nil || !report.Published || report.AppUserID != 1 || report.CardID != 1 || report.Version != 1 {
		t.Fatal("stored_report_invalid")
	}
	if report.EvidenceCount != 5 || !growthProviderSmokeEvidenceValid(report.Evidence, report.Coverage) {
		t.Fatal("stored_evidence_invalid")
	}
	growthProviderSmokeAssertClaims(t, report)
	growthProviderSmokeAssertProjection(t, view, report)
	for _, value := range []any{view, report} {
		body, err := json.Marshal(value)
		if err != nil || growthProviderSmokeContainsCanary(string(body)) {
			t.Fatal("stored_source_isolation_failed")
		}
	}
	if _, err := store.View(ctx, 2, 1); !errors.Is(err, growthinsight.ErrNotFound) {
		t.Fatal("cross_user_projection_failed")
	}
	secondary, err := store.View(ctx, 1, 3)
	if err != nil || secondary.ReportID != 0 || secondary.Summary != "" || len(secondary.Actions) != 0 {
		t.Fatal("secondary_projection_failed")
	}
}

// All failures use fixed categories: provider configuration, DSNs and responses
// must never become test output, including setup and cleanup failures.
func growthProviderSmokeDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Fatal("test_database_config_missing")
	}
	cfg, err := testdb.ParseSafeConfig(dsn)
	if err != nil || cfg == nil || strings.EqualFold(cfg.Database, "nx_admin") {
		t.Fatal("test_database_config_rejected")
	}
	cfg.RuntimeParams = map[string]string{"search_path": "pg_catalog", "statement_timeout": "30000"}
	admin := stdlib.OpenDB(*cfg)
	admin.SetMaxOpenConns(1)
	admin.SetMaxIdleConns(1)
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		_ = admin.Close()
		t.Fatal("test_schema_name_failed")
	}
	schema := "growth_provider_smoke_" + hex.EncodeToString(suffix)
	identifier := pgx.Identifier{schema}.Sanitize()
	setupCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(setupCtx, "CREATE SCHEMA "+identifier); err != nil {
		_ = admin.Close()
		t.Fatal("test_schema_create_failed")
	}
	scoped := cfg.Copy()
	scoped.RuntimeParams = map[string]string{"search_path": schema, "timezone": "UTC", "statement_timeout": "30000"}
	database := stdlib.OpenDB(*scoped)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if database.Close() != nil {
			t.Error("test_database_close_failed")
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error("test_schema_cleanup_failed")
		}
		if admin.Close() != nil {
			t.Error("test_admin_connection_close_failed")
		}
	})
	var currentDatabase, currentSchema string
	if err := database.QueryRowContext(setupCtx, `SELECT current_database(),current_schema()`).Scan(&currentDatabase, &currentSchema); err != nil || currentDatabase != cfg.Database || currentSchema != schema {
		t.Fatal("test_database_scope_invalid")
	}
	return database
}

func growthProviderSmokeSeed(t *testing.T, ctx context.Context, database *sql.DB, store *growthinsight.Store, now time.Time) {
	t.Helper()
	const tables = `CREATE TABLE app_users(id BIGINT PRIMARY KEY,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE app_quiz_submissions(id BIGINT PRIMARY KEY,app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE,result JSONB NOT NULL DEFAULT '{}',primary_type INT NOT NULL DEFAULT 4,wing_type INT NOT NULL DEFAULT 0,answers JSONB NOT NULL DEFAULT '[]',create_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_user_cards(id BIGINT PRIMARY KEY,app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE,card_type TEXT NOT NULL DEFAULT 'primary',status TEXT NOT NULL DEFAULT 'active',submission_id BIGINT REFERENCES app_quiz_submissions(id) ON DELETE SET NULL,enneagram INT NOT NULL DEFAULT 0,profile JSONB NOT NULL DEFAULT '{}',revision BIGINT NOT NULL DEFAULT 1,update_time TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE app_chat_sessions(id BIGINT PRIMARY KEY,app_user_id BIGINT REFERENCES app_users(id) ON DELETE CASCADE,card_id BIGINT REFERENCES app_user_cards(id) ON DELETE CASCADE,scene TEXT NOT NULL DEFAULT 'chat',skill_version_id BIGINT);
CREATE TABLE app_chat_messages(id BIGSERIAL PRIMARY KEY,session_id BIGINT REFERENCES app_chat_sessions(id) ON DELETE CASCADE,role TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',transcript TEXT NOT NULL DEFAULT '',create_time TIMESTAMPTZ NOT NULL DEFAULT now());`
	if _, err := database.ExecContext(ctx, tables); err != nil {
		t.Fatal("fixture_tables_failed")
	}
	schemaPath := strings.TrimSpace(os.Getenv("GROWTH_PROVIDER_SMOKE_SCHEMA_PATH"))
	if schemaPath == "" {
		schemaPath = "../db/growth_insight_schema.sql"
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal("growth_schema_read_failed")
	}
	if _, err := database.ExecContext(ctx, string(schema)); err != nil {
		t.Fatal("growth_schema_apply_failed")
	}
	const owners = `INSERT INTO app_users(id) VALUES(1),(2);
INSERT INTO app_quiz_submissions(id,app_user_id,result,answers) VALUES(1,1,'{"templateTrait":"SMOKE_ASSESSMENT_TEMPLATE_EXCLUDED"}','[{"questionId":1,"answer":"I prefer reflecting before answering."}]');
INSERT INTO app_user_cards(id,app_user_id,submission_id) VALUES(1,1,1),(2,2,NULL);
INSERT INTO app_user_cards(id,app_user_id,card_type) VALUES(3,1,'secondary');
INSERT INTO app_chat_sessions(id,app_user_id,card_id) VALUES(1,1,1),(2,2,2),(3,1,3);
INSERT INTO app_chat_sessions(id,app_user_id,card_id,scene,skill_version_id) VALUES(4,1,1,'skill_chat',1);`
	if _, err := database.ExecContext(ctx, owners); err != nil {
		t.Fatal("fixture_owners_failed")
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_quiz_submissions SET create_time=$1`, now.Add(-3*time.Hour)); err != nil {
		t.Fatal("fixture_assessment_failed")
	}
	for _, message := range []struct {
		id, session int
		role, text  string
	}{
		{1, 1, "user", "In a fictional planning meeting, I paused before replying and asked what the other person needed. I felt less rushed."},
		{2, 1, "user", "In another fictional meeting, I tried taking a short break but returned still frustrated. I want to ask for a clear agenda next time."},
		{101, 1, "assistant", "SMOKE_ASSISTANT_EXCLUDED"},
		{102, 2, "user", "SMOKE_OTHER_USER_EXCLUDED"},
		{103, 3, "user", "SMOKE_SECONDARY_EXCLUDED"},
		{104, 4, "user", "SMOKE_SKILL_EXCLUDED"},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO app_chat_messages(id,session_id,role,content,create_time) VALUES($1,$2,$3,$4,$5)`, message.id, message.session, message.role, message.text, now.Add(-2*time.Hour)); err != nil {
			t.Fatal("fixture_messages_failed")
		}
	}
	if enabled, err := store.SetConsent(ctx, 1, true); err != nil || !enabled {
		t.Fatal("fixture_consent_failed")
	}
	// A version-zero fixture supplies feedback foreign keys without generating a
	// preliminary report or calling the provider a second time.
	if _, err := database.ExecContext(ctx, `INSERT INTO app_growth_insight_reports(id,app_user_id,card_id,version,fingerprint,generated_at,source_through,evidence_count,payload) VALUES(1000,1,1,0,'fixture',$1,$1,0,'{"fixture":"SMOKE_PRIOR_REPORT_EXCLUDED"}')`, now.Add(-3*time.Hour)); err != nil {
		t.Fatal("fixture_prior_report_failed")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO app_growth_insight_actions(id,report_id,app_user_id,card_id,title,detail,status,outcome,note,updated_at) VALUES(1001,1000,1,1,'Take a short break','Pause briefly during the next fictional meeting.','attempted','not_helpful','The break did not resolve my frustration.',$1)`, now.Add(-time.Hour)); err != nil {
		t.Fatal("fixture_action_outcome_failed")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO app_growth_insight_corrections(id,app_user_id,card_id,report_id,kind,note,created_at) VALUES(1001,1,1,1000,'missing_context','The earlier pause was one occasion, not a lasting change.',$1)`, now.Add(-time.Hour)); err != nil {
		t.Fatal("fixture_correction_failed")
	}
}

func growthProviderSmokeContainsCanary(value string) bool {
	for _, canary := range growthProviderSmokeCanaries {
		if strings.Contains(value, canary) {
			return true
		}
	}
	return false
}

func growthProviderSmokeEvidenceValid(evidence []growthinsight.Evidence, coverage growthinsight.Coverage) bool {
	want := map[string]string{"assessment:1": "assessment", "message:1": "message", "message:2": "message", "action:1001": "action_outcome", "correction:1001": "correction"}
	if len(evidence) != len(want) || coverage.EligibleCount != 5 || coverage.IncludedCount != 5 || coverage.OmittedCount != 0 || coverage.TruncatedCount != 0 || coverage.Truncated {
		return false
	}
	for _, item := range evidence {
		if kind, ok := want[item.ID]; !ok || kind != item.Kind || strings.TrimSpace(item.Text) == "" {
			return false
		}
		delete(want, item.ID)
	}
	return len(want) == 0
}

func growthProviderSmokeAssertClaims(t *testing.T, report growthinsight.Report) {
	t.Helper()
	refs := make(map[string]time.Time, len(report.Evidence))
	for _, evidence := range report.Evidence {
		refs[evidence.ID] = evidence.OccurredAt
	}
	check := func(claim growthinsight.Claim, optional, weekly bool) {
		if optional && claim.Text == "" && len(claim.EvidenceRefs) == 0 {
			return
		}
		if strings.TrimSpace(claim.Text) == "" || len(claim.EvidenceRefs) == 0 {
			t.Fatal("claim_evidence_missing")
		}
		for _, ref := range claim.EvidenceRefs {
			at, ok := refs[ref]
			if !ok || (weekly && (at.Before(report.PeriodStart) || at.After(report.SourceThrough))) {
				t.Fatal("claim_evidence_reference_invalid")
			}
		}
	}
	a := report.Analysis
	check(a.Summary, false, false)
	check(a.WeeklyReview, true, true)
	check(a.TrendExplanation, true, false)
	for _, section := range [][]growthinsight.Claim{a.Observations, a.Patterns, a.Goals, a.Changes, a.Uncertainties, a.Recommendations, a.Strengths, a.StressPoints, a.AwarenessPrompts} {
		for _, claim := range section {
			check(claim, false, false)
		}
	}
	for _, action := range a.Actions {
		check(growthinsight.Claim{Text: action.Detail, EvidenceRefs: action.EvidenceRefs}, false, false)
	}
}

func growthProviderSmokeAssertProjection(t *testing.T, view growthinsight.UserView, report growthinsight.Report) {
	t.Helper()
	a := report.Analysis
	texts := func(claims []growthinsight.Claim) []string {
		result := make([]string, 0, len(claims))
		for _, claim := range claims {
			result = append(result, claim.Text)
		}
		return result
	}
	if view.Summary != a.Summary.Text || view.WeeklyReview != a.WeeklyReview.Text || view.TrendExplanation != a.TrendExplanation.Text ||
		!reflect.DeepEqual(view.Strengths, texts(a.Strengths)) || !reflect.DeepEqual(view.StressPoints, texts(a.StressPoints)) ||
		!reflect.DeepEqual(view.GrowthAdvice, texts(a.Recommendations)) || !reflect.DeepEqual(view.AwarenessPrompts, texts(a.AwarenessPrompts)) || len(view.Actions) != len(a.Actions) {
		t.Fatal("user_group_projection_invalid")
	}
	for i, action := range view.Actions {
		if action.ReportID != report.ID || action.CardID != 1 || action.Title != a.Actions[i].Title || action.Detail != a.Actions[i].Detail || action.Status != "pending" || action.Outcome != "unknown" {
			t.Fatal("user_action_projection_invalid")
		}
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal("user_projection_serialization_failed")
	}
	for _, field := range []string{"evidence", "evidenceRefs", "uncertainties", "analysis", "observations", "patterns", "goals", "changes", "coverage", "appUserId", "sourceFrom"} {
		if bytes.Contains(body, []byte(`"`+field+`":`)) {
			t.Fatal("internal_projection_field_leaked")
		}
	}
}
