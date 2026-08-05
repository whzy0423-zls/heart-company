package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func TestAskPrioritizesConversationCardMainType(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-2", Title: "2号 助人型", Content: "助人型重视关系和被需要。"},
		{ID: "type-6", Title: "6号 忠诚型", Content: "忠诚型重视安全和确定。"},
	})
	var input AskInput
	if err := json.Unmarshal([]byte(`{
		"question":"助人型在关系里该怎么和她沟通？",
		"userProfile":{"mainType":6},
		"conversationCard":{"name":"妈妈","relation":"家人","mainType":2,"wingType":1}
	}`), &input); err != nil {
		t.Fatal(err)
	}

	result, err := service.Ask(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) == 0 || result.Sources[0].ID != "type-2" {
		t.Fatalf("expected conversation card type-2 to lead retrieval, got %+v", result.Sources)
	}
	if !strings.Contains(result.Answer, "当前关注对象的主型") {
		t.Fatalf("fallback answer must describe the card subject, got %q", result.Answer)
	}
}

func TestSearchDoesNotUseMainTypeAsOnlyRelevanceEvidence(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-6", Title: "6号 忠诚型", Content: "忠诚型重视安全和确定。"},
	})

	matches := service.search("推荐几道菜以及具体做法", 6, 4)
	if len(matches) != 0 {
		t.Fatalf("main type alone must not select unrelated documents: %+v", matches)
	}
}

func TestSearchUsesMainTypeAsBoostAfterQuestionMatch(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-2", Title: "2号 助人型", Content: "关系沟通需要尊重彼此边界。"},
		{ID: "type-6", Title: "6号 忠诚型", Content: "关系沟通需要尊重彼此边界。"},
	})

	matches := service.search("关系沟通需要注意什么", 6, 4)
	if len(matches) != 2 || matches[0].doc.ID != "type-6" {
		t.Fatalf("expected textual matches with type-6 boosted first, got %+v", matches)
	}
}

