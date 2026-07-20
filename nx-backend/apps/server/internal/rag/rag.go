package rag

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const recentHistoryLimit = 20

const minRAGRelevanceScore = 6

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
	if isSmalltalkQuestion(question) {
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
				return Answer{Answer: strings.TrimSpace(generated), Sources: []Source{}, Suggestions: []string{}}, nil
			}
		}
		return Answer{Answer: buildSmalltalkFallback(question), Sources: []Source{}, Suggestions: []string{}}, nil
	}

	matches := s.search(question, relevantMainType(input), 4)
	if len(matches) == 0 {
		generationFailed := s.generator != nil
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
					Suggestions: buildSuggestions(nil),
				}, nil
			}
		}
		if generationFailed && !isExplicitEnneagramQuestion(question) {
			return Answer{
				Answer:      buildGenerationUnavailableAnswer(),
				Sources:     []Source{},
				Suggestions: buildSuggestions(nil),
			}, nil
		}
		return Answer{
			Answer:      buildFallbackAnswer(input.UserProfile),
			Sources:     []Source{},
			Suggestions: buildSuggestions(nil),
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

	suggestions := buildSuggestions(matches)

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

	generationFailed := s.generator != nil
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
	if generationFailed && !isExplicitEnneagramQuestion(question) {
		return Answer{
			Answer:      buildGenerationUnavailableAnswer(),
			Sources:     []Source{},
			Suggestions: buildSuggestions(nil),
		}, nil
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
	if isSmalltalkQuestion(question) {
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
				return Answer{Answer: strings.TrimSpace(generated), Sources: []Source{}, Suggestions: []string{}}, nil
			}
		}
		answer := buildSmalltalkFallback(question)
		if err := emitTextChunks(answer, emit); err != nil {
			return Answer{}, err
		}
		return Answer{Answer: answer, Sources: []Source{}, Suggestions: []string{}}, nil
	}

	matches := s.search(question, relevantMainType(input), 4)
	if len(matches) == 0 {
		generationFailed := s.generator != nil
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
					Suggestions: buildSuggestions(nil),
				}, nil
			}
		}
		if generationFailed && !isExplicitEnneagramQuestion(question) {
			answer := buildGenerationUnavailableAnswer()
			if err := emitTextChunks(answer, emit); err != nil {
				return Answer{}, err
			}
			return Answer{Answer: answer, Sources: []Source{}, Suggestions: buildSuggestions(nil)}, nil
		}
		answer := buildFallbackAnswer(input.UserProfile)
		if err := emitTextChunks(answer, emit); err != nil {
			return Answer{}, err
		}
		return Answer{
			Answer:      answer,
			Sources:     []Source{},
			Suggestions: buildSuggestions(nil),
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
	suggestions := buildSuggestions(matches)

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

	generationFailed := s.generator != nil
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
	if generationFailed && !isExplicitEnneagramQuestion(question) {
		answer := buildGenerationUnavailableAnswer()
		if err := emitTextChunks(answer, emit); err != nil {
			return Answer{}, err
		}
		return Answer{Answer: answer, Sources: []Source{}, Suggestions: buildSuggestions(nil)}, nil
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
	mainTypeToken := ""
	if mainType > 0 {
		mainTypeToken = string(rune('0' + mainType))
	}
	for _, doc := range s.docs {
		text := strings.ToLower(doc.Title + " " + doc.Content + " " + strings.Join(doc.Tags, " "))
		questionScore := 0
		for _, term := range terms {
			if term == "" || isGenericConversationTerm(term) {
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

func isSmalltalkQuestion(question string) bool {
	normalized := normalizeConversationalText(question)
	switch normalized {
	case "hi", "hello", "嗨", "哈喽", "你好", "您好", "在吗", "你在吗", "你在干嘛", "你在干什么", "你干嘛呢", "干嘛呢", "你是谁", "你是干嘛的", "你能做什么":
		return true
	default:
		return false
	}
}

func normalizeConversationalText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(text))
}

func buildSmalltalkFallback(question string) string {
	switch normalizeConversationalText(question) {
	case "在吗", "你在吗":
		return "在的，我一直在这里。你现在想聊点什么？"
	case "你好", "您好", "hi", "hello", "嗨", "哈喽":
		return "你好，我在这里。你可以和我聊九型人格、关系沟通，也可以直接问生活里的具体问题。"
	case "你是谁", "你是干嘛的", "你能做什么":
		return "我是九型问答里的成长陪伴助手，可以陪你聊天，也能结合人物画像帮你分析关系和具体问题。"
	default:
		return "我在这里，正在等你和我聊天。你现在想聊什么？"
	}
}

func buildGenerationUnavailableAnswer() string {
	return "我在，但刚才回答生成遇到了一点问题。你可以重新发一次，我会重新回答。"
}

func isExplicitEnneagramQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	for _, marker := range []string{
		"九型", "型号", "人格", "主型", "翼型",
		"完美型", "助人型", "成就型", "自我型", "艺术型", "观察型", "思想型",
		"忠诚型", "活跃型", "享乐型", "领袖型", "挑战型", "和平型", "调停型",
		"1号", "2号", "3号", "4号", "5号", "6号", "7号", "8号", "9号",
	} {
		if strings.Contains(question, marker) {
			return true
		}
	}
	return false
}

func isGenericConversationTerm(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if genericSearchTerms[term] {
		return true
	}
	runes := []rune(term)
	if len(runes) > 4 {
		return false
	}
	for _, r := range runes {
		if !strings.ContainsRune("你我他她它在有是的了吗嘛呢啊呀干做说想要么什怎哪谁好您好请问能会可", r) {
			return false
		}
	}
	return true
}

var genericSearchTerms = map[string]bool{
	"怎么": true, "什么": true, "如何": true, "需要": true, "可以": true,
	"这个": true, "那个": true, "一下": true, "告诉": true, "请问": true,
	"我们": true, "你们": true, "他们": true,
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

// buildSuggestions 用确定性规则生成 3 条追问建议：优先围绕命中的资料标题，
// 不足时用固定方向（关系 / 职场 / 亲密关系 / 成长练习）补齐。不触发任何模型调用。
func buildSuggestions(matches []scoredDoc) []string {
	const want = 3
	directions := []string{
		"在关系里我该怎么调整？",
		"在职场中我可以怎么发挥？",
		"亲密关系里怎么沟通更顺？",
		"适合我的成长练习是什么？",
	}

	suggestions := make([]string, 0, want)
	seen := map[string]bool{}
	add := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[text] || len(suggestions) >= want {
			return
		}
		seen[text] = true
		suggestions = append(suggestions, text)
	}

	for _, match := range matches {
		title := strings.TrimSpace(match.doc.Title)
		if title == "" {
			continue
		}
		add("想多了解“" + title + "”，能再展开讲讲吗？")
	}
	for _, dir := range directions {
		add(dir)
	}
	return suggestions
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
