package rag

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const recentHistoryLimit = 20

const minRAGRelevanceScore = 6

var (
	arabicEnneagramTypePattern    = regexp.MustCompile(`(?i)(^|[^a-z0-9])([1-9])(?:号)?型`)
	englishEnneagramTypePattern   = regexp.MustCompile(`(?i)(^|[^a-z0-9])type[\s_-]*([1-9])`)
	numberedTypeComparisonPattern = regexp.MustCompile(`([1-9一二三四五六七八九])号\s*(?:和|与|、|,|，|/)\s*([1-9一二三四五六七八九])号`)
)

var enneagramTypeAliases = [...][]string{
	nil,
	{"完美型"},
	{"助人型"},
	{"成就型"},
	{"自我型", "浪漫型"},
	{"思考型"},
	{"忠诚型", "疑惑型"},
	{"活跃型", "享乐型"},
	{"领袖型", "挑战型"},
	{"和平型"},
}

type Document struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type Source struct {
	ID      string `json:"id"`
	Snippet string `json:"snippet"`
	Title   string `json:"title"`
}

type UserProfile struct {
	Nickname string   `json:"nickname"`
	MainType int      `json:"mainType"`
	Memories []string `json:"memories,omitempty"`
}

type ConversationCard struct {
	CardType string `json:"cardType,omitempty"`
	Name     string `json:"name,omitempty"`
	Relation string `json:"relation,omitempty"`
	MainType int    `json:"mainType,omitempty"`
	WingType int    `json:"wingType,omitempty"`
	Profile  string `json:"profile,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AskInput struct {
	History             []Message        `json:"history"`
	ConversationSummary string           `json:"conversationSummary,omitempty"`
	Question            string           `json:"question"`
	UserProfile         UserProfile      `json:"userProfile"`
	ConversationCard    ConversationCard `json:"conversationCard,omitempty"`
	UserPreferences     []string         `json:"userPreferences,omitempty"`
	CurrentDirectives   []string         `json:"currentDirectives,omitempty"`
	Tier                string           `json:"tier,omitempty"`
}

type Answer struct {
	Answer      string   `json:"answer"`
	Sources     []Source `json:"sources"`
	Suggestions []string `json:"suggestions"`
}

type Service struct {
	docs      []Document
	generator Generator
}

type Generator interface {
	Generate(ctx context.Context, input GenerateInput) (string, error)
}

type StreamEmitter func(delta string) error

type StreamingGenerator interface {
	GenerateStream(ctx context.Context, input GenerateInput, emit StreamEmitter) (string, error)
}

type ConversationSummarizer interface {
	SummarizeConversation(ctx context.Context, previousSummary string, messages []Message) (string, error)
}

type GenerateInput struct {
	History             []Message        `json:"history"`
	ConversationSummary string           `json:"conversationSummary,omitempty"`
	Question            string           `json:"question"`
	Sources             []Source         `json:"sources"`
	UserProfile         UserProfile      `json:"userProfile"`
	ConversationCard    ConversationCard `json:"conversationCard,omitempty"`
	UserPreferences     []string         `json:"userPreferences,omitempty"`
	CurrentDirectives   []string         `json:"currentDirectives,omitempty"`
	Tier                string           `json:"tier,omitempty"`
}

type Option func(*Service)

func WithGenerator(generator Generator) Option {
	return func(s *Service) {
		s.generator = generator
	}
}

func NewService(docs []Document, options ...Option) *Service {
	copied := make([]Document, 0, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.ID) == "" || strings.TrimSpace(doc.Title) == "" {
			continue
		}
		doc.Content = strings.TrimSpace(doc.Content)
		if doc.Content == "" {
			continue
		}
		copied = append(copied, doc)
	}
	service := &Service{docs: copied}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Ask(ctx context.Context, input AskInput) (Answer, error) {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return Answer{}, errors.New("请输入想咨询的问题")
	}
	if utf8.RuneCountInString(question) > 300 {
		return Answer{}, errors.New("问题太长，请控制在 300 字以内")
	}
	if IsModelIdentityQuestion(question) {
		return Answer{
			Answer:      ModelIdentityReply,
			Sources:     []Source{},
			Suggestions: []string{},
		}, nil
	}

	matches := s.search(question, relevantMainType(input), 4)
	if len(matches) == 0 {
		// 检索未命中：仍尝试让 AI 结合九型常识作答（Sources 为空）；
		// 只有 AI 不可用或返回空时，才回退到固定兜底文案。
		if s.generator != nil {
			generated, err := s.generator.Generate(ctx, GenerateInput{
				History:             cleanHistory(input.History, recentHistoryLimit),
				ConversationSummary: input.ConversationSummary,
				Question:            question,
				Sources:             nil,
				UserProfile:         input.UserProfile,
				ConversationCard:    input.ConversationCard,
				UserPreferences:     input.UserPreferences,
				CurrentDirectives:   input.CurrentDirectives,
				Tier:                input.Tier,
			})
			if err == nil && strings.TrimSpace(generated) != "" {
				return Answer{
					Answer:      strings.TrimSpace(generated),
					Sources:     []Source{},
					Suggestions: buildSuggestions(input),
				}, nil
			}
		}
		return Answer{
			Answer:      buildFallbackAnswer(input.UserProfile),
			Sources:     []Source{},
			Suggestions: buildSuggestions(input),
		}, nil
	}

	sources := make([]Source, 0, len(matches))
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		snippet := trimRunes(match.doc.Content, 92)
		sources = append(sources, Source{
			ID:      match.doc.ID,
			Title:   match.doc.Title,
			Snippet: snippet,
		})
		parts = append(parts, "【"+match.doc.Title+"】"+snippet)
	}

	suggestions := buildSuggestions(input)

	name := strings.TrimSpace(input.UserProfile.Nickname)
	if name == "" {
		name = "你"
	}
	answer := name + "，我先按你问到的重点检索了九型资料："
	if input.ConversationCard.MainType > 0 {
		answer += "结合当前关注对象的主型，"
	} else if input.UserProfile.MainType > 0 {
		answer += "结合你最近的主型结果，"
	}
	answer += strings.Join(parts, "；") + "。你可以继续追问具体关系、职场、亲密关系或成长练习，我会沿着这些资料继续细化。"

	if s.generator != nil {
		generated, err := s.generator.Generate(ctx, GenerateInput{
			History:             cleanHistory(input.History, recentHistoryLimit),
			ConversationSummary: input.ConversationSummary,
			Question:            question,
			Sources:             sources,
			UserProfile:         input.UserProfile,
			ConversationCard:    input.ConversationCard,
			UserPreferences:     input.UserPreferences,
			CurrentDirectives:   input.CurrentDirectives,
			Tier:                input.Tier,
		})
		if err == nil && strings.TrimSpace(generated) != "" {
			return Answer{Answer: strings.TrimSpace(generated), Sources: sources, Suggestions: suggestions}, nil
		}
	}

	return Answer{Answer: answer, Sources: sources, Suggestions: suggestions}, nil
}

func (s *Service) AskStream(ctx context.Context, input AskInput, emit StreamEmitter) (Answer, error) {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return Answer{}, errors.New("请输入想咨询的问题")
	}
	if utf8.RuneCountInString(question) > 300 {
		return Answer{}, errors.New("问题太长，请控制在 300 字以内")
	}
	if IsModelIdentityQuestion(question) {
		if err := emitTextChunks(ModelIdentityReply, emit); err != nil {
			return Answer{}, err
		}
		return Answer{
			Answer:      ModelIdentityReply,
			Sources:     []Source{},
			Suggestions: []string{},
		}, nil
	}
	streamStarted := false
	trackedEmit := func(delta string) error {
		if delta != "" {
			streamStarted = true
		}
		if emit == nil {
			return nil
		}
		return emit(delta)
	}

	matches := s.search(question, relevantMainType(input), 4)
	if len(matches) == 0 {
		if s.generator != nil {
			generated, err := s.generateStreaming(ctx, GenerateInput{
				History:             cleanHistory(input.History, recentHistoryLimit),
				ConversationSummary: input.ConversationSummary,
				Question:            question,
				Sources:             nil,
				UserProfile:         input.UserProfile,
				ConversationCard:    input.ConversationCard,
				UserPreferences:     input.UserPreferences,
				CurrentDirectives:   input.CurrentDirectives,
				Tier:                input.Tier,
			}, trackedEmit)
			if err != nil && streamStarted {
				return Answer{}, err
			}
			if err == nil && strings.TrimSpace(generated) != "" {
				return Answer{
					Answer:      strings.TrimSpace(generated),
					Sources:     []Source{},
					Suggestions: buildSuggestions(input),
				}, nil
			}
		}
		answer := buildFallbackAnswer(input.UserProfile)
		if err := emitTextChunks(answer, emit); err != nil {
			return Answer{}, err
		}
		return Answer{
			Answer:      answer,
			Sources:     []Source{},
			Suggestions: buildSuggestions(input),
		}, nil
	}

	sources := make([]Source, 0, len(matches))
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		snippet := trimRunes(match.doc.Content, 92)
		sources = append(sources, Source{
			ID:      match.doc.ID,
			Title:   match.doc.Title,
			Snippet: snippet,
		})
		parts = append(parts, "【"+match.doc.Title+"】"+snippet)
	}
	suggestions := buildSuggestions(input)

	name := strings.TrimSpace(input.UserProfile.Nickname)
	if name == "" {
		name = "你"
	}
	answer := name + "，我先按你问到的重点检索了九型资料："
	if input.ConversationCard.MainType > 0 {
		answer += "结合当前关注对象的主型，"
	} else if input.UserProfile.MainType > 0 {
		answer += "结合你最近的主型结果，"
	}
	answer += strings.Join(parts, "；") + "。你可以继续追问具体关系、职场、亲密关系或成长练习，我会沿着这些资料继续细化。"

	if s.generator != nil {
		generated, err := s.generateStreaming(ctx, GenerateInput{
			History:             cleanHistory(input.History, recentHistoryLimit),
			ConversationSummary: input.ConversationSummary,
			Question:            question,
			Sources:             sources,
			UserProfile:         input.UserProfile,
			ConversationCard:    input.ConversationCard,
			UserPreferences:     input.UserPreferences,
			CurrentDirectives:   input.CurrentDirectives,
			Tier:                input.Tier,
		}, trackedEmit)
		if err != nil && streamStarted {
			return Answer{}, err
		}
		if err == nil && strings.TrimSpace(generated) != "" {
			return Answer{Answer: strings.TrimSpace(generated), Sources: sources, Suggestions: suggestions}, nil
		}
	}

	if err := emitTextChunks(answer, emit); err != nil {
		return Answer{}, err
	}
	return Answer{Answer: answer, Sources: sources, Suggestions: suggestions}, nil
}

func (s *Service) generateStreaming(ctx context.Context, input GenerateInput, emit StreamEmitter) (string, error) {
	if streamer, ok := s.generator.(StreamingGenerator); ok {
		return streamer.GenerateStream(ctx, input, emit)
	}
	generated, err := s.generator.Generate(ctx, input)
	if err != nil {
		return "", err
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return "", nil
	}
	if err := emitTextChunks(generated, emit); err != nil {
		return "", err
	}
	return generated, nil
}

func emitTextChunks(text string, emit StreamEmitter) error {
	if emit == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var builder strings.Builder
	runeCount := 0
	for _, r := range text {
		builder.WriteRune(r)
		runeCount++
		if shouldFlushStreamChunk(r, runeCount) {
			if err := emit(builder.String()); err != nil {
				return err
			}
			builder.Reset()
			runeCount = 0
		}
	}
	if builder.Len() > 0 {
		if err := emit(builder.String()); err != nil {
			return err
		}
	}
	return nil
}

func shouldFlushStreamChunk(r rune, runeCount int) bool {
	if runeCount >= 12 {
		return true
	}
	switch r {
	case '。', '！', '？', '；', '，', '.', '!', '?', ';', ',':
		return true
	default:
		return false
	}
}

type scoredDoc struct {
	doc   Document
	score int
}

func relevantMainType(input AskInput) int {
	if input.ConversationCard.MainType > 0 {
		return input.ConversationCard.MainType
	}
	return input.UserProfile.MainType
}

func (s *Service) search(question string, mainType int, limit int) []scoredDoc {
	terms := tokenize(question)
	scored := make([]scoredDoc, 0, len(s.docs))
	broadEnneagramQuestion := isBroadEnneagramKnowledgeQuestion(question)
	mainTypeToken := ""
	if mainType > 0 {
		mainTypeToken = string(rune('0' + mainType))
	}
	for _, doc := range s.docs {
		text := strings.ToLower(doc.Title + " " + doc.Content + " " + strings.Join(doc.Tags, " "))
		questionScore := 0
		for _, term := range terms {
			if term == "" {
				continue
			}
			if strings.Contains(text, term) {
				questionScore += 3
			}
			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					questionScore += 2
				}
			}
		}
		if questionScore == 0 {
			continue
		}
		score := questionScore
		if broadEnneagramQuestion && isEnneagramKnowledgeDocument(doc) {
			score += 3
		}
		if mainTypeToken != "" && (strings.Contains(doc.ID, "type-"+mainTypeToken) || strings.Contains(doc.Title, mainTypeToken+"号")) {
			score += 2
		}
		if score >= minRAGRelevanceScore {
			scored = append(scored, scoredDoc{doc: doc, score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].doc.ID < scored[j].doc.ID
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

func isBroadEnneagramKnowledgeQuestion(question string) bool {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(question)), "")
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"什么是九型", "九型是什么", "什么叫九型", "九型是什么意思",
		"什么是九型人格", "九型人格是什么", "介绍九型", "解释九型", "讲讲九型",
		"九型基础", "九型入门", "九型有哪些", "九型分别", "九型的分别",
		"九种类型", "九个类型", "九种型号", "九个型号",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return strings.Contains(normalized, "九型") &&
		containsAny(normalized, "是什么", "什么意思", "介绍", "解释", "讲讲", "分别", "有哪些", "类型", "型号")
}

func isEnneagramKnowledgeDocument(document Document) bool {
	text := strings.ToLower(document.ID + " " + document.Title + " " + document.Content + " " + strings.Join(document.Tags, " "))
	if containsAny(text, "九型", "主型", "翼型", "九种性格", "性格模式") {
		return true
	}
	for _, aliases := range enneagramTypeAliases {
		for _, alias := range aliases {
			if strings.Contains(text, strings.ToLower(alias)) {
				return true
			}
		}
	}
	return false
}

func tokenize(text string) []string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r >= utf8.RuneSelf {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)
	raw := strings.Fields(cleaned)
	terms := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, term := range raw {
		if utf8.RuneCountInString(term) < 2 && !unicode.IsNumber([]rune(term)[0]) {
			continue
		}
		addTerm(&terms, seen, term)
		if hasCJK(term) {
			for _, gram := range cjkNgrams(term, 2, 4) {
				addTerm(&terms, seen, gram)
			}
		}
	}
	return terms
}

func addTerm(terms *[]string, seen map[string]bool, term string) {
	if term == "" || seen[term] {
		return
	}
	seen[term] = true
	*terms = append(*terms, term)
}

func hasCJK(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func cjkNgrams(text string, min int, max int) []string {
	runes := []rune(text)
	if len(runes) < min {
		return nil
	}
	grams := []string{}
	for size := min; size <= max; size++ {
		if len(runes) < size {
			break
		}
		for i := 0; i+size <= len(runes); i++ {
			part := string(runes[i : i+size])
			if hasCJK(part) {
				grams = append(grams, part)
			}
		}
	}
	return grams
}

func trimRunes(text string, max int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "..."
}

func buildFallbackAnswer(profile UserProfile) string {
	name := strings.TrimSpace(profile.Nickname)
	if name == "" {
		name = "你"
	}
	return name + "，我暂时没有检索到特别匹配的资料。可以换个更具体的问题，比如“我的主型在亲密关系里怎么沟通”或“适合我的成长练习是什么”。"
}

// buildSuggestions 只围绕当前人物的九型主型和最近相关话题生成追问。
// 文档标题不能直接作为用户可见追问，避免把检索资料中的陌生概念带入对话。
func buildSuggestions(input AskInput) []string {
	mainType := suggestionMainType(input)
	if mainType == 0 {
		return nil
	}
	focus := recentEnneagramFocus(input, mainType)
	if focus == "" {
		return nil
	}

	typeLabel := fmt.Sprintf("%d号%s", mainType, enneagramTypeName(mainType))
	return []string{
		fmt.Sprintf("从%s的核心动机看，“%s”背后最在意什么？", typeLabel, focus),
		fmt.Sprintf("%s遇到“%s”时，惯性反应和压力点是什么？", typeLabel, focus),
		fmt.Sprintf("围绕“%s”，%s可以先做什么成长练习？", focus, typeLabel),
	}
}

func suggestionMainType(input AskInput) int {
	if mainType := input.ConversationCard.MainType; mainType >= 1 && mainType <= 9 {
		return mainType
	}
	return 0
}

func recentEnneagramFocus(input AskInput, mainType int) string {
	if hasExplicitOtherEnneagramType(input.Question, mainType) {
		return ""
	}
	if focus := normalizeSuggestionFocus(input.Question, mainType); focus != "" {
		return focus
	}
	if !isSuggestionContinuation(input.Question) {
		return ""
	}
	if focus, sawExplicitType := recentHistorySuggestionFocus(input.History, mainType); focus != "" {
		return focus
	} else if sawExplicitType {
		return ""
	}
	return normalizeSuggestionFocus(input.ConversationSummary, mainType)
}

func recentHistorySuggestionFocus(history []Message, mainType int) (string, bool) {
	activeType := 0
	sawExplicitType := false
	recentFocus := ""
	for _, message := range history {
		if strings.TrimSpace(message.Role) != "user" {
			continue
		}
		explicitTypes := explicitEnneagramTypes(message.Content)
		if len(explicitTypes) > 1 {
			activeType = -1
			sawExplicitType = true
			recentFocus = ""
			continue
		}
		if len(explicitTypes) == 1 {
			activeType = explicitTypes[0]
			sawExplicitType = true
		}
		if activeType != 0 && activeType != mainType {
			continue
		}
		if focus := normalizeSuggestionFocus(message.Content, mainType); focus != "" {
			recentFocus = focus
		}
	}
	return recentFocus, sawExplicitType
}

func isSuggestionContinuation(text string) bool {
	text = strings.TrimSpace(strings.Trim(text, "，。！？；：,.!?;: \t\n\r"))
	if text == "" || utf8.RuneCountInString(text) > 18 {
		return false
	}
	for _, prefix := range []string{"继续", "能再", "可以再", "再说", "再讲", "再展开", "再具体", "具体说"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	for _, prefix := range []string{"那", "这个", "这种", "这样"} {
		if strings.HasPrefix(text, prefix) &&
			(strings.Contains(text, "怎么办") || strings.Contains(text, "怎么做") || strings.Contains(text, "为什么")) {
			return true
		}
	}
	return text == "怎么办" || text == "为什么" || text == "怎么做" ||
		text == "还有呢" || text == "然后呢" || text == "那我呢" || text == "这种情况呢"
}

func normalizeSuggestionFocus(text string, mainType int) string {
	text = normalizeSuggestionUnicode(text)
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" || hasExplicitOtherEnneagramType(text, mainType) {
		return ""
	}
	relevanceText := text
	text = stripSuggestionRuntimeTerms(text)
	if text == "" || !isEnneagramRelevantFocus(relevanceText) {
		return ""
	}
	for _, prefix := range []string{
		"我想了解", "我想问", "想了解", "想问", "请问", "请帮我分析", "帮我分析", "帮我看看", "能不能说说", "可以说说",
	} {
		text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	text = strings.Trim(text, "，。！？；：,.!?;: \t\n\r")
	if text == "" {
		return ""
	}
	return trimSuggestionFocusRunes(text, 28)
}

func isEnneagramRelevantFocus(text string) bool {
	if enneagramTypeFromText(text) != 0 {
		return true
	}
	return containsSuggestionPsychologicalPattern(text)
}

func containsSuggestionPsychologicalPattern(text string) bool {
	for _, keyword := range []string{
		"内耗", "焦虑", "不安", "被拒绝", "拒绝感", "不敢表达", "不被理解", "不理解我", "拖延",
		"完美主义", "被认可", "想被认可", "不认可", "被否定", "担心", "控制欲", "害怕", "恐惧", "生气", "愤怒", "难过", "低落", "被需要", "被看见", "被忽略",
		"自我价值", "惯性反应", "回避冲突", "害怕冲突", "避免冲突", "迎合",
	} {
		searchFrom := 0
		for searchFrom < len(text) {
			relative := strings.Index(text[searchFrom:], keyword)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(keyword)
			if !isStrongPatternTechnicalMatch(text, keyword, start) {
				return true
			}
			searchFrom = end
		}
	}
	for _, keyword := range []string{
		"压力", "情绪", "感受", "动机", "边界", "性格", "人格", "安全感", "身份认同",
	} {
		searchFrom := 0
		for searchFrom < len(text) {
			relative := strings.Index(text[searchFrom:], keyword)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(keyword)
			if hasPersonalExperienceContext(text, keyword, start, end) {
				return true
			}
			searchFrom = end
		}
	}
	return containsAny(text, "九型", "主型", "翼型")
}

func isStrongPatternTechnicalMatch(text string, keyword string, start int) bool {
	if keyword != "被拒绝" {
		return false
	}
	before := strings.ToLower(trailingRunes(text[:start], 12))
	return containsAny(before, "连接", "请求", "http", "tcp")
}

func hasPersonalExperienceContext(text string, keyword string, start int, end int) bool {
	before := trailingRunes(text[:start], 20)
	after := leadingRunes(text[end:], 20)
	window := before + keyword + after
	if containsAny(window,
		"最近", "总是", "反复", "在关系中", "在关系里", "工作中", "对我来说",
		"不明白", "不知道", "想提升", "想改善", "想了解", "困扰", "让我",
		"守不住", "缺乏", "失控", "怎么办", "怎么调整", "很差", "迟钝", "低落") {
		return true
	}
	if keyword == "压力" && strings.HasPrefix(after, "很大") {
		trimmedBefore := strings.TrimSpace(before)
		return trimmedBefore == "" || hasPersonalPressureSubject(trimmedBefore) ||
			containsAny(trimmedBefore, "最近", "工作", "关系")
	}
	return false
}

func hasPersonalPressureSubject(before string) bool {
	lastPersonal := strings.LastIndex(before, "我")
	if lastPersonal < 0 {
		return false
	}
	tail := strings.TrimSpace(before[lastPersonal+len("我"):])
	return utf8.RuneCountInString(tail) <= 2 || containsAny(tail, "最近", "工作", "关系")
}

func hasExplicitOtherEnneagramType(text string, mainType int) bool {
	text = normalizeSuggestionUnicode(text)
	for candidate := 1; candidate <= 9; candidate++ {
		if candidate == mainType {
			continue
		}
		if referencesEnneagramType(text, candidate) {
			return true
		}
	}
	return false
}

type suggestionRuntimeTermPattern struct {
	pattern *regexp.Regexp
	ascii   bool
}

var suggestionRuntimeTermPatterns = compileSuggestionRuntimeTermPatterns()

func stripSuggestionRuntimeTerms(text string) string {
	text = normalizeSuggestionUnicode(text)
	for _, term := range suggestionRuntimeTermPatterns {
		if term.ascii {
			text = term.pattern.ReplaceAllString(text, `${1}${2}`)
		} else {
			text = term.pattern.ReplaceAllString(text, "")
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func compileSuggestionRuntimeTermPatterns() []suggestionRuntimeTermPattern {
	terms := make([]string, 0,
		len(chineseToolchainTerms)+len(englishToolchainTerms)+len(providerTerms)+len(modelIdentityTerms)+10)
	terms = append(terms, chineseToolchainTerms...)
	terms = append(terms, englishToolchainTerms...)
	terms = append(terms, providerTerms...)
	terms = append(terms, modelIdentityTerms...)
	terms = append(terms,
		"codex", "模型", "厂商", "版本", "参数", "底层环境", "运行环境", "内部运行环境")
	patterns := make([]suggestionRuntimeTermPattern, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, term := range terms {
		compact := []rune(strings.Join(strings.Fields(strings.ToLower(term)), ""))
		key := string(compact)
		if len(compact) == 0 || seen[key] {
			continue
		}
		seen[key] = true
		parts := make([]string, 0, len(compact))
		for _, r := range compact {
			parts = append(parts, regexp.QuoteMeta(string(r)))
		}
		ascii := isASCIIAlphaNumericRunes(compact)
		pattern := `(?i)` + strings.Join(parts, `[\s._-]*`)
		if ascii {
			pattern = `(?i)(^|[^a-z0-9])(?:` + strings.Join(parts, `[\s._-]*`) + `)([^a-z0-9]|$)`
		}
		patterns = append(patterns, suggestionRuntimeTermPattern{
			pattern: regexp.MustCompile(pattern),
			ascii:   ascii,
		})
	}
	return patterns
}

func isASCIIAlphaNumericRunes(runes []rune) bool {
	for _, r := range runes {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func trimSuggestionFocusRunes(text string, max int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

func normalizeSuggestionUnicode(text string) string {
	text = norm.NFKC.String(text)
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		switch r {
		case '\u0391', '\u03b1', '\u0410', '\u0430':
			return 'a'
		case '\u0392', '\u03b2', '\u0412', '\u0432':
			return 'b'
		case '\u0421', '\u0441':
			return 'c'
		case '\u0395', '\u03b5', '\u0415', '\u0435':
			return 'e'
		case '\u041d', '\u043d':
			return 'h'
		case '\u0399', '\u03b9', '\u0406', '\u0456':
			return 'i'
		case '\u0408', '\u0458':
			return 'j'
		case '\u039a', '\u03ba', '\u041a', '\u043a':
			return 'k'
		case '\u039c', '\u03bc', '\u041c', '\u043c':
			return 'm'
		case '\u039d', '\u03bd':
			return 'n'
		case '\u039f', '\u03bf', '\u041e', '\u043e':
			return 'o'
		case '\u03a1', '\u03c1', '\u0420', '\u0440':
			return 'p'
		case '\u03a4', '\u03c4', '\u0422', '\u0442':
			return 't'
		case '\u03a7', '\u03c7', '\u0425', '\u0445':
			return 'x'
		case '\u03a5', '\u03c5', '\u0423', '\u0443':
			return 'y'
		default:
			return r
		}
	}, text)
}

func enneagramTypeFromText(text string) int {
	types := explicitEnneagramTypes(text)
	if len(types) > 0 {
		return types[0]
	}
	return 0
}

func explicitEnneagramTypes(text string) []int {
	text = normalizeSuggestionUnicode(text)
	seen := [10]bool{}
	for mainType := 1; mainType <= 9; mainType++ {
		if referencesEnneagramType(text, mainType) {
			seen[mainType] = true
		}
	}
	for _, mainType := range adjacentComparedEnneagramTypes(text) {
		seen[mainType] = true
	}
	types := make([]int, 0, 2)
	for mainType := 1; mainType <= 9; mainType++ {
		if seen[mainType] {
			types = append(types, mainType)
		}
	}
	return types
}

func adjacentComparedEnneagramTypes(text string) []int {
	seen := [10]bool{}
	for _, match := range numberedTypeComparisonPattern.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 6 || hasOrdinaryNumberContext(text[:match[0]], text[match[1]:]) {
			continue
		}
		for _, capture := range []int{1, 2} {
			captureStart := capture * 2
			captureEnd := captureStart + 1
			if len(match) <= captureEnd || match[captureStart] < 0 || match[captureEnd] < 0 {
				continue
			}
			if mainType := enneagramDigitValue(text[match[captureStart]:match[captureEnd]]); mainType != 0 {
				seen[mainType] = true
			}
		}
	}
	types := make([]int, 0, 2)
	for mainType := 1; mainType <= 9; mainType++ {
		if seen[mainType] {
			types = append(types, mainType)
		}
	}
	return types
}

func enneagramDigitValue(text string) int {
	if len(text) == 1 && text[0] >= '1' && text[0] <= '9' {
		return int(text[0] - '0')
	}
	for index, number := range []string{"", "一", "二", "三", "四", "五", "六", "七", "八", "九"} {
		if text == number {
			return index
		}
	}
	return 0
}

func referencesEnneagramType(text string, mainType int) bool {
	if containsEnneagramTypeAlias(text, mainType) {
		return true
	}
	return referencesEnneagramNumber(text, mainType)
}

func containsEnneagramTypeAlias(text string, mainType int) bool {
	if mainType < 1 || mainType >= len(enneagramTypeAliases) {
		return false
	}
	for _, alias := range enneagramTypeAliases[mainType] {
		if strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

func referencesEnneagramNumber(text string, mainType int) bool {
	chineseNumbers := []string{"", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	digit := fmt.Sprintf("%d", mainType)
	chinese := chineseNumbers[mainType]
	if matchesCapturedType(arabicEnneagramTypePattern, text, mainType, 2) ||
		matchesCapturedType(englishEnneagramTypePattern, text, mainType, 2) {
		return true
	}
	for _, explicit := range []string{
		"第" + chinese + "型", chinese + "号型", chinese + "型",
	} {
		if containsChineseTypeToken(text, explicit) {
			return true
		}
	}
	for _, label := range []string{digit + "号", chinese + "号"} {
		searchFrom := 0
		for searchFrom < len(text) {
			relative := strings.Index(text[searchFrom:], label)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(label)
			if hasLocalEnneagramNumberContext(text[:start], text[end:]) {
				return true
			}
			searchFrom = end
		}
	}
	return false
}

func matchesCapturedType(pattern *regexp.Regexp, text string, mainType int, capture int) bool {
	for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
		captureStart := capture * 2
		captureEnd := captureStart + 1
		if len(match) <= captureEnd || match[captureStart] < 0 || match[captureEnd] < 0 {
			continue
		}
		captured := text[match[captureStart]:match[captureEnd]]
		if len(captured) == 1 && int(captured[0]-'0') == mainType &&
			!hasInvalidTypeTokenContinuation(text[match[1]:]) {
			return true
		}
	}
	return false
}

func containsChineseTypeToken(text string, token string) bool {
	searchFrom := 0
	for searchFrom < len(text) {
		relative := strings.Index(text[searchFrom:], token)
		if relative < 0 {
			return false
		}
		start := searchFrom + relative
		end := start + len(token)
		validPrefix := start == 0
		if !validPrefix {
			before, _ := utf8.DecodeLastRuneInString(text[:start])
			validPrefix = !strings.ContainsRune("一二三四五六七八九十百0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", before)
		}
		if validPrefix && !hasInvalidTypeTokenContinuation(text[end:]) {
			return true
		}
		searchFrom = end
	}
	return false
}

func hasInvalidTypeTokenContinuation(after string) bool {
	if after == "" {
		return false
	}
	if strings.HasPrefix(after, "设备") || strings.HasPrefix(after, "型号") {
		return true
	}
	next, _ := utf8.DecodeRuneInString(after)
	return next == '号' || next == '码' ||
		(next >= '0' && next <= '9') ||
		(next >= 'a' && next <= 'z') ||
		(next >= 'A' && next <= 'Z')
}

func hasLocalEnneagramNumberContext(before string, after string) bool {
	before = strings.TrimRight(before, "，,。.!！?？；;：: 	\n\r")
	after = strings.TrimLeft(after, "，,。.!！?？；;：: 	\n\r")
	if hasOrdinaryNumberContext(before, after) {
		return false
	}
	for _, prefix := range []string{
		"我是", "作为", "我像", "更像", "主型是", "翼型是", "类型是",
		"九型的", "九型里", "九型中的", "九型人格的", "分析", "看看",
	} {
		if strings.HasSuffix(before, prefix) {
			return true
		}
	}
	for _, suffix := range []string{
		"为什么", "为何", "怎么", "如何", "会", "总", "通常", "容易", "倾向", "模式",
		"在压力", "压力", "在关系", "在冲突", "面对", "遇到", "害怕", "恐惧", "焦虑", "不安",
		"的核心", "的动机", "的惯性", "的边界", "的性格", "的人格", "性格", "人格",
	} {
		if strings.HasPrefix(after, suffix) {
			return true
		}
	}
	nearbyAfter := leadingRunes(after, 16)
	for _, keyword := range []string{
		"性格", "人格", "类型", "核心模式", "核心动机", "惯性模式", "行为模式",
	} {
		if strings.Contains(nearbyAfter, keyword) {
			return true
		}
	}
	return false
}

func hasOrdinaryNumberContext(before string, after string) bool {
	if strings.HasSuffix(before, "月") {
		return true
	}
	for _, suffix := range []string{
		"地铁", "线路", "线", "桌", "桌位", "房", "房间", "门", "订单", "工位", "座位", "座", "车", "车厢", "柜", "包厢", "病床",
	} {
		if strings.HasPrefix(after, suffix) {
			return true
		}
	}
	return false
}

func leadingRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func trailingRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[len(runes)-limit:])
}

func enneagramTypeName(mainType int) string {
	names := [...]string{"", "完美型", "助人型", "成就型", "自我型", "思考型", "忠诚型", "活跃型", "领袖型", "和平型"}
	if mainType < 1 || mainType >= len(names) {
		return ""
	}
	return names[mainType]
}

func cleanHistory(history []Message, limit int) []Message {
	if len(history) == 0 || limit <= 0 {
		return nil
	}
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	cleaned := make([]Message, 0, len(history))
	for _, item := range history {
		role := strings.TrimSpace(item.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := trimRunes(item.Content, 220)
		if content == "" {
			continue
		}
		cleaned = append(cleaned, Message{Role: role, Content: content})
	}
	return cleaned
}
