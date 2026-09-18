package skillchat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBookRetrievalBackendIsInternalMetadata(t *testing.T) {
	metadata := sanitizeSessionSourceMetadata([]byte(`{"retrievalBackend":"local","sourceNeeded":false}`))
	if metadata.RetrievalBackend != "local" {
		t.Fatal("book release routing was lost")
	}
	raw, err := json.Marshal(metadata)
	if err != nil || strings.Contains(string(raw), "retrievalBackend") {
		t.Fatalf("internal routing exposed: %s %v", raw, err)
	}
}
