package answerhygiene

import (
	"regexp"
	"strings"
	"unicode"
)

const NeutralDirectAnswerFallback = "请再具体说一点，我会直接回答。"

var (
	englishTechnicalEntityPattern = regexp.MustCompile(`(?i)\b(apps?|clients?|backends?|apis?|interfaces?|pages?|websites?|servers?|sdks?|flutter|android|ios|software|codes?|frameworks?|requests?|caches?)\b`)
	englishTechnicalActionPattern = regexp.MustCompile(`(?i)\b(configure|deploy|implement|build|develop|debug|fix|integrate|call|cache|request)\b`)
	englishTechnicalCuePattern    = regexp.MustCompile(`(?i)\bhow\s+(to|do|can|should)\b`)
	englishDefinitionCuePattern   = regexp.MustCompile(`(?i)\bwhat\s+(is|are)\b`)
)

var technicalQuestionEntities = []string{
	"app端", "app", "flutter", "android", "ios", "sdk", "api", "客户端", "后台", "页面", "网站", "小程序", "软件", "服务器", "算法", "接口", "代码", "框架", "网络请求", "缓存",
	"技术实现", "内部实现", "系统实现", "模型配置",
}

var technicalQuestionActions = []string{
	"开发", "实现", "设计", "部署", "接入", "调用", "配置", "调试", "修复", "优化", "编写", "架构", "报错", "错误", "网络请求", "缓存",
	"改", "修改", "调整", "加", "添加", "删除", "升级", "刷新",
}

var technicalQuestionCues = []string{"怎么", "如何", "怎样", "为何", "为什么", "能否", "请", "帮我"}

var productImplementationEntities = []string{
	"app端", "客户端", "后台", "接口", "页面", "技术实现", "基础框架", "模型配置", "内部实现", "程序侧", "系统实现",
}

var productImplementationSemantics = []string{
	"处理", "实现", "方案", "配置", "刷新", "状态", "完成", "接入", "调用", "部署", "框架", "需要", "建议",
}

func IsExplicitTechnicalQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	if englishTechnicalCuePattern.MatchString(question) && englishTechnicalEntityPattern.MatchString(question) && englishTechnicalActionPattern.MatchString(question) {
		return true
	}
	if englishDefinitionCuePattern.MatchString(question) && englishTechnicalEntityPattern.MatchString(question) {
		return true
	}
	normalized := compactAnswerPrefix(question)
	if isChineseTechnicalDefinitionQuestion(normalized) {
		return true
	}
	entityPositions := termPositions(normalized, technicalQuestionEntities)
	actionPositions := termPositions(normalized, technicalQuestionActions)
	if len(entityPositions) == 0 || len(actionPositions) == 0 {
		return false
	}
	cuePositions := termPositions(normalized, technicalQuestionCues)
	for _, entity := range entityPositions {
		for _, action := range actionPositions {
			if absInt(entity-action) > 18 {
				continue
			}
			if hasTermAt(normalized, action, []string{"报错", "错误"}) {
				return true
			}
			for _, cue := range cuePositions {
				if absInt(cue-action) <= 18 || absInt(cue-entity) <= 18 {
					return true
				}
			}
		}
	}
	return false
}

func isChineseTechnicalDefinitionQuestion(value string) bool {
	value = strings.TrimRight(value, "?？。！!；;")
	for _, clause := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '，' || r == ',' || r == '；' || r == ';'
	}) {
		if isChineseTechnicalDefinitionClause(clause) {
			return true
		}
	}
	return false
}

func isChineseTechnicalDefinitionClause(value string) bool {
	for _, entity := range technicalQuestionEntities {
		searchFrom := 0
		for searchFrom < len(value) {
			relative := strings.Index(value[searchFrom:], entity)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(entity)
			prefix, suffix := value[:start], value[end:]
			if (definitionCueNearEntity(prefix, "什么是") || definitionCueNearEntity(prefix, "何为")) && isDefinitionQuestionTail(suffix) {
				return true
			}
			if strings.HasPrefix(suffix, "是什么") && isDefinitionQuestionTail(strings.TrimPrefix(suffix, "是什么")) {
				return true
			}
			searchFrom = end
		}
	}
	return false
}

