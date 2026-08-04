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
