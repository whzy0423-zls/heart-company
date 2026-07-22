package theorystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrVectorUnavailable = errors.New("theory embedding vector column unavailable")

func (s *Store) SaveCardSource(parent context.Context, source CardSource) (CardSource, error) {
	if err := s.available(); err != nil {
		return CardSource{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	normalizeCardSource(&source)
	if err := ValidateCardSource(source); err != nil {
		return CardSource{}, fmt.Errorf("save card source: %w", err)
	}
	saved, err := scanCardSource(s.db.QueryRowContext(ctx, `
		INSERT INTO theory_card_sources (
			card_id, work_id, file_id, source_role, chapter, page_start, page_end, location_label,
			quotation, interpretation_note, extraction_quality, quote_verified, verified_by, verified_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, card_id, work_id, file_id, source_role, chapter, page_start, page_end,
			location_label, quotation, interpretation_note, extraction_quality, quote_verified,
			verified_by, verified_at, create_time, update_time`,
		source.CardID, source.WorkID, source.FileID, source.SourceRole, source.Chapter, source.PageStart,
		source.PageEnd, source.LocationLabel, source.Quotation, source.InterpretationNote,
		source.ExtractionQuality, source.QuoteVerified, source.VerifiedBy, source.VerifiedAt))
	if err != nil {
		return CardSource{}, fmt.Errorf("save card source: %w", err)
	}
	return saved, nil
}

func (s *Store) SavePractice(parent context.Context, practice Practice) (Practice, error) {
	if err := s.available(); err != nil {
		return Practice{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	normalizePractice(&practice)
	if err := ValidatePractice(practice); err != nil {
		return Practice{}, fmt.Errorf("save practice: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Practice{}, fmt.Errorf("save practice: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := scanPractice(tx.QueryRowContext(ctx, `
		INSERT INTO theory_practices (
			card_id, goal, estimated_minutes, steps, reflection_prompts, expected_feedback,
			stop_conditions, professional_escalation, contraindications, practice_schema_version,
			status, version
		) VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$11,$12)
		ON CONFLICT (card_id, version) DO UPDATE SET
			goal=EXCLUDED.goal, estimated_minutes=EXCLUDED.estimated_minutes, steps=EXCLUDED.steps,
			reflection_prompts=EXCLUDED.reflection_prompts, expected_feedback=EXCLUDED.expected_feedback,
			stop_conditions=EXCLUDED.stop_conditions, professional_escalation=EXCLUDED.professional_escalation,
			contraindications=EXCLUDED.contraindications, practice_schema_version=EXCLUDED.practice_schema_version,
			status=EXCLUDED.status, update_time=now()
		RETURNING id, card_id, goal, estimated_minutes, steps, reflection_prompts, expected_feedback,
			stop_conditions, professional_escalation, contraindications, practice_schema_version,
			status, version, create_time, update_time`,
		practice.CardID, practice.Goal, practice.EstimatedMinutes, jsonArgument(practice.Steps, `[]`),
		jsonArgument(practice.ReflectionPrompts, `[]`), jsonArgument(practice.ExpectedFeedback, `[]`),
		jsonArgument(practice.StopConditions, `[]`), jsonArgument(practice.ProfessionalEscalation, `[]`),
		practice.Contraindications, practice.PracticeSchemaVersion, practice.Status, practice.Version))
	if err != nil {
		return Practice{}, fmt.Errorf("save practice: write: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_chunk_embeddings embedding SET status = 'stale', error_message = 'practice content changed'
		FROM theory_chunks chunk
		WHERE chunk.practice_id=$1 AND embedding.chunk_id=chunk.id AND embedding.status='ready'`, saved.ID); err != nil {
		return Practice{}, fmt.Errorf("save practice: invalidate embeddings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Practice{}, fmt.Errorf("save practice: commit: %w", err)
	}
	return saved, nil
}

func (s *Store) SaveRelation(parent context.Context, relation Relation) (Relation, error) {
	if err := s.available(); err != nil {
		return Relation{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	relation.Note = strings.TrimSpace(relation.Note)
	if err := ValidateRelation(relation); err != nil {
		return Relation{}, fmt.Errorf("save relation: %w", err)
	}
	saved, err := scanRelation(s.db.QueryRowContext(ctx, `
		INSERT INTO theory_card_relations (
			from_card_id, to_card_id, relation_type, note, confidence, status, created_by, reviewed_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (from_card_id, to_card_id, relation_type) DO UPDATE SET
			note=EXCLUDED.note, confidence=EXCLUDED.confidence, status=EXCLUDED.status,
			reviewed_by=EXCLUDED.reviewed_by, update_time=now()
		RETURNING id, from_card_id, to_card_id, relation_type, note, confidence, status,
			created_by, reviewed_by, create_time, update_time`,
		relation.FromCardID, relation.ToCardID, relation.RelationType, relation.Note, relation.Confidence,
		relation.Status, relation.CreatedBy, relation.ReviewedBy))
	if err != nil {
		return Relation{}, fmt.Errorf("save relation: %w", err)
	}
	return saved, nil
}

func (s *Store) SaveChunk(parent context.Context, chunk Chunk) (Chunk, error) {
	if err := s.available(); err != nil {
		return Chunk{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	normalizeChunk(&chunk)
	if err := ValidateChunk(chunk); err != nil {
		return Chunk{}, fmt.Errorf("save chunk: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Chunk{}, fmt.Errorf("save chunk: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := scanChunk(tx.QueryRowContext(ctx, `
		INSERT INTO theory_chunks (
			library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, keywords, tags,
			authority_level, evidence_level, clinical_safety, token_count, content_hash, version, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (library_id, chunk_key, version) DO UPDATE SET
			card_id=EXCLUDED.card_id, practice_id=EXCLUDED.practice_id, chunk_kind=EXCLUDED.chunk_kind,
			title=EXCLUDED.title, content=EXCLUDED.content, keywords=EXCLUDED.keywords, tags=EXCLUDED.tags,
			authority_level=EXCLUDED.authority_level, evidence_level=EXCLUDED.evidence_level,
			clinical_safety=EXCLUDED.clinical_safety, token_count=EXCLUDED.token_count,
			content_hash=EXCLUDED.content_hash, status=EXCLUDED.status, update_time=now()
		RETURNING id, library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, keywords,
			tags, authority_level, evidence_level, clinical_safety, token_count, content_hash, version,
			status, create_time, update_time`,
		chunk.LibraryID, chunk.CardID, chunk.PracticeID, chunk.ChunkKey, chunk.ChunkKind, chunk.Title,
		chunk.Content, jsonArgument(chunk.Keywords, `[]`), jsonArgument(chunk.Tags, `[]`), chunk.AuthorityLevel,
		chunk.EvidenceLevel, chunk.ClinicalSafety, chunk.TokenCount, chunk.ContentHash, chunk.Version, chunk.Status))
	if err != nil {
		return Chunk{}, fmt.Errorf("save chunk: write: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_chunk_embeddings SET status='stale', error_message='chunk content hash changed'
		WHERE chunk_id=$1 AND content_hash <> $2 AND status='ready'`, saved.ID, saved.ContentHash); err != nil {
		return Chunk{}, fmt.Errorf("save chunk: invalidate embeddings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Chunk{}, fmt.Errorf("save chunk: commit: %w", err)
	}
	return saved, nil
}

func (s *Store) SaveEmbeddingRecord(parent context.Context, record EmbeddingRecord) (EmbeddingRecord, error) {
	if err := s.available(); err != nil {
		return EmbeddingRecord{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	record.EmbeddingModel = strings.TrimSpace(record.EmbeddingModel)
	record.ContentHash = strings.TrimSpace(record.ContentHash)
	record.ErrorMessage = strings.TrimSpace(record.ErrorMessage)
	if err := ValidateEmbeddingRecord(record); err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", err)
	}
	if len(record.Embedding) == 0 {
		saved, err := scanEmbedding(s.db.QueryRowContext(ctx, `
			INSERT INTO theory_chunk_embeddings (chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (chunk_id, embedding_model, content_hash) DO UPDATE SET
				dimensions=EXCLUDED.dimensions, embedded_at=EXCLUDED.embedded_at,
				status=EXCLUDED.status, error_message=EXCLUDED.error_message
			RETURNING id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message`,
			record.ChunkID, record.EmbeddingModel, record.Dimensions, record.ContentHash,
			record.EmbeddedAt, record.Status, record.ErrorMessage))
		if err != nil {
			return EmbeddingRecord{}, fmt.Errorf("save embedding metadata: %w", err)
		}
		return saved, nil
	}
	capable, err := vectorColumnAvailable(ctx, s.db)
	if err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: detect vector capability: %w", err)
	}
	if !capable {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", ErrVectorUnavailable)
	}
	saved, err := scanEmbedding(s.db.QueryRowContext(ctx, `
		INSERT INTO theory_chunk_embeddings (chunk_id, embedding_model, dimensions, content_hash, embedding, embedded_at, status, error_message)
		VALUES ($1,$2,$3,$4,$5::vector,$6,$7,$8)
		ON CONFLICT (chunk_id, embedding_model, content_hash) DO UPDATE SET
			dimensions=EXCLUDED.dimensions, embedding = EXCLUDED.embedding, embedded_at=EXCLUDED.embedded_at,
			status=EXCLUDED.status, error_message=EXCLUDED.error_message
		RETURNING id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message`,
		record.ChunkID, record.EmbeddingModel, record.Dimensions, record.ContentHash,
		vectorArgument(record.Embedding), record.EmbeddedAt, record.Status, record.ErrorMessage))
	if err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding vector: %w", err)
	}
	saved.Embedding = append([]float32(nil), record.Embedding...)
	return saved, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func vectorColumnAvailable(ctx context.Context, db queryRower) (bool, error) {
	var capable bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema=current_schema() AND table_name='theory_chunk_embeddings' AND column_name='embedding'
		)`).Scan(&capable)
	return capable, err
}

func vectorArgument(values []float32) string {
	var builder strings.Builder
	builder.Grow(len(values) * 8)
	builder.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String()
}

func normalizeCardSource(source *CardSource) {
	source.Chapter = strings.TrimSpace(source.Chapter)
	source.LocationLabel = strings.TrimSpace(source.LocationLabel)
	source.Quotation = strings.TrimSpace(source.Quotation)
	source.InterpretationNote = strings.TrimSpace(source.InterpretationNote)
}

func normalizePractice(practice *Practice) {
	practice.Goal = strings.TrimSpace(practice.Goal)
	practice.Steps = normalizedJSON(practice.Steps, `[]`)
	practice.ReflectionPrompts = normalizedJSON(practice.ReflectionPrompts, `[]`)
	practice.ExpectedFeedback = normalizedJSON(practice.ExpectedFeedback, `[]`)
	practice.StopConditions = normalizedJSON(practice.StopConditions, `[]`)
	practice.ProfessionalEscalation = normalizedJSON(practice.ProfessionalEscalation, `[]`)
	practice.Contraindications = strings.TrimSpace(practice.Contraindications)
}

func normalizeChunk(chunk *Chunk) {
	chunk.ChunkKey = strings.TrimSpace(chunk.ChunkKey)
	chunk.Title = strings.TrimSpace(chunk.Title)
	chunk.Content = strings.TrimSpace(chunk.Content)
	chunk.Keywords = normalizedJSON(chunk.Keywords, `[]`)
	chunk.Tags = normalizedJSON(chunk.Tags, `[]`)
	chunk.ContentHash = strings.TrimSpace(chunk.ContentHash)
}

func scanPractice(row rowScanner) (Practice, error) {
	var p Practice
	err := row.Scan(&p.ID, &p.CardID, &p.Goal, &p.EstimatedMinutes, &p.Steps, &p.ReflectionPrompts,
		&p.ExpectedFeedback, &p.StopConditions, &p.ProfessionalEscalation, &p.Contraindications,
		&p.PracticeSchemaVersion, &p.Status, &p.Version, &p.CreateTime, &p.UpdateTime)
	return p, err
}

func scanRelation(row rowScanner) (Relation, error) {
	var r Relation
	err := row.Scan(&r.ID, &r.FromCardID, &r.ToCardID, &r.RelationType, &r.Note, &r.Confidence,
		&r.Status, &r.CreatedBy, &r.ReviewedBy, &r.CreateTime, &r.UpdateTime)
	return r, err
}

func scanChunk(row rowScanner) (Chunk, error) {
	var c Chunk
	err := row.Scan(&c.ID, &c.LibraryID, &c.CardID, &c.PracticeID, &c.ChunkKey, &c.ChunkKind,
		&c.Title, &c.Content, &c.Keywords, &c.Tags, &c.AuthorityLevel, &c.EvidenceLevel,
		&c.ClinicalSafety, &c.TokenCount, &c.ContentHash, &c.Version, &c.Status,
		&c.CreateTime, &c.UpdateTime)
	return c, err
}

func scanEmbedding(row rowScanner) (EmbeddingRecord, error) {
	var r EmbeddingRecord
	err := row.Scan(&r.ID, &r.ChunkID, &r.EmbeddingModel, &r.Dimensions, &r.ContentHash,
		&r.EmbeddedAt, &r.Status, &r.ErrorMessage)
	return r, err
}
