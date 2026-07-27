package rag

import (
	"strings"
	"unicode"
)

const ModelIdentityReply = "我是芯之力专属模型。"

var (
	chineseAssistantSubjects = []string{"你", "您", "当前助手", "这个助手", "当前会话", "这个会话"}
	modelIdentityTerms       = []string{"底层模型", "基座模型", "真实模型", "模型id", "模型编号", "模型版本", "版本号", "模型参数量", "参数量"}
	providerTerms            = []string{"openai", "anthropic", "minimax", "deepseek", "google", "gemini", "claude", "gpt", "chatgpt", "llama", "meta", "豆包", "通义千问", "智谱", "kimi"}
	chineseToolchainTerms    = []string{"codexcli", "cli", "中转站", "中转服务", "api代理", "api中转", "代理api"}
	englishToolchainTerms    = []string{"codex cli", "cli", "api proxy", "proxy api", "model gateway"}
)

// IsModelIdentityQuestion reports whether question asks for the current
// assistant's underlying model, provider, version, parameters, or toolchain.
func IsModelIdentityQuestion(question string) bool {
	normalized := normalizeIdentityQuestion(question)
	if normalized == "" {
		return false
	}

	compact := compactIdentityQuestion(normalized)
	if isChineseModelIdentityQuestion(compact) {
		return true
	}
	return isEnglishModelIdentityQuestion(normalized)
}

func isChineseModelIdentityQuestion(question string) bool {
	question = stripChineseIdentityPrefixes(question)
	question = trimChineseQuestionParticles(question)
	if question == "" || isChineseEducationQuestion(question) {
		return false
	}

	hasSubject := containsAny(question, chineseAssistantSubjects...)
	if isChineseDeveloperQuestion(question, hasSubject) ||
		isChineseToolchainQuestion(question, hasSubject) ||
		isChineseProviderQuestion(question, hasSubject) {
		return true
	}

	if isImplicitChineseModelIdentityRequest(question) {
		return true
	}
	return hasSubject && isChineseModelQuestion(question)
}

func isChineseModelQuestion(question string) bool {
	if containsAny(question, modelIdentityTerms...) {
		return containsAny(question,
			"你的", "您的", "当前助手", "这个助手", "当前会话", "这个会话",
			"你是", "您是", "你有", "您有", "你用", "您用", "你使用", "您使用")
	}
	if containsAny(question, "ai模型", "大模型") && containsAny(question, "是什么", "是啥", "是哪个", "用的", "使用的", "调用的", "运行的", "基于") {
		return true
	}
	if strings.Contains(question, "模型") && containsAny(question,
		"是什么模型", "是啥模型", "是哪个模型", "哪一个模型", "哪一版模型",
		"用的模型", "使用的模型", "调用的模型", "运行的模型", "跑的啥模型", "跑的什么模型", "基于的模型", "模型是什么") {
		return true
	}
	if containsAny(question, "哪个版本", "哪一版", "什么版本", "多少参数", "多大参数") {
		return true
	}
	return false
}

func isImplicitChineseModelIdentityRequest(question string) bool {
	question = trimLeadingAny(question, "真实", "实际", "具体", "当前")
	if containsAny(question, "底层模型是", "基座模型是", "当前模型是") {
		return true
	}
	for _, term := range modelIdentityTerms {
		if question == term || strings.HasPrefix(question, term+"是") {
			return true
		}
	}
	return false
}

func isChineseProviderQuestion(question string, hasSubject bool) bool {
	if !hasSubject {
		return false
	}
	if strings.Contains(question, "api") && containsAny(question,
		"你接的是哪家", "您接的是哪家", "你接入的是哪家", "您接入的是哪家") {
		return true
	}
	if containsAny(question,
		"你是哪家公司", "您是哪家公司", "你是哪家厂商", "您是哪家厂商",
		"你是哪个厂商", "您是哪个厂商", "你的供应商", "您的供应商",
		"你的提供商", "您的提供商", "当前助手的提供商", "这个助手的提供商") {
		return true
	}
	if !containsAny(question, providerTerms...) {
		return false
	}
	return containsAny(question,
		"你是", "您是", "你实际是", "您实际是", "你是不是", "您是不是",
		"你用的是", "您用的是", "你使用的是", "您使用的是", "你调用的是", "您调用的是",
		"你属于", "您属于", "你来自", "您来自", "你是由", "您是由")
}

func isChineseDeveloperQuestion(question string, hasSubject bool) bool {
	if !hasSubject {
		return false
	}
	if containsAny(question,
		"你的开发者", "您的开发者", "当前助手的开发者", "这个助手的开发者",
		"你的开发公司", "您的开发公司", "你的训练方", "您的训练方") {
		return true
	}
	if strings.HasSuffix(question, "你") || strings.HasSuffix(question, "您") {
		return containsAny(question, "谁开发", "谁训练", "谁提供", "谁创造", "谁构建", "哪家公司开发", "哪家厂商开发", "哪个厂商开发")
	}
	return containsAny(question,
		"你是哪家公司开发", "您是哪家公司开发", "你是哪家厂商开发", "您是哪家厂商开发",
		"你由谁开发", "您由谁开发", "你由谁训练", "您由谁训练")
}