func TestAskRecipeQuestionDoesNotAttachEnneagramSources(t *testing.T) {
	generator := &fakeGenerator{answer: "可以做番茄炒蛋：先炒蛋，再炒番茄，最后合炒调味。"}
	service := NewService([]Document{
		{ID: "type-6", Title: "6号 忠诚型", Content: "忠诚型面对决策时可能担心风险，重视安全和确定。"},
	}, WithGenerator(generator))

	answer, err := service.Ask(context.Background(), AskInput{
		Question:    "推荐几道菜以及具体做法",
		UserProfile: UserProfile{MainType: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Sources) != 0 || len(generator.input.Sources) != 0 {
		t.Fatalf("recipe request must not receive enneagram sources: answer=%+v generator=%+v", answer.Sources, generator.input.Sources)
	}
}

func TestAskSuggestionsStayOnCurrentCardTypeAndRecentFocus(t *testing.T) {
	service := NewService([]Document{
		{
			ID:      "type-4-understood",
			Title:   "共享词汇像通用货币：关系中的关键词需要重新定义",
			Content: "4号在关系中很在意自己的感受是否被真正理解，也容易因表达落差感到失落。",
		},
	}, WithGenerator(&fakeGenerator{answer: "先说结论。"}))

	answer, err := service.Ask(context.Background(), AskInput{
		Question: "我最近总觉得不被理解，为什么会这样？",
		ConversationCard: ConversationCard{
			CardType: "primary",
			Name:     "本人",
			MainType: 4,
			WingType: 5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Suggestions) != 3 {
		t.Fatalf("suggestions = %+v, want 3", answer.Suggestions)
	}
	for _, suggestion := range answer.Suggestions {
		if !strings.Contains(suggestion, "4号") {
			t.Fatalf("suggestion must stay on current type 4: %q", suggestion)
		}
		if strings.Contains(suggestion, "共享词汇") || strings.Contains(suggestion, "通用货币") {
			t.Fatalf("suggestion leaked an unrelated document title: %q", suggestion)
		}
	}
	if !containsSuggestionText(answer.Suggestions, "不被理解") {
		t.Fatalf("suggestions must follow the user's recent focus: %+v", answer.Suggestions)
	}
}

func TestAskSuggestionsUseRecentRelevantHistoryForShortContinuation(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "可以继续往下看。"}))

	answer, err := service.Ask(context.Background(), AskInput{
		Question: "那我该怎么办？",
		History: []Message{
			{Role: "user", Content: "我最近在关系里总觉得别人不理解我"},
			{Role: "assistant", Content: "这种落差可能会让你先退回自己的感受里。"},
		},
		ConversationCard: ConversationCard{CardType: "primary", Name: "本人", MainType: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Suggestions) != 3 {
		t.Fatalf("suggestions = %+v, want 3", answer.Suggestions)
	}
	for _, suggestion := range answer.Suggestions {
		if !strings.Contains(suggestion, "4号") {
			t.Fatalf("suggestion must stay on current type 4: %q", suggestion)
		}
	}
	if !containsSuggestionText(answer.Suggestions, "不理解") {
		t.Fatalf("suggestions must reuse the recent relevant topic: %+v", answer.Suggestions)
	}
}

func TestAskUnrelatedQuestionDoesNotProduceEnneagramSuggestions(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "先热锅，再炒鸡蛋。"}))

	answer, err := service.Ask(context.Background(), AskInput{
		Question:    "番茄炒蛋怎么做？",
		UserProfile: UserProfile{MainType: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Suggestions) != 0 {
		t.Fatalf("unrelated question suggestions = %+v, want empty", answer.Suggestions)
	}
}

func TestAskUnrelatedCurrentQuestionDoesNotReuseOlderEnneagramFocus(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "可以准备清淡一点。"}))

	for _, question := range []string{"妈妈今天想吃什么？", "番茄炒蛋怎么做？", "那晚饭吃什么？", "那明天天气呢？"} {
		t.Run(question, func(t *testing.T) {
			answer, err := service.Ask(context.Background(), AskInput{
				Question: question,
				History: []Message{
					{Role: "user", Content: "我最近在关系里总觉得别人不理解我"},
					{Role: "assistant", Content: "这可能和你的4号模式有关。"},
				},
				ConversationCard: ConversationCard{MainType: 4},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(answer.Suggestions) != 0 {
				t.Fatalf("unrelated current question suggestions = %+v, want empty", answer.Suggestions)
			}
		})
	}
}

func TestAskSuggestionsRequireCurrentConversationCardType(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "先说结论。"}))

	answer, err := service.Ask(context.Background(), AskInput{
		Question:    "我最近总觉得不被理解，为什么会这样？",
		UserProfile: UserProfile{MainType: 6},
		History: []Message{
			{Role: "assistant", Content: "这可能和你的4号模式有关。"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Suggestions) != 0 {
		t.Fatalf("suggestions without a current card type = %+v, want empty", answer.Suggestions)
	}
}

func TestAskSuggestionsDoNotReuseRecentTopicForAnotherType(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "可以继续往下看。"}))

	answer, err := service.Ask(context.Background(), AskInput{
		Question: "那我该怎么办？",
		History: []Message{
			{Role: "user", Content: "6号忠诚型为什么总担心没有安全感？"},
			{Role: "assistant", Content: "这和6号对风险的关注有关。"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Suggestions) != 0 {
		t.Fatalf("suggestions reused another type's topic = %+v, want empty", answer.Suggestions)
	}
}

func TestAskSuggestionsDoNotMixAnotherExplicitTypeIntoCurrentQuestion(t *testing.T) {
	service := NewService(nil, WithGenerator(&fakeGenerator{answer: "先区分两种模式。"}))

	for _, question := range []string{
		"6号忠诚型为什么总担心没有安全感？",
		"4号自我型和6号忠诚型有什么区别？",
	} {
		t.Run(question, func(t *testing.T) {
			answer, err := service.Ask(context.Background(), AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(answer.Suggestions) != 0 {
				t.Fatalf("suggestions mixed another explicit type = %+v, want empty", answer.Suggestions)
			}
		})
	}
}

func TestSuggestionsReplaceOlderFocusWhenCurrentQuestionChangesTopic(t *testing.T) {
	input := AskInput{
		Question: "我在工作中很害怕被否定，该怎么看？",
		History: []Message{
			{Role: "user", Content: "我总觉得不被理解"},
			{Role: "assistant", Content: "我们先看这种感受。"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}

	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 {
		t.Fatalf("suggestions = %+v, want 3", suggestions)
	}
	if !containsSuggestionText(suggestions, "工作中很害怕被否定") {
		t.Fatalf("suggestions did not use the current topic: %+v", suggestions)
	}
	if containsSuggestionText(suggestions, "不被理解") {
		t.Fatalf("suggestions retained the replaced topic: %+v", suggestions)
	}
}

func TestSuggestionsSkipRecentHistoryExplicitlyAboutAnotherType(t *testing.T) {
	input := AskInput{
		Question: "那我该怎么办？",
		History: []Message{
			{Role: "user", Content: "作为4号，我总觉得不被理解"},
			{Role: "assistant", Content: "先看见这种感受。"},
			{Role: "user", Content: "6号为什么总担心失去安全感？"},
			{Role: "assistant", Content: "这是6号常见的风险关注。"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}

	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 {
		t.Fatalf("suggestions = %+v, want 3", suggestions)
	}
	if !containsSuggestionText(suggestions, "不被理解") {
		t.Fatalf("suggestions did not inherit the nearest same-type topic: %+v", suggestions)
	}
	if containsSuggestionText(suggestions, "安全感") || containsSuggestionText(suggestions, "6号") {
		t.Fatalf("suggestions reused another type's recent topic: %+v", suggestions)
	}
}

func TestSuggestionsRequireValidConversationCardMainType(t *testing.T) {
	for _, mainType := range []int{-1, 0, 10} {
		t.Run(fmt.Sprintf("mainType_%d", mainType), func(t *testing.T) {
			input := AskInput{
				Question:            "我总觉得不被理解",
				UserProfile:         UserProfile{MainType: 4},
				ConversationSummary: "4号最近很在意是否被理解。",
				History: []Message{
					{Role: "user", Content: "作为4号，我总觉得不被理解"},
				},
				ConversationCard: ConversationCard{MainType: mainType},
			}
			if suggestions := buildSuggestions(input); len(suggestions) != 0 {
				t.Fatalf("suggestions with invalid current-card type = %+v, want empty", suggestions)
			}
		})
	}
}

func TestSuggestionsRejectBroadOrNonEnneagramTopics(t *testing.T) {
	questions := []string{
		"明天上海天气怎么样？",
		"Flutter 状态管理怎么选择？",
		"这周工作排期怎么安排？",
		"关系数据库的索引应该怎么设计？",
		"我该如何做这个选择？",
		"这段关系的编号是什么？",
		"4号地铁几点开？",
		"4号桌订单是什么？",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			input := AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}
			if suggestions := buildSuggestions(input); len(suggestions) != 0 {
				t.Fatalf("non-Enneagram topic produced suggestions: %+v", suggestions)
			}
		})
	}
}

func TestSuggestionsRejectTechnicalContextsContainingPsychologicalSubstrings(t *testing.T) {
	questions := []string{
		"Flutter 压力测试怎么做？",
		"API 边界条件怎么处理？",
		"HTTP 请求被拒绝怎么办？",
		"TCP 连接被拒绝怎么排查？",
		"内存压力怎么分析？",
		"CSS 边界溢出怎么修？",
		"我的服务器压力很大",
		"内存压力很大",
		"系统压力很大",
		"情绪识别 API 怎么调用？",
		"人格测试代码怎么写？",
		"安全感量表数据库如何设计？",
		"情绪分析代码怎么写？",
		"性格分类 API 怎么设计？",
		"人格画像数据库如何存储？",
		"压力锅怎么用？",
		"压力表怎么读？",
		"压力容器怎么检查？",
		"情绪识别算法怎么写？",
		"情绪识别模型怎么训练？",
		"情绪分类器怎么调用？",
		"压力计怎么读？",
		"压力传感器怎么用？",
		"情绪识别系统怎么搭建？",
		"情绪识别程序怎么开发？",
		"情绪分析平台怎么使用？",
		"我的情绪识别系统怎么搭建？",
		"我的人格测试代码怎么写？",
		"我的性格分类器怎么训练？",
		"我的情绪识别方案怎么实现？",
		"我的人格识别服务怎么做？",
		"我的性格分析模块如何接入？",
		"我在做情绪识别方案怎么实现？",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}); len(suggestions) != 0 {
				t.Fatalf("technical context produced suggestions: %+v", suggestions)
			}
		})
	}
}

