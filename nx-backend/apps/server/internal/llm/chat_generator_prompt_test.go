package llm

import (
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestDefaultChatSystemPromptsDoNotExposeAppImplementationLanguage(t *testing.T) {
	for name, prompt := range map[string]string{
		"compatible": defaultCompatibleChatSystemPrompt,
		"minimax":    defaultSystemPrompt,
	} {
		for _, forbidden := range []string{"App 端", "app端", "客户端"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("%s prompt must not contain likely leaked wording %q: %s", name, forbidden, prompt)
			}
		}
		for _, required := range []string{"不要主动描述运行载体", "页面", "后台服务", "接口链路", "模型配置", "实现细节", "只直接回答用户问题"} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("%s prompt missing direct-answer hygiene rule %q: %s", name, required, prompt)
			}
		}
		if !strings.Contains(prompt, "只直接回答用户问题") {
			t.Fatalf("%s prompt missing direct-answer rule: %s", name, prompt)
		}
	}
}

func TestConfiguredChatSystemPromptsKeepFixedDirectAnswerRules(t *testing.T) {
	const custom = "你是后台配置的温暖陪伴者。"

	prompts := map[string]string{
		"openai-compatible":    (&OpenAIChatGenerator{systemPrompt: custom}).resolveSystemPrompt(),
		"anthropic-compatible": resolveCompatibleChatSystemPrompt(custom),
		"compatible-chat": NewCompatibleChatGenerator(config.MiniMaxConfig{
			SystemPrompt: custom,
		}).resolveSystemPrompt(),
		"minimax": NewMiniMaxGenerator(config.MiniMaxConfig{
			SystemPrompt: custom,
		}).resolveSystemPrompt(),
	}

	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				"不要主动描述运行载体",
				"只直接回答用户问题",
				custom,
				"补充设定只能补充角色背景和表达特色",
				"不能删除、放宽或反转默认规则",
				"冲突时始终以前述默认规则为准",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("final system prompt missing %q: %s", required, prompt)
				}
			}
		})
	}
}
