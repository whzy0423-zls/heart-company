package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/knowledgeclient"
	"nine-xing/nx-backend/apps/server/internal/observability"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type knowledgeRetrieveClient interface {
	Retrieve(context.Context, knowledgeclient.RetrievalRequest) (knowledgeclient.RetrievalResponse, error)
}

type appKnowledgeRemoteAdapter struct {
	client          knowledgeRetrieveClient
	retrieveTimeout time.Duration
	metrics         *observability.Metrics
}

func (a appKnowledgeRemoteAdapter) Retrieve(ctx context.Context, input appknowledge.RemoteRequest) (result appknowledge.RemoteResult, resultErr error) {
	started := time.Now()
	defer func() {
		a.metrics.KnowledgeRemoteExit(time.Since(started), resultErr)
	}()
	if a.retrieveTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.retrieveTimeout)
		defer cancel()
	}
	profile := knowledgeclient.Profile{}
	if input.MainType >= 1 && input.MainType <= 9 {
		mainType := input.MainType
		profile.MainType = &mainType
	}
	if input.WingType >= 1 && input.WingType <= 9 {
		wingType := input.WingType
		profile.WingType = &wingType
	}
	response, err := a.client.Retrieve(ctx, knowledgeclient.RetrievalRequest{
		RequestID: input.RequestID,
		Query:     input.Query,
		Scene:     input.Scene,
		Scope: knowledgeclient.Scope{
			Public: input.Public, TheoryReleaseIDs: input.TheoryReleaseIDs,
			EnneagramReleaseIDs: input.EnneagramReleaseIDs,
		},
		Profile: profile,
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
	citations := make([]rag.Citation, 0, len(response.Documents))
	for _, document := range response.Documents {
		if !remoteDocumentInScope(document, input) {
			return appknowledge.RemoteResult{}, &appknowledge.RemoteError{StatusCode: 502, Err: errors.New("knowledge service returned document outside requested scope")}
		}
		title := strings.TrimSpace(document.Source)
		if title == "" {
			title = document.ID
		}
		tags := []string{"library:" + document.Library}
		if document.ReleaseID != nil {
			tags = append(tags, fmt.Sprintf("release:%d", *document.ReleaseID))
		}
		documents = append(documents, rag.Document{ID: document.ID, Title: title, Content: document.Content, Tags: tags})
		citations = append(citations, rag.Citation{DocumentID: document.ID, Source: title, Locator: document.Locator})
	}
	return appknowledge.RemoteResult{Documents: documents, Citations: citations, RetrievalMethod: response.Trace.RetrievalMethod, TraceID: response.RequestID}, nil
}

func remoteDocumentInScope(document knowledgeclient.Document, input appknowledge.RemoteRequest) bool {
	switch document.Library {
	case "public":
		return input.Public && document.ReleaseID == nil
	case "theory":
		return document.ReleaseID != nil && containsReleaseID(input.TheoryReleaseIDs, *document.ReleaseID)
	case "enneagram":
		return document.ReleaseID != nil && containsReleaseID(input.EnneagramReleaseIDs, *document.ReleaseID)
	default:
		return false
	}
}

func containsReleaseID(releases []int64, value int64) bool {
	for _, releaseID := range releases {
		if releaseID == value {
			return true
		}
	}
	return false
}
