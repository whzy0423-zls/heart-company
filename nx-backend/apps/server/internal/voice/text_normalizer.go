package voice

import (
	"strings"
)

var chineseTTSDigits = [...]rune{'零', '一', '二', '三', '四', '五', '六', '七', '八', '九'}

// NormalizeChineseTTSInput prepares model text for Chinese speech providers.
// It keeps the displayed/saved assistant text unchanged at call sites, while
// making Arabic numerals read naturally in Chinese audio (for example "7号"
// becomes "七号"). Technical tokens with ASCII letters, such as GPT-4 and
// v1.2, are preserved because spelling them is usually intentional.
func NormalizeChineseTTSInput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	var builder strings.Builder
	builder.Grow(len(text))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if isASCIIDigitRune(r) {
			if asciiTokenAroundContainsLetter(runes, i) {
				builder.WriteRune(r)
				continue
			}
			end := i + 1
			for end < len(runes) && isASCIIDigitRune(runes[end]) {
				end++
			}
			if i > 0 && runes[i-1] == '.' && !asciiTokenAroundContainsLetter(runes, i-1) {
				builder.WriteString(chineseTTSDigitsOneByOne(runes[i:end]))
			} else {
				builder.WriteString(chineseTTSDigitSequence(runes[i:end]))
			}
			i = end - 1
			continue
		}
		if r == '.' && isDigitAt(runes, i-1) && isDigitAt(runes, i+1) && !asciiTokenAroundContainsLetter(runes, i) {
			builder.WriteRune('点')
			continue
		}
		if isRangeSeparatorForChineseTTS(r) && isDigitAt(runes, i-1) && isDigitAt(runes, i+1) && !asciiTokenAroundContainsLetter(runes, i) {
			builder.WriteRune('到')
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// NormalizeStrictChineseTTSInput prepares assistant speech for Chinese-only
// playback. It keeps the saved/displayed text unchanged at call sites while
// ensuring the text sent to TTS does not contain ASCII letters that may make
// providers suddenly switch to English pronunciation.
func NormalizeStrictChineseTTSInput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	normalized := NormalizeChineseTTSInput(replaceASCIILetterTokensForChineseSpeech(text))
	return strings.Join(strings.Fields(normalized), "")
}

func replaceASCIILetterTokensForChineseSpeech(text string) string {
	runes := []rune(text)
	var builder strings.Builder
	builder.Grow(len(text))
	for i := 0; i < len(runes); {
		if isStrictASCIITokenStartRune(runes[i]) {
			end := i + 1
			for end < len(runes) && isStrictASCIITokenRune(runes[end]) {
				end++
			}
			token := string(runes[i:end])
			if asciiTokenContainsLetter(token) {
				builder.WriteString(chineseSpeechReplacementForASCIIToken(token))
			} else {
				builder.WriteString(token)
			}
			i = end
			continue
		}
		builder.WriteRune(runes[i])
		i++
	}
	return builder.String()
}

func chineseSpeechReplacementForASCIIToken(token string) string {
	body, suffix := stripStrictTTSTokenSuffixPunctuation(token)
	lower := strings.ToLower(body)
	if lower == "" {
		return suffix
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
		return "链接" + suffix
	}
	if strings.Contains(lower, "@") {
		return "邮箱地址" + suffix
	}
	if replacement, ok := commonChineseSpeechTokenReplacement(lower); ok {
		return replacement + suffix
	}
	if strings.HasPrefix(lower, "gpt-") && len(body) > len("gpt-") {
		return "大语言模型" + body[len("gpt-"):] + suffix
	}
	if isVersionLikeASCIIToken(lower) {
		return "版本" + body[1:] + suffix
	}
	if strings.ContainsAny(body, "/_+&") {
		parts := strings.FieldsFunc(body, func(r rune) bool {
			switch r {
			case '/', '_', '+', '&':
				return true
			default:
				return false
			}
		})
		replacements := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			replacements = append(replacements, chineseSpeechReplacementForASCIIToken(part))
		}
		if len(replacements) > 0 {
			return strings.Join(replacements, "和") + suffix
		}
	}
	if strings.Contains(lower, "api") {
		version := trailingVersionDigits(body)
		if version != "" {
			return "接口版本" + version + suffix
		}
		return "接口" + suffix
	}
	if digits := asciiDigitsInToken(body); digits != "" {
		return "编号" + digits + suffix
	}
	return "英文内容" + suffix
}

