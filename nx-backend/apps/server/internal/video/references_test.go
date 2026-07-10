package video

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCanonicalizeReferencesOrdersAndNumbersPerKind(t *testing.T) {
	refs := []Reference{
		{ID: "30", Kind: "video", Role: "reference_video", URL: "v2", SortOrder: 2},
		{ID: "20", Kind: "image", Role: "reference_image", URL: "i2", SortOrder: 1},
		{ID: "10", Kind: "image", Role: "first_frame", URL: "i1", SortOrder: 1},
		{ID: "40", Kind: "audio", Role: "reference_audio", URL: "a1", SortOrder: 0},
		{ID: "50", Kind: "video", Role: "edit_target", URL: "v1", SortOrder: 1},
	}
	original := append([]Reference(nil), refs...)

	got, err := CanonicalizeReferences(refs)
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}

	if !reflect.DeepEqual(refs, original) {
		t.Fatalf("CanonicalizeReferences() modified its input:\n got  %#v\n want %#v", refs, original)
	}
	if gotURLs := canonicalURLs(got); !reflect.DeepEqual(gotURLs, []string{"a1", "i1", "i2", "v1", "v2"}) {
		t.Fatalf("canonical URLs = %v, want [a1 i1 i2 v1 v2]", gotURLs)
	}
	if gotLabels := canonicalLabels(got); !reflect.DeepEqual(gotLabels, []string{"音频1", "图片1", "图片2", "视频1", "视频2"}) {
		t.Fatalf("canonical labels = %v, want [音频1 图片1 图片2 视频1 视频2]", gotLabels)
	}
	if gotOrdinals := canonicalOrdinals(got); !reflect.DeepEqual(gotOrdinals, []int{1, 1, 2, 1, 2}) {
		t.Fatalf("canonical ordinals = %v, want [1 1 2 1 2]", gotOrdinals)
	}
}

func TestCanonicalizeReferencesDeepCopiesDuration(t *testing.T) {
	duration := 3.5
	refs := []Reference{
		{ID: "1", Kind: "video", Role: "reference_video", URL: "v1", DurationSeconds: &duration},
	}

	got, err := CanonicalizeReferences(refs)
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}
	if got.References[0].DurationSeconds == nil {
		t.Fatal("canonical duration is nil, want copied duration")
	}

	*got.References[0].DurationSeconds = 7.25
	if value := *refs[0].DurationSeconds; value != 3.5 {
		t.Fatalf("modifying canonical duration changed input duration to %v, want 3.5", value)
	}

	*refs[0].DurationSeconds = 9.75
	if value := *got.References[0].DurationSeconds; value != 7.25 {
		t.Fatalf("modifying input duration changed canonical duration to %v, want 7.25", value)
	}
}

func TestCanonicalizeReferencesPreservesSameURLForDifferentRoleOrSource(t *testing.T) {
	refs := []Reference{
		{ID: "3", Kind: "image", Role: "reference_image", SourceType: "asset", SourceID: "scene-2", URL: "same"},
		{ID: "1", Kind: "image", Role: "first_frame", SourceType: "asset", SourceID: "scene-1", URL: "same"},
		{ID: "2", Kind: "image", Role: "reference_image", SourceType: "asset", SourceID: "scene-1", URL: "same"},
	}

	got, err := CanonicalizeReferences(refs)
	if err != nil {
		t.Fatalf("CanonicalizeReferences() error = %v", err)
	}
	if len(got.References) != len(refs) {
		t.Fatalf("canonical reference count = %d, want %d; same URL with a distinct role/source must be retained", len(got.References), len(refs))
	}
	if gotLabels := canonicalLabels(got); !reflect.DeepEqual(gotLabels, []string{"图片1", "图片2", "图片3"}) {
		t.Fatalf("canonical labels = %v, want [图片1 图片2 图片3]", gotLabels)
	}
}

func TestDuplicateReferenceRejectsExactIdentityWithoutUsingIDOrSortOrder(t *testing.T) {
	refs := []Reference{
		{ID: "900", Kind: "video", Role: "reference_video", SourceType: "upload", SourceID: "asset-7", URL: "same", SortOrder: 9},
		{ID: "1", Kind: "video", Role: "reference_video", SourceType: "upload", SourceID: "asset-7", URL: "same", SortOrder: 0},
	}

	_, err := CanonicalizeReferences(refs)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CanonicalizeReferences() error type = %T, want *ValidationError", err)
	}
	if validationErr.Code != "duplicate_reference" {
		t.Fatalf("error code = %q, want duplicate_reference", validationErr.Code)
	}
	if validationErr.Field != "references[1]" {
		t.Fatalf("error field = %q, want references[1]", validationErr.Field)
	}
	if !strings.Contains(validationErr.Message, "第 1 个") || !strings.Contains(validationErr.Message, "第 2 个") {
		t.Fatalf("error message must identify deterministic duplicate positions, got %q", validationErr.Message)
	}
	if strings.TrimSpace(validationErr.Fix) == "" {
		t.Fatalf("duplicate error must provide a repair action: %+v", validationErr)
	}
}

