package server

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBuildAppChatEnneagramReplyPlanClassifiesKnowledgeRequests(t *testing.T) {
	tests := []struct {
		name      string
		question  string
		wantTypes []int
	}{
		{name: "overview", question: "什么是九型人格？", wantTypes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{name: "all types", question: "九型人格所有类型分别有什么特点", wantTypes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{name: "single type", question: "1号是什么样的", wantTypes: []int{1}},
		{name: "single type with personality anchor", question: "1号性格为什么害怕犯错", wantTypes: []int{1}},
		{name: "target sentence", question: "1 2 3 4 这些型号的反馈", wantTypes: []int{1, 2, 3, 4}},
		{name: "partial punctuation", question: "九型人格中，4、2、4号分别有什么区别", wantTypes: []int{2, 4}},
		{name: "canonical names", question: "完美型和助人型", wantTypes: []int{1, 2}},
		{name: "canonical name knowledge", question: "领袖型的核心恐惧是什么", wantTypes: []int{8}},
		{name: "arabic numeric range", question: "九型人格1到4号分别解释", wantTypes: []int{1, 2, 3, 4}},
		{name: "chinese numeric range", question: "九型人格一至四号有什么区别", wantTypes: []int{1, 2, 3, 4}},
		{name: "chinese numeric list", question: "九型人格中的一、三、三、五号分别解释", wantTypes: []int{1, 3, 5}},
		{name: "explicit mechanism wins", question: "我是1号，为什么压力下总挑错", wantTypes: []int{1}},
		{name: "ambiguous numeric list", question: "1、2、3、4号", wantTypes: nil},
		{name: "ordinary numbered questions", question: "第1到9题", wantTypes: nil},
		{name: "ordinary file types", question: "所有类型的文件", wantTypes: nil},
		{name: "ordinary file question", question: "这个文件类型是什么", wantTypes: nil},
		{name: "ordinary phone models", question: "1号和2号手机型号有什么区别", wantTypes: nil},
		{name: "ambiguous numbered comparison", question: "1号和2号有什么区别", wantTypes: nil},
		{name: "emotional support", question: "我是1号，最近关系压力很大", wantTypes: nil},
		{name: "ordinary domain", question: "3号房间怎么走", wantTypes: nil},
		{name: "weak words only", question: "最近关系和成长怎么样", wantTypes: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := buildAppChatEnneagramReplyPlan(tt.question)
			if !reflect.DeepEqual(plan.RequestedTypes, tt.wantTypes) {
				t.Fatalf("RequestedTypes = %v, want %v", plan.RequestedTypes, tt.wantTypes)
			}
			if gotEnabled := len(plan.RequestedTypes) > 0; gotEnabled != plan.Enabled {
				t.Fatalf("Enabled = %v, want %v for requested types %v", plan.Enabled, gotEnabled, plan.RequestedTypes)
			}
			seen := make(map[int]bool)
			for _, number := range plan.RequestedTypes {
				if seen[number] {
					t.Fatalf("RequestedTypes contains duplicate %d: %v", number, plan.RequestedTypes)
				}
				seen[number] = true
			}
		})
	}
}

func TestBuildAppChatEnneagramReplyPlanContract(t *testing.T) {
	canonicalNames := []string{
		"1号完美型", "2号助人型", "3号成就型", "4号自我型", "5号思考型",
		"6号忠诚型", "7号活跃型", "8号领袖型", "9号和平型",
	}
	wantDimensions := []string{"核心欲望", "核心恐惧", "防御机制", "压力表现", "关系模式", "成长方向"}

	t.Run("overview has every canonical type and dimension", func(t *testing.T) {
		plan := buildAppChatEnneagramReplyPlan("什么是九型人格")
		if len(plan.TypeContracts) != 9 {
			t.Fatalf("len(TypeContracts) = %d, want 9", len(plan.TypeContracts))
		}
		for index, typeContract := range plan.TypeContracts {
			if typeContract.Number != index+1 {
				t.Errorf("TypeContracts[%d].Number = %d, want %d", index, typeContract.Number, index+1)
			}
			if typeContract.CanonicalName != canonicalNames[index] {
				t.Errorf("TypeContracts[%d].CanonicalName = %q, want %q", index, typeContract.CanonicalName, canonicalNames[index])
			}
			assertAppChatEnneagramDimensions(t, typeContract, wantDimensions)
		}
		if plan.MaxOutputTokens != 3480 {
			t.Errorf("MaxOutputTokens = %d, want 3480", plan.MaxOutputTokens)
		}
		if plan.CompletionTimeout < 70*time.Second {
			t.Errorf("CompletionTimeout = %v, want at least 70s", plan.CompletionTimeout)
		}
	})

	t.Run("partial request is ordered and requested only", func(t *testing.T) {
		plan := buildAppChatEnneagramReplyPlan("九型人格中4、2、4号分别解释")
		if !reflect.DeepEqual(plan.RequestedTypes, []int{2, 4}) {
			t.Fatalf("RequestedTypes = %v, want [2 4]", plan.RequestedTypes)
		}
		if len(plan.TypeContracts) != 2 {
			t.Fatalf("len(TypeContracts) = %d, want 2", len(plan.TypeContracts))
		}
		for index, number := range []int{2, 4} {
			contract := plan.TypeContracts[index]
			if contract.Number != number || contract.CanonicalName != canonicalNames[number-1] {
				t.Errorf("TypeContracts[%d] = {%d %q}, want type %d %q", index, contract.Number, contract.CanonicalName, number, canonicalNames[number-1])
			}
			assertAppChatEnneagramDimensions(t, contract, wantDimensions)
		}
		if plan.MaxOutputTokens != 1240 {
			t.Errorf("MaxOutputTokens = %d, want 1240", plan.MaxOutputTokens)
		}
		for _, unrelated := range []string{"1号完美型", "3号成就型", "5号思考型", "6号忠诚型", "7号活跃型", "8号领袖型", "9号和平型"} {
			if strings.Contains(plan.RuntimeInstructions, unrelated) {
				t.Errorf("RuntimeInstructions unexpectedly requires unrelated type %q", unrelated)
			}
		}
	})

	t.Run("single type uses formula budget", func(t *testing.T) {
		plan := buildAppChatEnneagramReplyPlan("1号是什么样的")
		if plan.MaxOutputTokens != 920 {
			t.Errorf("MaxOutputTokens = %d, want 920", plan.MaxOutputTokens)
		}
		if len(plan.TypeContracts) != 1 || plan.TypeContracts[0].CanonicalName != "1号完美型" {
			t.Fatalf("TypeContracts = %#v, want only 1号完美型", plan.TypeContracts)
		}
	})

	t.Run("contract preserves coverage without portrait or retrieval", func(t *testing.T) {
		plan := buildAppChatEnneagramReplyPlan("1 2 3 4 这些型号的反馈")
		if len(plan.TypeContracts) != 4 {
			t.Fatalf("len(TypeContracts) = %d, want 4 even without portrait or retrieved knowledge", len(plan.TypeContracts))
		}
		for _, phrase := range []string{
			"画像只能用于个性化示例",
			"不能替代任何请求型号或维度",
			"型号知识库资料缺失",
			"仍须保留该型号及全部六个维度",
			"不得推断用户尚未测定的类型",
			"不得将九型内容表述为临床诊断",
		} {
			if !strings.Contains(plan.RuntimeInstructions, phrase) {
				t.Errorf("RuntimeInstructions missing boundary %q", phrase)
			}
		}
	})

	t.Run("ordinary question has empty plan", func(t *testing.T) {
		plan := buildAppChatEnneagramReplyPlan("今天上海天气怎么样")
		if plan.Enabled || len(plan.RequestedTypes) != 0 || len(plan.TypeContracts) != 0 || plan.RuntimeInstructions != "" || plan.MaxOutputTokens != 0 || plan.CompletionTimeout != 0 {
			t.Fatalf("ordinary plan = %#v, want zero value", plan)
		}
	})
}

func assertAppChatEnneagramDimensions(t *testing.T, contract appChatEnneagramTypeContract, wantNames []string) {
	t.Helper()
	if len(contract.Dimensions) != len(wantNames) {
		t.Fatalf("%s dimensions = %d, want %d", contract.CanonicalName, len(contract.Dimensions), len(wantNames))
	}
	for index, dimension := range contract.Dimensions {
		if dimension.Name != wantNames[index] {
			t.Errorf("%s dimension[%d].Name = %q, want %q", contract.CanonicalName, index, dimension.Name, wantNames[index])
		}
		runeCount := utf8.RuneCountInString(dimension.Requirement)
		if runeCount < 20 || runeCount > 60 {
			t.Errorf("%s %s requirement length = %d, want 20-60: %q", contract.CanonicalName, dimension.Name, runeCount, dimension.Requirement)
		}
		if !strings.HasSuffix(dimension.Requirement, "。") {
			t.Errorf("%s %s requirement must be one concrete sentence: %q", contract.CanonicalName, dimension.Name, dimension.Requirement)
		}
	}
}
