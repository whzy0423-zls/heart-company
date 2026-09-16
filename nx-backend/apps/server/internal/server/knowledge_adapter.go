package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/knowledgeclient"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type knowledgeRetrieveClient interface {
	Retrieve(context.Context, knowledgeclient.RetrievalRequest) (knowledgeclient.RetrievalResponse, error)
}

type appKnowledgeRemoteAdapter struct {
	client          knowledgeRetrieveClient
	retrieveTimeout time.Duration
}

func (a appKnowledgeRemoteAdapter) Retrieve(ctx context.Context, input appknowledge.RemoteRequest) (appknowledge.RemoteResult, error) {
	if a.retrieveTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.retrieveTimeout)
		defer cancel()
	}
	mainType := input.MainType
	response, err := a.client.Retrieve(ctx, knowledgeclient.RetrievalRequest{
		RequestID: input.RequestID,
		Query:     input.Query,
		Scene:     input.Scene,
		Scope: knowledgeclient.Scope{
			Public: input.Public, TheoryReleaseIDs: input.TheoryReleaseIDs,
			EnneagramReleaseIDs: input.EnneagramReleaseIDs,
		},
		Profile: knowledgeclient.Profile{MainType: &mainType},
		Retrieval: knowledgeclient.RetrievalOptions{
			TopK: 8, VectorK: 20, LexicalK: 20, RerankK: 8, MaxContextRunes: 8000,
		},
	})
	if err != nil {
		var responseErr *knowledgeclient.ResponseError
		if errors.As(err, &responseErr) {
			return appknowledge.RemoteResult{}, &appknowledge.RemoteError{StatusCode: responseErr.StatusCode, Err: err}
		}
		return appknowledge.RemoteResult{}, &appknowledge.RemoteError{Err: err}
	}
	documents := make([]rag.Document, 0, len(response.Documents))
	for _, document := range response.Documents {
		title := strings.TrimSpace(document.Source)
		if title == "" {
			title = document.ID
		}
		tags := []string{"library:" + document.Library}
		if document.ReleaseID != nil {
			tags = append(tags, fmt.Sprintf("release:%d", *document.ReleaseID))
		}
		documents = append(documents, rag.Document{ID: document.ID, Title: title, Content: document.Content, Tags: tags})
	}
	return appknowledge.RemoteResult{Documents: documents, RetrievalMethod: response.Trace.RetrievalMethod}, nil
}
