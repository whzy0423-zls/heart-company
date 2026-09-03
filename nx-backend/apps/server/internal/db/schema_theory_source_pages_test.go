package db

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaIncludesTheorySourcePagesContract(t *testing.T) {
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.Join(strings.Fields(string(raw)), " ")
	definition := createTableDefinition(t, sqlText, "theory_source_pages")

	for _, fragment := range []string{
		"source_file_id BIGINT NOT NULL REFERENCES theory_source_files(id) ON DELETE RESTRICT",
		"page_number INTEGER NOT NULL CHECK (page_number > 0)",
		"enneagram_type SMALLINT CHECK (enneagram_type IS NULL OR enneagram_type BETWEEN 1 AND 9)",
		"ocr_text_hash TEXT NOT NULL CHECK (ocr_text_hash ~ '^[0-9a-f]{64}$')",
		"review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','reviewed','rejected'))",
		"UNIQUE (source_file_id, page_number)",
	} {
		if !strings.Contains(definition, fragment) {
			t.Errorf("theory_source_pages missing %q", fragment)
		}
	}
	if !strings.Contains(sqlText, "idx_theory_source_pages_type_review") {
		t.Fatal("theory_source_pages needs a type/review index")
	}
}
