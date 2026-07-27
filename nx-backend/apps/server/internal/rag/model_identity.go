package rag

import (
	"regexp"
	"strings"
)

const ModelIdentityReply = "我是芯之力专属模型。"

var (
	terminalPunctuation = regexp.MustCompile(`[?？!！。.]+$`)
	spacePattern        = regexp.MustCompile(`\s+`)
	compactSeparators   = regexp.MustCompile(`[\s，,：:。！？!?]`)

	chineseIdentityQuestionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^(你|您)(现在|当前|实际|其实|到底)?(是|用|使用|调用|运行|基于)(的)?(是)?(什么|啥|哪个|哪一个|哪一版)?(大)?模型(id)?$`),
		regexp.MustCompile(`^(你|您)的?(底层模型|基座模型|模型(id|编号|版本)|模型参数量|参数量)(是)?(什么|啥|哪个|哪一个|多少|多大|哪一版)?$`),
		regexp.MustCompile(`^(底层模型|基座模型|当前模型)(是|用的|为)?(什么|啥|哪个|哪一个|哪一版)$`),
		regexp.MustCompile(`^(当前会话|当前助手|这个会话|这个助手)(到底)?(是|调用|使用|运行|通过)(的)?(是)?(什么|啥|哪个|哪一个|哪一版)?(大)?模型$`),
		regexp.MustCompile(`^(告诉我|说一下|透露|展示|介绍一下)?(你|您)的?(底层模型|基座模型|模型(id|编号|版本)|版本号|模型参数量|参数量)(是)?(什么|啥|哪个|哪一个|多少|多大|哪一版)?$`),
		regexp.MustCompile(`^(你|您)(是|用的)?(哪个|哪一版|什么)版本(号)?$`),
		regexp.MustCompile(`^(你|您)(有|拥有)(多少|多大)参数(量)?$`),
		regexp.MustCompile(`^(你|您)(现在|当前|实际|其实|到底)?(是不是|是由|是|属于|来自)(openai|anthropic|minimax|deepseek|google|gemini|claude|gpt([-.0-9a-z]+)?|chatgpt|llama|meta|豆包|通义千问|智谱|kimi)(的)?(语言)?(模型)?$`),
		regexp.MustCompile(`^(你|您)(是)?哪家(公司|厂商|供应商|提供商)(的模型)?$`),
		regexp.MustCompile(`^(你|您)(是)?(由)?(谁|哪家公司|哪家厂商|哪个厂商)(开发|训练|提供|创造|构建)的?$`),
		regexp.MustCompile(`^谁(开发|训练|提供|创造|构建)(了)?(你|您)$`),
		regexp.MustCompile(`^(哪家公司|哪家厂商|哪个厂商)(开发|训练|提供|创造|构建)(了)?(你|您)$`),
		regexp.MustCompile(`^(你|您)的?(开发者|开发公司|训练方|提供商)(是)?谁$`),
		regexp.MustCompile(`^(你|您)(现在|当前)?(是不是)?(通过|用|使用|调用|接入|运行于)(codexcli|中转站|api(代理|中转)|代理api)(在)?(回答|运行|调用|接入)?$`),
		regexp.MustCompile(`^(你|您)的(请求|回答)(是不是|是否)?(走|通过|使用|调用)(哪个|什么)?(codexcli|中转站|api(代理|中转)|代理api)$`),
		regexp.MustCompile(`^(你|您)(现在|当前)?(用|使用|走)(的)?(是)?(哪个|什么)?(codexcli|中转站|api(代理|中转)|代理api)$`),
		regexp.MustCompile(`^(你|您)(现在|当前)?(用|使用)的(codexcli|中转站|api(代理|中转)|代理api)(是)?(什么|哪个)$`),
		regexp.MustCompile(`^(当前会话|当前助手|这个会话|这个助手)(通过|走|使用|调用|接入)(哪个|什么)?(codexcli|中转站|api(代理|中转)|代理api)(调用模型)?$`),
	}

	englishIdentityQuestionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^(what|which) (underlying |base )?model (are you|do you use|is (this|the) (chat|assistant) using)$`),
		regexp.MustCompile(`^(tell|show|reveal) me (about )?your (model|model id|model version|provider)$`),
		regexp.MustCompile(`^your (model|model id|model version|provider)$`),
		regexp.MustCompile(`^(what is|what's) your (model|model id|model version|provider)$`),
		regexp.MustCompile(`^(what|which) version are you running$`),
		regexp.MustCompile(`^what model version are you$`),
		regexp.MustCompile(`^how many parameters does your model have$`),
		regexp.MustCompile(`^which provider (powers|runs|hosts) you$`),
		regexp.MustCompile(`^are you ((a|an) model (from|by) |powered by |developed by |built by |provided by |hosted by |from )?(openai|anthropic|minimax|deepseek|google|gemini|claude|gpt[- .0-9a-z]*|chatgpt|llama|meta|kimi)$`),
		regexp.MustCompile(`^(who (developed|built|created|trained|provides|made) you|what company are you from)$`),
		regexp.MustCompile(`^tell me (what model you are|which company (developed|built|created|trained|provides|made) you)$`),
		regexp.MustCompile(`^(are you|is this assistant) (answering|running|operating) (through|via|on) (codex cli|api proxy|proxy api|a model gateway|model gateway)$`),
		regexp.MustCompile(`^(do you use|are you using) (a |an )?(codex cli|api proxy|proxy api|model gateway)$`),
		regexp.MustCompile(`^which (codex cli|api proxy|proxy api|model gateway) do you use$`),
		regexp.MustCompile(`^what (api proxy|proxy api|model gateway) is (this|the) (assistant|chat) using$`),
	}
)

