package theorystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrVectorUnavailable        = errors.New("theory embedding vector column unavailable")
	ErrChunkVersionConflict     = errors.New("theory chunk version already exists")
	ErrChunkCardVersionMismatch = errors.New("theory chunk version does not match published card")
	ErrChunkNotFound            = errors.New("theory chunk not found")
	ErrInvalidContentOwnership  = errors.New("theory content ownership mismatch")
	ErrOwnershipChanged         = errors.New("theory content ownership changed concurrently")
	ErrEmbeddingNotPending      = errors.New("theory embedding generation is not pending")
	ErrEmbeddingAlreadyReady    = errors.New("theory embedding generation is already ready")
	ErrPracticeNotEditable      = errors.New("theory practice is not an editable draft")
	ErrPracticeNotPublishable   = errors.New("theory practice is not publishable for this chunk")
)

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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CardSource{}, fmt.Errorf("save card source: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCardSourceScope(ctx, tx, source); err != nil {
		return CardSource{}, fmt.Errorf("save card source: %w", err)
	}
	saved, err := scanCardSource(tx.QueryRowContext(ctx, `
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
	if err := tx.Commit(); err != nil {
		return CardSource{}, fmt.Errorf("save card source: commit: %w", err)
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
	if practice.Status != StatusDraft {
		return Practice{}, fmt.Errorf("save practice: status %s: %w", practice.Status, ErrPracticeNotEditable)
	}
	if err := ValidatePractice(practice); err != nil {
		return Practice{}, fmt.Errorf("save practice: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Practice{}, fmt.Errorf("save practice: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockEditableCard(ctx, tx, practice.CardID, 0); err != nil {
		return Practice{}, fmt.Errorf("save practice: %w", err)
	}
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
		WHERE chunk.practice_id=$1 AND embedding.chunk_id=chunk.id AND embedding.status IN ('pending','ready')`, saved.ID); err != nil {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Relation{}, fmt.Errorf("save relation: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockEditableRelationCards(ctx, tx, relation.FromCardID, relation.ToCardID); err != nil {
		return Relation{}, fmt.Errorf("save relation: %w", err)
	}
	saved, err := scanRelation(tx.QueryRowContext(ctx, `
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
	if err := tx.Commit(); err != nil {
		return Relation{}, fmt.Errorf("save relation: commit: %w", err)
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
	if err := lockChunkScope(ctx, tx, chunk); err != nil {
		return Chunk{}, fmt.Errorf("save chunk: %w", err)
	}
	saved, err := scanChunk(tx.QueryRowContext(ctx, `
		INSERT INTO theory_chunks (
			library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, keywords, tags,
			authority_level, evidence_level, clinical_safety, token_count, content_hash, version, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (library_id, chunk_key, version) DO NOTHING
		RETURNING id, library_id, card_id, practice_id, chunk_key, chunk_kind, title, content, keywords,
			tags, authority_level, evidence_level, clinical_safety, token_count, content_hash, version,
			status, create_time, update_time`,
		chunk.LibraryID, chunk.CardID, chunk.PracticeID, chunk.ChunkKey, chunk.ChunkKind, chunk.Title,
		chunk.Content, jsonArgument(chunk.Keywords, `[]`), jsonArgument(chunk.Tags, `[]`), chunk.AuthorityLevel,
		chunk.EvidenceLevel, chunk.ClinicalSafety, chunk.TokenCount, chunk.ContentHash, chunk.Version, chunk.Status))
	if errors.Is(err, sql.ErrNoRows) {
		return Chunk{}, fmt.Errorf("save chunk: %w", ErrChunkVersionConflict)
	}
	if err != nil {
		return Chunk{}, fmt.Errorf("save chunk: write: %w", err)
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
	if len(record.Embedding) > 0 && record.Status != EmbeddingStatusReady {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: embedding must be empty unless status is ready")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var initialLibraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT library_id FROM theory_chunks WHERE id=$1`, record.ChunkID).Scan(&initialLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", ErrChunkNotFound)
		}
		return EmbeddingRecord{}, fmt.Errorf("save embedding: find chunk library: %w", err)
	}
	if err := lockTheoryLibraries(ctx, tx, initialLibraryID); err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: lock library: %w", err)
	}
	var lockedLibraryID int64
	var currentHash string
	if err := tx.QueryRowContext(ctx, `SELECT library_id, content_hash FROM theory_chunks WHERE id=$1 FOR UPDATE`, record.ChunkID).Scan(&lockedLibraryID, &currentHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", ErrChunkNotFound)
		}
		return EmbeddingRecord{}, fmt.Errorf("save embedding: lock chunk: %w", err)
	}
	if lockedLibraryID != initialLibraryID {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: chunk library changed: %w", ErrConcurrentUpdate)
	}
	if currentHash != record.ContentHash {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: input hash differs from current chunk: %w", ErrStaleEmbedding)
	}
	var existingStatus EmbeddingStatus
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM theory_chunk_embeddings
		WHERE chunk_id=$1 AND embedding_model=$2 AND content_hash=$3
		FOR UPDATE`, record.ChunkID, record.EmbeddingModel, record.ContentHash).Scan(&existingStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: lock generation: %w", err)
	}
	if err == nil && existingStatus == EmbeddingStatusStale && record.Status != EmbeddingStatusPending {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: generation was invalidated: %w", ErrStaleEmbedding)
	}
	if record.Status == EmbeddingStatusPending && err == nil {
		switch existingStatus {
		case EmbeddingStatusReady:
			return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", ErrEmbeddingAlreadyReady)
		case EmbeddingStatusPending:
			saved, scanErr := scanEmbedding(tx.QueryRowContext(ctx, `
				SELECT id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message
				FROM theory_chunk_embeddings
				WHERE chunk_id=$1 AND embedding_model=$2 AND content_hash=$3`, record.ChunkID, record.EmbeddingModel, record.ContentHash))
			if scanErr != nil {
				return EmbeddingRecord{}, fmt.Errorf("save embedding pending: reload: %w", scanErr)
			}
			if err := tx.Commit(); err != nil {
				return EmbeddingRecord{}, fmt.Errorf("save embedding pending: commit: %w", err)
			}
			return saved, nil
		case EmbeddingStatusFailed, EmbeddingStatusStale:
			saved, scanErr := scanEmbedding(tx.QueryRowContext(ctx, `
				UPDATE theory_chunk_embeddings SET dimensions=$4, embedded_at=NULL, status='pending', error_message=''
				WHERE chunk_id=$1 AND embedding_model=$2 AND content_hash=$3 AND status=$5
				RETURNING id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message`,
				record.ChunkID, record.EmbeddingModel, record.ContentHash, record.Dimensions, existingStatus))
			if errors.Is(scanErr, sql.ErrNoRows) {
				return EmbeddingRecord{}, fmt.Errorf("save embedding pending: %w", ErrConcurrentUpdate)
			}
			if scanErr != nil {
				return EmbeddingRecord{}, fmt.Errorf("save embedding pending: restart: %w", scanErr)
			}
			if err := tx.Commit(); err != nil {
				return EmbeddingRecord{}, fmt.Errorf("save embedding pending: commit: %w", err)
			}
			return saved, nil
		}
	}
	if (record.Status == EmbeddingStatusReady || record.Status == EmbeddingStatusFailed) && (errors.Is(err, sql.ErrNoRows) || existingStatus != EmbeddingStatusPending) {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: %s requires an existing pending generation: %w", record.Status, ErrEmbeddingNotPending)
	}
	if record.Status == EmbeddingStatusFailed {
		saved, err := scanEmbedding(tx.QueryRowContext(ctx, `
			UPDATE theory_chunk_embeddings SET embedded_at=$4, status='failed', error_message=$5
			WHERE chunk_id=$1 AND embedding_model=$2 AND content_hash=$3 AND status='pending'
			RETURNING id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message`,
			record.ChunkID, record.EmbeddingModel, record.ContentHash, record.EmbeddedAt, record.ErrorMessage))
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddingRecord{}, fmt.Errorf("save embedding failed result: %w", ErrEmbeddingNotPending)
		}
		if err != nil {
			return EmbeddingRecord{}, fmt.Errorf("save embedding failed result: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return EmbeddingRecord{}, fmt.Errorf("save embedding failed result: commit: %w", err)
		}
		return saved, nil
	}
	if record.Status != EmbeddingStatusReady {
		saved, err := scanEmbedding(tx.QueryRowContext(ctx, `
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
		if err := tx.Commit(); err != nil {
			return EmbeddingRecord{}, fmt.Errorf("save embedding metadata: commit: %w", err)
		}
		return saved, nil
	}
	capable, err := vectorColumnAvailable(ctx, tx)
	if err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: detect vector capability: %w", err)
	}
	if !capable {
		return EmbeddingRecord{}, fmt.Errorf("save embedding: %w", ErrVectorUnavailable)
	}
	saved, err := scanEmbedding(tx.QueryRowContext(ctx, `
		UPDATE theory_chunk_embeddings SET dimensions=$4, embedding=$5::vector, embedded_at=$6,
			status='ready', error_message=$7
		WHERE chunk_id=$1 AND embedding_model=$2 AND content_hash=$3 AND status='pending'
		RETURNING id, chunk_id, embedding_model, dimensions, content_hash, embedded_at, status, error_message`,
		record.ChunkID, record.EmbeddingModel, record.ContentHash, record.Dimensions,
		vectorArgument(record.Embedding), record.EmbeddedAt, record.ErrorMessage))
	if errors.Is(err, sql.ErrNoRows) {
		return EmbeddingRecord{}, fmt.Errorf("save embedding vector: %w", ErrEmbeddingNotPending)
	}
	if err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding vector: %w", err)
	}
	saved.Embedding = append([]float32(nil), record.Embedding...)
	if err := tx.Commit(); err != nil {
		return EmbeddingRecord{}, fmt.Errorf("save embedding vector: commit: %w", err)
	}
	return saved, nil
}

