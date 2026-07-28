package server

import "strings"

func appChatNginxStripComments(config string) string {
	var result strings.Builder
	result.Grow(len(config))
	var quote byte
	escaped := false
	for index := 0; index < len(config); index++ {
		char := config[index]
		if escaped {
			result.WriteByte(char)
			escaped = false
			continue
		}
		if quote != 0 {
			result.WriteByte(char)
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			result.WriteByte(char)
			continue
		}
		if char == '#' {
			for index < len(config) && config[index] != '\n' {
				index++
			}
			if index < len(config) {
				result.WriteByte('\n')
			}
			continue
		}
		result.WriteByte(char)
	}
	return result.String()
}

func appChatNginxLocationHasDirective(block, directive string) bool {
	open := strings.IndexByte(block, '{')
	close := strings.LastIndexByte(block, '}')
	if open < 0 || close <= open {
		return false
	}
	want := strings.Join(strings.Fields(directive), " ")
	body := block[open+1 : close]
	statementStart := 0
	var quote byte
	escaped := false
	for index := 0; index < len(body); index++ {
		char := body[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char != ';' {
			continue
		}
		statement := strings.Join(strings.Fields(body[statementStart:index]), " ")
		if statement+";" == want {
			return true
		}
		statementStart = index + 1
	}
	return false
}
