package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/compatibility"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

type appCompatibilityRequest struct {
	CardAID      int64 `json:"cardAId"`
	CardBID      int64 `json:"cardBId"`
	CardAIDSnake int64 `json:"card_a_id"`
	CardBIDSnake int64 `json:"card_b_id"`
}

type appCompatibilityReport struct {
	ID                    int64                    `json:"id"`
	AppUserID             int64                    `json:"appUserId"`
	AppUserIDSnake        int64                    `json:"app_user_id"`
	CardAID               int64                    `json:"cardAId"`
	CardAIDSnake          int64                    `json:"card_a_id"`
	CardBID               int64                    `json:"cardBId"`
	CardBIDSnake          int64                    `json:"card_b_id"`
	CardAName             string                   `json:"cardAName"`
	CardANameSnake        string                   `json:"card_a_name"`
	CardBName             string                   `json:"cardBName"`
	CardBNameSnake        string                   `json:"card_b_name"`
	CardAType             int                      `json:"cardAType"`
	CardATypeSnake        int                      `json:"card_a_type"`
	CardBType             int                      `json:"cardBType"`
	CardBTypeSnake        int                      `json:"card_b_type"`
	Title                 string                   `json:"title"`
	Dynamics              string                   `json:"dynamics"`
	Strengths             string                   `json:"strengths"`
	Summary               string                   `json:"summary"`
	Highlights            []string                 `json:"highlights"`
	ConflictPoints        []string                 `json:"conflictPoints"`
	ConflictPointsSnake   []string                 `json:"conflict_points"`
	Advice                string                   `json:"advice"`
	Suggestions           []string                 `json:"suggestions"`
	IsFull                bool                     `json:"isFull"`
	IsFullSnake           bool                     `json:"is_full"`
	AlgorithmVersion      string                   `json:"algorithmVersion"`
	AlgorithmVersionSnake string                   `json:"algorithm_version"`
	RelationLevel         string                   `json:"relationLevel"`
	RelationLevelSnake    string                   `json:"relation_level"`
	Scores                compatibility.Scores     `json:"scores"`
	ExplainTags           []string                 `json:"explainTags"`
	ExplainTagsSnake      []string                 `json:"explain_tags"`
	Evidence              []compatibility.Evidence `json:"evidence"`
	CreatedAt             string                   `json:"createdAt"`
	CreatedAtSnake        string                   `json:"created_at"`
	CreateTime            string                   `json:"createTime"`
	CreateTimeSnake       string                   `json:"create_time"`
	UpdateTime            string                   `json:"updateTime"`
	UpdateTimeSnake       string                   `json:"update_time"`
}

// appCompatibilityRouter handles GET/POST /api/app/compatibility and
// GET /api/app/compatibility/:id.
func (s *Server) appCompatibilityRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "/api/app/compatibility" {
		switch r.Method {
		case http.MethodGet:
			s.appCompatibilityList(w, r)
		case http.MethodPost:
			s.appCompatibilityCreate(w, r)
		default:
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idText := strings.Trim(strings.TrimPrefix(path, "/api/app/compatibility/"), "/")
	s.appCompatibilityDetail(w, r, idText)
}

func (s *Server) appCompatibilityCreate(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var input appCompatibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cardAID := firstPositive(input.CardAID, input.CardAIDSnake)
	cardBID := firstPositive(input.CardBID, input.CardBIDSnake)
	if cardAID <= 0 || cardBID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "card ids required")
		return
	}
	if cardAID == cardBID {
		httpx.Fail(w, http.StatusBadRequest, "two different cards required")
		return
	}

	cardA, err := s.quiz.GetCard(r.Context(), userInfo.ID, cardAID)
	if errors.Is(err, quiz.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	cardB, err := s.quiz.GetCard(r.Context(), userInfo.ID, cardBID)
	if errors.Is(err, quiz.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	report := buildAppCompatibilityReport(userInfo.ID, cardA, cardB)
	highlightsJSON, _ := json.Marshal(report.Highlights)
	conflictsJSON, _ := json.Marshal(report.ConflictPoints)
	suggestionsJSON, _ := json.Marshal(report.Suggestions)
	scoresJSON, _ := json.Marshal(report.Scores)
	tagsJSON, _ := json.Marshal(report.ExplainTags)
	evidenceJSON, _ := json.Marshal(report.Evidence)

	var createTime, updateTime time.Time
	err = s.db.QueryRowContext(r.Context(), `
		INSERT INTO app_compatibility_reports
		  (app_user_id, card_a_id, card_b_id, card_a_name, card_b_name, card_a_type, card_b_type,
		   summary, highlights, conflict_points, suggestions, is_full,
		   algorithm_version, relation_level, scores, explain_tags, evidence)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id, create_time, update_time
	`, userInfo.ID, report.CardAID, report.CardBID, report.CardAName, report.CardBName,
		report.CardAType, report.CardBType, report.Summary, highlightsJSON, conflictsJSON,
		suggestionsJSON, report.IsFull, report.AlgorithmVersion, report.RelationLevel,
		scoresJSON, tagsJSON, evidenceJSON).Scan(&report.ID, &createTime, &updateTime)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "create failed")
		return
	}
	report.setAliasesAndTimes(createTime, updateTime)
	httpx.OK(w, report)
}

