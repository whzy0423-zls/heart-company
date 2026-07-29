package xinzhili

import (
	"regexp"
	"strings"
	"unicode"
)

const neutralDirectAnswerFallback = "请再具体说一点，我会直接回答。"

var (
	englishTechnicalEntityPattern = regexp.MustCompile(`(?i)\b(app|client|backend|api|interface|page|website|server|sdk|flutter|android|ios|software|code|framework|request|cache)\b`)
	englishTechnicalActionPattern = regexp.MustCompile(`(?i)\b(configure|deploy|implement|build|develop|debug|fix|integrate|call|cache|request)\b`)
	englishTechnicalCuePattern    = regexp.MustCompile(`(?i)\bhow\s+(to|do|can|should)\b`)
)

var technicalQuestionEntities = []string{
	"app", "flutter", "android", "ios", "sdk", "api", "客户端", "后台", "页面", "网站", "小程序", "软件", "服务器", "算法", "接口", "代码", "框架", "网络请求", "缓存",
	"技术实现", "内部实现", "系统实现", "模型配置",
}

var technicalQuestionActions = []string{
	"开发", "实现", "设计", "部署", "接入", "调用", "配置", "调试", "修复", "优化", "编写", "架构", "报错", "错误", "网络请求", "缓存",
}

var technicalQuestionCues = []string{"怎么", "如何", "怎样", "为何", "请", "帮我"}

var productImplementationEntities = []string{
	"app端", "客户端", "后台", "接口", "页面", "技术实现", "基础框架", "模型配置", "内部实现", "程序侧", "系统实现",
}

var productImplementationSemantics = []string{
	"处理", "实现", "方案", "配置", "刷新", "状态", "完成", "接入", "调用", "部署", "框架", "需要", "建议",
}

func isExplicitTechnicalQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	if englishTechnicalCuePattern.MatchString(question) && englishTechnicalEntityPattern.MatchString(question) && englishTechnicalActionPattern.MatchString(question) {
		return true
	}
	normalized := compactAnswerPrefix(question)
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

func isProductMetaSentence(sentence string) bool {
	normalized := compactAnswerPrefix(stripAnswerListPrefix(sentence))
	return containsAnyTerm(normalized, productImplementationEntities) && containsAnyTerm(normalized, productImplementationSemantics)
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

type answerSentenceBuffer struct{ buffer []rune }

func (b *answerSentenceBuffer) Push(delta string) []string {
	b.buffer = append(b.buffer, []rune(delta)...)
	return b.take(false)
}

func (b *answerSentenceBuffer) Flush() []string { return b.take(true) }

func (b *answerSentenceBuffer) take(flush bool) []string {
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