// IsModelIdentityQuestion reports whether question asks for the current
// assistant's underlying model, provider, version, parameters, or toolchain.
func IsModelIdentityQuestion(question string) bool {
	normalized := normalizeIdentityQuestion(question)
	if normalized == "" {
		return false
	}

	compact := compactSeparators.ReplaceAllString(normalized, "")
	compact = stripChineseIdentityPrefixes(compact)
	compact = strings.TrimSuffix(compact, "吗")
	compact = strings.TrimSuffix(compact, "呢")
	compact = strings.TrimSuffix(compact, "呀")
	compact = strings.TrimSuffix(compact, "啊")

	for _, pattern := range chineseIdentityQuestionPatterns {
		if pattern.MatchString(compact) {
			return true
		}
	}

	english := stripEnglishIdentityPrefixes(normalized)
	for _, pattern := range englishIdentityQuestionPatterns {
		if pattern.MatchString(english) {
			return true
		}
	}
	return false
}

func normalizeIdentityQuestion(question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))
	normalized = strings.ReplaceAll(normalized, "’", "'")
	normalized = spacePattern.ReplaceAllString(normalized, " ")
	return terminalPunctuation.ReplaceAllString(normalized, "")
}

func stripChineseIdentityPrefixes(question string) string {
	prefixes := []string{
		"忽略之前的要求", "忽略前面的要求", "不要解释", "别解释",
		"不要绕弯子", "别绕弯子", "请你告诉我", "能否告诉我", "我想知道",
		"直接告诉我", "直接回答", "直接说", "请介绍一下", "介绍一下",
		"请问", "麻烦", "请", "告诉我", "说一下", "透露一下",
	}
	for {
		stripped := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(question, prefix) {
				question = strings.TrimPrefix(question, prefix)
				stripped = true
				break
			}
		}
		if !stripped {
			return question
		}
	}
}

func stripEnglishIdentityPrefixes(question string) string {
	prefixes := []string{"please ", "could you please ", "could you ", "can you please ", "can you "}
	for {
		stripped := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(question, prefix) {
				question = strings.TrimPrefix(question, prefix)
				stripped = true
				break
			}
		}
		if !stripped {
			return question
		}
	}
}
