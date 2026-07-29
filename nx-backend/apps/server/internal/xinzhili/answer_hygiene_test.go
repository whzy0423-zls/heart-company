package xinzhili

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnswerHygieneRecognizesExplicitProductImplementationQuestions(t *testing.T) {
	tests := []struct {
		question string
		want     bool
	}{
		{question: "后台页面怎么配置", want: true},
		{question: "APP 的缓存怎么处理", want: true},
		{question: "这个 API 怎么调用", want: true},
		{question: "网站怎么开发", want: true},
		{question: "服务器怎么部署", want: true},
		{question: "APP 端怎么改主题？", want: true},
		{question: "后台为什么会刷新页面？", want: true},
		{question: "APP 是什么，和网站有什么区别？", want: true},
		{question: "能否添加一个接口？", want: true},
		{question: "内部实现是什么？", want: true},
		{question: "什么是 App 端？", want: true},
		{question: "这个接口是什么？", want: true},
		{question: "何为客户端？", want: true},
		{question: "How to configure this API?", want: true},
		{question: "What is this API?", want: true},
		{question: "What are client interfaces?", want: true},
		{question: "How do I deploy this server?", want: true},
		{question: "How can we integrate this SDK?", want: true},
		{question: "How should I debug this Android app?", want: true},
		{question: "How to implement this API?", want: true},
		{question: "How can I build this Android app?", want: true},
		{question: "How should we develop this website?", want: true},
		{question: "How do I fix this client?", want: true},
		{question: "How to call this API?", want: true},
		{question: "How should I cache this request?", want: true},
		{question: "How do I request this API?", want: true},
		{question: "这个页面是什么颜色，推荐几道菜", want: false},
		{question: "这个网站怎么这么好看，推荐几个菜", want: false},
		{question: "我喜欢这个软件，推荐几道菜", want: false},
		{question: "缓存的照片很好看，推荐几道菜", want: false},
		{question: "推荐几个开胃菜 appetizer", want: false},
		{question: "How beautiful is this website? Recommend some dishes.", want: false},
	}
	for _, test := range tests {
		if got := isExplicitTechnicalQuestion(test.question); got != test.want {
			t.Errorf("isExplicitTechnicalQuestion(%q)=%v want=%v", test.question, got, test.want)
		}
	}
}

func TestAnswerHygieneRecognizesProductEntityTitlesAndContextSentences(t *testing.T) {
	for _, title := range []string{"### App 端\n", "App 端：\n", "> 客户端："} {
		if !isProductMetaTitle(title) {
			t.Errorf("title %q must start product meta context", title)
		}
	}
	for _, sentence := range []string{"需要刷新状态。", "- 建议统一状态。", "需要处理配置。"} {
		if !isPureImplementationSentence(sentence) {
			t.Errorf("sentence %q must remain inside product meta context", sentence)
		}
	}
	if isPureImplementationSentence("建议番茄炒蛋。") {
		t.Fatal("a normal recommendation must release product meta context")
	}
}

func TestAnswerHygieneRecognizesMarkdownProductMetaPrefixes(t *testing.T) {
	for _, sentence := range []string{
		"### 页面需要先刷新状态。",
		"1. 接口建议统一返回格式。",
		"> 内部实现可以采用缓存。",
		"- 程序侧需要增加判断。",
		"建议从 App 端处理状态。",
		"对于 App 端而言，需要刷新页面。",
		"建议通过后台接口处理。",
		"这部分在客户端完成。",
	} {
		if !isProductMetaSentence(sentence) {
			t.Errorf("sentence %q must be filtered for a non-technical question", sentence)
		}
	}
	for _, sentence := range []string{"这个页面很好看。", "我在后台等你。", "这个 App 很漂亮。"} {
		if isProductMetaSentence(sentence) {
			t.Errorf("sentence %q has no implementation semantics", sentence)
		}
	}
}

func TestAnswerSentenceBufferWaitsForBoundaryAndEmitsFollowingAnswerSeparately(t *testing.T) {
	var buffer answerSentenceBuffer
	longMeta := "App 端需要" + strings.Repeat("统一内部状态和接口逻辑", 10)
	if got := buffer.Push(longMeta); len(got) != 0 {
		t.Fatalf("unterminated sentence emitted early: %q", got)
	}
	got := buffer.Push("。有效回答：番茄炒蛋。")
	want := []string{longMeta + "。", "有效回答：番茄炒蛋。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sentences=%q want=%q", got, want)
	}
}
