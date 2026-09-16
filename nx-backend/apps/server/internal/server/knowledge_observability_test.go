package server

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/observability"
)

func TestObserveKnowledgeShadowRecordsMetricsAndRedactsDocumentIDs(t *testing.T) {
	metrics := observability.New()
	var logged string
	comparison := appknowledge.ShadowComparison{
		RequestID:         "request-safe",
		LocalDocumentIDs:  []string{"private-local-document"},
		RemoteDocumentIDs: []string{"private-remote-document"},
		Duration:          25 * time.Millisecond,
	}

	observeKnowledgeShadow(metrics, comparison, func(format string, args ...any) {
		logged = fmt.Sprintf(format, args...)
	})

	snapshot := metrics.Snapshot()
	if snapshot.KnowledgeShadowTotal != 1 || snapshot.KnowledgeShadowComparedDocs != 1 || snapshot.KnowledgeShadowRunMsTotal < 25 {
		t.Fatalf("shadow metrics=%+v", snapshot)
	}
	if !strings.Contains(logged, "request-safe") || !strings.Contains(logged, "local_count=1") || !strings.Contains(logged, "remote_count=1") {
		t.Fatalf("missing low-cardinality shadow fields: %q", logged)
	}
	if strings.Contains(logged, "private-local-document") || strings.Contains(logged, "private-remote-document") {
		t.Fatalf("shadow log leaked document ids: %q", logged)
	}
}
