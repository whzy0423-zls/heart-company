package llm

import (
	"strings"
	"testing"
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
