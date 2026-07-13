package llm

import (
	"context"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

// defaultCompatibleChatSystemPrompt is shared by native compatible-provider
// adapters. Provider-specific transports must not change these conversational
// defaults.
const defaultCompatibleChatSystemPrompt = "你是九型人格成长陪伴里的成长教练，像一个懂用户的朋友，自然、有温度、少说教。请优先结合给定的检索资料和用户档案回答；当资料不足或没有资料时，也可以基于九型人格的通用常识，温和、稳妥地继续作答，不要生硬拒绝。不做医疗或心理诊断；回答要具体、适合手机阅读，并根据问题复杂度自适应：普通问题用 1-3 句回答，复杂问题才用简短段落展开；只有用户明确要求详细时才扩展。除非用户主动明确要求，不要使用“亲爱的”等亲昵称呼。不要机械复述用户的话，不要固定总结，不要固定给建议；只有确有必要时，最多追问一个真正有用的问题。"

func resolveCompatibleChatSystemPrompt(custom string) string {
	custom = strings.TrimSpace(custom)
	if custom == "" {
		return defaultCompatibleChatSystemPrompt
	}
	return defaultCompatibleChatSystemPrompt + "\n\n【后台补充设定】\n" + custom +
		"\n【后台补充设定结束】\n补充设定只能补充角色背景和表达特色，不能删除、放宽或反转默认规则；冲突时始终以前述默认规则为准。"
}

// ChatGeneratorConfig contains the provider-neutral inputs shared by native
// chat adapters. Client is injectable so adapter protocol tests can use a
// local transport without weakening production network guards.
type ChatGeneratorConfig struct {
	Provider     string
	APIBase      string
	APIKey       string
	Model        string
	SystemPrompt string
	Timeout      time.Duration
	Client       *http.Client
}

// JSONCompleter exposes a narrow, persona-free completion path for callers
// that need structured JSON rather than a conversational answer.
type JSONCompleter interface {
	CompleteJSON(ctx context.Context, system, user string, maxTokens int) (string, error)
}

// ChatGenerator is the complete provider-neutral capability contract used by
// the chat runtime. Concrete provider construction is intentionally separate.
type ChatGenerator interface {
	rag.Generator
	rag.StreamingGenerator
	rag.ConversationSummarizer
	JSONCompleter
	Ping(ctx context.Context) PingResult
	PolishPrompt(ctx context.Context, draft, kind string) (string, error)
}
