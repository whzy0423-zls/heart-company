package rag

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	Nickname string `json:"nickname"`
	MainType int    `json:"mainType"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AskInput struct {
	History     []Message   `json:"history"`
	Question    string      `json:"question"`
	UserProfile UserProfile `json:"userProfile"`
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

type GenerateInput struct {
	History     []Message   `json:"history"`
	Question    string      `json:"question"`
	Sources     []Source    `json:"sources"`
	UserProfile UserProfile `json:"userProfile"`
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

	matches := s.search(question, input.UserProfile.MainType, 4)
	if len(matches) == 0 {
		// 检索未命中：仍尝试让 AI 结合九型常识作答（Sources 为空）；
		// 只有 AI 不可用或返回空时，才回退到固定兜底文案。
		if s.generator != nil {
			generated, err := s.generator.Generate(ctx, GenerateInput{
				History:     cleanHistory(input.History, 6),
				Question:    question,
				Sources:     nil,
				UserProfile: input.UserProfile,
			})
			if err == nil && strings.TrimSpace(generated) != "" {
				return Answer{
					Answer:      strings.TrimSpace(generated),
					Sources:     []Source{},
					Suggestions: buildSuggestions(nil),
				}, nil
			}
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
	if input.UserProfile.MainType > 0 {
		answer += "结合你最近的主型结果，"
	}
	answer += strings.Join(parts, "；") + "。你可以继续追问具体关系、职场、亲密关系或成长练习，我会沿着这些资料继续细化。"

	if s.generator != nil {
		generated, err := s.generator.Generate(ctx, GenerateInput{
			History:     cleanHistory(input.History, 6),
			Question:    question,
			Sources:     sources,
			UserProfile: input.UserProfile,
		})
		if err == nil && strings.TrimSpace(generated) != "" {
			return Answer{Answer: strings.TrimSpace(generated), Sources: sources, Suggestions: suggestions}, nil
		}
	}

	return Answer{Answer: answer, Sources: sources, Suggestions: suggestions}, nil
}

type scoredDoc struct {
	doc   Document
	score int
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
		score := 0
		for _, term := range terms {
			if term == "" {
				continue
			}
			if strings.Contains(text, term) {
				score += 3
			}
			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), term) {
					score += 2
				}
			}
		}
		if mainTypeToken != "" && (strings.Contains(doc.ID, "type-"+mainTypeToken) || strings.Contains(doc.Title, mainTypeToken+"号")) {
			score += 2
		}
		if score > 0 {
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
