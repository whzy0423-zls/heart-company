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

func TestSkillRuntimePromptKeepsAnswersDirectWhilePreservingClarifyingFlow(t *testing.T) {
	prompt := resolveRuntimeSystemPrompt("当前技能：学习方法", rag.GenerateInput{
		RuntimeInstructions: "默认工作流：先定位问题，再给出行动。",
	})
	for _, required := range []string{
		"技能会话输出契约",
		"第一段先用当前技能直接回答用户此刻的问题",
		"先直接回应当前问题",
		"基于现有信息给出暂定回答",
		"只追问一个最关键的问题",
		"用户不知道怎么提问时，先推荐一个最适合的切入点",
		"给出一个可以直接复制的问题示例",
		"不要只罗列选项",
		"不要把澄清问题写成一长串信息采集清单",
		"最多选一至两个方法落到当前场景",
		"不要输出原始 Markdown 标记",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("skill runtime prompt missing direct-answer workflow %q: %s", required, prompt)
		}
	}
	if strings.LastIndex(prompt, "技能会话输出契约") < strings.Index(prompt, "【已审核技能行为规则结束】") {
		t.Fatalf("output contract must be applied after skill-specific rules: %s", prompt)
	}
}
