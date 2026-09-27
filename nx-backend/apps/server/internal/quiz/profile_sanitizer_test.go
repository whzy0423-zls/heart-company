package quiz

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeProfileJSONRemovesWingKeysRecursively(t *testing.T) {
	raw := json.RawMessage(`{
		"summary":"保留",
		"wingType":2,
		"nested":{"wing_label":"删除","mainType":1},
		"items":[{"wingLabel":"删除","name":"保留"},{"wing_type":9}]
	}`)

	got := SanitizeProfileJSON(raw)
	if strings.Contains(strings.ToLower(string(got)), "wing") {
		t.Fatalf("sanitized profile still contains wing keys: %s", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("sanitized profile is invalid JSON: %v", err)
	}
	if decoded["summary"] != "保留" {
		t.Fatalf("unrelated profile data was changed: %s", got)
	}
	nested := decoded["nested"].(map[string]any)
	if nested["mainType"] != float64(1) {
		t.Fatalf("nested unrelated data was changed: %s", got)
	}
	items := decoded["items"].([]any)
	if items[0].(map[string]any)["name"] != "保留" {
		t.Fatalf("array content was changed: %s", got)
	}
}

func TestSanitizeProfileJSONFailsClosedForInvalidJSON(t *testing.T) {
	if got := string(SanitizeProfileJSON(json.RawMessage(`{"wingType":`))); got != `{}` {
		t.Fatalf("invalid profile = %s, want {}", got)
	}
}
