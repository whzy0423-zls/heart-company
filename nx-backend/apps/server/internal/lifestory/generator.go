package lifestory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/llm"
)

var ErrSafetyBlocked = errors.New("life story generation was blocked by safety review")

type SafetyError struct {
	Phase string
	Code  string
}

func (e *SafetyError) Error() string {
	return fmt.Sprintf("life story safety block (%s/%s)", e.Phase, e.Code)
}

func (e *SafetyError) Unwrap() error { return ErrSafetyBlocked }

func newSafetyError(phase, code string) error {
	return &SafetyError{Phase: strings.TrimSpace(phase), Code: strings.TrimSpace(code)}
}

type JSONCompleter interface {
	CompleteJSON(context.Context, string, string, int) (string, error)
}

type GeneratorConfig struct {
	Completer JSONCompleter
	Model     string
	MaxTokens int
}

type Generator struct {
	completer JSONCompleter
	model     string
	maxTokens int
}

func NewGenerator(config GeneratorConfig) *Generator {
	maxTokens := config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 5600
	}
	return &Generator{completer: config.Completer, model: strings.TrimSpace(config.Model), maxTokens: maxTokens}
}

func (g *Generator) Generate(ctx context.Context, snapshot StorySnapshot) (Version, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return Version{}, err
	}
	safeSnapshot, tokenMap := TokenizeSnapshot(snapshot)
	return g.generateTokenized(ctx, safeSnapshot, tokenMap)
}

// GenerateTokenized is used by durable workers whose persisted snapshot has
// already crossed the PII boundary. The encrypted token map is used only to
// turn model placeholders into approved aliases or generic labels.
func (g *Generator) GenerateTokenized(ctx context.Context, safeSnapshot StorySnapshot, tokenMap TokenMap) (Version, error) {
	if err := ValidateSnapshot(safeSnapshot); err != nil {
		return Version{}, err
	}
	return g.generateTokenized(ctx, safeSnapshot, tokenMap)
}

func (g *Generator) generateTokenized(ctx context.Context, safeSnapshot StorySnapshot, tokenMap TokenMap) (Version, error) {
	if g == nil || g.completer == nil {
		return Version{}, errors.New("life story generator is not configured")
	}
	if err := validateStoryInputSafety(safeSnapshot); err != nil {
		return Version{}, err
	}
	storyStyle, styleInstruction, err := storyStylePrompt(safeSnapshot.Outline.StoryStyle)
	if err != nil {
		return Version{}, err
	}
	system := `你是“我的故事”真实经历写作引擎。只根据用户已确认的事实卡和提纲写作，不新增未确认的关键人物、地点、时间、关系或结局。提纲中的 perspective、tone 和 storyStyle 是唯一文体约束。输出严格 JSON，不要 Markdown。正文分成 4-6 章。请把 chapters 数组中所有 body 合计写到 3000-3800 个中文字符；这是为了确保最终正文通过不少于 2500 字的硬性校验，title、summary 和 reflection 都不计入正文字符数。reflection 另写 200-400 字。不能模仿具体作者。对创伤、自伤或暴力经历保持不诊断、不美化、不鼓励，不提供伤害方法或实施步骤；如故事涉及当下风险，使用温和的求助提示。成长回望必须独立放在 reflection 字段，不得把九型人格判断写进正文。JSON 结构：{"perspective":"first_person|third_person","tone":"warm|plain|healing","chapters":[{"order":1,"title":"","summary":"","body":""}],"reflection":""}` + "\n【本次文体要求】\n" + styleInstruction
	userRaw, _ := json.Marshal(safeSnapshot)
	user := "【已确认故事资料】\n" + string(userRaw)
	if instruction := strings.TrimSpace(safeSnapshot.RevisionInstruction); instruction != "" {
		user += "\n【用户本次定向修改】\n" + instruction + "\n【定向修改结束】"
	}
	user += "\n只输出上述结构的 JSON。提交前请自行核对 chapters 中全部 body 合计不少于 2500 个中文字符。"
	raw, err := g.completer.CompleteJSON(ctx, system, user, g.maxTokens)
	if err != nil {
		if errors.Is(err, llm.ErrContentFiltered) {
			return Version{}, newSafetyError("provider", "content_filtered")
		}
		return Version{}, err
	}
	version, err := ParseGeneratedVersion(raw)
	if err != nil {
		return Version{}, err
	}
	version.Status = VersionPublished
	version.Model = g.model
	version.StoryStyle = storyStyle
	version = restoreSafeTokens(version, tokenMap)
	if err := validateStoryOutputSafety(version); err != nil {
		return Version{}, err
	}
	if err := ValidateVersion(version); err != nil {
		return Version{}, err
	}
	if err := ValidateVersionAgainstFacts(version, safeSnapshot.FactCard); err != nil {
		return Version{}, err
	}
	if ContainsSensitiveToken(version, safeSnapshot, tokenMap) {
		return Version{}, newSafetyError("output", "privacy_leak")
	}
	version.CharacterCount = version.CharacterCountValue()
	version.WordCount = version.WordCountValue()
	return version, nil
}

