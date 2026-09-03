package location

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNormalizeSearchInputStripsControlsAndPreservesUnicodeRuneLimits(t *testing.T) {
	input := SearchInput{
		Query: "  深圳\n大学\x00城  ",
		City:  " 广东省\r\n ",
	}

	got, err := NormalizeSearchInput(input)
	if err != nil {
		t.Fatalf("NormalizeSearchInput() error = %v", err)
	}
	if got.Query != "深圳大学城" {
		t.Fatalf("query = %q, want controls removed and spaces trimmed", got.Query)
	}
	if got.City != "广东省" {
		t.Fatalf("city = %q, want controls removed and spaces trimmed", got.City)
	}

	if _, err := NormalizeSearchInput(SearchInput{Query: ""}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty query error = %v, want ErrInvalidRequest", err)
	}
	if _, err := NormalizeSearchInput(SearchInput{Query: strings.Repeat("字", 81)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("81-rune query error = %v, want ErrInvalidRequest", err)
	}
	if _, err := NormalizeSearchInput(SearchInput{Query: "学校", City: strings.Repeat("市", 41)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("41-rune city error = %v, want ErrInvalidRequest", err)
	}
}

func TestNormalizeReverseInputRequiresFiniteGCJ02Coordinates(t *testing.T) {
	valid := ReverseInput{Latitude: 22.5901, Longitude: 113.9734, CoordinateSystem: "gcj02"}
	got, err := NormalizeReverseInput(valid)
	if err != nil {
		t.Fatalf("valid reverse input error = %v", err)
	}
	if got.CoordinateSystem != CoordinateSystemGCJ02 {
		t.Fatalf("coordinate system = %q, want %q", got.CoordinateSystem, CoordinateSystemGCJ02)
	}

	cases := []ReverseInput{
		{Latitude: math.NaN(), Longitude: 0, CoordinateSystem: "gcj02"},
		{Latitude: math.Inf(1), Longitude: 0, CoordinateSystem: "gcj02"},
		{Latitude: 91, Longitude: 0, CoordinateSystem: "gcj02"},
		{Latitude: 0, Longitude: 181, CoordinateSystem: "gcj02"},
		{Latitude: 0, Longitude: 0, CoordinateSystem: "wgs84"},
	}
	for _, input := range cases {
		if _, err := NormalizeReverseInput(input); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("input %+v error = %v, want ErrInvalidRequest", input, err)
		}
	}
}

func TestNormalizeCandidatesCapsCountAndFieldsWithoutBreakingUnicode(t *testing.T) {
	items := make([]Candidate, MaxCandidates+5)
	for i := range items {
		items[i] = Candidate{
			Name:             strings.Repeat("名", MaxCandidateFieldCodePoints+10),
			Address:          strings.Repeat("址", MaxCandidateFieldCodePoints+10),
			Province:         strings.Repeat("省", MaxCandidateFieldCodePoints+10),
			City:             strings.Repeat("市", MaxCandidateFieldCodePoints+10),
			District:         strings.Repeat("区", MaxCandidateFieldCodePoints+10),
			Latitude:         22.5,
			Longitude:        113.9,
			CoordinateSystem: CoordinateSystemGCJ02,
		}
	}

	got := NormalizeCandidates(items)
	if len(got) != MaxCandidates {
		t.Fatalf("candidate count = %d, want %d", len(got), MaxCandidates)
	}
	if got[0].Name != strings.Repeat("名", MaxCandidateFieldCodePoints) {
		t.Fatalf("name rune cap mismatch: got %d runes", len([]rune(got[0].Name)))
	}
	if len([]rune(got[0].Address)) != MaxCandidateFieldCodePoints ||
		len([]rune(got[0].Province)) != MaxCandidateFieldCodePoints ||
		len([]rune(got[0].City)) != MaxCandidateFieldCodePoints ||
		len([]rune(got[0].District)) != MaxCandidateFieldCodePoints {
		t.Fatal("candidate fields were not capped by Unicode code points")
	}
}

func TestCandidateJSONUsesStableContract(t *testing.T) {
	got := Candidate{
		Name:             "深圳大学城",
		Address:          "留仙大道",
		Province:         "广东省",
		City:             "深圳市",
		District:         "南山区",
		Latitude:         22.5901,
		Longitude:        113.9734,
		CoordinateSystem: CoordinateSystemGCJ02,
	}
	raw, err := got.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	var decoded Candidate
	if err := decoded.UnmarshalJSON(raw); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if decoded != got {
		t.Fatalf("round trip = %+v, want %+v", decoded, got)
	}
}