func TestSuggestionsKeepPsychologicalConcernInsideTechnicalWorkContext(t *testing.T) {
	questions := []string{
		"做 Flutter 开发时我害怕被同事否定",
		"负责 API 项目让我压力很大且担心不认可",
		"做 Flutter 开发时我不敢表达真实意见",
		"我在 API 团队里总觉得被忽略",
		"人格测试结果让我焦虑",
		"情绪识别项目中我害怕被否定",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if len(suggestions) != 3 {
				t.Fatalf("mixed technical and psychological context suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				if !strings.Contains(suggestion, "4号自我型") {
					t.Fatalf("suggestion did not use current type 4: %q", suggestion)
				}
			}
		})
	}
}

func TestSuggestionsKeepPersonalPsychologicalContexts(t *testing.T) {
	questions := []string{
		"我压力很大",
		"我总觉得边界守不住",
		"我害怕被拒绝",
		"我的情绪很低落",
		"我的压力很大",
		"我对自己的性格很困扰",
		"我的情绪识别能力很差怎么办",
		"我想提升自己的情绪识别能力",
		"我在关系里对情绪识别很迟钝",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}); len(suggestions) != 3 {
				t.Fatalf("personal psychological context suggestions = %+v, want 3", suggestions)
			}
		})
	}
}

func TestSuggestionsRecognizeCurrentPsychologicalPatterns(t *testing.T) {
	questions := []string{
		"我最近压力很大，该怎么看？",
		"我不明白自己真正的动机是什么",
		"为什么我总是出现同样的惯性反应？",
		"我不知道怎么守住自己的边界",
		"边界冲突让我很困扰",
		"我总在回避冲突，怎么调整？",
		"我在压力下会反复迎合别人",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if len(suggestions) != 3 {
				t.Fatalf("psychological topic suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				if !strings.Contains(suggestion, "4号") {
					t.Fatalf("suggestion must explicitly use the current type: %q", suggestion)
				}
			}
		})
	}
}

func TestSuggestionsRecognizeExplicitEnneagramTypeNameButNotBareNumber(t *testing.T) {
	positive := []string{
		"4号自我型在关系里通常怎么看自己？",
		"自我型在关系里通常怎么看自己？",
	}
	for _, question := range positive {
		t.Run(question, func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}); len(suggestions) != 3 {
				t.Fatalf("explicit Enneagram type name suggestions = %+v, want 3", suggestions)
			}
		})
	}
}

func TestSuggestionsDoNotTreatOrdinaryNumbersAsAnotherEnneagramType(t *testing.T) {
	questions := []string{
		"坐4号地铁让我压力很大",
		"4号桌的安排让我很焦虑",
		"8月4号开会让我很焦虑",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 6},
			})
			if len(suggestions) != 3 {
				t.Fatalf("ordinary-number psychological topic suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				if !strings.Contains(suggestion, "6号忠诚型") {
					t.Fatalf("suggestion did not use current type 6: %q", suggestion)
				}
				if strings.Contains(suggestion, "4号自我型") {
					t.Fatalf("ordinary number was treated as type 4: %q", suggestion)
				}
			}
		})
	}
}

func TestSuggestionsStillTreatPsychologicalNumberAsAnotherEnneagramType(t *testing.T) {
	input := AskInput{
		Question:         "4号在压力下为什么会反复退缩？",
		ConversationCard: ConversationCard{MainType: 6},
	}
	if suggestions := buildSuggestions(input); len(suggestions) != 0 {
		t.Fatalf("explicit other-type psychological topic suggestions = %+v, want empty", suggestions)
	}
}

func TestSuggestionsRecognizeNumberedTypeWithStrongContextAfterIntermediateWords(t *testing.T) {
	questions := []string{
		"4号有哪些性格特点？",
		"4号的典型人格表现是什么？",
	}
	for _, question := range questions {
		t.Run(question+"_other_type", func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 6},
			}); len(suggestions) != 0 {
				t.Fatalf("strong type context was not treated as another type: %+v", suggestions)
			}
		})

		t.Run(question+"_current_type", func(t *testing.T) {
			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if len(suggestions) != 3 {
				t.Fatalf("current numbered type suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				if !strings.Contains(suggestion, "4号自我型") {
					t.Fatalf("suggestion did not use current type 4: %q", suggestion)
				}
			}
		})
	}
}

