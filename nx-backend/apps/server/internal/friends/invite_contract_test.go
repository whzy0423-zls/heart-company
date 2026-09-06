package friends

import (
	"os"
	"strings"
	"testing"
)

func TestGetOrCreateInviteCodePreservesExistingCode(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, expected := range []string{
		"func (s *Store) GetOrCreateInviteCode",
		"COALESCE(NULLIF(btrim(invite_code),''),",
		"RETURNING invite_code",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("missing stable invite implementation fragment %q", expected)
		}
	}
}
