package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/knowledgeclient"
)

type knowledgeRetrieveClientStub struct {
	request  knowledgeclient.RetrievalRequest
	response knowledgeclient.RetrievalResponse
	err      error
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
