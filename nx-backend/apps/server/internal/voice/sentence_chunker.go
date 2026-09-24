package voice

import "strings"

const defaultSentenceChunkerMinRunes = 10

// SentenceChunker 将流式文字切成可尽早送给 TTS 的有序句子。
type SentenceChunker struct {
	maxRunes      int
	minRunes      int
	firstMaxRunes int
	buffer        []rune
	emittedChunk  bool
}

func NewSentenceChunker(maxRunes int) *SentenceChunker {
	return NewSentenceChunkerWithMin(maxRunes, defaultSentenceChunkerMinRunes)
}

// NewSentenceChunkerWithMin lets a latency-sensitive caller start a complete
// short sentence earlier without changing the default realtime TTS cadence.
func NewSentenceChunkerWithMin(maxRunes, minRunes int) *SentenceChunker {
	return NewSentenceChunkerWithMinAndFirst(maxRunes, minRunes, 0)
}

// NewSentenceChunkerWithMinAndFirst uses a smaller limit for the first
// unpunctuated chunk, allowing latency-sensitive TTS to start while the model
// is still generating the rest of an answer. Subsequent chunks use maxRunes.
func NewSentenceChunkerWithMinAndFirst(maxRunes, minRunes, firstMaxRunes int) *SentenceChunker {
	if maxRunes <= 0 {
		maxRunes = 42
	}
	if minRunes <= 0 {
		minRunes = defaultSentenceChunkerMinRunes
	}
	if minRunes > maxRunes {
		minRunes = maxRunes
	}
	if firstMaxRunes > 0 {
		if firstMaxRunes < minRunes {
			firstMaxRunes = minRunes
		}
		if firstMaxRunes > maxRunes {
			firstMaxRunes = maxRunes
		}
	}
	return &SentenceChunker{maxRunes: maxRunes, minRunes: minRunes, firstMaxRunes: firstMaxRunes}
}

func (c *SentenceChunker) Push(delta string) []string {
	var chunks []string
	for _, r := range []rune(delta) {
		c.buffer = append(c.buffer, r)
		if isSentenceBoundary(r) && len(c.buffer) >= c.minRunes {
			if chunk := strings.TrimSpace(string(c.buffer)); chunk != "" {
				chunks = append(chunks, chunk)
				c.emittedChunk = true
			}
			c.buffer = c.buffer[:0]
			continue
		}
		limit := c.maxRunes
		if !c.emittedChunk && c.firstMaxRunes > 0 {
			limit = c.firstMaxRunes
		}
		if len(c.buffer) >= limit {
			cut := c.naturalPauseCut(limit)
			if chunk := strings.TrimSpace(string(c.buffer[:cut])); chunk != "" {
				chunks = append(chunks, chunk)
				c.emittedChunk = true
			}
			c.buffer = append(c.buffer[:0], c.buffer[cut:]...)
		}
	}
	return chunks
}

func (c *SentenceChunker) naturalPauseCut(limit int) int {
	minimum := c.minRunes - 1
	for index := limit - 1; index >= minimum; index-- {
		if isNaturalPause(c.buffer[index]) {
			return index + 1
		}
	}
	return limit
}

func (c *SentenceChunker) Flush() []string {
	chunk := strings.TrimSpace(string(c.buffer))
	c.buffer = c.buffer[:0]
	if chunk == "" {
		return nil
	}
	c.emittedChunk = true
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
