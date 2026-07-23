package privacy

import (
	"regexp"
	"strings"
)

var mainlandPhonePattern = regexp.MustCompile(`1[3-9][0-9]{9}`)

// MaskPhone returns a recognizable but non-sensitive representation of a phone value.
func MaskPhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isMaskedPhone(value) {
		return value
	}
	if mainlandPhonePattern.MatchString(value) && len(value) == 11 {
		return value[:3] + "****" + value[7:]
	}

	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[0]) + strings.Repeat("*", len(runes)-3) + string(runes[len(runes)-2:])
}

// MaskPhonesInText masks standalone mainland mobile numbers without changing other numbers.
func MaskPhonesInText(text string) string {
	indexes := mainlandPhonePattern.FindAllStringIndex(text, -1)
	if len(indexes) == 0 {
		return text
	}

	var masked strings.Builder
	masked.Grow(len(text))
	last := 0
	for _, index := range indexes {
		start, end := index[0], index[1]
		if (start > 0 && isASCIIDigit(text[start-1])) || (end < len(text) && isASCIIDigit(text[end])) {
			continue
		}
		masked.WriteString(text[last:start])
		masked.WriteString(MaskPhone(text[start:end]))
		last = end
	}
	masked.WriteString(text[last:])
	return masked.String()
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isMaskedPhone(value string) bool {
	if len(value) != 11 || value[0] != '1' || value[1] < '3' || value[1] > '9' {
		return false
	}
	if value[3:7] != "****" {
		return false
	}
	for _, index := range []int{1, 2, 7, 8, 9, 10} {
		if !isASCIIDigit(value[index]) {
			return false
		}
	}
	return true
}
