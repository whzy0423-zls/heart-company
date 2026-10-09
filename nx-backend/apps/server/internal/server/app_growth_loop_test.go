package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/growthinsight"
)

type growthLoopFixtureStore struct {
	consent growthinsight.ConsentState
	view    growthinsight.UserView
	err     error
	writes  int
	reads   int
	userID  int64
}

func (f *growthLoopFixtureStore) ConsentMetadata(_ context.Context, userID int64) (growthinsight.ConsentState, error) {
	f.userID = userID
	return f.consent, f.err
}
func (f *growthLoopFixtureStore) SetConsent(_ context.Context, userID int64, enabled bool) (bool, error) {
	f.userID = userID
	f.writes++
	f.consent.Enabled = enabled
	return enabled, f.err
}
func (f *growthLoopFixtureStore) View(_ context.Context, userID, cardID int64) (growthinsight.UserView, error) {
	f.userID = userID
	f.reads++
	return f.view, f.err
}
func (f *growthLoopFixtureStore) FeedbackAction(_ context.Context, userID, actionID int64, feedback growthinsight.Feedback) (growthinsight.Action, error) {
	f.userID = userID
	f.writes++
	return growthinsight.Action{ID: actionID, Status: feedback.Status, Outcome: feedback.Outcome}, f.err
}
func (f *growthLoopFixtureStore) Correct(_ context.Context, userID, reportID int64, kind, note string) error {
	f.userID = userID
	f.writes++
	return f.err
}
func (f *growthLoopFixtureStore) AdminList(context.Context, int64) (growthinsight.AdminList, error) {
	return growthinsight.AdminList{Reports: []growthinsight.ReportSummary{}}, f.err
}
func (f *growthLoopFixtureStore) Report(context.Context, int64) (growthinsight.Report, error) {
	return growthinsight.Report{}, f.err
}
func (f *growthLoopFixtureStore) Enqueue(context.Context, int64) error { f.writes++; return f.err }

func growthRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	return r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: 19}))
}

func TestGrowthLoopConsentWorksWithoutMembershipAndHasNoReportContent(t *testing.T) {
	f := &growthLoopFixtureStore{consent: growthinsight.ConsentState{Enabled: true, ReportID: 8, DisclosureVersion: growthinsight.DisclosureVersion}, view: growthinsight.UserView{Summary: "private"}}
	s := &Server{growthInsights: f}
	w := httptest.NewRecorder()
	s.appGrowthLoop(w, growthRequest(http.MethodGet, "/api/app/growth-loop/consent", ""))
	if w.Code != 200 || strings.Contains(w.Body.String(), "private") || f.reads != 0 || f.userID != 19 {
		t.Fatalf("status=%d body=%s reads=%d", w.Code, w.Body, f.reads)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("privacy response must not be cached")
	}
	w = httptest.NewRecorder()
	s.appGrowthLoop(w, growthRequest(http.MethodPut, "/api/app/growth-loop/consent", `{"enabled":false}`))
	if w.Code != 200 || f.consent.Enabled || f.writes != 1 {
		t.Fatalf("optout status=%d body=%s", w.Code, w.Body)
	}
}

func TestGrowthLoopRejectsMalformedWritesBeforeStore(t *testing.T) {
	for _, body := range []string{`{}`, `{"enabled":"true"}`, `{"enabled":true,"appUserId":7}`, `{"enabled":true} {}`} {
		f := &growthLoopFixtureStore{}
		s := &Server{growthInsights: f}
		w := httptest.NewRecorder()
		s.appGrowthLoop(w, growthRequest(http.MethodPut, "/api/app/growth-loop/consent", body))
		if w.Code != 400 || f.writes != 0 {
			t.Fatalf("%s: status=%d writes=%d", body, w.Code, f.writes)
		}
	}
}

func TestGrowthLoopRequiresAuthenticationAndFailsClosed(t *testing.T) {
	s := &Server{growthInsights: &growthLoopFixtureStore{}}
	w := httptest.NewRecorder()
	s.appGrowthLoop(w, httptest.NewRequest(http.MethodGet, "/api/app/growth-loop/consent", nil))
	if w.Code != 401 {
		t.Fatalf("anonymous status=%d", w.Code)
	}
	w = httptest.NewRecorder()
	s.appGrowthLoop(w, growthRequest(http.MethodGet, "/api/app/growth-loop?cardId=1", ""))
	if w.Code != 503 {
		t.Fatalf("membership unavailable should fail closed, got %d", w.Code)
	}
}

