package articlestore

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStripMarkdownRemovesSyntax(t *testing.T) {
	md := "# 标题\n\n这是**加粗**和*斜体*，还有[链接](https://x.com)。\n\n> 引用\n\n- 列表项\n\n`code` 和\n```\nblock\n```\n"
	got := stripMarkdown(md)
	for _, marker := range []string{"#", "**", "](http", ">", "```"} {
		if strings.Contains(got, marker) {
			t.Fatalf("stripped text still contains %q: %q", marker, got)
		}
	}
	if !strings.Contains(got, "标题") || !strings.Contains(got, "加粗") || !strings.Contains(got, "链接") {
		t.Fatalf("stripped text lost content: %q", got)
	}
	if strings.Contains(got, "block") {
		t.Fatalf("code block content should be dropped: %q", got)
	}
}

func TestSplitForTTSRespectsLimit(t *testing.T) {
	// 构造超过单片上限的文本。
	para := strings.Repeat("这是一段用于测试的中文文本。", 50) // 远小于 limit 的一段
	text := strings.Repeat(para+"\n", 40)               // 整体远超 limit
	chunks := splitForTTS(text, 1000)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > 1000 {
			t.Fatalf("chunk %d exceeds limit: %d runes", i, n)
		}
		if strings.TrimSpace(c) == "" {
			t.Fatalf("chunk %d is empty", i)
		}
	}
}

func TestSplitForTTSHardCutsLongSentence(t *testing.T) {
	// 没有句末标点的超长串，必须被硬切。
	text := strings.Repeat("字", 3000)
	chunks := splitForTTS(text, 500)
	if len(chunks) < 6 {
		t.Fatalf("expected >=6 hard-cut chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > 500 {
			t.Fatalf("chunk %d exceeds limit: %d runes", i, n)
		}
	}
}