func lockEditableCard(ctx context.Context, tx *sql.Tx, cardID, expectedLibraryID int64) (int64, error) {
	var libraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT library_id FROM theory_cards WHERE id=$1`, cardID).Scan(&libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrCardNotFound
		}
		return 0, err
	}
	if expectedLibraryID > 0 && libraryID != expectedLibraryID {
		return 0, ErrConcurrentUpdate
	}
	if err := lockTheoryLibraries(ctx, tx, libraryID); err != nil {
		return 0, err
	}
	if err := requireDraftCardRow(ctx, tx, cardID, libraryID); err != nil {
		return 0, err
	}
	return libraryID, nil
}

func lockCardSourceScope(ctx context.Context, tx *sql.Tx, source CardSource) error {
	var cardLibraryID, workLibraryID int64
	var fileLibraryID, fileWorkID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT card.library_id, work.library_id, file_work.library_id, file.work_id
		FROM theory_cards card
		JOIN theory_source_works work ON work.id=$2
		LEFT JOIN theory_source_files file ON file.id=$3
		LEFT JOIN theory_source_works file_work ON file_work.id=file.work_id
		WHERE card.id=$1`, source.CardID, source.WorkID, source.FileID).Scan(&cardLibraryID, &workLibraryID, &fileLibraryID, &fileWorkID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidContentOwnership
		}
		return err
	}
	libraries := []int64{cardLibraryID, workLibraryID}
	if fileLibraryID.Valid {
		libraries = append(libraries, fileLibraryID.Int64)
	}
	if err := lockContentLibraries(ctx, tx, libraries); err != nil {
		return err
	}
	var status CardStatus
	var lockedCardLibraryID, lockedWorkLibraryID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT card.status, card.library_id, work.library_id
		FROM theory_cards card
		JOIN theory_source_works work ON work.id=$2
		WHERE card.id=$1
		FOR UPDATE OF card FOR SHARE OF work`, source.CardID, source.WorkID).Scan(&status, &lockedCardLibraryID, &lockedWorkLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidContentOwnership
		}
		return err
	}
	if status != StatusDraft {
		return ErrCardNotEditable
	}
	if lockedCardLibraryID != cardLibraryID || lockedWorkLibraryID != workLibraryID || lockedCardLibraryID != lockedWorkLibraryID {
		return ErrInvalidContentOwnership
	}
	if source.FileID != nil {
		var lockedFileWorkID int64
		if err := tx.QueryRowContext(ctx, `SELECT file.work_id FROM theory_source_files file WHERE file.id=$1 FOR SHARE`, *source.FileID).Scan(&lockedFileWorkID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidContentOwnership
			}
			return err
		}
		if lockedFileWorkID != source.WorkID {
			return ErrInvalidContentOwnership
		}
	}
	return nil
}

func lockChunkScope(ctx context.Context, tx *sql.Tx, chunk Chunk) error {
	var cardLibraryID int64
	var practiceCardID, practiceLibraryID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT card.library_id, practice.card_id, practice_card.library_id
		FROM theory_cards card
		LEFT JOIN theory_practices practice ON practice.id=$2
		LEFT JOIN theory_cards practice_card ON practice_card.id=practice.card_id
		WHERE card.id=$1`, chunk.CardID, chunk.PracticeID).Scan(&cardLibraryID, &practiceCardID, &practiceLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidContentOwnership
		}
		return err
	}
	libraries := []int64{chunk.LibraryID, cardLibraryID}
	if practiceLibraryID.Valid {
		libraries = append(libraries, practiceLibraryID.Int64)
	}
	if err := lockContentLibraries(ctx, tx, libraries); err != nil {
		return err
	}
	var status CardStatus
	var lockedCardLibraryID int64
	var lockedCardVersion int
	if err := tx.QueryRowContext(ctx, `
		SELECT card.status, card.library_id, card.version
		FROM theory_cards card
		WHERE card.id=$1
		FOR UPDATE OF card`, chunk.CardID).Scan(&status, &lockedCardLibraryID, &lockedCardVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidContentOwnership
		}
		return err
	}
	if status != StatusPublished {
		return ErrCardNotEditable
	}
	if lockedCardLibraryID != chunk.LibraryID {
		return ErrInvalidContentOwnership
	}
	if lockedCardVersion != chunk.Version {
		return ErrChunkCardVersionMismatch
	}
	if chunk.PracticeID != nil {
		var lockedPracticeCardID, lockedPracticeLibraryID int64
		var lockedPracticeStatus CardStatus
		var lockedPracticeVersion int
		if err := tx.QueryRowContext(ctx, `
			SELECT practice.card_id, practice_card.library_id, practice.status, practice.version
			FROM theory_practices practice
			JOIN theory_cards practice_card ON practice_card.id=practice.card_id
			WHERE practice.id=$1
			FOR SHARE OF practice, practice_card`, *chunk.PracticeID).Scan(&lockedPracticeCardID, &lockedPracticeLibraryID, &lockedPracticeStatus, &lockedPracticeVersion); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidContentOwnership
			}
			return err
		}
		if lockedPracticeCardID != chunk.CardID || lockedPracticeLibraryID != chunk.LibraryID {
			return ErrInvalidContentOwnership
		}
		if lockedPracticeStatus != StatusPublished || lockedPracticeVersion != chunk.Version {
			return ErrPracticeNotPublishable
		}
	}
	return nil
}

