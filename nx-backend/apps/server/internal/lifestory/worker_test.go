package lifestory

import (
	"errors"
	"testing"
)

func TestPublicGenerationErrorExplainsInvalidModelOutput(t *testing.T) {
	err := newGeneratedOutputError("length", errors.New("story character count 800 outside 1000-2000"))
	got := publicGenerationError(err)
	want := "故事模型返回的内容不完整，请重试；若持续失败请检查后台故事模型配置"
	if got != want {
		t.Fatalf("public error=%q, want %q", got, want)
	}
}