func TestSuggestionsRecognizeExpandedExplicitTypeFormats(t *testing.T) {
	otherTypeQuestions := []string{
		"第六型为何总缺乏安全感？",
		"6型为何总缺乏安全感？",
		"Type 6 为何总缺乏安全感？",
		"６号为什么总缺乏安全感？",
		"六号型为何总缺乏安全感？",
		"疑惑型为何总缺乏安全感？",
		"忠诚型为何总缺乏安全感？",
		"享乐型为何总追逐新的可能？",
		"挑战型为何总想掌控局面？",
	}
	for _, question := range otherTypeQuestions {
		t.Run(question, func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}); len(suggestions) != 0 {
				t.Fatalf("expanded other-type format produced current-type suggestions: %+v", suggestions)
			}
		})
	}

	for _, question := range []string{"浪漫型为何很在意被理解？", "4号浪漫型为何很在意被理解？"} {
		t.Run(question, func(t *testing.T) {
			if suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			}); len(suggestions) != 3 {
				t.Fatalf("current-type alias suggestions = %+v, want 3", suggestions)
			}
		})
	}
}

func TestEnneagramTypeFromTextRecognizesExpandedFormatsAndAliases(t *testing.T) {
	tests := map[string]int{
		"第六型":    6,
		"6型":     6,
		"Type 6": 6,
		"６号为什么":  6,
		"六号型":    6,
		"浪漫型":    4,
		"4号浪漫型":  4,
		"疑惑型":    6,
		"忠诚型":    6,
		"享乐型":    7,
		"挑战型":    8,
	}
	for text, want := range tests {
		t.Run(text, func(t *testing.T) {
			if got := enneagramTypeFromText(text); got != want {
				t.Fatalf("enneagramTypeFromText(%q) = %d, want %d", text, got, want)
			}
		})
	}
}

func TestEnneagramTypeFromTextRejectsEmbeddedOrMultiDigitFormats(t *testing.T) {
	for _, text := range []string{
		"A6型号", "第16型设备", "Type 16", "Type 60",
		"6型号", "第6型号", "六型号", "6型设备", "Type 6A",
	} {
		t.Run(text, func(t *testing.T) {
			if got := enneagramTypeFromText(text); got != 0 {
				t.Fatalf("enneagramTypeFromText(%q) = %d, want 0", text, got)
			}
		})
	}
}

func TestSuggestionsKeepOrdinaryNumberInNonComparisonHistory(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "我和同事坐4号地铁时很焦虑"},
			{Role: "assistant", Content: "这段经历让你很紧张。"},
		},
		ConversationCard: ConversationCard{MainType: 6},
	}
	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 || !containsSuggestionText(suggestions, "焦虑") {
		t.Fatalf("ordinary number history did not stay with current type: %+v", suggestions)
	}
	for _, suggestion := range suggestions {
		if !strings.Contains(suggestion, "6号忠诚型") {
			t.Fatalf("suggestion did not use current type 6: %q", suggestion)
		}
	}
}

func TestSuggestionsKeepAdjacentOrdinaryNumbersInCurrentTypeHistory(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "坐4号和6号地铁时我很焦虑"},
			{Role: "assistant", Content: "这段经历让你很紧张。"},
		},
		ConversationCard: ConversationCard{MainType: 6},
	}
	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 || !containsSuggestionText(suggestions, "焦虑") {
		t.Fatalf("adjacent ordinary-number history did not stay with current type: %+v", suggestions)
	}
	for _, suggestion := range suggestions {
		if !strings.Contains(suggestion, "6号忠诚型") {
			t.Fatalf("suggestion did not use current type 6: %q", suggestion)
		}
	}
}

func TestSuggestionsTreatAdjacentTypeNumbersAsMixedHistoryBarrier(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "4号和6号在安全感上有什么区别？"},
			{Role: "user", Content: "这种区别让我很焦虑"},
		},
		ConversationCard: ConversationCard{MainType: 6},
	}
	if suggestions := buildSuggestions(input); len(suggestions) != 0 {
		t.Fatalf("true adjacent type comparison did not create a mixed barrier: %+v", suggestions)
	}
}

func TestSuggestionsDoNotInheritImplicitContinuationFromAnotherTypeChain(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "6号为何缺安全感？"},
			{Role: "assistant", Content: "先看见对风险的担忧。"},
			{Role: "user", Content: "这种不安让我很难受"},
			{Role: "assistant", Content: "这份不安确实很消耗。"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}
	if suggestions := buildSuggestions(input); len(suggestions) != 0 {
		t.Fatalf("implicit other-type chain was inherited: %+v", suggestions)
	}
}

func TestSuggestionsSkipAnotherTypeChainAndRestoreEarlierCurrentTypeFocus(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "4号为什么总觉得不被理解？"},
			{Role: "assistant", Content: "先看见表达与理解之间的落差。"},
			{Role: "user", Content: "6号为何缺安全感？"},
			{Role: "assistant", Content: "先看见对风险的担忧。"},
			{Role: "user", Content: "这种不安让我很难受"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}
	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 || !containsSuggestionText(suggestions, "不被理解") {
		t.Fatalf("did not restore earlier current-type focus: %+v", suggestions)
	}
	if containsSuggestionText(suggestions, "不安") || containsSuggestionText(suggestions, "安全感") {
		t.Fatalf("suggestions leaked another-type chain: %+v", suggestions)
	}
}

func TestSuggestionsDoNotAssignMixedTypeHistoryChainToCurrentType(t *testing.T) {
	comparisons := []string{
		"4号和6号在压力下有什么差异？",
		"四号与六号在压力下有什么差异？",
		"4号、6号在压力下有什么差异？",
	}
	for _, comparison := range comparisons {
		t.Run(comparison, func(t *testing.T) {
			input := AskInput{
				Question: "那怎么办？",
				History: []Message{
					{Role: "user", Content: comparison},
					{Role: "assistant", Content: "两种模式关注的重点不同。"},
					{Role: "user", Content: "这种差异让我焦虑"},
				},
				ConversationCard: ConversationCard{MainType: 4},
			}
			if suggestions := buildSuggestions(input); len(suggestions) != 0 {
				t.Fatalf("mixed-type history chain was assigned to current type: %+v", suggestions)
			}
		})
	}
}

