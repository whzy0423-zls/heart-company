package server

import (
	"context"
	"testing"
)

func TestSkillVoiceCleanupContextOutlivesCanceledRequest(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	cleanupCtx, cancelCleanup := skillVoiceCleanupContext(requestCtx)
	defer cancelCleanup()
	if err := cleanupCtx.Err(); err != nil {
		t.Fatalf("cleanup inherited request cancellation: %v", err)
	}
}
