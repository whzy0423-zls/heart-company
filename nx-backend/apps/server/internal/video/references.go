package video

import (
	"fmt"
	"sort"
)

// CanonicalReferences is the single ordered reference value shared by prompt
// compilation and gateway encoding.
type CanonicalReferences struct {
	References []CanonicalReference `json:"references"`
}

// CanonicalReference couples an original reference with its per-kind ordinal.
// Reference is embedded so consumers cannot accidentally lose payload fields
// while using the canonical order.
type CanonicalReference struct {
	Reference
	Ordinal int    `json:"ordinal"`
	Label   string `json:"label"`
}

type indexedReference struct {
	reference CanonicalReference
	identity  referenceIdentity
	index     int
}

type referenceIdentity struct {
	kind       string
	role       string
	sourceType string
	sourceID   string
	url        string
}

// CanonicalizeReferences returns a sorted copy of references with image,
// video, and audio numbering maintained independently. It never merges by URL.
func CanonicalizeReferences(input []Reference) (CanonicalReferences, error) {
	indexed := make([]indexedReference, 0, len(input))
	seen := make(map[referenceIdentity]int, len(input))
	allNumericBySortOrder := make(map[int]bool, len(input))

	for index, reference := range input {
		prefix, ok := referenceLabelPrefix(reference.Kind)
		if !ok {
			return CanonicalReferences{}, validationError(
				"reference_kind_unsupported",
				fmt.Sprintf("references[%d].kind", index),
				fmt.Sprintf("不支持引用素材类型 %q。", reference.Kind),
				"引用素材类型只能是 image、video 或 audio。",
				nil,
			)
		}

		identity := referenceIdentity{
			kind:       reference.Kind,
			role:       reference.Role,
			sourceType: reference.SourceType,
			sourceID:   reference.SourceID,
			url:        reference.URL,
		}
		if firstIndex, exists := seen[identity]; exists {
			return CanonicalReferences{}, validationError(
				"duplicate_reference",
				fmt.Sprintf("references[%d]", index),
				fmt.Sprintf("第 %d 个引用与第 %d 个引用完全重复。", index+1, firstIndex+1),
				"删除重复引用，或修改其用途或来源后重试。",
				nil,
			)
		}
		seen[identity] = index

		canonicalReference := reference
		if reference.DurationSeconds != nil {
			duration := *reference.DurationSeconds
			canonicalReference.DurationSeconds = &duration
		}
		indexed = append(indexed, indexedReference{
			reference: CanonicalReference{Reference: canonicalReference, Label: prefix},
			identity:  identity,
			index:     index,
		})
		allNumeric, exists := allNumericBySortOrder[reference.SortOrder]
		if !exists {
			allNumeric = true
		}
		allNumericBySortOrder[reference.SortOrder] = allNumeric && isCanonicalDecimalID(reference.ID)
	}

	sort.Slice(indexed, func(i, j int) bool {
		return lessIndexedReference(indexed[i], indexed[j], allNumericBySortOrder[indexed[i].reference.SortOrder])
	})

	counts := map[string]int{"image": 0, "video": 0, "audio": 0}
	result := CanonicalReferences{References: make([]CanonicalReference, 0, len(indexed))}
	for _, item := range indexed {
		kind := item.reference.Kind
		counts[kind]++
		item.reference.Ordinal = counts[kind]
		item.reference.Label = fmt.Sprintf("%s%d", item.reference.Label, item.reference.Ordinal)
		result.References = append(result.References, item.reference)
	}
	return result, nil
}

func lessIndexedReference(left, right indexedReference, numericOrder bool) bool {
	if left.reference.SortOrder != right.reference.SortOrder {
		return left.reference.SortOrder < right.reference.SortOrder
	}
	if left.reference.ID != right.reference.ID {
		if numericOrder {
			return compareCanonicalDecimalIDs(left.reference.ID, right.reference.ID) < 0
		}
		return left.reference.ID < right.reference.ID
	}
	if comparison := compareReferenceIdentity(left.identity, right.identity); comparison != 0 {
		return comparison < 0
	}
	return left.index < right.index
}

func isCanonicalDecimalID(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareCanonicalDecimalIDs(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return compareStrings(left, right)
}

func compareReferenceIdentity(left, right referenceIdentity) int {
	for _, pair := range [][2]string{
		{left.kind, right.kind},
		{left.role, right.role},
		{left.sourceType, right.sourceType},
		{left.sourceID, right.sourceID},
		{left.url, right.url},
	} {
		if comparison := compareStrings(pair[0], pair[1]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func compareStrings(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func referenceLabelPrefix(kind string) (string, bool) {
	switch kind {
	case "image":
		return "图片", true
	case "video":
		return "视频", true
	case "audio":
		return "音频", true
	default:
		return "", false
	}
}
