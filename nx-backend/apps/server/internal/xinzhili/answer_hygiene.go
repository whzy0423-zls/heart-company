package xinzhili

import (
	"regexp"
	"strings"
	"unicode"
)

const neutralDirectAnswerFallback = "请再具体说一点，我会直接回答。"

var englishTechnicalQuestionPattern = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(app|flutter|android|ios|sdk|api)([^a-z0-9_]|$)`)

var chineseTechnicalQuestionTerms = []string{
	"客户端", "后台", "页面", "网站", "小程序", "软件", "服务器", "部署", "算法", "接口", "代码", "框架", "网络请求", "缓存",
	"技术实现", "内部实现", "系统实现", "模型配置",
}

var productMetaSentencePrefixes = []string{
	"app端", "app 端", "客户端", "后台", "从后台", "接口", "从接口", "技术实现", "从技术实现", "内部实现", "系统实现",
	"页面", "从页面", "程序侧", "从程序侧", "基础框架", "从基础框架", "模型配置", "对app端", "对app 端", "对客户端", "在app端", "在app 端", "在客户端",
}

func isExplicitTechnicalQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	if englishTechnicalQuestionPattern.MatchString(question) {
		return true
	}
	for _, term := range chineseTechnicalQuestionTerms {
		if strings.Contains(question, term) {
			return true
		}
	}
	return false
}

func isProductMetaSentence(sentence string) bool {
	normalized := strings.ToLower(stripAnswerListPrefix(sentence))
	for _, prefix := range productMetaSentencePrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
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

func endsStrongSentence(value string) bool {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return false
	}
	for len(runes) > 0 && isClosingQuote(runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	return len(runes) > 0 && isStrongSentenceEndAt(runes, len(runes)-1)
}
