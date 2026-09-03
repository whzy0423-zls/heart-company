package enneagramcatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func LoadCatalog(directory string) (Catalog, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return Catalog{}, fmt.Errorf("load enneagram catalog: directory is required")
	}
	var manifest Manifest
	if err := decodeStrictJSON(filepath.Join(directory, "manifest.json"), &manifest); err != nil {
		return Catalog{}, fmt.Errorf("load enneagram catalog manifest: %w", err)
	}
	catalog := Catalog{Manifest: manifest, Packages: make([]Package, 0, len(manifest.Packages))}
	for _, entry := range manifest.Packages {
		if err := validatePackageFilename(entry.File); err != nil {
			return Catalog{}, fmt.Errorf("load enneagram catalog: %w", err)
		}
		var packageValue Package
		if err := decodeStrictJSON(filepath.Join(directory, entry.File), &packageValue); err != nil {
			return Catalog{}, fmt.Errorf("load enneagram catalog package %s: %w", entry.File, err)
		}
		catalog.Packages = append(catalog.Packages, packageValue)
	}
	if err := ValidateCatalog(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func ValidateCatalog(catalog Catalog) error {
	if catalog.Manifest.SchemaVersion != SCHEMA_VERSION {
		return fmt.Errorf("validate enneagram catalog: unsupported schema version %q", catalog.Manifest.SchemaVersion)
	}
	if !sha256Pattern.MatchString(catalog.Manifest.SourceMapSHA256) {
		return fmt.Errorf("validate enneagram catalog: invalid source map digest")
	}
	if len(catalog.Manifest.Sources) != 2 {
		return fmt.Errorf("validate enneagram catalog: expected two sources")
	}
	sources := make(map[string]ManifestSource, len(catalog.Manifest.Sources))
	for _, source := range catalog.Manifest.Sources {
		if source.SourceID == "" || source.DisplayName == "" || source.PageCount <= 0 || !sha256Pattern.MatchString(source.SHA256) {
			return fmt.Errorf("validate enneagram catalog: invalid source %q", source.SourceID)
		}
		if _, exists := sources[source.SourceID]; exists {
			return fmt.Errorf("validate enneagram catalog: duplicate source %q", source.SourceID)
		}
		sources[source.SourceID] = source
	}
	if len(catalog.Manifest.Packages) != 10 || len(catalog.Packages) != 10 {
		return fmt.Errorf("validate enneagram catalog: expected core and nine type packages")
	}

	seenKeys := make(map[string]struct{})
	for index := range catalog.Packages {
		packageValue := catalog.Packages[index]
		entry := catalog.Manifest.Packages[index]
		if err := validatePackageFilename(entry.File); err != nil {
			return fmt.Errorf("validate enneagram catalog: %w", err)
		}
		if err := validatePackageIdentity(index, packageValue, entry); err != nil {
			return err
		}
		items, err := validatePackageShape(packageValue)
		if err != nil {
			return err
		}
		for _, item := range items {
			if _, exists := seenKeys[item.ContentKey]; exists {
				return fmt.Errorf("validate enneagram catalog: duplicate content key %q", item.ContentKey)
			}
			seenKeys[item.ContentKey] = struct{}{}
			if err := validateItem(packageValue, item, sources); err != nil {
				return err
			}
		}
		digest, err := packageDigest(packageValue)
		if err != nil {
			return fmt.Errorf("validate enneagram catalog: compute content digest: %w", err)
		}
		if packageValue.ContentDigest != digest || entry.ContentDigest != digest {
			return fmt.Errorf("validate enneagram catalog: content digest mismatch for %s", packageValue.LibraryID)
		}
	}
	return nil
}

func validatePackageIdentity(index int, packageValue Package, entry ManifestPackage) error {
	expectedFile := "core.json"
	expectedLibrary := "enneagram-core"
	expectedKind := KindCore
	var expectedType *int
	if index > 0 {
		number := index
		expectedFile = fmt.Sprintf("type-%02d.json", number)
		expectedLibrary = fmt.Sprintf("enneagram-type-%02d", number)
		expectedKind = KindEnneagramType
		expectedType = &number
	}
	if entry.File != expectedFile || entry.LibraryID != expectedLibrary || packageValue.LibraryID != expectedLibrary ||
		entry.Kind != expectedKind || packageValue.Kind != expectedKind || !sameOptionalInt(entry.EnneagramType, expectedType) ||
		!sameOptionalInt(packageValue.EnneagramType, expectedType) {
		return fmt.Errorf("validate enneagram catalog: invalid enneagram type or package identity at index %d", index)
	}
	if packageValue.SchemaVersion != SCHEMA_VERSION || strings.TrimSpace(packageValue.Title) == "" || strings.TrimSpace(packageValue.SourceChapter) == "" {
		return fmt.Errorf("validate enneagram catalog: incomplete package %s", expectedLibrary)
	}
	return nil
}

func validatePackageShape(packageValue Package) ([]Item, error) {
	if packageValue.Kind == KindCore {
		if len(packageValue.Items) == 0 || len(packageValue.Dimensions) != 0 {
			return nil, fmt.Errorf("validate enneagram catalog: core items are required")
		}
		return packageValue.Items, nil
	}
	if len(packageValue.Items) != 0 || len(packageValue.Dimensions) != len(RequiredDimensions) {
		return nil, fmt.Errorf("validate enneagram catalog: all eight dimensions are required for %s", packageValue.LibraryID)
	}
	items := make([]Item, 0)
	for _, dimension := range RequiredDimensions {
		dimensionItems, exists := packageValue.Dimensions[dimension]
		if !exists || len(dimensionItems) == 0 {
			return nil, fmt.Errorf("validate enneagram catalog: dimensions are incomplete for %s", packageValue.LibraryID)
		}
		for _, item := range dimensionItems {
			if item.Dimension != dimension {
				return nil, fmt.Errorf("validate enneagram catalog: item dimension mismatch for %q", item.ContentKey)
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func validateItem(packageValue Package, item Item, sources map[string]ManifestSource) error {
	if strings.TrimSpace(item.ContentKey) == "" || strings.TrimSpace(item.Text) == "" || strings.TrimSpace(item.Dimension) == "" {
		return fmt.Errorf("validate enneagram catalog: incomplete item in %s", packageValue.LibraryID)
	}
	switch item.ProvenanceKind {
	case ProvenanceSource:
		if len(item.SourcePages) == 0 {
			return fmt.Errorf("validate enneagram catalog: source item %q has no pages", item.ContentKey)
		}
	case ProvenanceProjectRule:
		if len(item.SourcePages) != 0 {
			return fmt.Errorf("validate enneagram catalog: project rule %q must not claim source pages", item.ContentKey)
		}
		return nil
	default:
		return fmt.Errorf("validate enneagram catalog: invalid provenance for %q", item.ContentKey)
	}
	for _, page := range item.SourcePages {
		source, exists := sources[page.SourceID]
		if !exists || page.PageNumber <= 0 || page.PageNumber > source.PageCount {
			return fmt.Errorf("validate enneagram catalog: invalid source page for %q", item.ContentKey)
		}
		if page.OCRStatus != "recognized" || page.ManualReviewStatus != "reviewed" {
			return fmt.Errorf("validate enneagram catalog: source page is not reviewed for %q", item.ContentKey)
		}
		if !sha256Pattern.MatchString(page.OCRTextHash) || !safeRelativePath(page.OCRTextURI) {
			return fmt.Errorf("validate enneagram catalog: invalid OCR metadata for %q", item.ContentKey)
		}
		if packageValue.EnneagramType != nil && page.EnneagramType != *packageValue.EnneagramType {
			return fmt.Errorf("validate enneagram catalog: source page crosses type boundary for %q", item.ContentKey)
		}
		if page.EnneagramType < 1 || page.EnneagramType > 9 {
			return fmt.Errorf("validate enneagram catalog: invalid source enneagram type for %q", item.ContentKey)
		}
	}
	return nil
}

func packageDigest(packageValue Package) (string, error) {
	raw, err := json.Marshal(packageValue)
	if err != nil {
		return "", err
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "", err
	}
	delete(payload, "content_digest")
	canonical, err := canonicalJSON(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// ContentDigest returns the canonical digest used by catalog imports after
// editable package content has changed.
func ContentDigest(packageValue Package) (string, error) {
	return packageDigest(packageValue)
}

func canonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func validatePackageFilename(file string) error {
	expected := map[string]struct{}{"core.json": {}}
	for number := 1; number <= 9; number++ {
		expected[fmt.Sprintf("type-%02d.json", number)] = struct{}{}
	}
	if _, exists := expected[file]; !exists || filepath.Base(file) != file || !safeRelativePath(file) {
		return fmt.Errorf("unsafe or unexpected package file %q", file)
	}
	return nil
}

func safeRelativePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return false
	}
	clean := filepath.Clean(value)
	return clean == value && clean != "." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func decodeStrictJSON(path string, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%s contains trailing JSON", filepath.Base(path))
	}
	return nil
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sortedPackageItems(packageValue Package) []Item {
	if packageValue.Kind == KindCore {
		return append([]Item(nil), packageValue.Items...)
	}
	items := make([]Item, 0)
	for _, dimension := range RequiredDimensions {
		items = append(items, packageValue.Dimensions[dimension]...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ContentKey < items[j].ContentKey })
	return items
}