func TestSuggestionsDoNotRestoreOlderFocusAcrossMixedTypeHistoryChain(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "4号为什么总觉得不被理解？"},
			{Role: "assistant", Content: "先看见表达与理解之间的落差。"},
			{Role: "user", Content: "4号和6号在压力下有什么差异？"},
			{Role: "assistant", Content: "两种模式关注的重点不同。"},
			{Role: "user", Content: "这种差异让我焦虑"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}
	if suggestions := buildSuggestions(input); len(suggestions) != 0 {
		t.Fatalf("older focus crossed an uncertain mixed-type chain: %+v", suggestions)
	}
}

func TestSuggestionsResumeAfterMixedTypeChainIsExplicitlyReassignedToCurrentType(t *testing.T) {
	input := AskInput{
		Question: "那怎么办？",
		History: []Message{
			{Role: "user", Content: "4号和6号在压力下有什么差异？"},
			{Role: "user", Content: "这种差异让我焦虑"},
			{Role: "user", Content: "回到4号，为什么我总觉得不被理解？"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}
	suggestions := buildSuggestions(input)
	if len(suggestions) != 3 || !containsSuggestionText(suggestions, "不被理解") {
		t.Fatalf("explicit current type did not resume after mixed chain: %+v", suggestions)
	}
}

func TestSuggestionsDoNotFallbackWhenCurrentQuestionExplicitlyNamesAnotherType(t *testing.T) {
	input := AskInput{
		Question: "那6号为什么害怕被否定？",
		History: []Message{
			{Role: "user", Content: "作为4号，我总觉得不被理解"},
		},
		ConversationCard: ConversationCard{MainType: 4},
	}

	if suggestions := buildSuggestions(input); len(suggestions) != 0 {
		t.Fatalf("another-type current question fell back to older focus: %+v", suggestions)
	}
}

func TestSuggestionFocusRemovesInternalRuntimeTermsAndCompactVariants(t *testing.T) {
	questions := []string{
		"我害怕不被理解，通过 Codex CLI 回答时该怎么办？",
		"我害怕不被理解，通过 C o d e x C L I 回答时该怎么办？",
		"我害怕不被理解，通过 Ｃｏｄｅｘ ＣＬＩ 回答时该怎么办？",
		"我害怕不被理解，通过 Co\u200Bdex C\u200DLI 回答时该怎么办？",
		"我害怕不被理解，通过 Сodex 回答时该怎么办？",
		"我害怕不被理解，通过 Codеx 回答时该怎么办？",
		"我害怕不被理解，通过 Codeх 回答时该怎么办？",
		"我害怕不被理解，通过 OрenAI 回答时该怎么办？",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			focus := normalizeSuggestionFocus(question, 4)
			if focus == "" || !strings.Contains(focus, "害怕不被理解") {
				t.Fatalf("psychological focus was lost: %q", focus)
			}
			if strings.ContainsAny(focus, "СсЕеХхРр") {
				t.Fatalf("focus leaked confusable runtime term: %q", focus)
			}
			compact := compactSuggestionTestText(focus)
			for _, forbidden := range []string{"codexcli", "cli"} {
				if strings.Contains(compact, forbidden) {
					t.Fatalf("focus leaked internal runtime term %q: %q", forbidden, focus)
				}
			}

			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if len(suggestions) != 3 {
				t.Fatalf("suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				compactSuggestion := compactSuggestionTestText(suggestion)
				for _, forbidden := range []string{"codexcli", "cli"} {
					if strings.Contains(compactSuggestion, forbidden) {
						t.Fatalf("suggestion leaked internal runtime term %q: %q", forbidden, suggestion)
					}
				}
			}
		})
	}
}

func TestSuggestionFocusRemovesProviderAndModelIdentityTerms(t *testing.T) {
	questions := []string{
		"我因为 GPT 模型版本和参数问题感到焦虑",
		"我害怕不被理解，底层模型是 OpenAI 吗？",
		"厂商和底层环境让我不安",
	}
	for _, question := range questions {
		t.Run(question, func(t *testing.T) {
			focus := normalizeSuggestionFocus(question, 4)
			if focus == "" {
				t.Fatal("psychological focus was removed")
			}
			compact := compactSuggestionTestText(focus)
			for _, forbidden := range []string{"gpt", "openai", "模型", "厂商", "版本", "参数", "底层环境"} {
				if strings.Contains(compact, forbidden) {
					t.Fatalf("focus leaked model identity term %q: %q", forbidden, focus)
				}
			}

			suggestions := buildSuggestions(AskInput{
				Question:         question,
				ConversationCard: ConversationCard{MainType: 4},
			})
			if len(suggestions) != 3 {
				t.Fatalf("suggestions = %+v, want 3", suggestions)
			}
			for _, suggestion := range suggestions {
				compactSuggestion := compactSuggestionTestText(suggestion)
				for _, forbidden := range []string{"gpt", "openai", "模型", "厂商", "版本", "参数", "底层环境"} {
					if strings.Contains(compactSuggestion, forbidden) {
						t.Fatalf("suggestion leaked model identity term %q: %q", forbidden, suggestion)
					}
				}
			}
		})
	}
}

func TestSuggestionsKeepOrdinaryPersonalityThemeAfterIdentityFiltering(t *testing.T) {
	suggestions := buildSuggestions(AskInput{
		Question:         "我想了解自己的人格模式和性格特点",
		ConversationCard: ConversationCard{MainType: 4},
	})
	if len(suggestions) != 3 {
		t.Fatalf("ordinary personality theme suggestions = %+v, want 3", suggestions)
	}
}

func compactSuggestionTestText(text string) string {
	text = norm.NFKC.String(strings.ToLower(text))
	return strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "",
		"\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "",
	).Replace(text)
}

