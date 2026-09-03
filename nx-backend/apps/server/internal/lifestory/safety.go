package lifestory

import "strings"

var (
	harmTopics = []string{
		"自杀", "自残", "自伤", "杀人", "伤害他人", "下毒", "投毒", "制作炸弹", "制造爆炸物",
	}
	harmMethodMarkers = []string{
		"如何", "怎么", "具体步骤", "操作步骤", "方法", "教程", "剂量", "所需工具", "所需材料", "准备工具", "准备材料",
	}
	harmOutputMethodMarkers = []string{
		"具体步骤", "操作步骤", "步骤一", "第一步", "教程", "具体剂量", "所需工具", "所需材料", "准备工具", "准备材料",
	}
	directRequestMarkers = []string{
		"请", "教我", "告诉我", "帮我", "给我", "写出", "列出", "提供", "演示", "说明",
	}
	harmEncouragementMarkers = []string{
		"自杀是唯一的解脱", "自杀才是解脱", "你应该自杀", "你应该去自杀",
		"自残是勇敢的", "自伤是勇敢的", "伤害自己是值得的", "死亡是美丽的",
		"暴力是荣耀的", "杀了他才能解脱", "杀了她才能解脱",
	}
)

func validateStoryInputSafety(snapshot StorySnapshot) error {
	values := []string{snapshot.RevisionInstruction, snapshot.FactCard.Setting, snapshot.FactCard.Conflict,
		snapshot.FactCard.TurningPoint, snapshot.FactCard.CentralQuestion, snapshot.FactCard.Ending, snapshot.FactCard.Unresolved}
	for _, material := range snapshot.Materials {
		values = append(values, material.Text, material.Transcript)
	}
	for _, question := range snapshot.FactCard.Questions {
		values = append(values, question.Prompt, question.Answer)
	}
	for _, event := range append(append([]FactEvent{}, snapshot.FactCard.Events...), snapshot.FactCard.Timeline...) {
		values = append(values, event.Description, event.TurningPoint, event.Outcome)
	}
	for _, chapter := range snapshot.Outline.Chapters {
		values = append(values, chapter.Title, chapter.Summary, chapter.Beat)
		values = append(values, chapter.KeyBeats...)
	}
	for _, value := range values {
		if containsExplicitHarmInstruction(value) {
			return newSafetyError("input", "harmful_instruction")
		}
	}
	return nil
}

func validateStoryOutputSafety(version Version) error {
	for _, clause := range storyClauses(versionText(version)) {
		compact := compactSafetyText(clause)
		if containsAny(compact, harmEncouragementMarkers) {
			return newSafetyError("output", "harmful_guidance")
		}
		if containsAny(compact, harmTopics) && containsAny(compact, harmOutputMethodMarkers) {
			return newSafetyError("output", "harmful_guidance")
		}
	}
	return nil
}

func containsExplicitHarmInstruction(value string) bool {
	for _, clause := range storyClauses(value) {
		compact := compactSafetyText(clause)
		if containsAny(compact, harmTopics) &&
			containsAny(compact, harmMethodMarkers) &&
			containsAny(compact, directRequestMarkers) {
			return true
		}
	}
	return false
}

func storyClauses(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '。', '！', '？', '；', '\n', '\r', '.', '!', '?', ';':
			return true
		default:
			return false
		}
	})
}

func compactSafetyText(value string) string {
	return strings.NewReplacer(" ", "", "\t", "", "\u3000", "").Replace(strings.ToLower(value))
}

func containsAny(value string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