func lockContentLibraries(ctx context.Context, tx *sql.Tx, libraryIDs []int64) error {
	unique := make([]int64, 0, 3)
	seen := make(map[int64]struct{}, 3)
	for _, id := range libraryIDs {
		if id <= 0 {
			return ErrInvalidContentOwnership
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	var row *sql.Row
	switch len(unique) {
	case 1:
		row = tx.QueryRowContext(ctx, `SELECT lock_theory_libraries(ARRAY[$1]::BIGINT[])`, unique[0])
	case 2:
		row = tx.QueryRowContext(ctx, `SELECT lock_theory_libraries(ARRAY[$1,$2]::BIGINT[])`, unique[0], unique[1])
	case 3:
		row = tx.QueryRowContext(ctx, `SELECT lock_theory_libraries(ARRAY[$1,$2,$3]::BIGINT[])`, unique[0], unique[1], unique[2])
	default:
		return ErrInvalidContentOwnership
	}
	var result any
	return row.Scan(&result)
}

func requireDraftCardRow(ctx context.Context, tx *sql.Tx, cardID, libraryID int64) error {
	var status CardStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM theory_cards WHERE id=$1 AND library_id=$2 FOR UPDATE`, cardID, libraryID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCardNotFound
		}
		return err
	}
	if status != StatusDraft {
		return ErrCardNotEditable
	}
	return nil
}

func lockEditableRelationCards(ctx context.Context, tx *sql.Tx, fromCardID, toCardID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, library_id FROM theory_cards WHERE id IN ($1,$2) ORDER BY id`, fromCardID, toCardID)
	if err != nil {
		return err
	}
	defer rows.Close()
	libraries := make(map[int64]int64, 2)
	for rows.Next() {
		var id, libraryID int64
		if err := rows.Scan(&id, &libraryID); err != nil {
			return err
		}
		libraries[id] = libraryID
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(libraries) != 2 {
		return ErrCardNotFound
	}
	if err := lockTheoryLibraries(ctx, tx, libraries[fromCardID], libraries[toCardID]); err != nil {
		return err
	}
	locked, err := tx.QueryContext(ctx, `SELECT id, library_id, status FROM theory_cards WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, fromCardID, toCardID)
	if err != nil {
		return err
	}
	defer locked.Close()
	count := 0
	for locked.Next() {
		var id, libraryID int64
		var status CardStatus
		if err := locked.Scan(&id, &libraryID, &status); err != nil {
			return err
		}
		count++
		if libraries[id] != libraryID {
			return ErrOwnershipChanged
		}
		if status != StatusDraft {
			return ErrCardNotEditable
		}
	}
	if err := locked.Err(); err != nil {
		return err
	}
	if count != 2 {
		return ErrCardNotFound
	}
	return nil
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