func (s *Server) appCompatibilityList(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, app_user_id, card_a_id, card_b_id, card_a_name, card_b_name, card_a_type, card_b_type,
		       summary, highlights, conflict_points, suggestions, is_full,
		       algorithm_version, relation_level, scores, explain_tags, evidence,
		       create_time, update_time
		FROM app_compatibility_reports
		WHERE app_user_id = $1
		ORDER BY create_time DESC, id DESC
	`, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var out []appCompatibilityReport
	for rows.Next() {
		report, err := scanAppCompatibilityReport(rows)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		out = append(out, report)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if out == nil {
		out = []appCompatibilityReport{}
	}
	httpx.OK(w, out)
}

func (s *Server) appCompatibilityDetail(w http.ResponseWriter, r *http.Request, idText string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}

	report, err := scanAppCompatibilityReport(s.db.QueryRowContext(r.Context(), `
		SELECT id, app_user_id, card_a_id, card_b_id, card_a_name, card_b_name, card_a_type, card_b_type,
		       summary, highlights, conflict_points, suggestions, is_full,
		       algorithm_version, relation_level, scores, explain_tags, evidence,
		       create_time, update_time
		FROM app_compatibility_reports
		WHERE id = $1 AND app_user_id = $2
	`, id, userInfo.ID))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	httpx.OK(w, report)
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func buildAppCompatibilityReport(userID int64, cardA, cardB quiz.Card) appCompatibilityReport {
	result := compatibility.Analyze(cardA, cardB)
	report := appCompatibilityReport{
		AppUserID:        userID,
		CardAID:          cardA.ID,
		CardBID:          cardB.ID,
		CardAName:        cardA.Name,
		CardBName:        cardB.Name,
		CardAType:        cardA.MainType,
		CardBType:        cardB.MainType,
		Summary:          result.Summary,
		Highlights:       result.Highlights,
		ConflictPoints:   result.ConflictPoints,
		Suggestions:      result.Suggestions,
		IsFull:           true,
		AlgorithmVersion: result.AlgorithmVersion,
		RelationLevel:    string(result.Level),
		Scores:           result.Scores,
		ExplainTags:      result.ExplainTags,
		Evidence:         result.Evidence,
	}
	report.setAliases()
	return report
}

type appCompatibilityScanner interface {
	Scan(dest ...interface{}) error
}

func scanAppCompatibilityReport(row appCompatibilityScanner) (appCompatibilityReport, error) {
	var report appCompatibilityReport
	var highlightsRaw, conflictsRaw, suggestionsRaw, scoresRaw, tagsRaw, evidenceRaw []byte
	var createTime, updateTime time.Time
	err := row.Scan(&report.ID, &report.AppUserID, &report.CardAID, &report.CardBID,
		&report.CardAName, &report.CardBName, &report.CardAType, &report.CardBType,
		&report.Summary, &highlightsRaw, &conflictsRaw, &suggestionsRaw, &report.IsFull,
		&report.AlgorithmVersion, &report.RelationLevel, &scoresRaw, &tagsRaw, &evidenceRaw,
		&createTime, &updateTime)
	if err != nil {
		return report, err
	}
	_ = json.Unmarshal(highlightsRaw, &report.Highlights)
	_ = json.Unmarshal(conflictsRaw, &report.ConflictPoints)
	_ = json.Unmarshal(suggestionsRaw, &report.Suggestions)
	_ = json.Unmarshal(scoresRaw, &report.Scores)
	_ = json.Unmarshal(tagsRaw, &report.ExplainTags)
	_ = json.Unmarshal(evidenceRaw, &report.Evidence)
	report.setAliasesAndTimes(createTime, updateTime)
	return report, nil
}

func (r *appCompatibilityReport) setAliasesAndTimes(createTime, updateTime time.Time) {
	r.setAliases()
	r.CreateTime = formatAppCompatibilityTime(createTime)
	r.CreateTimeSnake = r.CreateTime
	r.CreatedAt = r.CreateTime
	r.CreatedAtSnake = r.CreatedAt
	r.UpdateTime = formatAppCompatibilityTime(updateTime)
	r.UpdateTimeSnake = r.UpdateTime
}

func (r *appCompatibilityReport) setAliases() {
	r.AppUserIDSnake = r.AppUserID
	r.CardAIDSnake = r.CardAID
	r.CardBIDSnake = r.CardBID
	r.CardANameSnake = r.CardAName
	r.CardBNameSnake = r.CardBName
	r.CardATypeSnake = r.CardAType
	r.CardBTypeSnake = r.CardBType
	r.ConflictPointsSnake = r.ConflictPoints
	r.IsFullSnake = r.IsFull
	if r.AlgorithmVersion == "" {
		r.AlgorithmVersion = "v1"
	}
	r.AlgorithmVersionSnake = r.AlgorithmVersion
	r.RelationLevelSnake = r.RelationLevel
	r.ExplainTagsSnake = r.ExplainTags
	r.Title = r.CardAName + " 与 " + r.CardBName + " 的关系合盘"
	r.Dynamics = r.Summary
	r.Strengths = strings.Join(r.Highlights, "\n")
	r.Advice = strings.Join(r.Suggestions, "\n")
}

func formatAppCompatibilityTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}
