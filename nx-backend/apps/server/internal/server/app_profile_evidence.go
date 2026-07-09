package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (s *Server) recordAppProfileEvidenceAsync(appUserID, cardID int64, sourceType string, sourceID int64, text string) {
	if s == nil || s.db == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.recordAppProfileEvidence(ctx, appUserID, cardID, sourceType, sourceID, text)
	}()
}

func (s *Server) recordAppProfileEvidence(ctx context.Context, appUserID, cardID int64, sourceType string, sourceID int64, text string) {
	if s == nil || s.db == nil || appUserID <= 0 || cardID <= 0 {
		return
	}
	sourceType = strings.TrimSpace(sourceType)
	text = strings.TrimSpace(text)
	if sourceType == "" || len([]rune(text)) < 6 {
		return
	}
	var ownedCardID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM app_user_cards WHERE id=$1 AND app_user_id=$2 AND status='active'`, cardID, appUserID).Scan(&ownedCardID); err != nil || ownedCardID <= 0 {
		return
	}
	if len([]rune(text)) > 240 {
		text = string([]rune(text)[:240])
	}
	roundNo, err := s.currentAppProfileRound(ctx, appUserID, cardID)
	if err != nil || roundNo <= 0 {
		return
	}
	traitScores, typeScores, emotionScores, behaviorScores, confidence := deriveProfileEvidenceScores(sourceType, text)
	traitJSON, _ := json.Marshal(traitScores)
	typeJSON, _ := json.Marshal(typeScores)
	emotionJSON, _ := json.Marshal(emotionScores)
	behaviorJSON, _ := json.Marshal(behaviorScores)
	var source any
	if sourceID > 0 {
		source = sourceID
	} else {
		source = nil
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO app_profile_evidence
			(app_user_id, card_id, round_no, source_type, source_id, evidence_text, trait_scores, type_scores, emotion_scores, behavior_scores, confidence, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11,'active')
	`, appUserID, cardID, roundNo, sourceType, source, text, string(traitJSON), string(typeJSON), string(emotionJSON), string(behaviorJSON), confidence)
}

func (s *Server) currentAppProfileRound(ctx context.Context, appUserID, cardID int64) (int, error) {
	var roundNo int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			(SELECT MAX(round_no)+1 FROM app_reassessment_jobs WHERE app_user_id=$1 AND card_id=$2 AND status IN ('accepted','rejected','expired')),
			(SELECT MAX(round_no) FROM app_reassessment_jobs WHERE app_user_id=$1 AND card_id=$2 AND status IN ('pending','generating','generated')),
			(SELECT MAX(round_no) FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2),
			1
		)
	`, appUserID, cardID).Scan(&roundNo)
	if err == sql.ErrNoRows {
		return 1, nil
	}
	return roundNo, err
}

func deriveProfileEvidenceScores(sourceType, text string) (map[string]float64, map[string]float64, map[string]float64, map[string]float64, float64) {
	traitScores := map[string]float64{}
	typeScores := map[string]float64{}
	emotionScores := map[string]float64{}
	behaviorScores := map[string]float64{}
	confidence := 0.55
	lower := strings.ToLower(text)
	if textContainsAny(lower, "担心", "害怕", "焦虑", "风险", "确认", "不稳定", "出错") {
		traitScores["securitySeeking"] = 0.78
		traitScores["riskPrediction"] = 0.72
		typeScores["6"] = 0.76
		emotionScores["anxiety"] = 0.7
		confidence = 0.72
	}
	if textContainsAny(lower, "关系", "对方", "被喜欢", "连接", "照顾") {
		traitScores["relationshipSensitivity"] = 0.64
		typeScores["2"] = 0.42
		confidence = maxFloat(confidence, 0.66)
	}
	if textContainsAny(lower, "完成", "连续", "每天", "稳定", "打卡") || sourceType == "behavior" {
		behaviorScores["completionConsistency"] = 0.75
		confidence = maxFloat(confidence, 0.62)
	}
	if strings.HasPrefix(sourceType, "voice") {
		emotionScores["voiceSemanticSignal"] = maxFloat(emotionScores["voiceSemanticSignal"], 0.6)
		confidence = maxFloat(confidence, 0.62)
	}
	return traitScores, typeScores, emotionScores, behaviorScores, confidence
}

func textContainsAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
