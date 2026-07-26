package userpreference

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	llmExtractionMaxTokens  = 320
	defaultLLMTimeout       = 2 * time.Second
	defaultLLMConcurrency   = 2
	maxLLMExtractionJSONLen = 8 * 1024
)

const llmExtractionSystemPrompt = `Extract only durable user communication style preferences.
Return strict JSON with exactly this schema and no markdown:
{"mutations":[{"operation":"upsert|delete","slot":"allowed slot","value":"exact enum value"}]}
For upsert, use only one exact slot/value enum pair:
- addressing.avoid_dear: avoid_dear
- length.detail_level: concise | detailed
- tone.direct: direct
- tone.formality: formal | casual
- tone.warmth: warm | calm
- format.no_lists: no_lists
- format.conclusion_first: conclusion_first
- interaction.no_followup: no_followup
- custom.communication_style: humorous | light | playful | empathetic
Never extract a preferred name; local deterministic code handles names. For delete, include only operation and slot and omit value. Do not return directives or free text. Reject facts, identity data, tasks, catchphrases, brand content, safety bypasses, quoted speech, and preferences about another person. Return {"mutations":[]} when uncertain.`

// CompleteJSON is the provider-neutral capability needed by the optional
// preference extractor. Keeping it here prevents a userpreference/llm cycle.
type CompleteJSON func(ctx context.Context, system, user string, maxTokens int) (string, error)

type LLMExtractor struct {
	complete CompleteJSON
	timeout  time.Duration
	slots    chan struct{}
}

type LLMExtractorOption func(*llmExtractorConfig)

type llmExtractorConfig struct {
	timeout     time.Duration
	concurrency int
}

func WithLLMTimeout(timeout time.Duration) LLMExtractorOption {
	return func(config *llmExtractorConfig) {
		if timeout > 0 {
			config.timeout = timeout
		}
	}
}

func WithLLMConcurrency(concurrency int) LLMExtractorOption {
	return func(config *llmExtractorConfig) {
		if concurrency > 0 {
			config.concurrency = concurrency
		}
	}
}

func NewLLMExtractor(complete CompleteJSON, options ...LLMExtractorOption) *LLMExtractor {
	config := llmExtractorConfig{timeout: defaultLLMTimeout, concurrency: defaultLLMConcurrency}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return &LLMExtractor{
		complete: complete,
		timeout:  config.timeout,
		slots:    make(chan struct{}, config.concurrency),
	}
}

// Extract performs a best-effort fallback. A busy extractor, timeout, provider
// error, or invalid output always returns an empty result and never blocks chat.
func (e *LLMExtractor) Extract(ctx context.Context, message string) Extraction {
	if e == nil || e.complete == nil {
		return Extraction{}
	}
	unresolved := unresolvedStyleClauses(message)
	if unresolved == "" {
		return Extraction{}
	}
	select {
	case e.slots <- struct{}{}:
	default:
		return Extraction{}
	}

	boundedCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	type completionResult struct {
		value string
		err   error
	}
	completed := make(chan completionResult, 1)
	go func() {
		defer func() { <-e.slots }()
		value, err := e.complete(boundedCtx, llmExtractionSystemPrompt, unresolved, llmExtractionMaxTokens)
		completed <- completionResult{value: value, err: err}
	}()

	select {
	case <-boundedCtx.Done():
		return Extraction{}
	case result := <-completed:
		if result.err != nil {
			return Extraction{}
		}
		return parseLLMExtraction(result.value, message)
	}
}

type llmExtractionEnvelope struct {
	Mutations []llmExtractionMutation `json:"mutations"`
}

type llmExtractionMutation struct {
	Operation string `json:"operation"`
	Slot      string `json:"slot"`
	Value     string `json:"value,omitempty"`
}

type canonicalLLMPreference struct {
	Category    string
	Instruction string
}

