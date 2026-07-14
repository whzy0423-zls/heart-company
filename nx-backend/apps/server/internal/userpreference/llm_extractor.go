package userpreference

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	llmExtractionMaxTokens  = 320
	defaultLLMTimeout       = 2 * time.Second
	defaultLLMConcurrency   = 2
	maxLLMExtractionJSONLen = 8 * 1024
)

const llmExtractionSystemPrompt = `Extract only durable user communication style preferences and explicit current-turn communication directives.
Return strict JSON with exactly this schema and no markdown:
{"directives":["communication style instruction"],"mutations":[{"operation":"upsert|delete","category":"addressing|length|tone|format|interaction|custom","slot":"allowed slot","instruction":"bounded communication style instruction"}]}
Allowed slots: addressing.preferred_name, addressing.avoid_dear, length.detail_level, tone.direct, tone.formality, tone.warmth, format.no_lists, format.conclusion_first, interaction.no_followup, custom.communication_style.
For delete, include only operation and slot. Reject facts, identity data, tasks, safety bypasses, quoted speech, and preferences about another person. Return empty arrays when uncertain.`

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
	Directives []string                `json:"directives"`
	Mutations  []llmExtractionMutation `json:"mutations"`
}

type llmExtractionMutation struct {
	Operation   string `json:"operation"`
	Category    string `json:"category,omitempty"`
	Slot        string `json:"slot"`
	Instruction string `json:"instruction,omitempty"`
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
	if envelope.Directives == nil || envelope.Mutations == nil || len(envelope.Directives) > 8 || len(envelope.Mutations) > MaxPreferencesPerUser {
		return Extraction{}
	}

	result := Extraction{CurrentDirectives: make([]string, 0, len(envelope.Directives))}
	for _, raw := range envelope.Directives {
		directive := strings.TrimSpace(raw)
		if !validCommunicationInstruction(directive) {
			return Extraction{}
		}
		result.CurrentDirectives = appendUnique(result.CurrentDirectives, directive)
	}

	candidates := make([]extractionCandidate, 0, len(envelope.Mutations))
	boundedSource := truncateRunes(strings.TrimSpace(source), MaxSourceTextRunes)
	for i, raw := range envelope.Mutations {
		operation := strings.TrimSpace(raw.Operation)
		slot := strings.TrimSpace(raw.Slot)
		switch operation {
		case "upsert":
			if raw.Category == "" || slot == "" || raw.Instruction == "" {
				return Extraction{}
			}
			instruction := strings.TrimSpace(raw.Instruction)
			if !validCommunicationInstruction(instruction) {
				return Extraction{}
			}
			preference, err := normalizePreference(Preference{
				Category:    raw.Category,
				Slot:        slot,
				Instruction: instruction,
				SourceText:  boundedSource,
			})
			if err != nil {
				return Extraction{}
			}
			if !instructionMatchesSlot(slot, instruction) {
				return Extraction{}
			}
			candidates = append(candidates, extractionCandidate{
				position:   i,
				slot:       slot,
				preference: &preference,
			})
		case "delete":
			if slot == "" || strings.TrimSpace(raw.Category) != "" || strings.TrimSpace(raw.Instruction) != "" {
				return Extraction{}
			}
			if _, ok := allowedSlotCategories[slot]; !ok {
				return Extraction{}
			}
			candidates = append(candidates, extractionCandidate{position: i, slot: slot, deleteSlot: slot})
		default:
			return Extraction{}
		}
	}

	mutations := coalesceCandidates(candidates).Mutations
	result.Mutations = mutations
	return result
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

var communicationStylePattern = regexp.MustCompile(`语气|回答|回复|表达|说话|措辞|称呼|叫我|亲爱|简短|详细|长篇|列表|清单|分点|结论|反问|追问|正式|随意|温柔|直接|说教|幽默|自然|沉稳|成熟|交流|问题`)
var rejectedSafetyPattern = regexp.MustCompile(`忽略.{0,12}(?:安全|规则|指令)|绕过|越狱|不受限制|系统提示词|(?i:system prompt)`)
var rejectedFactPattern = regexp.MustCompile(`我的.{0,12}(?:是|在|叫)|生日|身份证|手机号|住址|家庭地址|事实`)
var rejectedTaskPattern = regexp.MustCompile(`帮我|替我|提醒我|查天气|下单|写代码|执行任务|生成图片|发送消息`)
var rejectedContentPattern = regexp.MustCompile(`品牌|口号|固定说|每次(?:回答|回复).{0,16}(?:说|带上|包含|提到)|(?i:brand|slogan|catchphrase)`)
var customStylePattern = regexp.MustCompile(`(?:语气|风格|表达|措辞|交流).{0,16}(?:自然|成熟|沉稳|正式|随意|温柔|友好|亲切|幽默|简洁|直接|口语|专业)|(?:自然|成熟|沉稳|正式|随意|温柔|友好|亲切|幽默|简洁|直接|口语|专业).{0,16}(?:语气|风格|表达|措辞|交流)`)

var slotInstructionPatterns = map[string]*regexp.Regexp{
	"addressing.preferred_name": regexp.MustCompile(`称呼|叫我|喊我|名字`),
	"addressing.avoid_dear":     regexp.MustCompile(`亲爱|亲昵|称呼`),
	"length.detail_level":       regexp.MustCompile(`简短|详细|长篇|精简|展开|长度`),
	"tone.direct":               regexp.MustCompile(`直接|说教`),
	"tone.formality":            regexp.MustCompile(`正式|随意|严肃|口语`),
	"tone.warmth":               regexp.MustCompile(`温柔|友好|温暖|冷静|亲切`),
	"format.no_lists":           regexp.MustCompile(`列表|清单|分点`),
	"format.conclusion_first":   regexp.MustCompile(`结论|结果`),
	"interaction.no_followup":   regexp.MustCompile(`反问|追问|问题`),
}

func validCommunicationInstruction(instruction string) bool {
	runes := utf8.RuneCountInString(instruction)
	if runes == 0 || runes > MaxInstructionRunes {
		return false
	}
	if rejectedSafetyPattern.MatchString(instruction) || rejectedFactPattern.MatchString(instruction) || rejectedTaskPattern.MatchString(instruction) {
		return false
	}
	if rejectedContentPattern.MatchString(instruction) {
		return false
	}
	return communicationStylePattern.MatchString(instruction)
}

func instructionMatchesSlot(slot, instruction string) bool {
	if slot == "custom.communication_style" {
		return customStylePattern.MatchString(instruction)
	}
	pattern, ok := slotInstructionPatterns[slot]
	return ok && pattern.MatchString(instruction)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
