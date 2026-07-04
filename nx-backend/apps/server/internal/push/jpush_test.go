package push

import (
	"context"
	"fmt"
	"testing"
)

func TestNewPusherFailsClosedInProductionWithoutCredentials(t *testing.T) {
	pusher := NewPusher("production", "", "")
	_, err := pusher.Push(context.Background(), []string{"rid-1"}, Message{Title: "标题", Content: "内容"})
	if err == nil {
		t.Fatal("expected production pusher without credentials to fail closed")
	}
}

func TestBatchPusherSplitsLargeAudiences(t *testing.T) {
	inner := &recordingPusher{}
	pusher := NewBatchPusher(inner, 1000)
	ids := make([]string, 2501)
	for i := range ids {
		ids[i] = fmt.Sprintf("rid-%d", i)
	}

	result, err := pusher.Push(context.Background(), ids, Message{Title: "标题", Content: "内容"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 2501 {
		t.Fatalf("expected sent count 2501, got %d", result.Sent)
	}
	if len(inner.batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(inner.batches))
	}
	if len(inner.batches[0]) != 1000 || len(inner.batches[1]) != 1000 || len(inner.batches[2]) != 501 {
		t.Fatalf("unexpected batch sizes: %d %d %d", len(inner.batches[0]), len(inner.batches[1]), len(inner.batches[2]))
	}
}

type recordingPusher struct {
	batches [][]string
}

func (p *recordingPusher) Push(_ context.Context, registrationIDs []string, _ Message) (PushResult, error) {
	copied := append([]string(nil), registrationIDs...)
	p.batches = append(p.batches, copied)
	return PushResult{MsgID: fmt.Sprintf("msg-%d", len(p.batches)), Sent: len(registrationIDs)}, nil
}
