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
		if len(c.buffer) >= c.maxRunes || isSentenceBoundary(r) && len(c.buffer) >= c.minRunes {
			if chunk := strings.TrimSpace(string(c.buffer)); chunk != "" {
				chunks = append(chunks, chunk)
			}
			c.buffer = c.buffer[:0]
		}
	}
	return chunks
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
