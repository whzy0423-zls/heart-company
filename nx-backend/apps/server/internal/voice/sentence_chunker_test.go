package voice

import "testing"

func TestSentenceChunkerFlushesChinesePunctuationAndPreservesOrder(t *testing.T) {
	c := NewSentenceChunker(20)
	var got []string
	got = append(got, c.Push("我听见你了。接下来")...)
	got = append(got, c.Push("我们慢慢说！最后一句")...)
	got = append(got, c.Flush()...)

	want := []string{"我听见你了。", "接下来我们慢慢说！", "最后一句"}
	if len(got) != len(want) {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSentenceChunkerUsesSafeLengthForLongUnpunctuatedText(t *testing.T) {
	c := NewSentenceChunker(6)
	got := c.Push("一二三四五六七八")
	if len(got) != 1 || got[0] != "一二三四五六" {
		t.Fatalf("chunks = %#v", got)
	}
	if rest := c.Flush(); len(rest) != 1 || rest[0] != "七八" {
		t.Fatalf("rest = %#v", rest)
	}
}
