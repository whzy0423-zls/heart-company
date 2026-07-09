package server_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAppDailyQuizAPICreatesFiveQuestionBatchAndProgress(t *testing.T) {
	handler, _ := newTestServer(t)
	token := appTokenWithPrimaryCard(t, handler, "13910001001")
	cardID := primaryCardID(t, handler, token)
	makePrimaryCardOldEnoughForDailyQuiz(t, cardID)

	today := perform(handler, http.MethodGet, fmt.Sprintf("/api/app/daily-quiz/today?cardId=%d", cardID), token, nil)
	if today.Code != http.StatusOK {
		t.Fatalf("today expected 200, got %d body=%s", today.Code, today.Body.String())
	}
	var todayBody struct {
		Code int `json:"code"`
		Data struct {
			ID            int64  `json:"id"`
			CardID        int64  `json:"cardId"`
			Date          string `json:"date"`
			AnsweredCount int    `json:"answeredCount"`
			Completed     bool   `json:"completed"`
			Questions     []struct {
				ID        int64  `json:"id"`
				Body      string `json:"body"`
				Dimension string `json:"dimension"`
				Options   []struct {
					ID      string `json:"id"`
					Label   string `json:"label"`
					Text    string `json:"text"`
					Weights any    `json:"weights,omitempty"`
				} `json:"options"`
			} `json:"questions"`
			Progress struct {
				Answered      int `json:"answered"`
				Total         int `json:"total"`
				TodayAnswered int `json:"todayAnswered"`
				TodayTotal    int `json:"todayTotal"`
			} `json:"progress"`
		} `json:"data"`
	}
	if err := json.Unmarshal(today.Body.Bytes(), &todayBody); err != nil {
		t.Fatal(err)
	}
	if todayBody.Data.ID <= 0 || todayBody.Data.CardID != cardID || todayBody.Data.Date == "" {
		t.Fatalf("unexpected batch identity: %+v", todayBody.Data)
	}
	if len(todayBody.Data.Questions) != 5 {
		t.Fatalf("expected 5 questions, got %d body=%s", len(todayBody.Data.Questions), today.Body.String())
	}
	if todayBody.Data.Questions[0].Options[0].Weights != nil {
		t.Fatalf("daily quiz response must not expose option weights: %+v", todayBody.Data.Questions[0].Options[0])
	}
	if todayBody.Data.Progress.Total != 100 || todayBody.Data.Progress.TodayTotal != 5 {
		t.Fatalf("unexpected progress defaults: %+v", todayBody.Data.Progress)
	}

	for _, q := range todayBody.Data.Questions {
		if len(q.Options) == 0 {
			t.Fatalf("question %d has no options", q.ID)
		}
		resp := perform(handler, http.MethodPost, "/api/app/daily-quiz/answer", token, map[string]any{
			"batchId":    todayBody.Data.ID,
			"questionId": q.ID,
			"optionId":   q.Options[0].ID,
		})
		if resp.Code != http.StatusOK {
			t.Fatalf("answer question %d failed: %d %s", q.ID, resp.Code, resp.Body.String())
		}
	}
	complete := perform(handler, http.MethodPost, "/api/app/daily-quiz/complete", token, map[string]any{"batchId": todayBody.Data.ID})
	if complete.Code != http.StatusOK {
		t.Fatalf("complete expected 200, got %d body=%s", complete.Code, complete.Body.String())
	}

	progress := perform(handler, http.MethodGet, fmt.Sprintf("/api/app/daily-quiz/progress?cardId=%d", cardID), token, nil)
	if progress.Code != http.StatusOK {
		t.Fatalf("progress expected 200, got %d body=%s", progress.Code, progress.Body.String())
	}
	var progressBody struct {
		Data struct {
			Answered      int `json:"answered"`
			Total         int `json:"total"`
			TodayAnswered int `json:"todayAnswered"`
			TodayTotal    int `json:"todayTotal"`
		} `json:"data"`
	}
	if err := json.Unmarshal(progress.Body.Bytes(), &progressBody); err != nil {
		t.Fatal(err)
	}
	if progressBody.Data.Answered != 5 || progressBody.Data.TodayAnswered != 5 || progressBody.Data.Total != 100 || progressBody.Data.TodayTotal != 5 {
		t.Fatalf("unexpected progress after completion: %+v body=%s", progressBody.Data, progress.Body.String())
	}
}