func commonChineseSpeechTokenReplacement(lower string) (string, bool) {
	replacements := map[string]string{
		"ai":        "人工智能",
		"asr":       "语音识别",
		"tts":       "语音合成",
		"llm":       "大语言模型",
		"gpt":       "大语言模型",
		"ok":        "好",
		"okay":      "好",
		"yes":       "是的",
		"no":        "不是",
		"sorry":     "抱歉",
		"thanks":    "谢谢",
		"thank":     "谢谢",
		"hello":     "你好",
		"hi":        "你好",
		"worry":     "担心",
		"app":       "应用",
		"api":       "接口",
		"sdk":       "开发工具包",
		"email":     "邮箱",
		"mail":      "邮箱",
		"url":       "链接",
		"link":      "链接",
		"openai":    "人工智能服务",
		"minimax":   "语音模型",
		"dashscope": "语音服务",
		"cosyvoice": "语音模型",
	}
	replacement, ok := replacements[lower]
	return replacement, ok
}

func stripStrictTTSTokenSuffixPunctuation(token string) (string, string) {
	runes := []rune(token)
	suffix := ""
	for len(runes) > 0 {
		var replacement string
		switch runes[len(runes)-1] {
		case '.':
			replacement = "。"
		case ',':
			replacement = "，"
		case '!':
			replacement = "！"
		case '?':
			replacement = "？"
		case ';':
			replacement = "；"
		default:
			return string(runes), suffix
		}
		suffix = replacement + suffix
		runes = runes[:len(runes)-1]
	}
	return "", suffix
}

func isVersionLikeASCIIToken(lower string) bool {
	if len(lower) < 2 || lower[0] != 'v' {
		return false
	}
	hasDigit := false
	for _, r := range lower[1:] {
		if isASCIIDigitRune(r) {
			hasDigit = true
			continue
		}
		if r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return hasDigit
}

func trailingVersionDigits(token string) string {
	lower := strings.ToLower(token)
	idx := strings.LastIndex(lower, "v")
	if idx < 0 || idx+1 >= len(token) {
		return ""
	}
	candidate := token[idx+1:]
	for _, r := range candidate {
		if !isASCIIDigitRune(r) && r != '.' && r != '-' && r != '_' {
			return ""
		}
	}
	return candidate
}

func asciiDigitsInToken(token string) string {
	var builder strings.Builder
	for _, r := range token {
		if isASCIIDigitRune(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func asciiTokenContainsLetter(token string) bool {
	for _, r := range token {
		if isASCIILetterRune(r) {
			return true
		}
	}
	return false
}

func isASCIILetterRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isStrictASCIITokenStartRune(r rune) bool {
	return isASCIIDigitRune(r) || isASCIILetterRune(r)
}

func isStrictASCIITokenRune(r rune) bool {
	if isASCIITechnicalTokenRune(r) {
		return true
	}
	switch r {
	case '?', '=', '&', '#', '!', ',', ';':
		return true
	default:
		return false
	}
}

func chineseTTSDigitSequence(digits []rune) string {
	if len(digits) == 2 && digits[0] != '0' {
		tens := int(digits[0] - '0')
		ones := int(digits[1] - '0')
		switch {
		case tens == 1 && ones == 0:
			return "十"
		case tens == 1:
			return "十" + string(chineseTTSDigits[ones])
		case ones == 0:
			return string(chineseTTSDigits[tens]) + "十"
		default:
			return string(chineseTTSDigits[tens]) + "十" + string(chineseTTSDigits[ones])
		}
	}
	return chineseTTSDigitsOneByOne(digits)
}

func chineseTTSDigitsOneByOne(digits []rune) string {
	var builder strings.Builder
	builder.Grow(len(digits) * 3)
	for _, digit := range digits {
		builder.WriteRune(chineseTTSDigits[digit-'0'])
	}
	return builder.String()
}

func asciiTokenAroundContainsLetter(runes []rune, index int) bool {
	start := index
	for start > 0 && isASCIITechnicalTokenRune(runes[start-1]) {
		start--
	}
	end := index + 1
	for end < len(runes) && isASCIITechnicalTokenRune(runes[end]) {
		end++
	}
	for _, r := range runes[start:end] {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isASCIITechnicalTokenRune(r rune) bool {
	if isASCIIDigitRune(r) || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	switch r {
	case '.', '-', '_', '/', ':', '@', '%', '+':
		return true
	default:
		return false
	}
}

func isASCIIDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func isDigitAt(runes []rune, index int) bool {
	return index >= 0 && index < len(runes) && isASCIIDigitRune(runes[index])
}

func isRangeSeparatorForChineseTTS(r rune) bool {
	switch r {
	case '-', '~', '～':
		return true
	default:
		return false
	}
}
