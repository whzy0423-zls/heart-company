package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/auth"
	appdb "nine-xing/nx-backend/apps/server/internal/db"
)

func TestNormalizeAppTrialCreditGrantDefaultsToThreeDays(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := normalizeAppTrialCreditGrantInput(appTrialCreditGrantInput{
		Amount:         20,
		Reason:         "分享活动奖励",
		IdempotencyKey: "grant-42-1",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExpiresAt.Equal(now.Add(72 * time.Hour)) {
		t.Fatalf("expiresAt=%s, want %s", got.ExpiresAt, now.Add(72*time.Hour))
	}
}

func TestNormalizeAppTrialCreditGrantAcceptsCustomExpiry(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	want := now.Add(5 * 24 * time.Hour)
	got, err := normalizeAppTrialCreditGrantInput(appTrialCreditGrantInput{
		Amount:         20,
		Reason:         "线下推广奖励",
		ExpiresAt:      want.Format(time.RFC3339),
		IdempotencyKey: "grant-42-custom",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt=%s, want %s", got.ExpiresAt, want)
	}
}

func TestNormalizeAppTrialCreditGrantValidatesFields(t *testing.T) {
	now := time.Now()
	tests := []appTrialCreditGrantInput{
		{Amount: 0, Reason: "推广", IdempotencyKey: "key"},
		{Amount: 1001, Reason: "推广", IdempotencyKey: "key"},
		{Amount: 10, Reason: " ", IdempotencyKey: "key"},
		{Amount: 10, Reason: "推广", IdempotencyKey: ""},
		{Amount: 10, Reason: "推广", IdempotencyKey: "key", ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339)},
	}
	for _, input := range tests {
		if _, err := normalizeAppTrialCreditGrantInput(input, now); err == nil {
			t.Fatalf("expected validation error for %+v", input)
		}
	}
}

func TestParseAppTrialCreditPath(t *testing.T) {
	userID, grantID, action, err := parseAppTrialCreditPath("/api/app-users/42/trial-chat-credits/7/revoke")
	if err != nil {
		t.Fatal(err)
	}
	if userID != 42 || grantID != 7 || action != "revoke" {
		t.Fatalf("unexpected path result user=%d grant=%d action=%q", userID, grantID, action)
	}

	userID, grantID, action, err = parseAppTrialCreditPath("/api/app-users/42/trial-chat-credits")
	if err != nil || userID != 42 || grantID != 0 || action != "collection" {
		t.Fatalf("unexpected collection result user=%d grant=%d action=%q err=%v", userID, grantID, action, err)
	}
}

func TestAppTrialCreditPermission(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{method: http.MethodGet, want: "Customer:App:List"},
		{method: http.MethodPost, want: "Customer:AppTrialCredit:Grant"},
	}
	for _, tt := range tests {
		got, err := appTrialCreditPermission(tt.method)
		if err != nil {
			t.Fatalf("method %s: %v", tt.method, err)
		}
		if got != tt.want {
			t.Fatalf("method %s permission=%q, want %q", tt.method, got, tt.want)
		}
	}
	if _, err := appTrialCreditPermission(http.MethodPut); err == nil {
		t.Fatal("expected unsupported method error")
	}
}

func TestAdminAppTrialCreditGrantListAndRevokeIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run trial credit admin integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	database, err := appdb.Open(ctx, dsn, "trial-credit-admin", "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var userID, operatorID int64
	phone := fmt.Sprintf("198%08d", time.Now().UnixNano()%100000000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
	if err := database.QueryRowContext(ctx, `SELECT id FROM users WHERE username='trial-credit-admin'`).Scan(&operatorID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: database, auditLogs: auditlog.NewStore(database)}
	payload := map[string]any{
		"amount":         20,
		"reason":         "分享活动奖励",
		"idempotencyKey": fmt.Sprintf("admin-grant-%d", userID),
	}
	grantResponse := performAdminTrialCreditRequest(t, s, http.MethodPost, fmt.Sprintf("/api/app-users/%d/trial-chat-credits", userID), payload, operatorID)
	if grantResponse.Code != http.StatusOK {
		t.Fatalf("grant status=%d body=%s", grantResponse.Code, grantResponse.Body.String())
	}
	var grantBody struct {
		Data adminAppTrialCreditGrant `json:"data"`
	}
	if err := json.Unmarshal(grantResponse.Body.Bytes(), &grantBody); err != nil {
		t.Fatal(err)
	}
	if grantBody.Data.Remaining != 20 || time.Until(mustParseTrialCreditTime(t, grantBody.Data.ExpiresAt)) < 71*time.Hour {
		t.Fatalf("unexpected grant: %+v", grantBody.Data)
	}

	duplicateResponse := performAdminTrialCreditRequest(t, s, http.MethodPost, fmt.Sprintf("/api/app-users/%d/trial-chat-credits", userID), payload, operatorID)
	if duplicateResponse.Code != http.StatusOK {
		t.Fatalf("idempotent grant status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	var grantAuditCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM admin_operation_logs
		WHERE action='app_trial_credit.grant' AND target_type='app_user' AND target_id=$1`, fmt.Sprint(userID)).Scan(&grantAuditCount); err != nil {
		t.Fatal(err)
	}
	if grantAuditCount != 1 {
		t.Fatalf("idempotent grant audit count=%d, want 1", grantAuditCount)
	}

	listResponse := performAdminTrialCreditRequest(t, s, http.MethodGet, fmt.Sprintf("/api/app-users/%d/trial-chat-credits", userID), nil, operatorID)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listBody struct {
		Data adminAppTrialCreditList `json:"data"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if listBody.Data.TrialChatRemaining != 20 || len(listBody.Data.Items) != 1 {
		t.Fatalf("unexpected list: %+v", listBody.Data)
	}
	var otherUserID int64
	otherPhone := fmt.Sprintf("196%08d", time.Now().UnixNano()%100000000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, otherPhone).Scan(&otherUserID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, otherUserID)
	crossUserResponse := performAdminTrialCreditRequest(t, s, http.MethodPost, fmt.Sprintf("/api/app-users/%d/trial-chat-credits/%d/revoke", otherUserID, grantBody.Data.ID), nil, operatorID)
	if crossUserResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user revoke status=%d body=%s", crossUserResponse.Code, crossUserResponse.Body.String())
	}

	revokeResponse := performAdminTrialCreditRequest(t, s, http.MethodPost, fmt.Sprintf("/api/app-users/%d/trial-chat-credits/%d/revoke", userID, grantBody.Data.ID), nil, operatorID)
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokeResponse.Code, revokeResponse.Body.String())
	}
	var remaining int
	var status string
	if err := database.QueryRowContext(ctx, `SELECT remaining,status FROM app_chat_trial_credit_grants WHERE id=$1`, grantBody.Data.ID).Scan(&remaining, &status); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 || status != "revoked" {
		t.Fatalf("revoke remaining=%d status=%s", remaining, status)
	}
	var revokeAuditCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM admin_operation_logs
		WHERE action='app_trial_credit.revoke' AND target_type='app_trial_credit' AND target_id=$1`, fmt.Sprint(grantBody.Data.ID)).Scan(&revokeAuditCount); err != nil {
		t.Fatal(err)
	}
	if revokeAuditCount != 1 {
		t.Fatalf("revoke audit count=%d, want 1", revokeAuditCount)
	}
	repeatedRevokeResponse := performAdminTrialCreditRequest(t, s, http.MethodPost, fmt.Sprintf("/api/app-users/%d/trial-chat-credits/%d/revoke", userID, grantBody.Data.ID), nil, operatorID)
	if repeatedRevokeResponse.Code != http.StatusConflict {
		t.Fatalf("repeated revoke status=%d body=%s", repeatedRevokeResponse.Code, repeatedRevokeResponse.Body.String())
	}
}

func performAdminTrialCreditRequest(t *testing.T, s *Server, method, path string, payload any, operatorID int64) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request = request.WithContext(withUser(request.Context(), auth.UserInfo{ID: operatorID, Username: "trial-credit-admin"}))
	response := httptest.NewRecorder()
	s.adminAppTrialCredits(response, request)
	return response
}

func mustParseTrialCreditTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
