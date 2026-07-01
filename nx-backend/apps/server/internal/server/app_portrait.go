package server

import (
	"errors"
	"net/http"
	"strconv"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

// portraitResp 成长状态画像的响应体，字段与 App 端 GrowthPortrait 合约一一对应。
type portraitResp struct {
	HasEnoughData        bool     `json:"hasEnoughData"`                  // 数据是否足以生成画像
	Summary              string   `json:"summary"`                        // 一句话状态概述
	StateLabel           string   `json:"stateLabel,omitempty"`           // 当前状态标签
	StressPoints         []string `json:"stressPoints,omitempty"`         // 压力点
	RelationshipPatterns []string `json:"relationshipPatterns,omitempty"` // 关系模式
	GrowthAdvice         []string `json:"growthAdvice,omitempty"`         // 成长建议
	AwarenessPrompts     []string `json:"awarenessPrompts,omitempty"`     // 自我觉察提示
	GuidingQuestions     []string `json:"guidingQuestions,omitempty"`     // 引导问题
	MainType             int      `json:"mainType,omitempty"`             // 主型 id
	UpdatedAt            string   `json:"updatedAt,omitempty"`            // 更新时间，格式 YYYY/MM/DD HH:mm:ss
}

// appCardPortrait 返回指定人物卡的成长状态画像。
// idText 为已剥离 /portrait 后缀的卡片 id 文本。
func (s *Server) appCardPortrait(w http.ResponseWriter, r *http.Request, userID int64, idText string) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}

	card, err := s.quiz.GetCard(r.Context(), userID, id)
	if errors.Is(err, quiz.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query failed")
		return
	}

	resp := buildPortrait(card)
	httpx.OK(w, resp)
}

// buildPortrait 依据人物卡的主型，组装成长状态画像。
// 主型无效（未测评 / 数据不足）时返回空态，由 App 引导用户先完成测评。
func buildPortrait(card quiz.Card) portraitResp {
	meta, ok := quiz.TypeResults[card.MainType]
	if !ok || card.MainType <= 0 {
		return portraitResp{
			HasEnoughData: false,
			Summary:       "暂时还没有足够的资料生成成长画像，先完成一次测评，我就能为你描绘当前的状态了。",
		}
	}

	resp := portraitResp{
		HasEnoughData:        true,
		Summary:              meta.Summary,
		StateLabel:           meta.Title,
		StressPoints:         dedupeNonEmpty(meta.Challenges),
		RelationshipPatterns: relationshipPatterns(card.MainType),
		GrowthAdvice:         growthAdvice(meta),
		AwarenessPrompts:     awarenessPrompts(meta),
		GuidingQuestions:     guidingQuestions(card.MainType),
		MainType:             card.MainType,
		UpdatedAt:            card.UpdateTime,
	}
	return resp
}

// relationshipPatterns 由主型的核心动机推导关系层面的典型模式提示。
func relationshipPatterns(mainType int) []string {
	meta, ok := quiz.TypeResults[mainType]
	if !ok {
		return nil
	}
	out := []string{}
	if meta.Motive != "" {
		out = append(out, "在关系中，你"+meta.Motive+"。")
	}
	// 取主型优势中的人际相关特质，作为关系里的正向模式。
	for _, s := range meta.Strengths {
		out = append(out, "你的「"+s+"」常在亲密与协作关系中显现。")
		break
	}
	return dedupeNonEmpty(out)
}

// growthAdvice 把成长方向拆成可读的建议列表。
func growthAdvice(meta quiz.TypeResult) []string {
	out := []string{}
	if meta.Growth != "" {
		out = append(out, meta.Growth)
	}
	return dedupeNonEmpty(out)
}

// awarenessPrompts 由主型的挑战点生成自我觉察提示。
func awarenessPrompts(meta quiz.TypeResult) []string {
	out := []string{}
	for _, c := range meta.Challenges {
		out = append(out, "当你察觉到「"+c+"」时，停下来问问自己：此刻我真正需要的是什么？")
	}
	return dedupeNonEmpty(out)
}

// guidingQuestions 复用每日内容里的引导问题，按主型聚合去重。
func guidingQuestions(mainType int) []string {
	items, ok := quiz.DailyPractices[mainType]
	if !ok {
		return nil
	}
	out := []string{}
	for _, it := range items {
		out = append(out, it.Question)
	}
	return dedupeNonEmpty(out)
}

// dedupeNonEmpty 过滤空串并去重，保持原有顺序。
func dedupeNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
