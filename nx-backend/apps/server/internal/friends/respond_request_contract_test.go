package friends

import (
	"os"
	"strings"
	"testing"
)

func TestRespondRequestCastsFriendshipPairIDsToBigint(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}

	const expected = "VALUES (LEAST($1::bigint,$2::bigint),GREATEST($1::bigint,$2::bigint),'active')"
	if !strings.Contains(string(raw), expected) {
		t.Fatalf("friendship insert must cast driver parameters to bigint: missing %q", expected)
	}
}