func TestSuggestionFocusIsAtMostTwentyEightRunes(t *testing.T) {
	focus := normalizeSuggestionFocus("我在工作中害怕被否定，也害怕表达真实感受以后不被理解和被忽略，这让我非常焦虑内耗", 4)
	if got := utf8.RuneCountInString(focus); got > 28 {
		t.Fatalf("focus length = %d, want <= 28: %q", got, focus)
	}
}

func containsSuggestionText(suggestions []string, text string) bool {
	for _, suggestion := range suggestions {
		if strings.Contains(suggestion, text) {
			return true
		}
	}
	return false
}

func TestAskStreamKeepsBroadEnneagramSourcesForGeneralDefinition(t *testing.T) {
	generator := &capturingStreamingGenerator{answer: "九型是一套观察性格模式的地图。"}
	service := NewService([]Document{
		{ID: "kb-nine-types", Title: "九型基础", Content: "九型人格用于理解九种性格模式与核心动机。", Tags: []string{"九型", "基础"}},
		{ID: "theory:map", Title: "九型是观察地图", Content: "九型是观察地图，不是身份标签。", Tags: []string{"九型", "理论"}},
	}, WithGenerator(generator))

	answer, err := service.AskStream(context.Background(), AskInput{Question: "什么是九型"}, nil)
	if err != nil {
		t.Fatalf("AskStream returned error: %v", err)
	}
	if len(generator.input.Sources) < 2 {
		t.Fatalf("generator sources = %+v, want broad knowledge and theory sources", generator.input.Sources)
	}
	if answer.Sources[0].ID != "kb-nine-types" || answer.Sources[1].ID != "theory:map" {
		t.Fatalf("answer sources = %+v, want knowledge and theory in order", answer.Sources)
	}
}

func TestAskStreamDoesNotAttachBroadEnneagramSourcesToUnrelatedQuestion(t *testing.T) {
	generator := &capturingStreamingGenerator{answer: "可以先看天气预报。"}
	service := NewService([]Document{
		{ID: "kb-nine-types", Title: "九型基础", Content: "九型人格用于理解九种性格模式与核心动机。", Tags: []string{"九型", "基础"}},
	}, WithGenerator(generator))

	answer, err := service.AskStream(context.Background(), AskInput{Question: "明天上海天气怎么样"}, nil)
	if err != nil {
		t.Fatalf("AskStream returned error: %v", err)
	}
	if len(generator.input.Sources) != 0 || len(answer.Sources) != 0 {
		t.Fatalf("unrelated question got sources: generator=%+v answer=%+v", generator.input.Sources, answer.Sources)
	}
}

