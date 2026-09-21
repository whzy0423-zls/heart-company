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
	Strengths            []string `json:"strengths,omitempty"`            // 天赋优势
	PotentialDirections  []string `json:"potentialDirections,omitempty"`  // 可继续发展的潜力方向
	SupportResources     []string `json:"supportResources,omitempty"`     // 压力中可调用的内在资源
	PositivePatterns     []string `json:"positivePatterns,omitempty"`     // 画像中较稳定的正向倾向
	StressPoints         []string `json:"stressPoints,omitempty"`         // 压力点
	StressSolutions      []string `json:"stressSolutions,omitempty"`      // 对应压力点的应对方案
	EmotionSupport       []string `json:"emotionSupport,omitempty"`       // 情绪压抑等状态的照顾方案
	RelationshipPatterns []string `json:"relationshipPatterns,omitempty"` // 关系模式
	GrowthAdvice         []string `json:"growthAdvice,omitempty"`         // 成长建议
	AwarenessPrompts     []string `json:"awarenessPrompts,omitempty"`     // 自我觉察提示
	GuidingQuestions     []string `json:"guidingQuestions,omitempty"`     // 引导问题
	MainType             int      `json:"mainType,omitempty"`             // 主型 id
	UpdatedAt            string   `json:"updatedAt,omitempty"`            // 更新时间，格式 YYYY/MM/DD HH:mm:ss
	membershipResourceMetadata
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
	access, accessErr := s.cardMembershipResourceMetadata(
		r.Context(), userID, id, "历史画像已保留，请升级后继续使用",
	)
	if accessErr != nil {
		httpx.Fail(w, http.StatusInternalServerError, "membership access unavailable")
		return
	}
	resp.membershipResourceMetadata = access
	redactPortraitContent(&resp, access)
	httpx.OK(w, resp)
}

// redactPortraitContent keeps the card identity and update timestamp useful
// for history navigation while withholding the generated portrait sections
// when the underlying card is no longer entitled to them.
func redactPortraitContent(resp *portraitResp, access membershipResourceMetadata) {
	if resp == nil || !membershipContentLocked(access) {
		return
	}
	resp.HasEnoughData = false
	resp.Summary = ""
	resp.StateLabel = ""
	resp.Strengths = nil
	resp.PotentialDirections = nil
	resp.SupportResources = nil
	resp.PositivePatterns = nil
	resp.StressPoints = nil
	resp.StressSolutions = nil
	resp.EmotionSupport = nil
	resp.RelationshipPatterns = nil
	resp.GrowthAdvice = nil
	resp.AwarenessPrompts = nil
	resp.GuidingQuestions = nil
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
		Strengths:            dedupeNonEmpty(meta.Strengths),
		PotentialDirections:  potentialDirections(card.MainType),
		SupportResources:     supportResources(card.MainType),
		PositivePatterns:     positivePatterns(card.MainType),
		StressPoints:         dedupeNonEmpty(meta.Challenges),
		StressSolutions:      stressSolutions(card.MainType),
		EmotionSupport:       emotionSupport(card.MainType),
		RelationshipPatterns: relationshipPatterns(card.MainType),
		GrowthAdvice:         growthAdvice(meta),
		AwarenessPrompts:     awarenessPrompts(meta),
		GuidingQuestions:     guidingQuestions(card.MainType),
		MainType:             card.MainType,
		UpdatedAt:            card.UpdateTime,
	}
	return resp
}

// stressSolutions 为每个型号的常见压力反应提供低门槛、可立即执行的动作。
func stressSolutions(mainType int) []string {
	plans := map[int][]string{
		1: {"先把标准分成“必须做到”和“可以更好”，只完成今天真正重要的一项。", "发现批评声音时，改用一句事实描述替代评价，再决定是否需要行动。"},
		2: {"先写下自己的一个需要，再决定是否答应他人的请求。", "练习用“我现在能做到的是……”表达边界，保留休息和被照顾的空间。"},
		3: {"把待办缩减为一个核心目标，并预留一段不追求产出的恢复时间。", "每天记录一件与成绩无关、但让自己感到真实的事情。"},
		4: {"给情绪十分钟被看见，然后做一个具体的小行动，让身体重新进入当下。", "低落时主动联系一位可靠的人，用事实说明近况和需要。"},
		5: {"先确认任务边界和最小交付，避免为了准备充分而持续延后行动。", "安排一次短而明确的交流，只分享一个想法或一种感受。"},
		6: {"把担忧分成“已发生、可能发生、可以准备”三栏，只处理可以准备的部分。", "设定一个决策时限，信息达到七成后先做可逆的小选择。"},
		7: {"减少同时进行的任务，选定一件事持续专注二十分钟。", "不急着转移不舒服，先命名感受并观察身体反应一分钟。"},
		8: {"冲突前先放慢呼吸和语速，用“我在意……”替代立即施压。", "把力量用于提出清晰请求，同时给对方回应和协商空间。"},
		9: {"从最小且最重要的一步开始，完成后再决定下一步。", "在关系中先说出自己的偏好，再倾听和整合他人的意见。"},
	}
	return dedupeNonEmpty(plans[mainType])
}

