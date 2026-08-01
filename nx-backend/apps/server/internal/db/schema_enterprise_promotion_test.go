package db

import (
	"strings"
	"testing"
)

func TestSchemaContainsEnterprisePromotionTables(t *testing.T) {
	for _, table := range []string{
		"enterprise_trainers", "training_topics", "training_cases",
		"enterprise_solutions", "training_case_media", "training_case_solutions",
		"training_case_topics", "training_case_testimonials", "training_case_claims",
		"enterprise_promotion_settings", "publication_consents",
		"training_case_consent_links", "enterprise_consultations",
		"enterprise_consultation_notes", "promotion_sessions", "promotion_events",
		"promotion_share_tokens", "consultation_privacy_requests",
		"enterprise_promotion_audit_logs",
		"promotion_media_assets", "promotion_media_upload_tasks",
		"promotion_media_processing_attempts", "promotion_media_qa_reviews",
	} {
		if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("schema missing table %s", table)
		}
	}
}

func TestEnterprisePromotionSchemaSmokeContainsMigrationBoundary(t *testing.T) {
	for _, snippet := range []string{
		"-- ----- 企业培训推广：媒体、案例、方案、授权、线索和基础归因 -----",
		"'qa_pending'",
		"CREATE TRIGGER trg_promotion_media_ready_requires_current_qa",
		"CREATE TRIGGER trg_promotion_media_qa_reviews_append_only",
	} {
		if !strings.Contains(schemaSQL, snippet) {
			t.Errorf("schema smoke marker missing %q", snippet)
		}
	}
}