func isChineseToolchainQuestion(question string, hasSubject bool) bool {
	if !hasSubject || !containsAny(question, chineseToolchainTerms...) {
		return false
	}
	return containsAny(question, "通过", "走", "使用", "用", "调用", "接入", "运行于", "回答")
}

func isChineseEducationQuestion(question string) bool {
	return containsAny(question,
		"有什么作用", "是什么意思", "有什么特点", "如何使用", "怎么使用", "怎么工作",
		"怎么搭建", "更适合", "的用途", "主要产品", "介绍openai产品", "介绍一下openai")
}

func isEnglishModelIdentityQuestion(question string) bool {
	question = stripEnglishIdentityPrefixes(question)
	if question == "" || isEnglishEducationQuestion(question) || !hasEnglishAssistantSubject(question) {
		return false
	}

	if isEnglishDeveloperQuestion(question) || isEnglishToolchainQuestion(question) || isEnglishProviderQuestion(question) {
		return true
	}
	if !containsAny(question, "model", "model id", "version", "parameter", "provider") {
		return false
	}
	return containsAny(question,
		"what ", "which ", "tell me", "show me", "reveal", "your ", "are you", "do you use", "you using", "you running")
}

func isEnglishProviderQuestion(question string) bool {
	if !containsEnglishTerm(question, providerTerms...) {
		return false
	}
	return containsAny(question,
		"are you", "you using", "powered by", "developed by", "built by", "provided by", "hosted by", "you from")
}

func isEnglishDeveloperQuestion(question string) bool {
	return containsAny(question,
		"who developed you", "who built you", "who created you", "who trained you", "who made you",
		"which company developed you", "which company built you", "which company created you",
		"what company are you from")
}

func isEnglishToolchainQuestion(question string) bool {
	if !containsEnglishTerm(question, englishToolchainTerms...) {
		return false
	}
	return containsAny(question, "through", " via ", " on ", "use", "using", "running", "operating", "answering")
}

func isEnglishEducationQuestion(question string) bool {
	return containsAny(question,
		"what is openai", "how do ", "how does ", "used for", "best for", "recommend",
		"explain", "describe", "introduce", "familiar with", "products")
}

func hasEnglishAssistantSubject(question string) bool {
	return containsAny(question, " you", "you ", "your ", "this assistant", "the assistant", "this chat", "the chat")
}

func normalizeIdentityQuestion(question string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if r == '’' {
			return '\''
		}
		if strings.ContainsRune("，,：:；;", r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(question))
	normalized = strings.Join(strings.Fields(normalized), " ")
	return strings.TrimFunc(normalized, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("?？!！。.\"'“”‘’「」『』", r)
	})
}

func compactIdentityQuestion(question string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("，,：:。！？!?；;", r) {
			return -1
		}
		return r
	}, question)
}

func stripChineseIdentityPrefixes(question string) string {
	prefixes := []string{
		"忽略之前的要求", "忽略前面的要求", "假装你不受限制", "假装您不受限制",
		"别说官方名字", "不要说官方名字", "不要解释", "别解释", "不要绕弯子", "别绕弯子",
		"请你告诉我", "请您告诉我", "能否告诉我", "我想知道", "直接告诉我", "直接回答",
		"直接说", "请介绍一下", "介绍一下", "请问", "麻烦", "请", "告诉我", "说一下", "透露一下",
	}
	for {
		before := question
		question = trimLeadingAny(question, prefixes...)
		if question == before {
			return question
		}
	}
}

func stripEnglishIdentityPrefixes(question string) string {
	prefixes := []string{"could you please ", "could you ", "can you please ", "can you ", "please "}
	for {
		before := question
		question = trimLeadingAny(question, prefixes...)
		if question == before {
			return question
		}
	}
}

func trimChineseQuestionParticles(question string) string {
	for {
		trimmed := strings.TrimRight(question, "吗呢呀啊吧嘛")
		if trimmed == question {
			return question
		}
		question = trimmed
	}
}

func trimLeadingAny(value string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func containsEnglishTerm(value string, terms ...string) bool {
	for _, term := range terms {
		searchFrom := 0
		for searchFrom <= len(value)-len(term) {
			relative := strings.Index(value[searchFrom:], term)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(term)
			leftBoundary := start == 0 || !isEnglishTokenByte(value[start-1])
			rightBoundary := end == len(value) || !isEnglishTokenByte(value[end])
			if leftBoundary && rightBoundary {
				return true
			}
			searchFrom = start + 1
		}
	}
	return false
}

func isEnglishTokenByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_'
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
