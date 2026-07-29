package xinzhili

import (
	"regexp"
	"strings"
	"unicode"
)

const neutralDirectAnswerFallback = "请再具体说一点，我会直接回答。"

var englishTechnicalQuestionPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(app|flutter|android|ios|sdk|api)([^a-z0-9_]|$)`)

var chineseTechnicalEntities = []string{
	"客户端", "后台", "页面", "网站", "小程序", "软件", "服务器", "算法", "接口", "代码", "框架", "网络请求", "缓存",
	"技术实现", "内部实现", "系统实现", "模型配置",
}

var technicalQuestionActions = []string{
	"开发", "实现", "设计", "部署", "接入", "调用", "配置", "调试", "修复", "优化", "编写", "架构", "怎么", "如何", "报错", "是什么",
}

var productMetaSentencePrefixes = []string{
	"app端", "客户端", "后台", "从后台", "接口", "从接口", "技术实现", "从技术实现", "内部实现", "系统实现",
	"页面", "从页面", "程序侧", "从程序侧", "基础框架", "从基础框架", "模型配置", "对app端", "对客户端", "在app端", "在客户端",
	"针对当前app端", "针对app端", "对于客户端", "在后台", "整体技术实现",
}

func isExplicitTechnicalQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	hasEntity := englishTechnicalQuestionPattern.MatchString(question)
	if !hasEntity {
		for _, entity := range chineseTechnicalEntities {
			if strings.Contains(question, entity) {
				hasEntity = true
				break
			}
		}
	}
	if !hasEntity {
		return false
	}
	for _, action := range technicalQuestionActions {
		if strings.Contains(question, action) {
			return true
		}
	}
	return false
}

func isProductMetaSentence(sentence string) bool {
	normalized := compactAnswerPrefix(stripAnswerListPrefix(sentence))
	for _, prefix := range productMetaSentencePrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
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
