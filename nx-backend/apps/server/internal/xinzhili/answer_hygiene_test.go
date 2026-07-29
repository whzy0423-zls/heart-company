package xinzhili

import "testing"

func TestAnswerHygieneRecognizesExplicitProductImplementationQuestions(t *testing.T) {
	for _, question := range []string{
		"后台页面怎么配置",
		"内部实现是什么",
		"APP 的缓存怎么处理",
		"这个 API 怎么调用",
	} {
		if !isExplicitTechnicalQuestion(question) {
			t.Errorf("question %q must preserve its technical answer", question)
		}
	}
	if isExplicitTechnicalQuestion("推荐几个开胃菜 appetizer") {
		t.Fatal("an English word containing app must not be treated as a technical question")
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
