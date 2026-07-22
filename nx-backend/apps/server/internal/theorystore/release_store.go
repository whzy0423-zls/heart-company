package theorystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrEmptyReleaseMapping     = errors.New("theory release mapping is empty")
	ErrDuplicateReleaseMapping = errors.New("duplicate theory release mapping")
	ErrInvalidReleaseMapping   = errors.New("invalid theory release mapping")
	ErrReleaseCountMismatch    = errors.New("theory release mapping counts do not match")
	ErrReleaseNotFound         = errors.New("theory release not found")
	ErrReleaseNotReady         = errors.New("theory release is not ready")
	ErrEmbeddingNotReady       = errors.New("theory chunk embedding is not ready")
	ErrStaleEmbedding          = errors.New("theory chunk embedding is stale")
)

type ReleaseMapping struct {
	CardID  int64
	ChunkID int64
}

const releaseReturningColumns = `
		id, library_id, version, status, embedding_model, embedding_dimensions, retrieval_mode,
		index_version, card_count, chunk_count, build_error, activated_by, activated_at,
		create_time, update_time`

func (s *Store) BuildRelease(parent context.Context, release Release, mappings []ReleaseMapping) (Release, error) {
	if err := s.available(); err != nil {
		return Release{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	if release.Status != ReleaseStatusDraft && release.Status != ReleaseStatusBuilding {
		return Release{}, fmt.Errorf("build release: status must be draft or building")
	}
	release.EmbeddingModel = stringTrim(release.EmbeddingModel)
	release.IndexVersion = stringTrim(release.IndexVersion)
	if err := ValidateRelease(release); err != nil {
		return Release{}, fmt.Errorf("build release: %w", err)
	}
	if err := validateReleaseMappings(mappings); err != nil {
		return Release{}, fmt.Errorf("build release: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, fmt.Errorf("build release: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTheoryLibraries(ctx, tx, release.LibraryID); err != nil {
		return Release{}, fmt.Errorf("build release: lock library: %w", err)
	}
	var lockedLibraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM theory_libraries WHERE id=$1 FOR SHARE`, release.LibraryID).Scan(&lockedLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Release{}, fmt.Errorf("build release: library: %w", ErrReleaseNotFound)
		}
		return Release{}, fmt.Errorf("build release: lock library row: %w", err)
	}
	building, err := scanRelease(tx.QueryRowContext(ctx, `
		INSERT INTO theory_library_releases (
			library_id, version, status, embedding_model, embedding_dimensions, retrieval_mode,
			index_version, card_count, chunk_count, build_error
		) VALUES ($1,$2,'building',$3,$4,$5,$6,0,0,'')
		ON CONFLICT (library_id, version) DO UPDATE SET
			status='building', embedding_model=EXCLUDED.embedding_model,
			embedding_dimensions=EXCLUDED.embedding_dimensions, retrieval_mode=EXCLUDED.retrieval_mode,
			index_version=EXCLUDED.index_version, card_count=0, chunk_count=0, build_error='',
			update_time=now()
		WHERE theory_library_releases.status IN ('draft','building','failed')
		RETURNING `+releaseReturningColumns,
		release.LibraryID, release.Version, release.EmbeddingModel, release.EmbeddingDimensions,
		release.RetrievalMode, release.IndexVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return Release{}, fmt.Errorf("build release: %w", ErrReleaseNotReady)
	}
	if err != nil {
		return Release{}, fmt.Errorf("build release: save building release: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM theory_release_cards WHERE release_id=$1`, building.ID); err != nil {
		return Release{}, fmt.Errorf("build release: clear mappings: %w", err)
	}
	if release.RetrievalMode == RetrievalHybrid {
		capable, err := vectorColumnAvailable(ctx, tx)
		if err != nil {
			return Release{}, fmt.Errorf("build release: detect vector capability: %w", err)
		}
		if !capable {
			return Release{}, fmt.Errorf("build release: %w", ErrVectorUnavailable)
		}
	}
	cards := make(map[int64]struct{}, len(mappings))
	written := 0
	for _, mapping := range mappings {
		state, err := loadReleaseMappingState(ctx, tx, release.LibraryID, mapping)
		if err != nil {
			return Release{}, fmt.Errorf("build release mapping card=%d chunk=%d: %w", mapping.CardID, mapping.ChunkID, err)
		}
		if release.RetrievalMode == RetrievalHybrid {
			if err := validateMappingEmbedding(ctx, tx, release, mapping.ChunkID, state.contentHash); err != nil {
				return Release{}, fmt.Errorf("build release chunk %d: %w", mapping.ChunkID, err)
			}
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO theory_release_cards (release_id, card_id, chunk_id) VALUES ($1,$2,$3)`, building.ID, mapping.CardID, mapping.ChunkID)
		if err != nil {
			return Release{}, fmt.Errorf("build release: insert mapping: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return Release{}, fmt.Errorf("build release: mapping rows affected: %w", err)
		}
		if affected != 1 {
			return Release{}, fmt.Errorf("build release: expected one mapping row, got %d: %w", affected, ErrReleaseCountMismatch)
		}
		written++
		cards[mapping.CardID] = struct{}{}
	}
	if written != len(mappings) {
		return Release{}, fmt.Errorf("build release: %w", ErrReleaseCountMismatch)
	}
	ready, err := scanRelease(tx.QueryRowContext(ctx, `
		UPDATE theory_library_releases SET status = 'ready', card_count=$2, chunk_count=$3,
			build_error='', update_time=now()
		WHERE id=$1 AND status='building'
		RETURNING `+releaseReturningColumns, building.ID, len(cards), written))
	if errors.Is(err, sql.ErrNoRows) {
		return Release{}, fmt.Errorf("build release: finalize: %w", ErrConcurrentUpdate)
	}
	if err != nil {
		return Release{}, fmt.Errorf("build release: finalize: %w", err)
	}
	if ready.CardCount != len(cards) || ready.ChunkCount != written {
		return Release{}, fmt.Errorf("build release: %w", ErrReleaseCountMismatch)
	}
	if err := tx.Commit(); err != nil {
		return Release{}, fmt.Errorf("build release: commit: %w", err)
	}
	return ready, nil
}

func (s *Store) ActivateRelease(parent context.Context, libraryID, releaseID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	if libraryID <= 0 || releaseID <= 0 {
		return fmt.Errorf("activate release: library and release ids must be positive")
	}
	ctx, cancel := storeContext(parent)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("activate release: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTheoryLibraries(ctx, tx, libraryID); err != nil {
		return fmt.Errorf("activate release: lock library scope: %w", err)
	}
	var lockedLibraryID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM theory_libraries WHERE id=$1 FOR UPDATE`, libraryID).Scan(&lockedLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("activate release: library: %w", ErrReleaseNotFound)
		}
		return fmt.Errorf("activate release: lock library: %w", err)
	}
	release, err := scanRelease(tx.QueryRowContext(ctx, `
		SELECT `+releaseReturningColumns+`
		FROM theory_library_releases
		WHERE id=$1 AND library_id=$2 AND status='ready'
		FOR UPDATE`, releaseID, libraryID))
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("activate release: %w", ErrReleaseNotReady)
	}
	if err != nil {
		return fmt.Errorf("activate release: lock ready release: %w", err)
	}
	mappings, err := loadReleaseMappings(ctx, tx, release.ID)
	if err != nil {
		return fmt.Errorf("activate release: load mappings: %w", err)
	}
	cardCount := distinctReleaseCards(mappings)
	if len(mappings) == 0 || cardCount != release.CardCount || len(mappings) != release.ChunkCount {
		return fmt.Errorf("activate release: stored card/chunk counts (%d/%d) differ from mappings (%d/%d): %w",
			release.CardCount, release.ChunkCount, cardCount, len(mappings), ErrReleaseCountMismatch)
	}
	if release.RetrievalMode == RetrievalHybrid {
		capable, err := vectorColumnAvailable(ctx, tx)
		if err != nil {
			return fmt.Errorf("activate release: detect vector capability: %w", err)
		}
		if !capable {
			return fmt.Errorf("activate release: %w", ErrVectorUnavailable)
		}
	}
	for _, mapping := range mappings {
		state, err := loadReleaseMappingState(ctx, tx, libraryID, mapping)
		if err != nil {
			return fmt.Errorf("activate release mapping card=%d chunk=%d: %w", mapping.CardID, mapping.ChunkID, err)
		}
		if release.RetrievalMode == RetrievalHybrid {
			if err := validateMappingEmbedding(ctx, tx, release, mapping.ChunkID, state.contentHash); err != nil {
				return fmt.Errorf("activate release chunk %d: %w", mapping.ChunkID, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_library_releases SET status = 'retired', update_time=now()
		WHERE library_id=$1 AND status='active' AND id<>$2`, libraryID, releaseID); err != nil {
		return fmt.Errorf("activate release: retire old active release: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE theory_library_releases SET status = 'active', activated_at=now(), update_time=now()
		WHERE id=$1 AND library_id=$2 AND status='ready'`, releaseID, libraryID)
	if err != nil {
		return fmt.Errorf("activate release: activate new release: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activate release: active rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("activate release: %w", ErrConcurrentUpdate)
	}
	result, err = tx.ExecContext(ctx, `UPDATE theory_libraries SET current_version=$2, update_time=now() WHERE id=$1`, libraryID, release.Version)
	if err != nil {
		return fmt.Errorf("activate release: update library version: %w", err)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activate release: library rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("activate release: library disappeared: %w", ErrConcurrentUpdate)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("activate release: commit: %w", err)
	}
	return nil
}

type releaseMappingState struct{ contentHash string }

func loadReleaseMappingState(ctx context.Context, tx *sql.Tx, libraryID int64, mapping ReleaseMapping) (releaseMappingState, error) {
	var actualCardID, actualLibraryID int64
	var chunkStatus ChunkStatus
	var contentHash string
	var cardStatus CardStatus
	var hasPrimary bool
	err := tx.QueryRowContext(ctx, `
		SELECT chunk.card_id, chunk.library_id, chunk.status, chunk.content_hash, card.status,
			EXISTS (
				SELECT 1 FROM theory_card_sources source
				WHERE source.card_id=card.id AND source.source_role='primary'
					AND source.extraction_quality >= 0.70
					AND (source.quotation='' OR (source.quote_verified AND source.verified_by IS NOT NULL AND source.verified_at IS NOT NULL))
			) AS has_primary
		FROM theory_chunks chunk
		JOIN theory_cards card ON card.id=chunk.card_id
		WHERE chunk.id=$1 AND card.id=$2
			AND btrim(card.definition) <> ''
			AND btrim(card.applicable_context) <> ''
			AND btrim(card.non_applicable_context) <> ''`, mapping.ChunkID, mapping.CardID).Scan(
		&actualCardID, &actualLibraryID, &chunkStatus, &contentHash, &cardStatus, &hasPrimary)
	if errors.Is(err, sql.ErrNoRows) {
		return releaseMappingState{}, ErrInvalidReleaseMapping
	}
	if err != nil {
		return releaseMappingState{}, err
	}
	if actualCardID != mapping.CardID || actualLibraryID != libraryID || chunkStatus != ChunkStatusEnabled ||
		(cardStatus != StatusPublished && cardStatus != StatusSuperseded) || !hasPrimary {
		return releaseMappingState{}, ErrInvalidReleaseMapping
	}
	return releaseMappingState{contentHash: contentHash}, nil
}

func validateMappingEmbedding(ctx context.Context, tx *sql.Tx, release Release, chunkID int64, chunkHash string) error {
	var model, hash string
	var dimensions int
	var status EmbeddingStatus
	var hasEmbedding bool
	err := tx.QueryRowContext(ctx, `
		SELECT embedding_model, dimensions, content_hash, status, embedding IS NOT NULL AS has_embedding
		FROM theory_chunk_embeddings
		WHERE chunk_id=$1
		ORDER BY (embedding_model=$2) DESC, (content_hash=$3) DESC, id DESC
		LIMIT 1`, chunkID, release.EmbeddingModel, chunkHash).Scan(&model, &dimensions, &hash, &status, &hasEmbedding)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEmbeddingNotReady
	}
	if err != nil {
		return err
	}
	if hash != chunkHash {
		return ErrStaleEmbedding
	}
	if model != release.EmbeddingModel || dimensions != 1536 || dimensions != release.EmbeddingDimensions || status != EmbeddingStatusReady || !hasEmbedding {
		return ErrEmbeddingNotReady
	}
	return nil
}

func validateReleaseMappings(mappings []ReleaseMapping) error {
	if len(mappings) == 0 {
		return ErrEmptyReleaseMapping
	}
	seenChunks := make(map[int64]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.CardID <= 0 || mapping.ChunkID <= 0 {
			return ErrInvalidReleaseMapping
		}
		if _, exists := seenChunks[mapping.ChunkID]; exists {
			return ErrDuplicateReleaseMapping
		}
		seenChunks[mapping.ChunkID] = struct{}{}
	}
	return nil
}

func loadReleaseMappings(ctx context.Context, tx *sql.Tx, releaseID int64) ([]ReleaseMapping, error) {
	rows, err := tx.QueryContext(ctx, `SELECT card_id, chunk_id FROM theory_release_cards mapping WHERE release_id=$1 ORDER BY card_id, chunk_id`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mappings []ReleaseMapping
	for rows.Next() {
		var mapping ReleaseMapping
		if err := rows.Scan(&mapping.CardID, &mapping.ChunkID); err != nil {
			return nil, err
		}
		mappings = append(mappings, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mappings, nil
}

func distinctReleaseCards(mappings []ReleaseMapping) int {
	cards := make(map[int64]struct{}, len(mappings))
	for _, mapping := range mappings {
		cards[mapping.CardID] = struct{}{}
	}
	return len(cards)
}

func scanRelease(row rowScanner) (Release, error) {
	var release Release
	err := row.Scan(&release.ID, &release.LibraryID, &release.Version, &release.Status,
		&release.EmbeddingModel, &release.EmbeddingDimensions, &release.RetrievalMode,
		&release.IndexVersion, &release.CardCount, &release.ChunkCount, &release.BuildError,
		&release.ActivatedBy, &release.ActivatedAt, &release.CreateTime, &release.UpdateTime)
	return release, err
}

func stringTrim(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n' || value[0] == '\r') {
		value = value[1:]
	}
	for len(value) > 0 {
		i := len(value) - 1
		if value[i] != ' ' && value[i] != '\t' && value[i] != '\n' && value[i] != '\r' {
			break
		}
		value = value[:i]
	}
	return value
}