func emotionSupport(mainType int) []string {
	plans := map[int][]string{
		1: {"当你发现自己在压抑情绪时，先用“我感到……，因为我在意……”写下一句话。", "允许情绪存在并不等于失去原则；可以先感受，再选择合适的表达方式。"},
		2: {"先区分“别人的感受”和“我的感受”，每天留十分钟只照顾自己的需要。", "如果委屈持续累积，尝试向可信任的人提出一个具体、可回应的请求。"},
		3: {"暂停表现得没事，给疲惫、失落或害怕一个准确的名字。", "用不带结果评价的方式记录今天的身体感受和情绪变化。"},
		4: {"情绪强烈时先回到身体：喝水、走动、慢呼吸，再整理意义。", "把感受表达成作品、文字或对话，但避免在情绪高峰做重大决定。"},
		5: {"尝试把分析改写成感受句：“我想到……时，身体有……的感觉”。", "选择安全关系做小剂量表达，不需要一次说完所有内容。"},
		6: {"焦虑出现时先确认此刻是否真的有危险，再把注意力放回可控制的一步。", "向可靠的人核对事实，但为反复求证设定次数和结束时间。"},
		7: {"给不舒服的情绪两分钟空间，不急着用新计划或娱乐盖过去。", "写下你真正不想面对的事情，并选一个温和而具体的处理动作。"},
		8: {"愤怒下面可能还有受伤、担心或失望，先尝试辨认第二层感受。", "用直接但不攻击的语言表达边界，并允许自己向可靠的人寻求支持。"},
		9: {"情绪麻木或压住需求时，先从身体紧绷、疲惫或拖延中寻找线索。", "每天主动表达一次偏好，让自己的声音逐渐进入关系。"},
	}
	return dedupeNonEmpty(plans[mainType])
}

// potentialDirections 使用主型的成长方向，呈现可继续培养的能力，而非已发生的进步。
func potentialDirections(mainType int) []string {
	typeInfo, ok := quiz.TypesInfo[mainType]
	if !ok {
		return nil
	}
	target, ok := quiz.TypeResults[typeInfo.Growth]
	if !ok {
		return nil
	}
	out := make([]string, 0, 2)
	for _, strength := range target.Strengths {
		out = append(out, "在成长过程中，可以逐步发展「"+strength+"」的能力。")
		if len(out) == 2 {
			break
		}
	}
	return dedupeNonEmpty(out)
}

// supportResources 是各主型在压力情境中可主动调动的已有能力。
func supportResources(mainType int) []string {
	resources := map[int][]string{
		1: {"用清晰的原则感辨别真正重要的事", "用持续改进的执行力推进一个小步骤"},
		2: {"用感受他人需要的敏锐度理解关系", "用主动建立连接的能力寻求合适支持"},
		3: {"用快速聚焦目标的能力理清优先级", "用灵活调整方法的能力应对变化"},
		4: {"用细腻的情绪感受识别真实需要", "用创意表达为复杂感受找到出口"},
		5: {"用独立分析能力梳理复杂问题", "用清晰的边界感保护思考空间"},
		6: {"用风险预见能力准备可行预案", "在可靠关系中借助协作获得支持"},
		7: {"用发现新可能的能力寻找替代方案", "用积极视角帮助自己恢复行动"},
		8: {"用直接行动的力量处理眼前问题", "用保护重要关系和边界的勇气作出选择"},
		9: {"用稳定情绪的能力缓和紧张", "用理解多方立场的包容度寻找共识"},
	}
	return dedupeNonEmpty(resources[mainType])
}

// positivePatterns 描述画像中的正向倾向，不把类型推断表述为近期行为事实。
func positivePatterns(mainType int) []string {
	patterns := map[int][]string{
		1: {"在需要秩序和标准的情境中，你可能更容易主动承担责任。"},
		2: {"在关系需要支持时，你可能更容易觉察并回应他人的感受。"},
		3: {"在目标清晰的情境中，你可能更容易组织行动并带动进度。"},
		4: {"在需要理解情绪与意义时，你可能更容易看见细微而独特的部分。"},
		5: {"在面对复杂问题时，你可能更容易保持客观并深入分析。"},
		6: {"在环境出现变化时，你可能更容易提前识别风险并维护重要关系。"},
		7: {"在遇到限制时，你可能更容易发现新的选择和可能性。"},
		8: {"在需要保护与决断时，你可能更容易挺身而出并推动行动。"},
		9: {"在意见不一致时，你可能更容易理解各方并帮助关系恢复稳定。"},
	}
	return dedupeNonEmpty(patterns[mainType])
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