func TestAskRetrievesRelevantKnowledge(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则、秩序和高标准，成长建议是允许不完美。", Tags: []string{"完美型", "原则"}},
		{ID: "course-basic", Title: "九型基础课", Content: "九型基础课适合想系统理解九种性格模式的人。", Tags: []string{"课程"}},
	})

	result, err := service.Ask(context.Background(), AskInput{
		Question: "我是完美型，怎么成长？",
		UserProfile: UserProfile{
			Nickname: "小九",
			MainType: 1,
		},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if !strings.Contains(result.Answer, "小九") || !strings.Contains(result.Answer, "完美型") {
		t.Fatalf("expected personalized answer, got %q", result.Answer)
	}
	if len(result.Sources) == 0 || result.Sources[0].ID != "type-1" {
		t.Fatalf("expected type-1 as top source, got %+v", result.Sources)
	}
}

func TestAskRejectsEmptyQuestion(t *testing.T) {
	_, err := NewService(nil).Ask(context.Background(), AskInput{Question: "   "})
	if err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAskMatchesChineseKeywordsInsideLongQuestion(t *testing.T) {
	service := NewService([]Document{
		{ID: "kb-enterprise", Title: "企业沟通课", Content: "企业沟通课适合团队冲突复盘和管理者沟通训练。", Tags: []string{"企业", "沟通"}},
	})

	result, err := service.Ask(context.Background(), AskInput{Question: "企业沟通课适合什么场景？"})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if len(result.Sources) == 0 || result.Sources[0].ID != "kb-enterprise" {
		t.Fatalf("expected Chinese keyword match, got %+v", result.Sources)
	}
}

func TestAskUsesGeneratorWithRetrievedContext(t *testing.T) {
	generator := &fakeGenerator{answer: "这是模型生成的回答"}
	service := NewService([]Document{
		{ID: "type-5", Title: "5号 观察型", Content: "观察型重视知识、边界和独处空间。", Tags: []string{"观察型"}},
	}, WithGenerator(generator))

	result, err := service.Ask(context.Background(), AskInput{
		Question:            "观察型怎么沟通？",
		ConversationSummary: "用户正在讨论与观察型同事的沟通问题。",
		UserProfile:         UserProfile{Nickname: "阿九", MainType: 5},
		History: []Message{
			{Role: "user", Content: "我刚测完"},
			{Role: "assistant", Content: "你可以问具体场景"},
		},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if result.Answer != "这是模型生成的回答" {
		t.Fatalf("expected generator answer, got %q", result.Answer)
	}
	if generator.input.Question != "观察型怎么沟通？" || len(generator.input.Sources) != 1 {
		t.Fatalf("generator did not receive retrieved context: %+v", generator.input)
	}
	if len(generator.input.History) != 2 {
		t.Fatalf("expected history to be passed, got %+v", generator.input.History)
	}
	if generator.input.ConversationSummary != "用户正在讨论与观察型同事的沟通问题。" {
		t.Fatalf("expected conversation summary to be passed, got %+v", generator.input)
	}
}

func TestAskPropagatesPreferencesAndCurrentDirectives(t *testing.T) {
	generator := &fakeGenerator{answer: "模型回答"}
	service := NewService(nil, WithGenerator(generator))

	_, err := service.Ask(context.Background(), AskInput{
		Question:          "这次详细说",
		UserPreferences:   []string{"回答简短，避免长篇大论"},
		CurrentDirectives: []string{"回答更详细"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(generator.input.UserPreferences, "|") != "回答简短，避免长篇大论" {
		t.Fatalf("preferences not propagated: %+v", generator.input)
	}
	if strings.Join(generator.input.CurrentDirectives, "|") != "回答更详细" {
		t.Fatalf("current directives not propagated: %+v", generator.input)
	}
}

func TestAskPropagatesConversationTier(t *testing.T) {
	generator := &fakeGenerator{answer: "模型回答"}
	service := NewService(nil, WithGenerator(generator))

	_, err := service.Ask(context.Background(), AskInput{Question: "帮我分析", Tier: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if generator.input.Tier != "deep" {
		t.Fatalf("conversation tier not propagated: %+v", generator.input)
	}
}

func TestAskLimitsGeneratorHistory(t *testing.T) {
	generator := &fakeGenerator{answer: "模型回答"}
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}},
	}, WithGenerator(generator))

	history := []Message{
		{Role: "system", Content: "不应该传给模型"},
		{Role: "user", Content: "旧问题"},
		{Role: "assistant", Content: "旧回答"},
		{Role: "user", Content: "问题1"},
		{Role: "assistant", Content: "回答1"},
		{Role: "user", Content: "问题2"},
		{Role: "assistant", Content: "回答2"},
		{Role: "user", Content: "问题3"},
		{Role: "assistant", Content: "回答3"},
		{Role: "user", Content: "问题4"},
		{Role: "assistant", Content: "回答4"},
		{Role: "user", Content: "问题5"},
		{Role: "assistant", Content: "回答5"},
		{Role: "user", Content: "问题6"},
		{Role: "user", Content: "问题7"},
		{Role: "assistant", Content: "回答7"},
		{Role: "user", Content: "问题8"},
		{Role: "assistant", Content: "回答8"},
		{Role: "user", Content: "问题9"},
		{Role: "assistant", Content: "回答9"},
		{Role: "user", Content: "问题10"},
		{Role: "assistant", Content: "回答10"},
		{Role: "user", Content: strings.Repeat("很长", 160)},
	}

	if _, err := service.Ask(context.Background(), AskInput{Question: "完美型怎么成长？", History: history}); err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}

	if len(generator.input.History) != 20 {
		t.Fatalf("expected 20 recent history messages, got %+v", generator.input.History)
	}
	if generator.input.History[0].Content != "问题1" {
		t.Fatalf("expected oldest excess messages to be removed, got %+v", generator.input.History)
	}
	last := generator.input.History[len(generator.input.History)-1]
	if len([]rune(last.Content)) != 223 || !strings.HasSuffix(last.Content, "...") {
		t.Fatalf("expected long history to be trimmed, got len=%d content=%q", len([]rune(last.Content)), last.Content)
	}
}

func TestAskFallsBackWhenGeneratorFails(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}},
	}, WithGenerator(&fakeGenerator{err: errors.New("llm unavailable")}))

	result, err := service.Ask(context.Background(), AskInput{
		Question:    "完美型怎么成长？",
		UserProfile: UserProfile{Nickname: "小九", MainType: 1},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if !strings.Contains(result.Answer, "我先按你问到的重点检索了九型资料") {
		t.Fatalf("expected fallback retrieval answer, got %q", result.Answer)
	}
}

func TestAskModelIdentityReturnsFixedReplyWithoutRetrievalOrGeneration(t *testing.T) {
	generator := &countingIdentityGenerator{answer: "不应调用模型"}
	service := NewService([]Document{
		{
			ID:      "model-identity-leak",
			Title:   "当前模型与技术实现",
			Content: "这里包含模型厂商、模型 ID、中转站和 Codex CLI 等技术说明。",
			Tags:    []string{"模型", "Codex CLI"},
		},
	}, WithGenerator(generator))

	answer, err := service.Ask(context.Background(), AskInput{Question: "你是什么模型？"})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if answer.Answer != ModelIdentityReply {
		t.Fatalf("answer = %q, want %q", answer.Answer, ModelIdentityReply)
	}
	if len(answer.Sources) != 0 {
		t.Fatalf("identity answer sources = %+v, want empty", answer.Sources)
	}
	if len(answer.Suggestions) != 0 {
		t.Fatalf("identity answer suggestions = %+v, want empty", answer.Suggestions)
	}
	if generator.generateCalls != 0 || generator.streamCalls != 0 {
		t.Fatalf("identity answer called generator: generate=%d stream=%d", generator.generateCalls, generator.streamCalls)
	}
}

type fakeGenerator struct {
	answer string
	err    error
	input  GenerateInput
}

type countingIdentityGenerator struct {
	answer        string
	generateCalls int
	streamCalls   int
}

func (g *countingIdentityGenerator) Generate(_ context.Context, _ GenerateInput) (string, error) {
	g.generateCalls++
	return g.answer, nil
}

func (g *countingIdentityGenerator) GenerateStream(_ context.Context, _ GenerateInput, emit StreamEmitter) (string, error) {
	g.streamCalls++
	if emit != nil {
		if err := emit(g.answer); err != nil {
			return "", err
		}
	}
	return g.answer, nil
}

func (f *fakeGenerator) Generate(_ context.Context, input GenerateInput) (string, error) {
	f.input = input
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

type partialFailStreamingGenerator struct {
	err error
}

type capturingStreamingGenerator struct {
	input  GenerateInput
	answer string
}

func (g *capturingStreamingGenerator) Generate(_ context.Context, input GenerateInput) (string, error) {
	g.input = input
	return g.answer, nil
}

func (g *capturingStreamingGenerator) GenerateStream(_ context.Context, input GenerateInput, emit StreamEmitter) (string, error) {
	g.input = input
	if emit != nil {
		if err := emit(g.answer); err != nil {
			return "", err
		}
	}
	return g.answer, nil
}

func (f *partialFailStreamingGenerator) Generate(context.Context, GenerateInput) (string, error) {
	return "", f.err
}

func (f *partialFailStreamingGenerator) GenerateStream(_ context.Context, _ GenerateInput, emit StreamEmitter) (string, error) {
	if err := emit("部分回答"); err != nil {
		return "", err
	}
	return "部分回答", f.err
}

func TestAskStreamReturnsErrorWithoutFallbackAfterPartialOutput(t *testing.T) {
	wantErr := errors.New("stream interrupted")
	service := NewService(nil, WithGenerator(&partialFailStreamingGenerator{err: wantErr}))

	var chunks []string
	_, err := service.AskStream(context.Background(), AskInput{Question: "我该怎么办？"}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected stream error, got %v", err)
	}
	if got := strings.Join(chunks, ""); got != "部分回答" {
		t.Fatalf("streamed chunks = %q, want partial output only", got)
	}
}

func TestAskStreamEmitsGeneratedChunksAndReturnsMetadata(t *testing.T) {
	generator := &fakeGenerator{answer: "第一段，第二段。"}
	service := NewService([]Document{{ID: "type-1", Title: "1号", Content: "1号孩子重视原则和秩序。"}}, WithGenerator(generator))

	var chunks []string
	answer, err := service.AskStream(context.Background(), AskInput{
		Question:            "1号孩子怎么办？",
		ConversationSummary: "用户之前提到孩子害怕犯错。",
	}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream returned error: %v", err)
	}
	if answer.Answer != "第一段，第二段。" {
		t.Fatalf("unexpected answer: %q", answer.Answer)
	}
	if len(answer.Sources) == 0 {
		t.Fatal("expected sources metadata to be returned")
	}
	if strings.Join(chunks, "") != "第一段，第二段。" {
		t.Fatalf("unexpected streamed chunks: %#v", chunks)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected generated answer to be split into multiple stream chunks, got %#v", chunks)
	}
	if generator.input.ConversationSummary != "用户之前提到孩子害怕犯错。" {
		t.Fatalf("streaming generator did not receive conversation summary: %+v", generator.input)
	}
}

func TestAskStreamPassesOriginalQuestionAndSourcesToGeneratorUnchanged(t *testing.T) {
	generator := &capturingStreamingGenerator{answer: "模型原样回答。"}
	service := NewService([]Document{
		{ID: "type-5", Title: "5号 观察型", Content: "观察型重视知识、边界和独处空间。", Tags: []string{"观察型"}},
	}, WithGenerator(generator))

	const question = "观察型怎么沟通？"
	var chunks []string
	answer, err := service.AskStream(context.Background(), AskInput{Question: question}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream returned error: %v", err)
	}
	if generator.input.Question != question {
		t.Fatalf("generator question = %q, want original %q", generator.input.Question, question)
	}
	if len(generator.input.Sources) != 1 || generator.input.Sources[0].ID != "type-5" {
		t.Fatalf("generator sources = %+v, want retrieved type-5", generator.input.Sources)
	}
	if answer.Answer != generator.answer || strings.Join(chunks, "") != generator.answer {
		t.Fatalf("model answer was rewritten: answer=%q chunks=%q", answer.Answer, strings.Join(chunks, ""))
	}
}

func TestAskStreamPropagatesPreferencesAndCurrentDirectives(t *testing.T) {
	generator := &capturingStreamingGenerator{answer: "流式回答"}
	service := NewService(nil, WithGenerator(generator))

	_, err := service.AskStream(context.Background(), AskInput{
		Question:          "直接说",
		UserPreferences:   []string{"语气温柔友好"},
		CurrentDirectives: []string{"表达直接，少说教"},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(generator.input.UserPreferences, "|") != "语气温柔友好" {
		t.Fatalf("stream preferences not propagated: %+v", generator.input)
	}
	if strings.Join(generator.input.CurrentDirectives, "|") != "表达直接，少说教" {
		t.Fatalf("stream current directives not propagated: %+v", generator.input)
	}
}

func TestAskStreamModelIdentityEmitsOnlyFixedReplyWithoutRetrievalOrGeneration(t *testing.T) {
	generator := &countingIdentityGenerator{answer: "不应调用流式模型"}
	service := NewService([]Document{
		{
			ID:      "model-identity-leak",
			Title:   "当前模型与技术实现",
			Content: "这里包含模型厂商、模型 ID、中转站和 Codex CLI 等技术说明。",
			Tags:    []string{"模型", "Codex CLI"},
		},
	}, WithGenerator(generator))

	var chunks []string
	answer, err := service.AskStream(context.Background(), AskInput{Question: "你通过 Codex CLI 回答吗？"}, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream returned error: %v", err)
	}
	if answer.Answer != ModelIdentityReply {
		t.Fatalf("answer = %q, want %q", answer.Answer, ModelIdentityReply)
	}
	if got := strings.Join(chunks, ""); got != ModelIdentityReply {
		t.Fatalf("streamed content = %q, want %q", got, ModelIdentityReply)
	}
	if len(answer.Sources) != 0 {
		t.Fatalf("identity stream sources = %+v, want empty", answer.Sources)
	}
	if len(answer.Suggestions) != 0 {
		t.Fatalf("identity stream suggestions = %+v, want empty", answer.Suggestions)
	}
	if generator.generateCalls != 0 || generator.streamCalls != 0 {
		t.Fatalf("identity stream called generator: generate=%d stream=%d", generator.generateCalls, generator.streamCalls)
	}
}
