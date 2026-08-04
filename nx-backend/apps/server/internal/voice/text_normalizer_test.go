package voice

import "testing"

func TestNormalizeChineseTTSInputReadsDigitsInChinese(t *testing.T) {
	got := NormalizeChineseTTSInput(" 我是7号，先做1次呼吸 ")
	if got != "我是七号，先做一次呼吸" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeChineseTTSInputKeepsTechnicalTokens(t *testing.T) {
	got := NormalizeChineseTTSInput("GPT-4 和 v1.2 先保留，版本3.14读中文。")
	want := "GPT-4 和 v1.2 先保留，版本三点一四读中文。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
