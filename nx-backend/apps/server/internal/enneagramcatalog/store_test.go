package enneagramcatalog

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesIndependentEnneagramImportLedger(t *testing.T) {
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS enneagram_catalog_imports",
		"UNIQUE (library_id, content_digest)",
		"CHECK (status IN ('draft','in_review','approved','published','failed'))",
		"CHECK ((kind = 'core' AND enneagram_type IS NULL) OR (kind = 'enneagram_type' AND enneagram_type BETWEEN 1 AND 9))",
		"CREATE TABLE IF NOT EXISTS enneagram_catalog_import_items",
		"UNIQUE (import_id, content_key)",
		"published_release_id BIGINT REFERENCES theory_library_releases(id) ON DELETE RESTRICT",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Errorf("enneagram import schema missing %q", fragment)
		}
	}
}

func TestBindingScopeForPackageNeverFallsBackAcrossTypes(t *testing.T) {
	core := Package{Kind: KindCore, LibraryID: "enneagram-core"}
	layer, personalityType, err := bindingScope(core)
	if err != nil || layer != "theory" || personalityType != nil {
		t.Fatalf("unexpected core scope: layer=%q type=%v err=%v", layer, personalityType, err)
	}
	for number := 1; number <= 9; number++ {
		packageValue := Package{Kind: KindEnneagramType, LibraryID: libraryIDForType(number), EnneagramType: intPointer(number)}
		layer, personalityType, err = bindingScope(packageValue)
		if err != nil || layer != "enneagram_type" || personalityType == nil || *personalityType != number {
			t.Fatalf("type %d was routed incorrectly: layer=%q type=%v err=%v", number, layer, personalityType, err)
		}
	}
	invalidType := 2
	if _, _, err := bindingScope(Package{Kind: KindEnneagramType, LibraryID: "enneagram-type-03", EnneagramType: &invalidType}); err == nil {
		t.Fatal("cross-type package identity must not produce a binding scope")
	}
}
