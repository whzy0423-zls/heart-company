package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/growthinsight"
	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestGrowthLoopPostgresHTTPPublicationFeedbackAndExpiredPrivacy(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "growth_http")
	initializePortraitTrendMembershipSchema(t, database)
	schema, err := os.ReadFile("../db/growth_insight_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = database.Exec(string(schema)); err != nil {
			t.Fatal(err)
		}
	}
	userID, cardID := seedPortraitTrendMembershipUser(t, database, "vip", "primary", "vip", false)
	store := growthinsight.NewStore(database)
	s := &Server{db: database, quiz: quiz.NewStore(database), growthInsights: store}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(contextWithAppUser(r.Context(), auth.UserInfo{ID: userID}))
		w := httptest.NewRecorder()
		s.appGrowthLoop(w, r)
		return w
	}
	w := request(http.MethodPut, "/api/app/growth-loop/consent", `{"enabled":true}`)
	if w.Code != 200 {
		t.Fatalf("consent %d %s", w.Code, w.Body)
	}
	var sessionID int64
	if err = database.QueryRow(`INSERT INTO app_chat_sessions(app_user_id,card_id,scene) VALUES($1,$2,'chat') RETURNING id`, userID, cardID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ids := []int64{}
	for _, text := range []string{"I felt tense before a meeting", "I tried pausing and felt calmer"} {
		var id int64
		if err = database.QueryRow(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES($1,'user',$2,$3) RETURNING id`, sessionID, text, now.Add(-time.Hour)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	claim := growthinsight.Claim{Text: "You described feeling calmer after pausing", EvidenceRefs: []string{fmt.Sprintf("message:%d", ids[1])}}
	analysis := growthinsight.Analysis{Summary: claim, Observations: []growthinsight.Claim{claim}, Recommendations: []growthinsight.Claim{{Text: "Try a short pause once", EvidenceRefs: claim.EvidenceRefs}}, Actions: []growthinsight.SuggestedAction{{Title: "Pause once", Detail: "Try a brief pause before the next meeting", EvidenceRefs: claim.EvidenceRefs}}}
	calls := 0
	complete := func(context.Context, string) (string, error) {
		calls++
		raw, _ := json.Marshal(analysis)
		return string(raw), nil
	}
	if err = store.Tick(context.Background(), complete, now); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("model calls=%d", calls)
	}
	w = request(http.MethodGet, fmt.Sprintf("/api/app/growth-loop?cardId=%d", cardID), "")
	if w.Code != 200 {
		t.Fatalf("view %d %s", w.Code, w.Body)
	}
	var result struct {
		Data growthinsight.UserView `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	v := result.Data
	if v.Status != "ready" || len(v.Actions) != 1 || v.Summary != claim.Text {
		t.Fatalf("view=%+v", v)
	}
	if strings.Contains(w.Body.String(), "evidenceRefs") || strings.Contains(w.Body.String(), "uncertainties") {
		t.Fatal("internal report fields leaked")
	}
	w = request(http.MethodPut, fmt.Sprintf("/api/app/growth-loop/actions/%d/feedback", v.Actions[0].ID), `{"status":"attempted","outcome":"helpful","note":"It helped me slow down"}`)
	if w.Code != 200 {
		t.Fatalf("feedback %d %s", w.Code, w.Body)
	}
	view, err := store.View(context.Background(), userID, cardID)
	if err != nil || view.Actions[0].Outcome != "helpful" {
		t.Fatalf("feedback did not persist: %+v %v", view, err)
	}
	w = request(http.MethodGet, fmt.Sprintf("/api/app/growth-loop?cardId=%d", cardID), "")
	if w.Code != 200 {
		t.Fatalf("feedback progress view %d %s", w.Code, w.Body)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	progress := result.Data.FeedbackProgress
	if progress == nil || progress.TotalCount != 1 || progress.IncludedCount != 0 || progress.PendingCount != 1 {
		t.Fatalf("saved feedback must remain pending before a new publication: %+v", progress)
	}
	if err = store.Tick(context.Background(), complete, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("24h cadence violated: calls=%d", calls)
	}
	portrait := httptest.NewRecorder()
	s.appCardPortrait(portrait, httptest.NewRequest(http.MethodGet, "/portrait", nil), userID, fmt.Sprint(cardID))
	if portrait.Code != 200 || !strings.Contains(portrait.Body.String(), claim.Text) {
		t.Fatalf("legacy portrait not projected: %d %s", portrait.Code, portrait.Body)
	}
	if _, err = database.Exec(`UPDATE app_users SET member_level='free',member_expires_at=NULL WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	w = request(http.MethodGet, fmt.Sprintf("/api/app/growth-loop?cardId=%d", cardID), "")
	if w.Code != 403 || strings.Contains(w.Body.String(), claim.Text) || strings.Contains(w.Body.String(), "feedbackProgress") {
		t.Fatalf("expired paid view %d %s", w.Code, w.Body)
	}
	w = request(http.MethodGet, "/api/app/growth-loop/consent", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`"reportId":%d`, v.ReportID)) {
		t.Fatalf("expired privacy state %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "feedbackProgress") {
		t.Fatal("paid feedback progress leaked through privacy metadata")
	}
	w = request(http.MethodPost, "/api/app/growth-loop/feedback", fmt.Sprintf(`{"reportId":%d,"kind":"inaccurate","note":"That only happened once"}`, v.ReportID))
	if w.Code != 200 {
		t.Fatalf("expired correction %d %s", w.Code, w.Body)
	}
	w = request(http.MethodPut, "/api/app/growth-loop/consent", `{"enabled":false}`)
	if w.Code != 200 {
		t.Fatalf("expired optout %d %s", w.Code, w.Body)
	}
	var reports int
	if err = database.QueryRow(`SELECT count(*) FROM app_growth_insight_reports WHERE app_user_id=$1`, userID).Scan(&reports); err != nil || reports != 0 {
		t.Fatalf("optout retained reports: %d %v", reports, err)
	}
}