func TestGrowthLoopCorrectionsRemainAvailableAndUseAuthenticatedOwner(t *testing.T) {
	f := &growthLoopFixtureStore{}
	s := &Server{growthInsights: f}
	w := httptest.NewRecorder()
	s.appGrowthLoop(w, growthRequest(http.MethodPost, "/api/app/growth-loop/feedback", `{"reportId":8,"kind":"inaccurate","note":"This does not describe me"}`))
	if w.Code != 200 || f.userID != 19 || f.writes != 1 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	f.err = growthinsight.ErrNotFound
	w = httptest.NewRecorder()
	s.appGrowthLoop(w, growthRequest(http.MethodPost, "/api/app/growth-loop/feedback", `{"reportId":9,"kind":"inaccurate","note":"Correction"}`))
	if w.Code != 404 {
		t.Fatalf("foreign report status=%d", w.Code)
	}
}

func TestGrowthPortraitOverlayPreservesBaselineUntilReadyAndClearsUnsupportedTraits(t *testing.T) {
	p := portraitResp{Summary: "baseline", MainType: 8, Strengths: []string{"type template"}, PotentialDirections: []string{"template direction"}}
	applyGrowthPortrait(&p, growthinsight.UserView{Enabled: true, Status: "pending"})
	if p.Summary != "baseline" {
		t.Fatal("pending erased baseline")
	}
	v := growthinsight.UserView{Enabled: true, Status: "ready", ReportID: 7, Summary: "based on your recent records", GrowthAdvice: []string{"try one small action"}, GeneratedAt: "2026-10-09T00:00:00Z", NextUpdateAt: "2026-10-16T00:00:00Z"}
	applyGrowthPortrait(&p, v)
	if p.Summary != v.Summary || p.MainType != 8 || len(p.Strengths) != 0 || len(p.PotentialDirections) != 0 || p.UpdatedAt != "2026/10/09 00:00:00" {
		t.Fatalf("unexpected overlay %+v", p)
	}
	redactPortraitContent(&p, membershipResourceMetadata{UpgradeRequired: true, State: resourceAccessReadOnlyOverLimit, RequiredPlanLevel: "vip"})
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), v.Summary) || strings.Contains(string(b), v.GrowthAdvice[0]) {
		t.Fatal("locked projection leaked")
	}
}

func TestGrowthWeeklyProjectionOnlyMatchesExplicitShanghaiWeek(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, shanghaiLoc)
	v := growthinsight.UserView{Enabled: true, Status: "ready", PeriodStart: start.UTC().Format(time.RFC3339), PeriodEnd: start.AddDate(0, 0, 7).UTC().Format(time.RFC3339), Timezone: "Asia/Shanghai", WeeklyReview: "This week's review"}
	if growthWeeklyReview(v, start) != v.WeeklyReview {
		t.Fatal("matching week lost review")
	}
	if growthWeeklyReview(v, start.AddDate(0, 0, -7)) != "" {
		t.Fatal("review copied into previous week")
	}
	v.PeriodStart = "invalid"
	if growthWeeklyReview(v, start) != "" {
		t.Fatal("invalid date accepted")
	}
}

func TestGrowthPrivacyExportIsAdditiveAndAllowlisted(t *testing.T) {
	data := growthinsight.ExportData{Consent: growthinsight.ConsentState{Enabled: true}, Reports: []growthinsight.UserView{{Summary: "own projection"}}}
	body, err := json.Marshal(appPrivacyExportResponse{GrowthAnalysis: &data})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"growthAnalysis"`) || !strings.Contains(string(body), "own projection") {
		t.Fatal(string(body))
	}
	for _, key := range []string{"evidenceRefs", "uncertainties", "claim_token"} {
		if strings.Contains(string(body), key) {
			t.Fatalf("export leaked %s", key)
		}
	}
}

func TestGrowthPrivacyPolicyExplainsOptionalProcessingAndOptout(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{}).appPrivacyPolicy(w, httptest.NewRequest(http.MethodGet, "/policy", nil))
	for _, phrase := range []string{"个性化成长分析默认关闭", "具备用户提炼数据权限", "个人分析不会进入公共知识库", "会员过期后仍可关闭"} {
		if !strings.Contains(w.Body.String(), phrase) {
			t.Fatalf("missing disclosure %s", phrase)
		}
	}
}

func TestGrowthAdminDoesNotClaimUnchangedEvidenceWasQueued(t *testing.T) {
	f := &growthLoopFixtureStore{err: growthinsight.ErrNoNewEvidence}
	s := &Server{growthInsights: f}
	w := httptest.NewRecorder()
	s.adminUserReports(w, httptest.NewRequest(http.MethodPost, "/api/app-user-reports", strings.NewReader(`{"appUserId":19}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"queued":false`) || !strings.Contains(w.Body.String(), `"reason":"no_new_evidence"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}
