package xinzhili

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnswerHygieneRecognizesExplicitProductImplementationQuestions(t *testing.T) {
	for _, question := range []string{
		"后台页面怎么配置",
		"内部实现是什么",
		"APP 的缓存怎么处理",
		"这个 API 怎么调用",
		"网站怎么开发",
		"服务器怎么部署",
	} {
		if !isExplicitTechnicalQuestion(question) {
			t.Errorf("question %q must preserve its technical answer", question)
		}
	}
	if isExplicitTechnicalQuestion("推荐几个开胃菜 appetizer") {
		t.Fatal("an English word containing app must not be treated as a technical question")
	}
	for _, question := range []string{
		"这个页面很好看，推荐几道菜",
		"我喜欢这个网站，推荐几道菜",
		"这个软件挺好用，推荐几道菜",
		"缓存的照片很好看，推荐几道菜",
	} {
		if isExplicitTechnicalQuestion(question) {
			t.Errorf("question %q mentions a technical entity without asking a technical question", question)
		}
	}
}

func TestAnswerHygieneRecognizesMarkdownProductMetaPrefixes(t *testing.T) {
	for _, sentence := range []string{
		"### 页面需要先刷新状态。",
		"1. 接口建议统一返回格式。",
		"> 内部实现可以采用缓存。",
		"- 程序侧需要增加判断。",
	} {
		if !isProductMetaSentence(sentence) {
			t.Errorf("sentence %q must be filtered for a non-technical question", sentence)
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
