package theorystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCardNotFound      = errors.New("theory card not found")
	ErrCardNotEditable   = errors.New("theory card is not an editable draft")
	ErrInvalidTransition = errors.New("invalid theory card transition")
	ErrConcurrentUpdate  = errors.New("theory card changed concurrently")
)

const cardReturningColumns = `
		id, library_id, canonical_key, canonical_name, aliases, domain, subdomain, card_kind,
		summary, definition, core_claim, mechanism, applicable_context, non_applicable_context,
		observable_signals, common_triggers, automatic_pattern, resource_state, shadow_or_risk,
		growth_direction, epistemic_status, evidence_level, clinical_safety, controversy_notes,
		cultural_context, authority_level, language, status, version, reviewed_by, reviewed_at,
		published_at, created_by, updated_by, create_time, update_time`

func (s *Store) CreateCard(parent context.Context, card Card) (Card, error) {
	if err := s.available(); err != nil {
		return Card{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	normalizeCard(&card)
	if card.Status == "" {
		card.Status = StatusDraft
	}
	if card.Status != StatusDraft {
		return Card{}, fmt.Errorf("create card: status %s: %w", card.Status, ErrCardNotEditable)
	}
	if err := ValidateCard(card); err != nil {
		return Card{}, fmt.Errorf("create card: %w", err)
	}
	created, err := scanCard(s.db.QueryRowContext(ctx, `
		INSERT INTO theory_cards (
			library_id, canonical_key, canonical_name, aliases, domain, subdomain, card_kind,
			summary, definition, core_claim, mechanism, applicable_context, non_applicable_context,
			observable_signals, common_triggers, automatic_pattern, resource_state, shadow_or_risk,
			growth_direction, epistemic_status, evidence_level, clinical_safety, controversy_notes,
			cultural_context, authority_level, language, status, version, reviewed_by, reviewed_at,
			published_at, created_by, updated_by
		) VALUES (
			$1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33
		) RETURNING `+cardReturningColumns,
		card.LibraryID, card.CanonicalKey, card.CanonicalName, jsonArgument(card.Aliases, `[]`), card.Domain,
		card.Subdomain, card.CardKind, card.Summary, card.Definition, card.CoreClaim, card.Mechanism,
		card.ApplicableContext, card.NonApplicableContext, jsonArgument(card.ObservableSignals, `[]`),
		jsonArgument(card.CommonTriggers, `[]`), card.AutomaticPattern, card.ResourceState, card.ShadowOrRisk,
		card.GrowthDirection, card.EpistemicStatus, card.EvidenceLevel, card.ClinicalSafety,
		card.ControversyNotes, card.CulturalContext, card.AuthorityLevel, card.Language, card.Status,
		card.Version, card.ReviewedBy, card.ReviewedAt, card.PublishedAt, card.CreatedBy, card.UpdatedBy,
	))
	if err != nil {
		return Card{}, fmt.Errorf("create card: %w", err)
	}
	return created, nil
}

func (s *Store) UpdateCard(parent context.Context, card Card) (Card, error) {
	if err := s.available(); err != nil {
		return Card{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	normalizeCard(&card)
	if card.ID <= 0 {
		return Card{}, fmt.Errorf("update card: id must be positive")
	}
	if card.Status != StatusDraft {
		return Card{}, fmt.Errorf("update card: status %s: %w", card.Status, ErrCardNotEditable)
	}
	if err := ValidateCard(card); err != nil {
		return Card{}, fmt.Errorf("update card: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, fmt.Errorf("update card: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	libraryID, err := findCardLibrary(ctx, tx, card.ID)
	if err != nil {
		return Card{}, fmt.Errorf("update card: %w", err)
	}
	if libraryID != card.LibraryID {
		return Card{}, fmt.Errorf("update card: %w", ErrConcurrentUpdate)
	}
	if err := lockTheoryLibraries(ctx, tx, libraryID); err != nil {
		return Card{}, fmt.Errorf("update card: lock library: %w", err)
	}
	updated, err := scanCard(tx.QueryRowContext(ctx, `
		UPDATE theory_cards SET
			canonical_key=$3, canonical_name=$4, aliases=$5::jsonb, domain=$6, subdomain=$7,
			card_kind=$8, summary=$9, definition=$10, core_claim=$11, mechanism=$12,
			applicable_context=$13, non_applicable_context=$14, observable_signals=$15::jsonb,
			common_triggers=$16::jsonb, automatic_pattern=$17, resource_state=$18, shadow_or_risk=$19,
			growth_direction=$20, epistemic_status=$21, evidence_level=$22, clinical_safety=$23,
			controversy_notes=$24, cultural_context=$25, authority_level=$26, language=$27,
			updated_by=$28, version=version+1, update_time=now()
		WHERE id=$1 AND library_id=$2 AND status=$29 AND version=$30
		RETURNING `+cardReturningColumns,
		card.ID, card.LibraryID, card.CanonicalKey, card.CanonicalName, jsonArgument(card.Aliases, `[]`),
		card.Domain, card.Subdomain, card.CardKind, card.Summary, card.Definition, card.CoreClaim,
		card.Mechanism, card.ApplicableContext, card.NonApplicableContext, jsonArgument(card.ObservableSignals, `[]`),
		jsonArgument(card.CommonTriggers, `[]`), card.AutomaticPattern, card.ResourceState, card.ShadowOrRisk,
		card.GrowthDirection, card.EpistemicStatus, card.EvidenceLevel, card.ClinicalSafety,
		card.ControversyNotes, card.CulturalContext, card.AuthorityLevel, card.Language, card.UpdatedBy,
		card.Status, card.Version,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, fmt.Errorf("update card: %w", ErrConcurrentUpdate)
	}
	if err != nil {
		return Card{}, fmt.Errorf("update card: write: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_chunk_embeddings embedding
		SET status = 'stale', error_message = 'card content changed'
		FROM theory_chunks chunk
		WHERE chunk.card_id = $1 AND embedding.chunk_id = chunk.id AND embedding.status IN ('pending','ready')`, card.ID); err != nil {
		return Card{}, fmt.Errorf("update card: invalidate embeddings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Card{}, fmt.Errorf("update card: commit: %w", err)
	}
	return updated, nil
}

func (s *Store) TransitionCard(parent context.Context, cardID int64, from, to CardStatus, reviewerID int64) (Card, error) {
	if err := s.available(); err != nil {
		return Card{}, err
	}
	if cardID <= 0 {
		return Card{}, fmt.Errorf("transition card: id must be positive")
	}
	if !allowedCardTransition(from, to) {
		return Card{}, fmt.Errorf("transition card %s to %s: %w", from, to, ErrInvalidTransition)
	}
	if to == StatusPublished && reviewerID <= 0 {
		return Card{}, fmt.Errorf("transition card: reviewer id must be positive")
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, fmt.Errorf("transition card: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	libraryID, err := findCardLibrary(ctx, tx, cardID)
	if err != nil {
		return Card{}, fmt.Errorf("transition card: %w", err)
	}
	if err := lockTheoryLibraries(ctx, tx, libraryID); err != nil {
		return Card{}, fmt.Errorf("transition card: lock library: %w", err)
	}
	card, err := scanCard(tx.QueryRowContext(ctx, `SELECT `+cardReturningColumns+` FROM theory_cards WHERE id=$1 AND library_id=$2 FOR UPDATE`, cardID, libraryID))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, fmt.Errorf("transition card: %w", ErrConcurrentUpdate)
	}
	if err != nil {
		return Card{}, fmt.Errorf("transition card: lock card: %w", err)
	}
	if card.Status != from {
		return Card{}, fmt.Errorf("transition card: expected %s, found %s: %w", from, card.Status, ErrConcurrentUpdate)
	}
	if to == StatusPublished {
		sources, err := loadCardSources(ctx, tx, card.ID)
		if err != nil {
			return Card{}, fmt.Errorf("transition card: load sources: %w", err)
		}
		publishCandidate := card
		publishCandidate.Status = StatusPublished
		if err := ValidateCardForPublish(publishCandidate, sources); err != nil {
			return Card{}, fmt.Errorf("transition card: publish validation: %w", err)
		}
		practices, err := loadCardPractices(ctx, tx, card.ID)
		if err != nil {
			return Card{}, fmt.Errorf("transition card: load practices: %w", err)
		}
		publishCandidate.Version = card.Version + 1
		draftPractices := 0
		for _, practice := range practices {
			if err := ValidatePracticeForPublish(practice, publishCandidate); err != nil {
				return Card{}, fmt.Errorf("transition card: practice %d publish validation: %w", practice.ID, err)
			}
			if practice.Status == StatusDraft {
				draftPractices++
			}
		}
		if draftPractices > 0 {
			result, err := tx.ExecContext(ctx, `UPDATE theory_practices SET status='published', version=$2, update_time=now() WHERE card_id=$1 AND status='draft'`, card.ID, publishCandidate.Version)
			if err != nil {
				return Card{}, fmt.Errorf("transition card: publish practices: %w", err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != int64(draftPractices) {
				return Card{}, fmt.Errorf("transition card: publish practice count changed: %w", ErrConcurrentUpdate)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE theory_cards SET status = 'superseded', updated_by=$4, update_time=now()
			WHERE library_id=$1 AND canonical_key=$2 AND status='published' AND id<>$3`, card.LibraryID, card.CanonicalKey, card.ID, reviewerID); err != nil {
			return Card{}, fmt.Errorf("transition card: supersede replacement: %w", err)
		}
	}
	updated, err := scanCard(tx.QueryRowContext(ctx, `
		UPDATE theory_cards SET
			status=$2,
			reviewed_by=CASE WHEN $2='published' THEN $3 ELSE reviewed_by END,
			reviewed_at=CASE WHEN $2='published' THEN now() ELSE reviewed_at END,
			published_at=CASE WHEN $2='published' THEN now() ELSE published_at END,
			updated_by=CASE WHEN $3 > 0 THEN $3 ELSE updated_by END,
			version=CASE WHEN $2='published' THEN version+1 ELSE version END, update_time=now()
		WHERE id=$1 AND status=$4
		RETURNING `+cardReturningColumns, card.ID, to, reviewerID, from))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, fmt.Errorf("transition card: %w", ErrConcurrentUpdate)
	}
	if err != nil {
		return Card{}, fmt.Errorf("transition card: update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Card{}, fmt.Errorf("transition card: commit: %w", err)
	}
	return updated, nil
}

func loadCardPractices(ctx context.Context, tx *sql.Tx, cardID int64) ([]Practice, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, card_id, goal, estimated_minutes, steps, reflection_prompts, expected_feedback,
			stop_conditions, professional_escalation, contraindications, practice_schema_version,
			status, version, create_time, update_time
		FROM theory_practices WHERE card_id=$1 ORDER BY id FOR UPDATE`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var practices []Practice
	for rows.Next() {
		practice, err := scanPractice(rows)
		if err != nil {
			return nil, err
		}
		practices = append(practices, practice)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return practices, nil
}

func allowedCardTransition(from, to CardStatus) bool {
	return (from == StatusDraft && to == StatusInReview) ||
		(from == StatusInReview && to == StatusDraft) ||
		(from == StatusInReview && to == StatusPublished) ||
		(from == StatusPublished && to == StatusSuperseded) ||
		(from == StatusSuperseded && to == StatusRetired)
}

func findCardLibrary(ctx context.Context, tx *sql.Tx, cardID int64) (int64, error) {
	var libraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT library_id FROM theory_cards WHERE id=$1`, cardID).Scan(&libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrCardNotFound
		}
		return 0, err
	}
	return libraryID, nil
}

func loadCardSources(ctx context.Context, tx *sql.Tx, cardID int64) ([]CardSource, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, card_id, work_id, file_id, source_role, chapter, page_start, page_end,
			location_label, quotation, interpretation_note, extraction_quality, quote_verified,
			verified_by, verified_at, create_time, update_time
		FROM theory_card_sources WHERE card_id=$1 ORDER BY id FOR UPDATE`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []CardSource
	for rows.Next() {
		source, err := scanCardSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sources, nil
}

func normalizeCard(card *Card) {
	card.CanonicalKey = strings.TrimSpace(card.CanonicalKey)
	card.CanonicalName = strings.TrimSpace(card.CanonicalName)
	card.Aliases = normalizedJSON(card.Aliases, `[]`)
	card.ObservableSignals = normalizedJSON(card.ObservableSignals, `[]`)
	card.CommonTriggers = normalizedJSON(card.CommonTriggers, `[]`)
	card.Language = strings.TrimSpace(card.Language)
}

func scanCard(row rowScanner) (Card, error) {
	var card Card
	err := row.Scan(&card.ID, &card.LibraryID, &card.CanonicalKey, &card.CanonicalName, &card.Aliases,
		&card.Domain, &card.Subdomain, &card.CardKind, &card.Summary, &card.Definition, &card.CoreClaim,
		&card.Mechanism, &card.ApplicableContext, &card.NonApplicableContext, &card.ObservableSignals,
		&card.CommonTriggers, &card.AutomaticPattern, &card.ResourceState, &card.ShadowOrRisk,
		&card.GrowthDirection, &card.EpistemicStatus, &card.EvidenceLevel, &card.ClinicalSafety,
		&card.ControversyNotes, &card.CulturalContext, &card.AuthorityLevel, &card.Language, &card.Status,
		&card.Version, &card.ReviewedBy, &card.ReviewedAt, &card.PublishedAt, &card.CreatedBy,
		&card.UpdatedBy, &card.CreateTime, &card.UpdateTime)
	return card, err
}

func scanCardSource(row rowScanner) (CardSource, error) {
	var source CardSource
	err := row.Scan(&source.ID, &source.CardID, &source.WorkID, &source.FileID, &source.SourceRole,
		&source.Chapter, &source.PageStart, &source.PageEnd, &source.LocationLabel, &source.Quotation,
		&source.InterpretationNote, &source.ExtractionQuality, &source.QuoteVerified, &source.VerifiedBy,
		&source.VerifiedAt, &source.CreateTime, &source.UpdateTime)
	return source, err
}