func storyStylePrompt(value StoryStyle) (StoryStyle, string, error) {
	style, err := NormalizeStoryStyle(value)
	if err != nil {
		return "", "", err
	}
	switch style {
	case StoryStyleRealistic:
		return style, "采用真实回忆录文体，语言克制、具体、平实真诚；忠于确认事实，不补造对白或关键细节。", nil
	case StoryStyleNovel:
		return style, "采用小说叙事文体，加强场景、节奏和情绪层次；可润色表达和概括对白，但不得借对白引入未确认事实。", nil
	case StoryStyleFairyTale:
		return style, "采用童话寓言文体，用温柔的象征角色、场景和意象重述经历。人物关系、事件顺序、核心冲突、情绪转折和真实结局必须保持不变；象征内容不得冒充现实事实。", nil
	case StoryStyleMyth:
		return style, "采用神话叙事文体，用旅程、考验和神话意象重述经历，不套用或模仿具体作品。人物关系、事件顺序、核心冲突、情绪转折和真实结局必须保持不变；象征内容不得冒充现实事实。", nil
	default:
		return "", "", fmt.Errorf("story style is invalid")
	}
}

type generatedChapterDTO struct {
	Order   int    `json:"order"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

type generatedVersionDTO struct {
	Perspective Perspective           `json:"perspective"`
	Tone        Tone                  `json:"tone"`
	Chapters    []generatedChapterDTO `json:"chapters"`
	Reflection  string                `json:"reflection"`
}

func (dto generatedVersionDTO) version() Version {
	chapters := make([]Chapter, len(dto.Chapters))
	for i, chapter := range dto.Chapters {
		chapters[i] = Chapter{
			Order: chapter.Order, Title: chapter.Title,
			Summary: chapter.Summary, Body: chapter.Body,
		}
	}
	return Version{
		Perspective: dto.Perspective,
		Tone:        dto.Tone,
		Chapters:    chapters,
		Reflection:  dto.Reflection,
	}
}

// ParseGeneratedVersion accepts only model-owned story fields, either as the
// canonical object or in a provider wrapper. Server identifiers, lifecycle
// state, model metadata, and generation config are deliberately ignored.
func ParseGeneratedVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
			raw = raw[idx+1:]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var direct generatedVersionDTO
	if err := json.Unmarshal([]byte(raw), &direct); err == nil && len(direct.Chapters) > 0 {
		return direct.version(), nil
	}
	var wrapped struct {
		Version generatedVersionDTO `json:"version"`
		Story   generatedVersionDTO `json:"story"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err != nil {
		return Version{}, fmt.Errorf("invalid generated story JSON: %w", err)
	}
	if len(wrapped.Version.Chapters) > 0 {
		return wrapped.Version.version(), nil
	}
	if len(wrapped.Story.Chapters) > 0 {
		return wrapped.Story.version(), nil
	}
	return Version{}, errors.New("generated story JSON has no chapters")
}

func ValidateSnapshot(snapshot StorySnapshot) error {
	if snapshot.StoryID <= 0 {
		return errors.New("story snapshot id is required")
	}
	if len(snapshot.Materials) == 0 {
		return fmt.Errorf("%w: story snapshot has no materials", ErrValidation)
	}
	if !snapshot.FactCard.Confirmed || !snapshot.Outline.Confirmed {
		return errors.New("facts and outline must be confirmed")
	}
	if err := ValidateOutline(snapshot.Outline); err != nil {
		return err
	}
	for _, m := range snapshot.Materials {
		if strings.TrimSpace(m.Text) == "" && strings.TrimSpace(m.Transcript) == "" {
			return errors.New("story snapshot contains empty material")
		}
	}
	return nil
}

func ValidateVersionAgainstFacts(version Version, facts FactCard) error {
	text := versionText(version)
	for _, character := range facts.Characters {
		realName := strings.TrimSpace(character.RealName)
		alias := strings.TrimSpace(character.Alias)
		if realName != "" && realName != alias && strings.Contains(text, realName) {
			return fmt.Errorf("generated story exposed a private real name")
		}
	}
	for _, event := range append(append([]FactEvent{}, facts.Events...), facts.Timeline...) {
		if strings.TrimSpace(event.Description) == "" {
			return errors.New("fact event description is required")
		}
	}
	for _, marker := range []string{"[未确认]", "未确认人物", "未确认地点", "<UNCONFIRMED>"} {
		if strings.Contains(text, marker) {
			return fmt.Errorf("generated story contains an unconfirmed fact marker")
		}
	}
	return nil
}

// CharacterCount is exported for API validation and tests.
func CharacterCount(version Version) int {
	return utf8.RuneCountInString(strings.Join(func() []string {
		out := make([]string, 0, len(version.Chapters))
		for _, c := range version.Chapters {
			out = append(out, c.Body)
		}
		return out
	}(), ""))
}

var _ JSONCompleter = (llm.JSONCompleter)(nil)
