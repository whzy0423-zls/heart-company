package server

import (
	"regexp"
	"testing"
)

func TestGenerateDistributionAgentCodeUsesSixUnambiguousCharacters(t *testing.T) {
	pattern := regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		code, err := generateDistributionAgentCode()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("code %q does not match the six-character format", code)
		}
		if seen[code] {
			t.Fatalf("duplicate code generated: %s", code)
		}
		seen[code] = true
	}
}

func TestValidateDistributionAgentCodeFormat(t *testing.T) {
	for _, tc := range []struct {
		code string
		want bool
	}{
		{code: "AB2K9Q", want: true},
		{code: "ab2k9q", want: false},
		{code: "A12IOO", want: false},
		{code: "AB12", want: false},
		{code: "AB12-3", want: false},
	} {
		if got := validateDistributionAgentCodeFormat(tc.code); got != tc.want {
			t.Errorf("validateDistributionAgentCodeFormat(%q)=%v, want %v", tc.code, got, tc.want)
		}
	}
}
