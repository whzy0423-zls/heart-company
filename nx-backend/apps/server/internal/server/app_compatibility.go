package server

import (
	"context"
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
	"nine-xing/nx-backend/apps/server/internal/rag"
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
	membershipResourceMetadata
}

// appCompatibilityRouter handles GET/POST /api/app/compatibility and
// GET /api/app/compatibility/:id.
func (s *Server) appCompatibilityRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	if strings.HasSuffix(path, "/ask") {
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.appCompatibilityAsk(w, r, strings.TrimSuffix(path, "/ask"))
		return
	}
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

func (s *Server) appCompatibilityAsk(w http.ResponseWriter, r *http.Request, reportPath string) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := s.ensureMembershipLevel(r.Context(), userInfo.ID, "vip"); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
	}
	idText := strings.Trim(strings.TrimPrefix(reportPath, "/api/app/compatibility/"), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Question string        `json:"question"`
		History  []rag.Message `json:"history"`
		Tier     string        `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		httpx.Fail(w, http.StatusBadRequest, "question required")
		return
	}
	report, err := scanAppCompatibilityReport(s.db.QueryRowContext(r.Context(), `
		SELECT id, app_user_id, card_a_id, card_b_id, card_a_name, card_b_name, card_a_type, card_b_type,
		       summary, highlights, conflict_points, suggestions, is_full,
		       algorithm_version, relation_level, scores, explain_tags, evidence,
		       create_time, update_time
		FROM app_compatibility_reports WHERE id = $1 AND app_user_id = $2
	`, id, userInfo.ID))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	if err := s.ensureCardWritable(r.Context(), userInfo.ID, report.CardAID); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
	}
	if err := s.ensureCardWritable(r.Context(), userInfo.ID, report.CardBID); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
	}
	tier, err := s.appChatTierForUser(r.Context(), userInfo.ID, body.Tier)
	if err != nil {
		failAppChatTier(w, err)
		return
	}
	quotaKey, _, err := s.reserveAppChatQuota(r.Context(), userInfo.ID)
	if err != nil {
		failAppChatQuota(w, err)
		return
	}
	defer s.releaseAppChatQuota(quotaKey)
	generator, timeout := s.chatRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	docs := []rag.Document{{
		ID:    "compatibility-report",
		Title: report.CardAName + " × " + report.CardBName + " 关系合盘",
		Content: strings.Join([]string{
			"关系层级：" + report.RelationLevel,
			"关系总结：" + report.Summary,
			"关系优势：" + strings.Join(report.Highlights, "；"),
			"潜在冲突：" + strings.Join(report.ConflictPoints, "；"),
			"相处建议：" + strings.Join(report.Suggestions, "；"),
		}, "\n"),
	}}
	answer, err := rag.NewService(docs, rag.WithGenerator(generator), rag.WithStrictGeneratorErrors()).Ask(ctx, rag.AskInput{
		History:  body.History,
		Question: strings.TrimSpace(body.Question),
		Tier:     tier,
		ConversationCard: rag.ConversationCard{
			Name:     report.CardBName,
			Relation: "关系合盘",
			MainType: report.CardBType,
			Profile:  report.Summary,
		},
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
		return
	}
	s.commitAppChatQuota(quotaKey)
	httpx.OK(w, askResponse{Answer: answer})
}

func (s *Server) appCompatibilityCreate(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := s.ensureMembershipLevel(r.Context(), userInfo.ID, "vip"); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
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
	if err := s.ensureCardWritable(r.Context(), userInfo.ID, cardAID); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
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
	if err := s.ensureCardWritable(r.Context(), userInfo.ID, cardBID); err != nil {
		if writeMembershipAccessError(w, err) {
			return
		}
	}

	report := buildAppCompatibilityReport(userInfo.ID, cardA, cardB)
	applyAppCompatibilityAccess(&report, membershipResourceMetadataForPlan(
		s.currentAppMembershipPlan(r.Context(), userInfo.ID).PlanLevel,
		"vip",
		"合盘报告已生成，会员状态可影响后续追问",
	))
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
	var reports []appCompatibilityReport
	for rows.Next() {
		report, err := scanAppCompatibilityReport(rows)
		if err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, "query failed")
			return
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}
	rows.Close()

	out := make([]appCompatibilityReport, 0, len(reports))
	for _, report := range reports {
		reportAccess, err := s.compatibilityReportAccess(r.Context(), userInfo.ID, report.CardAID, report.CardBID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "membership access unavailable")
			return
		}
		applyAppCompatibilityAccess(&report, reportAccess)
		out = append(out, report)
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
	access, accessErr := s.compatibilityReportAccess(r.Context(), userInfo.ID, report.CardAID, report.CardBID)
	if accessErr != nil {
		httpx.Fail(w, http.StatusInternalServerError, "membership access unavailable")
		return
	}
	applyAppCompatibilityAccess(&report, access)
	httpx.OK(w, report)
}

// compatibilityReportAccess combines the report's own VIP gate with the
// access state of both source cards. A report can outlive a card-capacity
// downgrade; if either source card is retained read-only, the report must be
// redacted as well so a history endpoint cannot recover the paid analysis.
// Callers may pass zero card IDs when they only need the report-level gate.
func (s *Server) compatibilityReportAccess(ctx context.Context, appUserID, cardAID, cardBID int64) (membershipResourceMetadata, error) {
	plan := s.currentAppMembershipPlan(ctx, appUserID)
	access := membershipResourceMetadataForPlan(
		plan.PlanLevel,
		"vip",
		"历史合盘已保留，请升级后继续使用",
	)
	if (cardAID <= 0 && cardBID <= 0) || s == nil || s.db == nil || s.appUsers == nil {
		return access, nil
	}
	seen := make(map[int64]struct{}, 2)
	lockedRequired := "free"
	lockedReason := "历史合盘关联的人物卡已超出当前会员额度，请升级后继续使用"
	for _, cardID := range []int64{cardAID, cardBID} {
		if cardID <= 0 {
			continue
		}
		if _, ok := seen[cardID]; ok {
			continue
		}
		seen[cardID] = struct{}{}
		state, found, err := s.compatibilityCardResourceAccess(ctx, appUserID, cardID, plan.PlanLevel)
		if err != nil {
			return access, err
		}
		if !found || (state.State != resourceAccessReadOnlyOverLimit && state.State != resourceAccessLockedUpgrade) {
			continue
		}
		required := compatibilityRequiredPlanLevel(state)
		// A stale ledger row must not hide a report after the user has upgraded
		// past the source card's actual requirement. Only retain the lock when
		// the current plan is still below that requirement.
		if membershipLevelRank(plan.PlanLevel) >= membershipLevelRank(required) {
			continue
		}
		if membershipLevelRank(required) > membershipLevelRank(lockedRequired) {
			lockedRequired = required
			if strings.TrimSpace(state.Reason) != "" {
				lockedReason = state.Reason
			}
		}
	}
	if lockedRequired != "free" {
		return membershipResourceMetadataForPlan(plan.PlanLevel, lockedRequired, lockedReason), nil
	}
	return access, nil
}

// compatibilityRequiredPlanLevel preserves the entitlement level recorded for
// the source card. Older rows may be incomplete; a locked row with no usable
// level fails closed at S VIP rather than silently exposing the report.
func compatibilityRequiredPlanLevel(state resourceAccessState) string {
	required := normalizeMembershipLevel(state.RequiredPlanLevel)
	if required == "free" && (state.State == resourceAccessReadOnlyOverLimit || state.State == resourceAccessLockedUpgrade) {
		return "svip"
	}
	return required
}

// compatibilityCardResourceAccess resolves a source card without treating
// primary cards as secondary-card ledger resources. A missing ledger is
// rebuilt lazily for current-card rows; migration-era missing tables and
// partial test drivers intentionally fall back to the report-level gate.
func (s *Server) compatibilityCardResourceAccess(ctx context.Context, appUserID, cardID int64, planLevel string) (resourceAccessState, bool, error) {
	var state resourceAccessState
	var cardType, cardStatus string
	err := s.db.QueryRowContext(ctx, `
		SELECT card_type,status
		FROM app_user_cards
		WHERE id = $1 AND app_user_id = $2`, cardID, appUserID).Scan(&cardType, &cardStatus)
	switch {
	case err == nil:
		if strings.EqualFold(strings.TrimSpace(cardType), "primary") {
			return resourceAccessState{ResourceID: cardID, State: resourceAccessActive, RequiredPlanLevel: "free"}, true, nil
		}
		if !strings.EqualFold(strings.TrimSpace(cardStatus), "active") {
			return state, false, nil
		}
	case errors.Is(err, sql.ErrNoRows):
		return state, false, nil
	case membershipLegacyDriverError(s.db, err):
		return state, false, nil
	default:
		return state, false, err
	}

	state, err = s.cardResourceAccess(ctx, appUserID, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		if _, rebuildErr := s.recomputeCardResourceAccess(ctx, appUserID, planLevel); rebuildErr == nil {
			state, err = s.cardResourceAccess(ctx, appUserID, cardID)
		} else if membershipLegacyDriverError(s.db, rebuildErr) {
			return resourceAccessState{}, false, nil
		} else {
			return resourceAccessState{}, false, rebuildErr
		}
	}
	if errors.Is(err, sql.ErrNoRows) || membershipLegacyDriverError(s.db, err) {
		return resourceAccessState{}, false, nil
	}
	if err != nil {
		return resourceAccessState{}, false, err
	}
	return state, true, nil
}

// applyAppCompatibilityAccess keeps the report identity and short summary
// available in history while withholding the paid analysis for a downgraded
// or otherwise locked resource. The aliases are updated explicitly because
// setAliases derives the full fields from the stored report body.
func applyAppCompatibilityAccess(report *appCompatibilityReport, access membershipResourceMetadata) {
	if report == nil {
		return
	}
	report.membershipResourceMetadata = access
	if !membershipContentLocked(access) {
		return
	}
	report.IsFull = false
	report.IsFullSnake = false
	report.Dynamics = ""
	report.Strengths = ""
	report.Advice = ""
	report.Highlights = []string{}
	report.ConflictPoints = []string{}
	report.ConflictPointsSnake = []string{}
	report.Suggestions = []string{}
	report.Scores = compatibility.Scores{}
	report.ExplainTags = []string{}
	report.ExplainTagsSnake = []string{}
	report.Evidence = []compatibility.Evidence{}
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
