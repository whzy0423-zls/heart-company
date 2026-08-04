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

func TestNormalizeStrictChineseTTSInputRewritesEnglishDriftForChineseSpeech(t *testing.T) {
	got := NormalizeStrictChineseTTSInput(" OK，我明白了。你是7号，不需要 worry。Sorry，AI 和 TTS 会用 GPT-4。 ")
	want := "好，我明白了。你是七号，不需要担心。抱歉，人工智能和语音合成会用大语言模型四。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	assertNoASCIILetters(t, got)
}

func TestNormalizeStrictChineseTTSInputHidesLinksEmailsAndVersionTokens(t *testing.T) {
	got := NormalizeStrictChineseTTSInput("请看 https://example.com 和 test@example.com，模型 v1.2 也不要直读英文。")
	want := "请看链接和邮箱地址，模型版本一点二也不要直读英文。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	assertNoASCIILetters(t, got)
}

func assertNoASCIILetters(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			t.Fatalf("strict Chinese TTS text still contains ASCII letter %q in %q", r, text)
		}
	}
}
