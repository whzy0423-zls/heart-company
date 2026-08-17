package voice

import (
	"strings"
	"testing"
)

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

func TestNormalizeStrictChineseTTSInputPreservesParagraphPauses(t *testing.T) {
	got := NormalizeStrictChineseTTSInput("第一段说完。\n\n第二段慢慢说。")
	want := "第一段说完。\n第二段慢慢说。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeMandarinPronunciationTTSInputKeepsDisplayTextRulesSeparate(t *testing.T) {
	visible := NormalizeStrictChineseTTSInput("噫吁嚱，危乎高哉！")
	if visible != "噫吁嚱，危乎高哉！" {
		t.Fatalf("strict visible text got %q", visible)
	}

	spoken := NormalizeMandarinPronunciationTTSInput("噫吁嚱，危乎高哉！")
	if spoken != "衣虚兮，微乎高哉！" {
		t.Fatalf("spoken got %q", spoken)
	}
}

func TestNormalizeMandarinPronunciationTTSInputNormalizesShuDaoNanHardWords(t *testing.T) {
	got := NormalizeMandarinPronunciationTTSInput("蚕丛及鱼凫，猿猱欲度愁攀援。扪参历井仰胁息，以手抚膺坐长叹。巉岩不可攀。飞湍瀑流争喧豗，砯崖转石万壑雷。剑阁峥嵘而崔嵬。磨牙吮血，侧身西望长咨嗟。")
	want := "蚕丛及鱼扶，元挠欲度，愁攀援。门申历井仰协息，以手抚英坐长叹。馋岩不可攀。飞湍铺流争喧灰，乒崖转石万贺雷。剑阁征荣而崔围。磨牙顺血，侧身西望长资接。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	for _, hard := range []string{"嚱", "凫", "猱", "扪参", "胁", "膺", "巉", "豗", "砯", "壑", "嵘", "嵬", "吮", "嗟"} {
		if strings.Contains(got, hard) {
			t.Fatalf("spoken text still contains hard token %q in %q", hard, got)
		}
	}
	assertNoASCIILetters(t, got)
}

func TestNormalizeMandarinPronunciationTTSInputAddsShuDaoNanProsodyBreaks(t *testing.T) {
	got := NormalizeMandarinPronunciationTTSInput("下有冲波逆折之回川。黄鹤之飞尚不得过，猿猱欲度愁攀援。")
	want := "下有冲波，逆折之回川。黄鹤之飞，尚不得过，元挠欲度，愁攀援。"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func assertNoASCIILetters(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			t.Fatalf("strict Chinese TTS text still contains ASCII letter %q in %q", r, text)
		}
	}
}
