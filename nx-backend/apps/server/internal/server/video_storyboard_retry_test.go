package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/llm"
)

func TestGenerateVideoStoryboardWithRetryRetriesOnceAndSummarizesFailures(t *testing.T) {
	calls := 0
	_, err := generateVideoStoryboardWithRetry(context.Background(), func(context.Context) (llm.VideoStoryboardResult, error) {
		calls++
		if calls == 1 {
			return llm.VideoStoryboardResult{}, errors.New("first EOF")
		}
		return llm.VideoStoryboardResult{}, errors.New("second empty shots")
	})
	if err == nil {
		t.Fatal("expected final failure")
	}
	if calls != 2 {
		t.Fatalf("expected exactly one automatic retry, got %d calls", calls)
	}
	message := err.Error()
	if !strings.Contains(message, "第 1 次失败：first EOF") || !strings.Contains(message, "第 2 次失败：second empty shots") {
		t.Fatalf("expected both failure reasons in final error, got %q", message)
	}
}

func TestGenerateVideoStoryboardWithRetryReturnsSecondAttemptSuccess(t *testing.T) {
	calls := 0
	result, err := generateVideoStoryboardWithRetry(context.Background(), func(context.Context) (llm.VideoStoryboardResult, error) {
		calls++
		if calls == 1 {
			return llm.VideoStoryboardResult{}, errors.New("transient EOF")
		}
		return llm.VideoStoryboardResult{Title: "重试成功"}, nil
	})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if calls != 2 || result.Title != "重试成功" {
		t.Fatalf("unexpected retry result calls=%d result=%+v", calls, result)
	}
}
