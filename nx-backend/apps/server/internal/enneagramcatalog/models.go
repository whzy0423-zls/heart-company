package enneagramcatalog

const (
	SCHEMA_VERSION = "enneagram-backend-catalog.v1"

	KindCore          = "core"
	KindEnneagramType = "enneagram_type"

	ProvenanceSource      = "source"
	ProvenanceProjectRule = "project_rule"
)

var RequiredDimensions = []string{
	"core_motivation_and_fear",
	"strengths",
	"risks",
	"formation_factors",
	"relationships",
	"workplace",
	"stress_and_defenses",
	"growth_practices",
}

type Catalog struct {
	Manifest Manifest
	Packages []Package
}

type Manifest struct {
	SchemaVersion   string            `json:"schema_version"`
	SourceMapSHA256 string            `json:"source_map_sha256"`
	Sources         []ManifestSource  `json:"sources"`
	Packages        []ManifestPackage `json:"packages"`
}

type ManifestSource struct {
	SourceID    string `json:"source_id"`
	DisplayName string `json:"display_name"`
	PageCount   int    `json:"page_count"`
	SHA256      string `json:"sha256"`
}

type ManifestPackage struct {
	File          string `json:"file"`
	LibraryID     string `json:"library_id"`
	Kind          string `json:"kind"`
	EnneagramType *int   `json:"enneagram_type"`
	ContentDigest string `json:"content_digest"`
}

type Package struct {
	SchemaVersion string            `json:"schema_version"`
	LibraryID     string            `json:"library_id"`
	Kind          string            `json:"kind"`
	EnneagramType *int              `json:"enneagram_type"`
	Title         string            `json:"title"`
	SourceChapter string            `json:"source_chapter"`
	Items         []Item            `json:"items,omitempty"`
	Dimensions    map[string][]Item `json:"dimensions,omitempty"`
	ContentDigest string            `json:"content_digest"`
}

type Item struct {
	ContentKey     string       `json:"content_key"`
	Dimension      string       `json:"dimension"`
	Text           string       `json:"text"`
	ProvenanceKind string       `json:"provenance_kind"`
	SourcePages    []SourcePage `json:"source_pages"`
}

type SourcePage struct {
	SourceID           string `json:"source_id"`
	PageNumber         int    `json:"page_number"`
	EnneagramType      int    `json:"enneagram_type"`
	OCRTextURI         string `json:"ocr_text_uri"`
	OCRTextHash        string `json:"ocr_text_hash"`
	OCRStatus          string `json:"ocr_status"`
	ManualReviewStatus string `json:"manual_review_status"`
}

func libraryIDForType(number int) string {
	return "enneagram-type-0" + string(rune('0'+number))
}
