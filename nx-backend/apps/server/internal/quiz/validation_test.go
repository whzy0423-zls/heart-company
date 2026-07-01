package quiz

import "testing"

func TestValidateAnswersRejectsUnknownQuestion(t *testing.T) {
	weights := map[int64]map[string]map[int]int{
		1: {"a": {1: 2}},
	}
	if _, err := validateAnswers([]AnswerItem{{QuestionID: 2, OptionID: "a"}}, weights); err == nil {
		t.Fatal("expected unknown question to be rejected")
	}
}

func TestValidateAnswersRejectsUnknownOption(t *testing.T) {
	weights := map[int64]map[string]map[int]int{
		1: {"a": {1: 2}},
	}
	if _, err := validateAnswers([]AnswerItem{{QuestionID: 1, OptionID: "missing"}}, weights); err == nil {
		t.Fatal("expected unknown option to be rejected")
	}
}

func TestValidateAnswersRejectsDuplicateQuestion(t *testing.T) {
	weights := map[int64]map[string]map[int]int{
		1: {"a": {1: 2}, "b": {2: 1}},
	}
	if _, err := validateAnswers([]AnswerItem{
		{QuestionID: 1, OptionID: "a"},
		{QuestionID: 1, OptionID: "b"},
	}, weights); err == nil {
		t.Fatal("expected duplicate question to be rejected")
	}
}

func TestValidateAnswersReturnsScoreForValidAnswers(t *testing.T) {
	weights := map[int64]map[string]map[int]int{
		1: {"a": {1: 2}},
		2: {"b": {2: 3}},
	}
	score, err := validateAnswers([]AnswerItem{
		{QuestionID: 1, OptionID: "a"},
		{QuestionID: 2, OptionID: "b"},
	}, weights)
	if err != nil {
		t.Fatal(err)
	}
	if score[1] != 2 || score[2] != 3 {
		t.Fatalf("unexpected score: %+v", score)
	}
}
