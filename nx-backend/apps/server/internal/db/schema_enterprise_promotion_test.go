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

func TestEnterprisePromotionSchemaContainsBusinessConstraints(t *testing.T) {
	for _, snippet := range []string{
		"slug TEXT NOT NULL UNIQUE",
		"key TEXT NOT NULL UNIQUE",
		"trainer_id BIGINT NOT NULL REFERENCES enterprise_trainers(id) ON DELETE RESTRICT",
		"CREATE TABLE IF NOT EXISTS training_case_media (\n  id BIGSERIAL PRIMARY KEY",
		"UNIQUE(case_id, role, position)",
		"sort_order INT NOT NULL DEFAULT 0",
		"version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0)",
		"consultation_reference_hash TEXT NOT NULL UNIQUE",
		"request_idempotency_hash TEXT NOT NULL UNIQUE",
		"idempotency_key TEXT NOT NULL UNIQUE",
		"source_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT",
		"UNIQUE(asset_id, attempt_number)",
		"qa_result TEXT NOT NULL DEFAULT 'pending'",
		"qa_approved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT",
		"qa_approved_at TIMESTAMPTZ",
		"qa_note TEXT NOT NULL DEFAULT ''",
		"subject_type TEXT NOT NULL",
		"subject_id BIGINT NOT NULL",
		"use_scope TEXT NOT NULL",
		"case_id BIGINT REFERENCES training_cases(id) ON DELETE RESTRICT",
		"approval_txid BIGINT NOT NULL DEFAULT txid_current()",
		"CREATE TRIGGER trg_promotion_media_ready_requires_current_qa",
		"CREATE TRIGGER trg_promotion_media_qa_reviews_append_only",
		"CREATE TRIGGER trg_promotion_media_qa_reviews_stamp",
		"ALTER TABLE training_case_consent_links ALTER COLUMN case_id DROP NOT NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_promotion_events_idempotency_key",
	} {
		if !strings.Contains(schemaSQL, snippet) {
			t.Errorf("schema missing constraint/column %q", snippet)
		}
	}
}

func TestEnterprisePromotionSchemaProtectsPIIAndMediaReadiness(t *testing.T) {
	for _, snippet := range []string{
		"company_name_encrypted BYTEA NOT NULL",
		"contact_name_encrypted BYTEA NOT NULL",
		"phone_encrypted BYTEA NOT NULL",
		"wechat_encrypted BYTEA",
		"CHECK (state <> 'ready' OR (qa_result = 'passed' AND qa_approved_by IS NOT NULL AND qa_approved_at IS NOT NULL))",
		"CHECK (key IN ('team-communication', 'leadership', 'cohesion', 'culture', 'employee-growth'))",
	} {
		if !strings.Contains(schemaSQL, snippet) {
			t.Errorf("schema missing privacy/readiness constraint %q", snippet)
		}
	}
}
