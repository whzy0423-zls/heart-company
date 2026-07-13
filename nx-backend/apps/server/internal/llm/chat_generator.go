package llm

import (
	"context"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

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