func definitionCueNearEntity(prefix, cue string) bool {
	index := strings.LastIndex(prefix, cue)
	if index < 0 {
		return false
	}
	return len([]rune(prefix[index+len(cue):])) <= 8
}

func isDefinitionQuestionTail(value string) bool {
	switch value {
	case "", "呢", "吗", "呀", "啊":
		return true
	default:
		return false
	}
}

func termPositions(value string, terms []string) []int {
	runes := []rune(value)
	positions := make([]int, 0, len(terms))
	for _, term := range terms {
		termRunes := []rune(term)
		if len(termRunes) == 0 || len(termRunes) > len(runes) {
			continue
		}
		for index := 0; index+len(termRunes) <= len(runes); index++ {
			if string(runes[index:index+len(termRunes)]) == term {
				positions = append(positions, index)
			}
		}
	}
	return positions
}

func hasTermAt(value string, position int, terms []string) bool {
	runes := []rune(value)
	for _, term := range terms {
		termRunes := []rune(term)
		if position >= 0 && position+len(termRunes) <= len(runes) && string(runes[position:position+len(termRunes)]) == term {
			return true
		}
	}
	return false
}

func IsProductMetaSentence(sentence string) bool {
	normalized := compactAnswerPrefix(stripAnswerListPrefix(sentence))
	return containsAnyTerm(normalized, productImplementationEntities) && containsAnyTerm(normalized, productImplementationSemantics)
}

func IsProductMetaTitle(sentence string) bool {
	raw := strings.TrimSpace(sentence)
	markdownCandidate := strings.TrimSpace(strings.TrimLeft(raw, ">"))
	isMarkdownHeading := strings.HasPrefix(markdownCandidate, "#")
	isColonHeading := strings.HasSuffix(raw, ":") || strings.HasSuffix(raw, "：")
	if !isMarkdownHeading && !isColonHeading {
		return false
	}
	normalized := compactAnswerPrefix(stripAnswerListPrefix(raw))
	normalized = strings.Trim(normalized, ":：-—–")
	if isColonHeading && len([]rune(normalized)) > 24 {
		return false
	}
	return containsAnyTerm(normalized, productImplementationEntities)
}

func IsPureImplementationSentence(sentence string) bool {
	normalized := compactAnswerPrefix(stripAnswerListPrefix(sentence))
	matched := 0
	for _, semantic := range productImplementationSemantics {
		if strings.Contains(normalized, semantic) {
			matched++
		}
	}
	return matched >= 2
}

func containsAnyTerm(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func compactAnswerPrefix(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, value)
}

func stripAnswerListPrefix(value string) string {
	value = strings.TrimSpace(value)
	for value != "" {
		previous := value
		value = strings.TrimLeftFunc(value, func(r rune) bool {
			switch r {
			case '-', '*', '+', '•', '·', '—', '–', '>', '#', '「', '『', '“', '"', '\'', '`':
				return true
			default:
				return unicode.IsSpace(r)
			}
		})
		value = strings.TrimLeftFunc(value, func(r rune) bool {
			return unicode.IsDigit(r) || r == '.' || r == '、' || r == ')' || r == '）' || unicode.IsSpace(r)
		})
		if value == previous {
			break
		}
	}
	return value
}

type SentenceBuffer struct{ buffer []rune }

func (b *SentenceBuffer) Push(delta string) []string {
	b.buffer = append(b.buffer, []rune(delta)...)
	return b.take(false)
}

func (b *SentenceBuffer) Flush() []string { return b.take(true) }

