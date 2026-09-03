package enneagramcatalog

import (
	"strings"
	"testing"
)

func TestValidateCatalogRejectsInvalidPackages(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Catalog)
		want string
	}{
		{
			name: "illegal type",
			edit: func(catalog *Catalog) {
				catalog.Packages[1].EnneagramType = intPointer(10)
			},
			want: "enneagram type",
		},
		{
			name: "missing dimension",
			edit: func(catalog *Catalog) {
				delete(catalog.Packages[1].Dimensions, RequiredDimensions[0])
			},
			want: "dimensions",
		},
		{
			name: "unreviewed source",
			edit: func(catalog *Catalog) {
				catalog.Packages[1].Dimensions[RequiredDimensions[0]][0].SourcePages[0].ManualReviewStatus = "pending"
			},
			want: "not reviewed",
		},
		{
			name: "cross type source",
			edit: func(catalog *Catalog) {
				catalog.Packages[1].Dimensions[RequiredDimensions[0]][0].SourcePages[0].EnneagramType = 2
			},
			want: "type boundary",
		},
		{
			name: "duplicate content key",
			edit: func(catalog *Catalog) {
				catalog.Packages[1].Dimensions[RequiredDimensions[1]][0].ContentKey =
					catalog.Packages[1].Dimensions[RequiredDimensions[0]][0].ContentKey
			},
			want: "duplicate content key",
		},
		{
			name: "digest mismatch",
			edit: func(catalog *Catalog) {
				catalog.Packages[1].ContentDigest = strings.Repeat("f", 64)
			},
			want: "content digest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := validCatalog(t)
			test.edit(&catalog)
			if err := ValidateCatalog(catalog); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateCatalogAcceptsCoreAndNineIsolatedTypePackages(t *testing.T) {
	catalog := validCatalog(t)
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Packages) != 10 {
		t.Fatalf("expected ten packages, got %d", len(catalog.Packages))
	}
}

func TestValidateManifestRejectsUnsafeOrMismatchedFiles(t *testing.T) {
	for name, file := range map[string]string{
		"traversal":  "../type-01.json",
		"absolute":   "/tmp/type-01.json",
		"unexpected": "other.json",
	} {
		t.Run(name, func(t *testing.T) {
			catalog := validCatalog(t)
			catalog.Manifest.Packages[1].File = file
			if err := ValidateCatalog(catalog); err == nil {
				t.Fatal("expected manifest file to be rejected")
			}
		})
	}
}

func validCatalog(t *testing.T) Catalog {
	t.Helper()
	sources := []ManifestSource{
		{SourceID: "text-1", DisplayName: "文字1.pdf", PageCount: 25, SHA256: strings.Repeat("a", 64)},
		{SourceID: "text-2", DisplayName: "文字2.pdf", PageCount: 21, SHA256: strings.Repeat("b", 64)},
	}
	core := Package{
		SchemaVersion: SCHEMA_VERSION,
		LibraryID:     "enneagram-core",
		Kind:          KindCore,
		Title:         "共享九型理论核心",
		SourceChapter: "chapters/ch00-enneagram-core.md",
		Items: []Item{{
			ContentKey:     "enneagram-core:judgment_rules:01",
			Dimension:      "judgment_rules",
			Text:           "先看事实，再看倾向。",
			ProvenanceKind: ProvenanceSource,
			SourcePages:    []SourcePage{validSourcePage(1)},
		}},
	}
	core.ContentDigest = mustDigest(t, core)

	packages := []Package{core}
	for number := 1; number <= 9; number++ {
		packageValue := Package{
			SchemaVersion: SCHEMA_VERSION,
			LibraryID:     libraryIDForType(number),
			Kind:          KindEnneagramType,
			EnneagramType: intPointer(number),
			Title:         "型号",
			SourceChapter: "chapters/type.md",
			Dimensions:    make(map[string][]Item, len(RequiredDimensions)),
		}
		for index, dimension := range RequiredDimensions {
			packageValue.Dimensions[dimension] = []Item{{
				ContentKey:     packageValue.LibraryID + ":" + dimension + ":01",
				Dimension:      dimension,
				Text:           "内容",
				ProvenanceKind: ProvenanceSource,
				SourcePages: []SourcePage{{
					SourceID: "text-2", PageNumber: index + 1, EnneagramType: number,
					OCRTextURI: "sources/ocr/type.md", OCRTextHash: strings.Repeat("c", 64),
					OCRStatus: "recognized", ManualReviewStatus: "reviewed",
				}},
			}}
		}
		packageValue.ContentDigest = mustDigest(t, packageValue)
		packages = append(packages, packageValue)
	}

	manifest := Manifest{SchemaVersion: SCHEMA_VERSION, SourceMapSHA256: strings.Repeat("d", 64), Sources: sources}
	for index, packageValue := range packages {
		file := "core.json"
		if index > 0 {
			file = "type-0" + string(rune('0'+index)) + ".json"
		}
		manifest.Packages = append(manifest.Packages, ManifestPackage{
			File: file, LibraryID: packageValue.LibraryID, Kind: packageValue.Kind,
			EnneagramType: packageValue.EnneagramType, ContentDigest: packageValue.ContentDigest,
		})
	}
	return Catalog{Manifest: manifest, Packages: packages}
}

func validSourcePage(personalityType int) SourcePage {
	return SourcePage{
		SourceID: "text-1", PageNumber: 1, EnneagramType: personalityType,
		OCRTextURI: "sources/ocr/type.md", OCRTextHash: strings.Repeat("c", 64),
		OCRStatus: "recognized", ManualReviewStatus: "reviewed",
	}
}

func mustDigest(t *testing.T, packageValue Package) string {
	t.Helper()
	digest, err := packageDigest(packageValue)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func intPointer(value int) *int { return &value }