func TestAppDailyQuizHundredAnswersCreatesReassessmentAndAcceptUpdatesPrimaryCard(t *testing.T) {
	handler, _ := newTestServer(t)
	token := appTokenWithPrimaryCard(t, handler, "13910001002")
	cardID := primaryCardID(t, handler, token)
	makePrimaryCardOldEnoughForDailyQuiz(t, cardID)
	seedDailyQuizCompletedAnswers(t, cardID, 95, 1)

	today := perform(handler, http.MethodGet, fmt.Sprintf("/api/app/daily-quiz/today?cardId=%d", cardID), token, nil)
	if today.Code != http.StatusOK {
		t.Fatalf("today expected 200, got %d body=%s", today.Code, today.Body.String())
	}
	var todayBody struct {
		Data struct {
			ID        int64 `json:"id"`
			Questions []struct {
				ID      int64 `json:"id"`
				Options []struct {
					ID string `json:"id"`
				} `json:"options"`
			} `json:"questions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(today.Body.Bytes(), &todayBody); err != nil {
		t.Fatal(err)
	}
	if len(todayBody.Data.Questions) != 5 {
		t.Fatalf("expected 5 questions, got %d", len(todayBody.Data.Questions))
	}
	for _, q := range todayBody.Data.Questions {
		resp := perform(handler, http.MethodPost, "/api/app/daily-quiz/answer", token, map[string]any{
			"batchId":    todayBody.Data.ID,
			"questionId": q.ID,
			"optionId":   q.Options[0].ID,
		})
		if resp.Code != http.StatusOK {
			t.Fatalf("answer failed: %d %s", resp.Code, resp.Body.String())
		}
	}
	complete := perform(handler, http.MethodPost, "/api/app/daily-quiz/complete", token, map[string]any{"batchId": todayBody.Data.ID})
	if complete.Code != http.StatusOK {
		t.Fatalf("complete failed: %d %s", complete.Code, complete.Body.String())
	}

	latest := perform(handler, http.MethodGet, fmt.Sprintf("/api/app/reassessment/latest?cardId=%d", cardID), token, nil)
	if latest.Code != http.StatusOK {
		t.Fatalf("latest expected 200, got %d body=%s", latest.Code, latest.Body.String())
	}
	var latestBody struct {
		Data struct {
			ID                int64          `json:"id"`
			CardID            int64          `json:"cardId"`
			Status            string         `json:"status"`
			OldMainType       int            `json:"oldMainType"`
			SuggestedMainType int            `json:"suggestedMainType"`
			Confidence        float64        `json:"confidence"`
			Summary           string         `json:"summary"`
			Profile           map[string]any `json:"profile"`
			Reasons           []string       `json:"reasons"`
			Evidence          []struct {
				Kind  string `json:"kind"`
				Label string `json:"label"`
				Text  string `json:"text"`
			} `json:"evidence"`
		} `json:"data"`
	}
	if err := json.Unmarshal(latest.Body.Bytes(), &latestBody); err != nil {
		t.Fatal(err)
	}
	if latestBody.Data.ID <= 0 || latestBody.Data.CardID != cardID || latestBody.Data.Status != "generated" {
		t.Fatalf("unexpected latest report: %+v body=%s", latestBody.Data, latest.Body.String())
	}
	if latestBody.Data.SuggestedMainType <= 0 || latestBody.Data.Confidence <= 0 || latestBody.Data.Summary == "" || len(latestBody.Data.Evidence) == 0 {
		t.Fatalf("report missing required explanation fields: %+v", latestBody.Data)
	}

	detail := perform(handler, http.MethodGet, fmt.Sprintf("/api/app/reassessment/%d", latestBody.Data.ID), token, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail expected 200, got %d body=%s", detail.Code, detail.Body.String())
	}

	accept := perform(handler, http.MethodPost, fmt.Sprintf("/api/app/reassessment/%d/accept", latestBody.Data.ID), token, nil)
	if accept.Code != http.StatusOK {
		t.Fatalf("accept expected 200, got %d body=%s", accept.Code, accept.Body.String())
	}

	primary := perform(handler, http.MethodGet, "/api/app/cards/primary", token, nil)
	if primary.Code != http.StatusOK {
		t.Fatalf("primary expected 200, got %d body=%s", primary.Code, primary.Body.String())
	}
	var primaryBody struct {
		Data struct {
			ID       int64 `json:"id"`
			MainType int   `json:"mainType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(primary.Body.Bytes(), &primaryBody); err != nil {
		t.Fatal(err)
	}
	if primaryBody.Data.ID != cardID || primaryBody.Data.MainType != latestBody.Data.SuggestedMainType {
		t.Fatalf("accept should update primary card to suggested type, primary=%+v report=%+v", primaryBody.Data, latestBody.Data)
	}
}

func TestAppReassessmentRejectKeepsPrimaryCard(t *testing.T) {
	handler, _ := newTestServer(t)
	token := appTokenWithPrimaryCard(t, handler, "13910001003")
	cardID := primaryCardID(t, handler, token)
	originalType := primaryCardMainType(t, handler, token)
	reportID := insertGeneratedReassessment(t, cardID, originalType, differentType(originalType))

	reject := perform(handler, http.MethodPost, fmt.Sprintf("/api/app/reassessment/%d/reject", reportID), token, nil)
	if reject.Code != http.StatusOK {
		t.Fatalf("reject expected 200, got %d body=%s", reject.Code, reject.Body.String())
	}
	if got := primaryCardMainType(t, handler, token); got != originalType {
		t.Fatalf("reject must keep primary card type, got %d want %d", got, originalType)
	}
}

func appTokenWithPrimaryCard(t *testing.T, handler http.Handler, phone string) string {
	t.Helper()
	accessToken, _ := appLogin(t, handler, phone)
	questionsResp := perform(handler, http.MethodGet, "/api/app/quiz/questions", "", nil)
	if questionsResp.Code != http.StatusOK {
		t.Fatalf("questions failed: %d %s", questionsResp.Code, questionsResp.Body.String())
	}
	var questionsBody struct {
		Data []struct {
			ID      int64 `json:"id"`
			Options []struct {
				ID string `json:"id"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.Unmarshal(questionsResp.Body.Bytes(), &questionsBody); err != nil {
		t.Fatal(err)
	}
	answers := make([]map[string]any, 0, len(questionsBody.Data))
	for _, q := range questionsBody.Data {
		if len(q.Options) == 0 {
			continue
		}
		answers = append(answers, map[string]any{"questionId": q.ID, "optionId": q.Options[0].ID})
	}
	if len(answers) == 0 {
		t.Fatal("no quiz answers available")
	}
	submit := perform(handler, http.MethodPost, "/api/app/quiz/submit", accessToken, map[string]any{"answers": answers})
	if submit.Code != http.StatusOK {
		t.Fatalf("quiz submit failed: %d %s", submit.Code, submit.Body.String())
	}
	return accessToken
}

func primaryCardID(t *testing.T, handler http.Handler, token string) int64 {
	t.Helper()
	primary := perform(handler, http.MethodGet, "/api/app/cards/primary", token, nil)
	if primary.Code != http.StatusOK {
		t.Fatalf("primary failed: %d %s", primary.Code, primary.Body.String())
	}
	var body struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(primary.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID <= 0 {
		t.Fatalf("missing primary card id body=%s", primary.Body.String())
	}
	return body.Data.ID
}

func primaryCardMainType(t *testing.T, handler http.Handler, token string) int {
	t.Helper()
	primary := perform(handler, http.MethodGet, "/api/app/cards/primary", token, nil)
	if primary.Code != http.StatusOK {
		t.Fatalf("primary failed: %d %s", primary.Code, primary.Body.String())
	}
	var body struct {
		Data struct {
			MainType int `json:"mainType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(primary.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data.MainType
}

func makePrimaryCardOldEnoughForDailyQuiz(t *testing.T, cardID int64) {
	t.Helper()
	db := openCalibrationTestDB(t)
	_, err := db.Exec(`UPDATE app_user_cards SET create_time = now() - interval '2 day' WHERE id = $1`, cardID)
	if err != nil {
		t.Fatalf("age primary card: %v", err)
	}
}

func seedDailyQuizCompletedAnswers(t *testing.T, cardID int64, count int, roundNo int) {
	t.Helper()
	db := openCalibrationTestDB(t)
	var userID int64
	if err := db.QueryRow(`SELECT app_user_id FROM app_user_cards WHERE id=$1`, cardID).Scan(&userID); err != nil {
		t.Fatalf("query card user: %v", err)
	}
	for i := 0; i < count; i++ {
		date := time.Now().AddDate(0, 0, -30+i/5).Format("2006-01-02")
		var batchID int64
		questionIDs := fmt.Sprintf("[%d]", 100000+i)
		if err := db.QueryRow(`
			INSERT INTO app_daily_quiz_batches (app_user_id, card_id, quiz_date, round_no, question_ids, answered_count, completed, completed_at)
			VALUES ($1,$2,$3,$4,$5::jsonb,1,true,now())
			RETURNING id`, userID, cardID, date, roundNo, questionIDs).Scan(&batchID); err != nil {
			t.Fatalf("insert seed batch: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO app_daily_quiz_answers (batch_id, app_user_id, card_id, round_no, question_id, option_id, type_delta)
			VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, batchID, userID, cardID, roundNo, int64(100000+i), "a", `{"6":1}`); err != nil {
			t.Fatalf("insert seed answer: %v", err)
		}
	}
}

func insertGeneratedReassessment(t *testing.T, cardID int64, oldType, suggestedType int) int64 {
	t.Helper()
	db := openCalibrationTestDB(t)
	var userID int64
	if err := db.QueryRow(`SELECT app_user_id FROM app_user_cards WHERE id=$1`, cardID).Scan(&userID); err != nil {
		t.Fatalf("query card user: %v", err)
	}
	var id int64
	report := fmt.Sprintf(`{"id":0,"cardId":%d,"status":"generated","oldMainType":%d,"suggestedMainType":%d,"confidence":0.8,"summary":"测试报告","profile":{"mainType":%d},"reasons":["测试"],"evidence":[{"kind":"daily_quiz","label":"每日题","text":"测试"}]}`, cardID, oldType, suggestedType, suggestedType)
	if err := db.QueryRow(`
		INSERT INTO app_reassessment_jobs (app_user_id, card_id, round_no, trigger_reason, daily_answer_count, old_main_type, suggested_main_type, confidence, status, report_json)
		VALUES ($1,$2,1,'test',100,$3,$4,0.8,'generated',$5::jsonb)
		RETURNING id`, userID, cardID, oldType, suggestedType, report).Scan(&id); err != nil {
		t.Fatalf("insert reassessment: %v", err)
	}
	return id
}

func differentType(current int) int {
	if current == 6 {
		return 4
	}
	return 6
}

func openCalibrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run server integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
