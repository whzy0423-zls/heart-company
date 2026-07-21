package voice

import "strings"

// SentenceChunker 将流式文字切成可尽早送给 TTS 的有序句子。
type SentenceChunker struct {
	maxRunes int
	buffer   []rune
}

func NewSentenceChunker(maxRunes int) *SentenceChunker {
	if maxRunes <= 0 {
		maxRunes = 42
	}
	return &SentenceChunker{maxRunes: maxRunes}
}

func (c *SentenceChunker) Push(delta string) []string {
	var chunks []string
	for _, r := range []rune(delta) {
		c.buffer = append(c.buffer, r)
		if isSentenceBoundary(r) || len(c.buffer) >= c.maxRunes {
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
