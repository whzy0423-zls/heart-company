package rag

import "strings"

const (
	retrievalContextUserTurns = 2
	retrievalContextRunes     = 600
)

var contextualQuestionMarkers = []string{
	"刚才", "上面", "前面", "之前提到", "你说的", "那个", "这个", "这些",
	"第一个", "第二个", "第三个", "前一个", "后一个", "继续", "接着",
	"具体例子", "举个例子", "展开说", "详细说", "按这个", "按刚才", "那我", "那具体",
	"那他", "那她", "那它", "那这种", "那如果", "那为什么", "那怎么", "那应该",
	"那么我", "这种情况", "这种方式", "这件事", "再详细", "再具体", "再展开",
	"具体怎么", "具体该", "具体应该", "该怎么办", "怎么做呢", "为什么呢", "然后呢", "还有呢",
}

// BuildRetrievalQuery supplements context-dependent follow-ups with recent user
// questions. Assistant answers are intentionally excluded from retrieval evidence.
func BuildRetrievalQuery(question string, history []Message, summary string) string {
	question = strings.TrimSpace(question)
	if question == "" || !isContextDependentQuestion(question) {
		return question
	}

	recent := make([]string, 0, retrievalContextUserTurns)
	remaining := retrievalContextRunes
	for index := len(history) - 1; index >= 0 && len(recent) < retrievalContextUserTurns && remaining > 0; index-- {
		message := history[index]
		if strings.ToLower(strings.TrimSpace(message.Role)) != "user" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" || content == question {
			continue
		}
		content = trimRunes(content, remaining)
		remaining -= len([]rune(content))
		recent = append(recent, content)
	}
	for left, right := 0, len(recent)-1; left < right; left, right = left+1, right-1 {
		recent[left], recent[right] = recent[right], recent[left]
	}
	if len(recent) > 0 {
		return "前文用户问题：" + strings.Join(recent, "\n") + "\n当前问题：" + question
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		return "会话摘要：" + trimRunes(summary, retrievalContextRunes) + "\n当前问题：" + question
	}
	return question
}

func isContextDependentQuestion(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	for _, marker := range contextualQuestionMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
