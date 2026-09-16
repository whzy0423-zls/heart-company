package server

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/knowledgeclient"
	"nine-xing/nx-backend/apps/server/internal/observability"
)

type knowledgeRetrieveClientStub struct {
	request  knowledgeclient.RetrievalRequest
	response knowledgeclient.RetrievalResponse
	err      error
}

type blockingKnowledgeRetrieveClient struct{}

func (blockingKnowledgeRetrieveClient) Retrieve(ctx context.Context, _ knowledgeclient.RetrievalRequest) (knowledgeclient.RetrievalResponse, error) {
	<-ctx.Done()
	return knowledgeclient.RetrievalResponse{}, ctx.Err()
}

func (s *knowledgeRetrieveClientStub) Retrieve(_ context.Context, request knowledgeclient.RetrievalRequest) (knowledgeclient.RetrievalResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestAppKnowledgeRemoteAdapterMapsScopeAndDocuments(t *testing.T) {
	releaseID := int64(101)
	client := &knowledgeRetrieveClientStub{response: knowledgeclient.RetrievalResponse{
		RequestID: "req-1",
		Documents: []knowledgeclient.Document{{ID: "doc-1", Content: "内容", Library: "theory", ReleaseID: &releaseID, Source: "book.pdf"}},
		Trace:     knowledgeclient.RetrievalTrace{RetrievalMethod: "hybrid"},
	}}
	adapter := appKnowledgeRemoteAdapter{client: client}

	result, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{
		RequestID: "req-1", Query: "问题", Scene: "app_chat", Public: true,
		TheoryReleaseIDs: []int64{101}, EnneagramReleaseIDs: []int64{103}, MainType: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(client.request.Scope.TheoryReleaseIDs, []int64{101}) || client.request.Profile.MainType == nil || *client.request.Profile.MainType != 3 {
		t.Fatalf("request=%+v", client.request)
	}
	if len(result.Documents) != 1 || result.Documents[0].Title != "book.pdf" || result.RetrievalMethod != "hybrid" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAppKnowledgeRemoteAdapterPreservesHTTPStatusForFallbackPolicy(t *testing.T) {
	client := &knowledgeRetrieveClientStub{err: &knowledgeclient.ResponseError{StatusCode: 400, Body: "bad scope"}}
	adapter := appKnowledgeRemoteAdapter{client: client}

	_, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{})
	var remoteErr *appknowledge.RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != 400 {
		t.Fatalf("expected mapped remote error, got %v", err)
	}
}

func TestAppKnowledgeRemoteAdapterOmitsInvalidOptionalProfileTypes(t *testing.T) {
	client := &knowledgeRetrieveClientStub{}
	adapter := appKnowledgeRemoteAdapter{client: client}

	if _, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{Scene: "skill_chat"}); err != nil {
		t.Fatal(err)
	}
	if client.request.Profile.MainType != nil || client.request.Profile.WingType != nil {
		t.Fatalf("optional profile=%+v", client.request.Profile)
	}
}

func TestAppKnowledgeRemoteAdapterRejectsDocumentOutsideRequestedScope(t *testing.T) {
	wrongRelease := int64(999)
	client := &knowledgeRetrieveClientStub{response: knowledgeclient.RetrievalResponse{
		Documents: []knowledgeclient.Document{{ID: "leak", Content: "不应返回", Library: "theory", ReleaseID: &wrongRelease, Source: "other.pdf"}},
	}}
	adapter := appKnowledgeRemoteAdapter{client: client}

	_, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{TheoryReleaseIDs: []int64{101}})
	var remoteErr *appknowledge.RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != 502 {
		t.Fatalf("expected cross-scope response to fail closed, got %v", err)
	}
}

func TestAppKnowledgeRemoteAdapterRecordsLowCardinalityMetrics(t *testing.T) {
	metrics := observability.New()
	client := &knowledgeRetrieveClientStub{response: knowledgeclient.RetrievalResponse{}}
	adapter := appKnowledgeRemoteAdapter{client: client, metrics: metrics}

	if _, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{}); err != nil {
		t.Fatal(err)
	}
	client.err = errors.New("network")
	_, _ = adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{})
	snapshot := metrics.Snapshot()
	if snapshot.KnowledgeRemoteTotal != 2 || snapshot.KnowledgeRemoteErrors != 1 {
		t.Fatalf("knowledge metrics=%+v", snapshot)
	}
}

func TestAppKnowledgeRemoteAdapterEnforcesRetrieveTimeout(t *testing.T) {
	adapter := appKnowledgeRemoteAdapter{client: blockingKnowledgeRetrieveClient{}, retrieveTimeout: 20 * time.Millisecond}
	started := time.Now()

	_, err := adapter.Retrieve(context.Background(), appknowledge.RemoteRequest{})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("retrieve timeout took %v", elapsed)
	}
}
