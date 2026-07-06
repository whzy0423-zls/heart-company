package quiz

import (
	"strings"
	"testing"
)

func TestNormalizeQuestionInputRejectsEnabledOptionWithoutWeights(t *testing.T) {
	_, err := NormalizeQuestionInput(QuestionInput{
		Body: "  你更偏好的沟通方式？ ",
		Options: []Option{
			{ID: "a", Text: "直接表达", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "保持观察"},
		},
		Status: "enabled",
	})
	if err == nil || !strings.Contains(err.Error(), "weights") {
		t.Fatalf("expected weights validation error, got %v", err)
	}
}

func TestNormalizeQuestionInputRejectsInvalidStatusAndDuplicateOptionID(t *testing.T) {
	_, err := NormalizeQuestionInput(QuestionInput{Body: "题目", Status: "archived", Options: []Option{
		{ID: "a", Text: "A", Weights: map[int]int{1: 1}},
		{ID: "a", Text: "B", Weights: map[int]int{2: 1}},
	}})
	if err == nil || !strings.Contains(err.Error(), "状态") {
		t.Fatalf("expected status validation error, got %v", err)
	}

	_, err = NormalizeQuestionInput(QuestionInput{Body: "题目", Status: "enabled", Options: []Option{
		{ID: "a", Text: "A", Weights: map[int]int{1: 1}},
		{ID: "a", Text: "B", Weights: map[int]int{2: 1}},
	}})
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate option validation error, got %v", err)
	}
}

func TestNormalizeQuestionInputAllowsDisabledDraftWithoutWeights(t *testing.T) {
	got, err := NormalizeQuestionInput(QuestionInput{
		Body:    "  草稿题  ",
		Options: []Option{{ID: " a ", Text: "  待补权重  "}, {ID: "b", Text: "选项 B"}},
		Status:  "disabled",
	})
	if err != nil {
		t.Fatalf("disabled draft should be accepted: %v", err)
	}
	if got.Body != "草稿题" || got.Options[0].ID != "a" || got.Options[0].Text != "待补权重" || got.Status != "disabled" {
		t.Fatalf("unexpected normalized question: %+v", got)
	}
}