func TestCanonicalizeReferencesSortsNumericAndLexicalIDsDeterministically(t *testing.T) {
	tests := []struct {
		name string
		refs []Reference
		want []string
	}{
		{
			name: "canonical decimal IDs use numeric order",
			refs: []Reference{
				{ID: "10", Kind: "image", Role: "reference_image", URL: "ten"},
				{ID: "2", Kind: "image", Role: "reference_image", URL: "two"},
			},
			want: []string{"two", "ten"},
		},
		{
			name: "non-numeric and UUID IDs use lexical order",
			refs: []Reference{
				{ID: "uuid-b", Kind: "image", Role: "reference_image", URL: "b"},
				{ID: "alpha", Kind: "image", Role: "reference_image", URL: "alpha"},
				{ID: "uuid-a", Kind: "image", Role: "reference_image", URL: "a"},
			},
			want: []string{"alpha", "a", "b"},
		},
		{
			name: "huge decimal IDs do not overflow",
			refs: []Reference{
				{ID: "100000000000000000000000000000", Kind: "image", Role: "reference_image", URL: "larger"},
				{ID: "99999999999999999999999999999", Kind: "image", Role: "reference_image", URL: "smaller"},
			},
			want: []string{"smaller", "larger"},
		},
		{
			name: "negative IDs use safe lexical fallback",
			refs: []Reference{
				{ID: "-2", Kind: "image", Role: "reference_image", URL: "minus-two"},
				{ID: "-10", Kind: "image", Role: "reference_image", URL: "minus-ten"},
			},
			want: []string{"minus-ten", "minus-two"},
		},
		{
			name: "mixed canonical and fallback IDs use lexical order",
			refs: []Reference{
				{ID: "2", Kind: "image", Role: "reference_image", URL: "two"},
				{ID: "-1", Kind: "image", Role: "reference_image", URL: "minus-one"},
			},
			want: []string{"minus-one", "two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeReferences(tt.refs)
			if err != nil {
				t.Fatalf("CanonicalizeReferences() error = %v", err)
			}
			if gotURLs := canonicalURLs(got); !reflect.DeepEqual(gotURLs, tt.want) {
				t.Fatalf("canonical URLs = %v, want %v", gotURLs, tt.want)
			}
		})
	}
}

func TestCanonicalizeReferencesUsesIdentityToBreakEqualIDTies(t *testing.T) {
	first := []Reference{
		{ID: "7", Kind: "image", Role: "reference_image", SourceType: "asset", SourceID: "b", URL: "b"},
		{ID: "7", Kind: "image", Role: "reference_image", SourceType: "asset", SourceID: "a", URL: "a"},
	}
	second := []Reference{first[1], first[0]}

	gotFirst, err := CanonicalizeReferences(first)
	if err != nil {
		t.Fatalf("CanonicalizeReferences(first) error = %v", err)
	}
	gotSecond, err := CanonicalizeReferences(second)
	if err != nil {
		t.Fatalf("CanonicalizeReferences(second) error = %v", err)
	}
	want := []string{"a", "b"}
	if got := canonicalURLs(gotFirst); !reflect.DeepEqual(got, want) {
		t.Fatalf("first canonical URLs = %v, want %v", got, want)
	}
	if got := canonicalURLs(gotSecond); !reflect.DeepEqual(got, want) {
		t.Fatalf("reversed-input canonical URLs = %v, want %v", got, want)
	}
}

func TestCanonicalizeReferencesRejectsUnknownKind(t *testing.T) {
	_, err := CanonicalizeReferences([]Reference{{ID: "1", Kind: "document", Role: "reference_image", URL: "x"}})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CanonicalizeReferences() error type = %T, want *ValidationError", err)
	}
	if validationErr.Code != "reference_kind_unsupported" || validationErr.Field != "references[0].kind" {
		t.Fatalf("unknown-kind error = %+v, want typed fail-closed error at references[0].kind", validationErr)
	}
	if strings.TrimSpace(validationErr.Fix) == "" {
		t.Fatalf("unknown-kind error must provide a repair action: %+v", validationErr)
	}
}

func canonicalURLs(refs CanonicalReferences) []string {
	values := make([]string, 0, len(refs.References))
	for _, reference := range refs.References {
		values = append(values, reference.URL)
	}
	return values
}

func canonicalLabels(refs CanonicalReferences) []string {
	values := make([]string, 0, len(refs.References))
	for _, reference := range refs.References {
		values = append(values, reference.Label)
	}
	return values
}

func canonicalOrdinals(refs CanonicalReferences) []int {
	values := make([]int, 0, len(refs.References))
	for _, reference := range refs.References {
		values = append(values, reference.Ordinal)
	}
	return values
}
