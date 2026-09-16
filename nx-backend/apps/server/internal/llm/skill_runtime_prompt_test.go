package llm

import (
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestSkillRuntimePromptDoesNotInheritOrdinaryEnneagramPersona(t *testing.T) {
	prompt := resolveRuntimeSystemPrompt("普通九型人格人物卡系统提示", rag.GenerateInput{
		RuntimeInstructions: "只基于学习之道回答。",
		ConversationCard:    rag.ConversationCard{MainType: 8},
	})
	if strings.Contains(prompt, "普通九型人格人物卡系统提示") {
		t.Fatalf("skill runtime inherited ordinary persona: %s", prompt)
	}
	for _, required := range []string{"只基于学习之道回答", "不引入人物卡", "其他会话", "不得回退到其他知识库", "平台规则为准"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("skill runtime prompt missing %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"当前人物卡口吻", "八号", "直接有力"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("skill runtime inherited current-card voice %q: %s", forbidden, prompt)
		}
	}
}
