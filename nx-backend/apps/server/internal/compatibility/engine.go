package compatibility

import (
	"fmt"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/quiz"
)

const AlgorithmVersion = "v1"

type Level string

const (
	LevelResonant   Level = "resonant"
	LevelStable     Level = "stable"
	LevelBalanced   Level = "balanced"
	LevelDeveloping Level = "developing"
	LevelSensitive  Level = "sensitive"
)

type Scores struct {
	Resonance     int `json:"resonance"`
	Complement    int `json:"complement"`
	Communication int `json:"communication"`
	ConflictRisk  int `json:"conflictRisk"`
	Growth        int `json:"growth"`
	Stability     int `json:"stability"`
}

type Evidence struct {
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type Result struct {
	AlgorithmVersion string     `json:"algorithmVersion"`
	Scores           Scores     `json:"scores"`
	Level            Level      `json:"level"`
	ExplainTags      []string   `json:"explainTags"`
	Evidence         []Evidence `json:"evidence"`
	Summary          string     `json:"summary"`
	Highlights       []string   `json:"highlights"`
	ConflictPoints   []string   `json:"conflictPoints"`
	Suggestions      []string   `json:"suggestions"`
}

type normalizedCard struct {
	Name           string
	Type           int
	WingType       int
	TypeName       string
	Center         string
	CenterName     string
	FallbackType   bool
	MissingName    bool
	MissingProfile bool
}

// Analyze calculates a deterministic relationship compatibility report for two quiz cards.
// It is intentionally pure: no time, randomness, network calls, storage access, or model calls.
func Analyze(cardA, cardB quiz.Card) Result {
	a := normalizeCard(cardA, "A方")
	b := normalizeCard(cardB, "B方")

	distance := typeDistance(a.Type, b.Type)
	sameType := a.Type == b.Type && !a.FallbackType && !b.FallbackType
	sameCenter := a.Center == b.Center
	growthLink := isGrowthLink(a.Type, b.Type)
	stressLink := isStressLink(a.Type, b.Type)
	wingBridge := isWingBridge(a, b)

	scores := calculateScores(scoreContext{
		Distance:       distance,
		SameType:       sameType,
		SameCenter:     sameCenter,
		GrowthLink:     growthLink,
		StressLink:     stressLink,
		WingBridge:     wingBridge,
		HasFallback:    a.FallbackType || b.FallbackType,
		HasMissingName: a.MissingName || b.MissingName,
		MissingProfile: a.MissingProfile || b.MissingProfile,
	})

	tags := explainTags(a, b, sameType, sameCenter, growthLink, stressLink, wingBridge)
	level := levelFromScores(scores)

	return Result{
		AlgorithmVersion: AlgorithmVersion,
		Scores:           scores,
		Level:            level,
		ExplainTags:      tags,
		Evidence:         buildEvidence(a, b, distance, sameType, sameCenter),
		Summary:          buildSummary(a, b, level),
		Highlights:       buildHighlights(a, b, sameType, sameCenter, growthLink, wingBridge),
		ConflictPoints:   buildConflictPoints(a, b, sameType, sameCenter, stressLink),
		Suggestions:      buildSuggestions(a, b, growthLink, stressLink),
	}
}

type scoreContext struct {
	Distance       int
	SameType       bool
	SameCenter     bool
	GrowthLink     bool
	StressLink     bool
	WingBridge     bool
	HasFallback    bool
	HasMissingName bool
	MissingProfile bool
}

func calculateScores(ctx scoreContext) Scores {
	resonance := 62 + (4-ctx.Distance)*5
	complement := 56 + ctx.Distance*5
	communication := 60 + (4-ctx.Distance)*4
	growth := 60
	stability := 62 + (4-ctx.Distance)*5
	conflictRisk := 42 + ctx.Distance*5

	if ctx.SameType {
		resonance += 12
		communication += 6
		growth += 2
		stability += 12
		conflictRisk += 6
	}
	if ctx.SameCenter {
		resonance += 8
		complement -= 8
		communication += 10
		stability += 6
		conflictRisk += 8
	} else {
		resonance -= 4
		complement += 18
		growth += 8
		conflictRisk += 3
	}
	if ctx.GrowthLink {
		complement += 10
		communication += 8
		growth += 24
		stability += 4
		conflictRisk -= 6
	}
	if ctx.StressLink {
		complement += 4
		communication -= 12
		growth += 12
		stability -= 8
		conflictRisk += 12
	}
	if ctx.WingBridge {
		resonance += 6
		communication += 5
		stability += 4
		conflictRisk -= 4
	}
	if ctx.Distance == 2 || ctx.Distance == 3 {
		growth += 5
	}
	if !ctx.MissingProfile {
		stability += 3
	}

	qualityPenalty := 0
	if ctx.HasFallback {
		qualityPenalty += 8
	}
	if ctx.HasMissingName {
		qualityPenalty += 2
	}
	if ctx.MissingProfile {
		qualityPenalty += 4
		stability -= 3
	}
	resonance -= qualityPenalty
	communication -= qualityPenalty
	growth -= qualityPenalty / 2
	stability -= qualityPenalty
	conflictRisk += qualityPenalty

	return Scores{
		Resonance:     clampScore(resonance),
		Complement:    clampScore(complement),
		Communication: clampScore(communication),
		ConflictRisk:  clampScore(conflictRisk),
		Growth:        clampScore(growth),
		Stability:     clampScore(stability),
	}
}

func normalizeCard(card quiz.Card, fallbackName string) normalizedCard {
	name := strings.TrimSpace(card.Name)
	missingName := name == ""
	if missingName {
		name = fallbackName
	}

	typeID := card.MainType
	fallbackType := false
	if !validType(typeID) {
		typeID = 5
		fallbackType = true
	}
	info := quiz.TypesInfo[typeID]
	centerName := ""
	if center, ok := quiz.Centers[info.Center]; ok {
		centerName = center.Name
	}

	wingType := card.WingType
	if !validType(wingType) {
		wingType = 0
	}

	profileText := strings.TrimSpace(string(card.Profile))
	missingProfile := profileText == "" || profileText == "null"

	return normalizedCard{
		Name:           name,
		Type:           typeID,
		WingType:       wingType,
		TypeName:       info.Name,
		Center:         info.Center,
		CenterName:     centerName,
		FallbackType:   fallbackType,
		MissingName:    missingName,
		MissingProfile: missingProfile,
	}
}

func explainTags(a, b normalizedCard, sameType, sameCenter, growthLink, stressLink, wingBridge bool) []string {
	tags := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(tag string) {
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}

	if sameType {
		add("same_type_resonance")
	}
	if sameCenter {
		add("same_center_alignment")
	} else {
		add("cross_center_complement")
	}
	if growthLink {
		add("growth_direction")
	}
	if stressLink {
		add("stress_direction")
	}
	if wingBridge {
		add("wing_bridge")
	}
	if a.FallbackType || b.FallbackType {
		add("fallback_type")
	}
	if a.MissingName || b.MissingName {
		add("missing_name")
	}
	if a.MissingProfile || b.MissingProfile {
		add("missing_profile")
	}
	return tags
}

func buildEvidence(a, b normalizedCard, distance int, sameType, sameCenter bool) []Evidence {
	relation := "不同型号"
	if sameType {
		relation = "同型共振"
	} else if sameCenter {
		relation = "同中心呼应"
	}

	evidence := []Evidence{
		{
			Code:  "type_pair",
			Title: "型号组合",
			Detail: fmt.Sprintf("%s：%d号%s × %s：%d号%s，关系特征为%s。",
				a.Name, a.Type, a.TypeName, b.Name, b.Type, b.TypeName, relation),
		},
		{
			Code:   "center_pair",
			Title:  "三中心",
			Detail: fmt.Sprintf("%s位于%s，%s位于%s。", a.Name, a.CenterName, b.Name, b.CenterName),
		},
		{
			Code:   "type_distance",
			Title:  "型号距离",
			Detail: fmt.Sprintf("九型环形距离为%d，用于平衡相似度、互补度与冲突风险。", distance),
		},
	}
	if a.FallbackType || b.FallbackType || a.MissingName || b.MissingName || a.MissingProfile || b.MissingProfile {
		evidence = append(evidence, Evidence{
			Code:   "data_quality",
			Title:  "输入兜底",
			Detail: "部分资料不完整时，会采用中性的关系观察角度，先给出基础建议。",
		})
	}
	return evidence
}

func buildSummary(a, b normalizedCard, level Level) string {
	prefix := fmt.Sprintf("%s与%s的关系合盘显示：", a.Name, b.Name)
	switch level {
	case LevelResonant:
		return prefix + "双方有很强的共振和承接感，适合把默契转化为稳定行动。"
	case LevelStable:
		return prefix + "这段关系整体稳定，关键在于保留相似处带来的理解，也给差异留下表达空间。"
	case LevelBalanced:
		return prefix + "双方既有互补也有节奏差异，越能把期待说清楚，越容易形成舒服的配合。"
	case LevelDeveloping:
		return prefix + "关系需要更多校准和练习，先建立安全的沟通节奏，再推进复杂议题。"
	default:
		return prefix + "当前组合对耐心和边界感要求较高，建议从低压力的具体协作开始。"
	}
}

func buildHighlights(a, b normalizedCard, sameType, sameCenter, growthLink, wingBridge bool) []string {
	highlights := []string{
		fmt.Sprintf("%s的%s特质与%s的%s特质可以互相提供参照。", a.Name, a.TypeName, b.Name, b.TypeName),
	}
	if sameType {
		highlights = append(highlights, "双方关注点接近，容易理解彼此在压力下的反应。")
	} else if sameCenter {
		highlights = append(highlights, "双方属于同一中心，底层关切相近，容易在重要议题上形成共同语言。")
	} else {
		highlights = append(highlights, "双方来自不同中心，一个补充另一个较少使用的观察角度。")
	}
	if growthLink {
		highlights = append(highlights, "这组型号存在成长方向连接，能提醒彼此看见更成熟的应对方式。")
	}
	if wingBridge {
		highlights = append(highlights, "侧翼与主型之间有桥接，能降低理解彼此风格的成本。")
	}
	return highlights
}

func buildConflictPoints(a, b normalizedCard, sameType, sameCenter, stressLink bool) []string {
	conflicts := []string{
		"节奏不一致时，容易把对方的停顿或推进理解成拒绝、催促或不重视。",
	}
	if sameType {
		conflicts = append(conflicts, "相似的盲点可能被同时放大，需要有人主动换一个视角。")
	} else if sameCenter {
		conflicts = append(conflicts, "同中心关系容易在同一类议题上较劲，要避免只争谁的感受更真实。")
	} else {
		conflicts = append(conflicts, fmt.Sprintf("%s更常从%s出发，%s更常从%s出发，表达顺序不同会制造误读。",
			a.Name, a.CenterName, b.Name, b.CenterName))
	}
	if stressLink {
		conflicts = append(conflicts, "压力方向连接会让某些互动显得更敏感，冲突中尤其需要降速。")
	}
	return conflicts
}

func buildSuggestions(a, b normalizedCard, growthLink, stressLink bool) []string {
	suggestions := []string{
		"讨论重要事情前，先各自说出此刻最在意的一件事。",
		"把评价改成具体请求，例如说出需要的时间、边界、行动或回应。",
	}
	if growthLink {
		suggestions = append(suggestions, "把对方当作成长提醒，而不是修正对象：先确认善意，再给建议。")
	}
	if stressLink {
		suggestions = append(suggestions, "冲突升温时先暂停十分钟，回到事实、感受、请求三个层次分别沟通。")
	} else {
		suggestions = append(suggestions, fmt.Sprintf("让%s负责提出方向，让%s补充风险或感受，形成清晰分工。", a.Name, b.Name))
	}
	return suggestions
}

func levelFromScores(scores Scores) Level {
	connection := (scores.Resonance + scores.Complement + scores.Communication + scores.Growth + scores.Stability + (100 - scores.ConflictRisk)) / 6
	switch {
	case connection >= 84:
		return LevelResonant
	case connection >= 72:
		return LevelStable
	case connection >= 60:
		return LevelBalanced
	case connection >= 48:
		return LevelDeveloping
	default:
		return LevelSensitive
	}
}

func typeDistance(a, b int) int {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	if diff > 4 {
		return 9 - diff
	}
	return diff
}

func isGrowthLink(a, b int) bool {
	return quiz.TypesInfo[a].Growth == b || quiz.TypesInfo[b].Growth == a
}

func isStressLink(a, b int) bool {
	return quiz.TypesInfo[a].Stress == b || quiz.TypesInfo[b].Stress == a
}

func isWingBridge(a, b normalizedCard) bool {
	return (a.WingType != 0 && a.WingType == b.Type) || (b.WingType != 0 && b.WingType == a.Type)
}

func validType(typeID int) bool {
	return typeID >= 1 && typeID <= 9
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
