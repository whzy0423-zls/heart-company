package voice

import "strings"

const defaultSentenceChunkerMinRunes = 10

// SentenceChunker 将流式文字切成可尽早送给 TTS 的有序句子。
type SentenceChunker struct {
	maxRunes int
	minRunes int
	buffer   []rune
}

func NewSentenceChunker(maxRunes int) *SentenceChunker {
	if maxRunes <= 0 {
		maxRunes = 42
	}
	minRunes := defaultSentenceChunkerMinRunes
	if minRunes > maxRunes {
		minRunes = maxRunes
	}
	return &SentenceChunker{maxRunes: maxRunes, minRunes: minRunes}
}

func (c *SentenceChunker) Push(delta string) []string {
	var chunks []string
	for _, r := range []rune(delta) {
		c.buffer = append(c.buffer, r)
		if isSentenceBoundary(r) && len(c.buffer) >= c.minRunes {
			if chunk := strings.TrimSpace(string(c.buffer)); chunk != "" {
				chunks = append(chunks, chunk)
			}
			c.buffer = c.buffer[:0]
			continue
		}
		if len(c.buffer) >= c.maxRunes {
			cut := c.naturalPauseCut()
			if chunk := strings.TrimSpace(string(c.buffer[:cut])); chunk != "" {
				chunks = append(chunks, chunk)
			}
			c.buffer = append(c.buffer[:0], c.buffer[cut:]...)
		}
	}
	return chunks
}

func (c *SentenceChunker) naturalPauseCut() int {
	minimum := c.minRunes - 1
	for index := len(c.buffer) - 1; index >= minimum; index-- {
		if isNaturalPause(c.buffer[index]) {
			return index + 1
		}
	}
	return c.maxRunes
}

func (c *SentenceChunker) Flush() []string {
	chunk := strings.TrimSpace(string(c.buffer))
	c.buffer = c.buffer[:0]
	if chunk == "" {
		return nil
	}
	return []string{chunk}
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '\n':
		return true
	default:
		return false
	}
}

func isNaturalPause(r rune) bool {
	if isSentenceBoundary(r) {
		return true
	}
	switch r {
	case '，', ',', '、', '：', ':':
		return true
	default:
		return false
	}
}