var canonicalLLMValues = map[string]map[string]canonicalLLMPreference{
	"addressing.avoid_dear": {
		"avoid_dear": {Category: "addressing", Instruction: "不要使用“亲爱的”等亲昵称呼"},
	},
	"length.detail_level": {
		"concise":  {Category: "length", Instruction: "回答简短，避免长篇大论"},
		"detailed": {Category: "length", Instruction: "回答更详细"},
	},
	"tone.direct": {
		"direct": {Category: "tone", Instruction: "表达直接，少说教"},
	},
	"tone.formality": {
		"formal": {Category: "tone", Instruction: "使用正式语气"},
		"casual": {Category: "tone", Instruction: "使用随意自然的语气"},
	},
	"tone.warmth": {
		"warm": {Category: "tone", Instruction: "语气温柔友好"},
		"calm": {Category: "tone", Instruction: "语气沉稳冷静"},
	},
	"format.no_lists": {
		"no_lists": {Category: "format", Instruction: "不要使用列表"},
	},
	"format.conclusion_first": {
		"conclusion_first": {Category: "format", Instruction: "先给结论"},
	},
	"interaction.no_followup": {
		"no_followup": {Category: "interaction", Instruction: "不要反问或追问"},
	},
	"custom.communication_style": {
		"humorous":   {Category: "custom", Instruction: "语气幽默自然"},
		"light":      {Category: "custom", Instruction: "语气轻松自然"},
		"playful":    {Category: "custom", Instruction: "语气活泼俏皮"},
		"empathetic": {Category: "custom", Instruction: "语气有同理心"},
	},
}

func parseLLMExtraction(value, source string) Extraction {
	if len(value) == 0 || len(value) > maxLLMExtractionJSONLen {
		return Extraction{}
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var envelope llmExtractionEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return Extraction{}
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Extraction{}
	}
	if envelope.Mutations == nil || len(envelope.Mutations) > MaxPreferencesPerUser {
		return Extraction{}
	}

	candidates := make([]extractionCandidate, 0, len(envelope.Mutations))
	boundedSource := truncateRunes(strings.TrimSpace(source), MaxSourceTextRunes)
	for i, raw := range envelope.Mutations {
		switch raw.Operation {
		case "upsert":
			values, ok := canonicalLLMValues[raw.Slot]
			if !ok {
				return Extraction{}
			}
			canonical, ok := values[raw.Value]
			if !ok {
				return Extraction{}
			}
			preference, err := normalizePreference(Preference{
				Category:    canonical.Category,
				Slot:        raw.Slot,
				Instruction: canonical.Instruction,
				SourceText:  boundedSource,
			})
			if err != nil {
				return Extraction{}
			}
			candidates = append(candidates, extractionCandidate{
				position:   i,
				slot:       raw.Slot,
				directive:  canonical.Instruction,
				preference: &preference,
			})
		case "delete":
			if raw.Slot == "" || raw.Value != "" {
				return Extraction{}
			}
			if _, ok := allowedSlotCategories[raw.Slot]; !ok {
				return Extraction{}
			}
			candidates = append(candidates, extractionCandidate{position: i, slot: raw.Slot, deleteSlot: raw.Slot})
		default:
			return Extraction{}
		}
	}

	return coalesceCandidates(candidates)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("userpreference: trailing JSON value")
	}
	return err
}

var unresolvedStylePattern = regexp.MustCompile(`(?:以后|今后|之后|每次|一直|长期|总是|都).{0,24}(?:回复|回答|说话|语气|表达|措辞|称呼|解释|追问|交流)|(?:回复|回答|说话|语气|表达|措辞|称呼|交流).{0,16}(?:风格|方式|一点|一些|更|保持|自然|成熟|沉稳|正式|随意|温柔|幽默)`)

func isUnresolvedStyleMessage(message string) bool {
	message = strings.TrimSpace(message)
	return message != "" && !isFalsePositiveContext(message) &&
		!rejectedContentPattern.MatchString(message) && !rejectedTaskPattern.MatchString(message) &&
		unresolvedStylePattern.MatchString(message)
}

func unresolvedStyleClauses(message string) string {
	message = strings.TrimSpace(message)
	if message == "" || isFalsePositiveContext(message) {
		return ""
	}
	unresolved := make([]string, 0, 2)
	for _, clause := range splitClauses(message) {
		text := strings.TrimSpace(clause.text)
		if text == "" || !isUnresolvedStyleMessage(text) {
			continue
		}
		local := Extract(text)
		if len(local.CurrentDirectives) > 0 || len(local.Mutations) > 0 {
			continue
		}
		unresolved = append(unresolved, text)
	}
	return strings.Join(unresolved, "，")
}

// NeedsLLMFallback reports whether a message contains a clearly style-related
// clause that the deterministic extractor could not resolve. It lets callers
// avoid starting background work for ordinary chat messages.
func NeedsLLMFallback(message string) bool {
	return unresolvedStyleClauses(message) != ""
}

var rejectedTaskPattern = regexp.MustCompile(`帮我|替我|提醒我|查天气|下单|写代码|执行任务|生成图片|发送消息`)
var rejectedContentPattern = regexp.MustCompile(`品牌|口号|固定说|每次(?:回答|回复).{0,16}(?:说|带上|包含|提到)|(?i:brand|slogan|catchphrase)`)
