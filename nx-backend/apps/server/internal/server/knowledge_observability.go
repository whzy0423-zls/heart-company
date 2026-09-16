package server

import (
	"context"
	"errors"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/observability"
)

func observeKnowledgeShadow(metrics *observability.Metrics, comparison appknowledge.ShadowComparison, logf func(string, ...any)) {
	metrics.KnowledgeShadowCompared(comparison.LocalDocumentIDs, comparison.RemoteDocumentIDs, comparison.RemoteError, comparison.Duration)
	if logf == nil {
		return
	}
	logf(
		"knowledge shadow request=%s local_count=%d remote_count=%d overlap=%d duration_ms=%d error_class=%s",
		comparison.RequestID,
		len(comparison.LocalDocumentIDs),
		len(comparison.RemoteDocumentIDs),
		knowledgeDocumentOverlap(comparison.LocalDocumentIDs, comparison.RemoteDocumentIDs),
		comparison.Duration.Milliseconds(),
		knowledgeErrorClass(comparison.RemoteError),
	)
}

func knowledgeDocumentOverlap(localIDs, remoteIDs []string) int {
	remote := make(map[string]struct{}, len(remoteIDs))
	for _, id := range remoteIDs {
		remote[id] = struct{}{}
	}
	overlap := 0
	for _, id := range localIDs {
		if _, ok := remote[id]; ok {
			overlap++
		}
	}
	return overlap
}

func knowledgeErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var remoteErr *appknowledge.RemoteError
	if errors.As(err, &remoteErr) && remoteErr.StatusCode > 0 {
		switch {
		case remoteErr.StatusCode == 429:
			return "rate_limited"
		case remoteErr.StatusCode >= 500:
			return "remote_5xx"
		case remoteErr.StatusCode >= 400:
			return "remote_4xx"
		}
	}
	return "transport"
}