func (b *SentenceBuffer) take(flush bool) []string {
	var sentences []string
	for len(b.buffer) > 0 {
		cut := 0
		for index := range b.buffer {
			if !isStrongSentenceEndAt(b.buffer, index) || isNumberedListMarkerPeriod(b.buffer, index) {
				continue
			}
			cut = index + 1
			for cut < len(b.buffer) && isClosingQuote(b.buffer[cut]) {
				cut++
			}
			break
		}
		if cut == 0 && flush {
			cut = len(b.buffer)
		}
		if cut == 0 {
			break
		}
		if sentence := strings.TrimSpace(string(b.buffer[:cut])); sentence != "" {
			sentences = append(sentences, sentence)
		}
		b.buffer = trimLeftSpaceRunes(b.buffer[cut:])
	}
	return sentences
}

func isNumberedListMarkerPeriod(value []rune, index int) bool {
	if index < 0 || index >= len(value) || value[index] != '.' || index+1 >= len(value) || !unicode.IsSpace(value[index+1]) {
		return false
	}
	prefix := strings.TrimSpace(string(value[:index]))
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isStrongSentenceEndAt(value []rune, index int) bool {
	r := value[index]
	switch r {
	case '。', '！', '？', '!', '?', ';', '；', '\n':
		return true
	case '…':
		return index+1 >= len(value) || value[index+1] != '…'
	case '.':
		if index+1 < len(value) && value[index+1] == '.' {
			return false
		}
		if index > 0 && index+1 < len(value) && isASCIIAlphaNumeric(value[index-1]) && isASCIIAlphaNumeric(value[index+1]) {
			return false
		}
		if isInitialismBeforePeriod(value, index) && nextNonSpaceIsAlphaNumeric(value, index+1) {
			return false
		}
		return true
	default:
		return false
	}
}

func isASCIIAlphaNumeric(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isInitialismBeforePeriod(value []rune, period int) bool {
	start := period - 1
	for start >= 0 && (isASCIIAlphaNumeric(value[start]) || value[start] == '.') {
		start--
	}
	token := value[start+1 : period]
	if len(token) < 3 {
		return false
	}
	parts := strings.Split(string(token), ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len([]rune(part)) != 1 || !isASCIIAlphaNumeric([]rune(part)[0]) {
			return false
		}
	}
	return true
}

func nextNonSpaceIsAlphaNumeric(value []rune, start int) bool {
	for start < len(value) && unicode.IsSpace(value[start]) {
		start++
	}
	return start < len(value) && isASCIIAlphaNumeric(value[start])
}

func isClosingQuote(r rune) bool {
	switch r {
	case '”', '’', '"', '\'', '」', '』', '》', '）', ')', '】', ']':
		return true
	default:
		return false
	}
}

func trimLeftSpaceRunes(value []rune) []rune {
	for len(value) > 0 && unicode.IsSpace(value[0]) {
		value = value[1:]
	}
	return value
}

// Clean removes unsolicited product-implementation meta commentary while
// preserving direct answers. Explicit technical questions are returned intact.
func Clean(question, answer string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return NeutralDirectAnswerFallback
	}
	if IsExplicitTechnicalQuestion(question) {
		return answer
	}

	var buffer SentenceBuffer
	sentences := buffer.Push(answer)
	sentences = append(sentences, buffer.Flush()...)
	kept := make([]string, 0, len(sentences))
	productMetaContext := false
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		if IsProductMetaTitle(sentence) {
			productMetaContext = true
			continue
		}
		if productMetaContext {
			if IsPureImplementationSentence(sentence) || IsProductMetaSentence(sentence) {
				continue
			}
			productMetaContext = false
		}
		if IsProductMetaSentence(sentence) {
			continue
		}
		kept = append(kept, sentence)
	}
	cleaned := strings.TrimSpace(strings.Join(kept, ""))
	if cleaned == "" {
		return NeutralDirectAnswerFallback
	}
	return cleaned
}
