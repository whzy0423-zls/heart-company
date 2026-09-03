package enneagramcatalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/theorystore"
)

type Store struct {
	db     *sql.DB
	theory *theorystore.Store
}

type ImportResult struct {
	ImportID      int64  `json:"importId"`
	LibraryID     int64  `json:"libraryId"`
	LibraryKey    string `json:"libraryKey"`
	ContentDigest string `json:"contentDigest"`
	Status        string `json:"status"`
	Created       bool   `json:"created"`
}

type PublishResult struct {
	ImportID   int64  `json:"importId"`
	LibraryID  int64  `json:"libraryId"`
	LibraryKey string `json:"libraryKey"`
	ReleaseID  int64  `json:"releaseId"`
	Version    int    `json:"version"`
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, theory: theorystore.NewStore(db)}
}

func (s *Store) ImportCatalog(ctx context.Context, catalog Catalog, actorID int64) ([]ImportResult, error) {
	if s == nil || s.db == nil || s.theory == nil {
		return nil, errors.New("enneagram catalog store unavailable")
	}
	if actorID <= 0 {
		return nil, errors.New("enneagram catalog import actor is required")
	}
	if err := ValidateCatalog(catalog); err != nil {
		return nil, err
	}
	results := make([]ImportResult, 0, len(catalog.Packages))
	for _, packageValue := range catalog.Packages {
		result, err := s.stagePackage(ctx, catalog.Manifest, packageValue, actorID)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", packageValue.LibraryID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) stagePackage(ctx context.Context, manifest Manifest, packageValue Package, actorID int64) (result ImportResult, retErr error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && retErr == nil {
			retErr = err
		}
	}()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "enneagram-catalog:"+packageValue.LibraryID); err != nil {
		return result, err
	}
	var libraryID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO theory_libraries(key,name,description,status,created_by,updated_by)
		VALUES ($1,$2,'九型人格分层知识库','draft',$3,$3)
		ON CONFLICT (key) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description,
			updated_by=EXCLUDED.updated_by, update_time=now()
		RETURNING id
	`, packageValue.LibraryID, packageValue.Title, actorID).Scan(&libraryID); err != nil {
		return result, fmt.Errorf("upsert library: %w", err)
	}
	result = ImportResult{LibraryID: libraryID, LibraryKey: packageValue.LibraryID, ContentDigest: packageValue.ContentDigest}
	if err := tx.QueryRowContext(ctx, `
		SELECT id,status FROM enneagram_catalog_imports
		WHERE library_id=$1 AND content_digest=$2
	`, libraryID, packageValue.ContentDigest).Scan(&result.ImportID, &result.Status); err == nil {
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}

	payload, err := json.Marshal(packageValue)
	if err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO enneagram_catalog_imports(
			library_id,content_digest,source_map_sha256,schema_version,kind,enneagram_type,
			title,source_chapter,payload,status,created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,'draft',$10)
		RETURNING id,status
	`, libraryID, packageValue.ContentDigest, manifest.SourceMapSHA256, packageValue.SchemaVersion,
		packageValue.Kind, packageValue.EnneagramType, packageValue.Title, packageValue.SourceChapter,
		payload, actorID).Scan(&result.ImportID, &result.Status); err != nil {
		return result, fmt.Errorf("create import ledger: %w", err)
	}

	fileIDs, workIDs, err := ensureSources(ctx, tx, libraryID, manifest.Sources)
	if err != nil {
		return result, err
	}
	pageMetadata, err := collectPackagePages(packageValue)
	if err != nil {
		return result, err
	}
	if err := ensureSourcePages(ctx, tx, pageMetadata, fileIDs, actorID); err != nil {
		return result, err
	}
	for index, item := range sortedPackageItems(packageValue) {
		if item.ProvenanceKind != ProvenanceSource {
			continue
		}
		cardID, chunkID, err := stageItem(ctx, tx, libraryID, packageValue, item, index, workIDs, fileIDs, actorID)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO enneagram_catalog_import_items(import_id,card_id,chunk_id,content_key,dimension,sort_order)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, result.ImportID, cardID, chunkID, item.ContentKey, item.Dimension, index); err != nil {
			return result, fmt.Errorf("link imported item %s: %w", item.ContentKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.Created = true
	return result, nil
}

func ensureSources(ctx context.Context, tx *sql.Tx, libraryID int64, sources []ManifestSource) (map[string]int64, map[string]int64, error) {
	fileIDs := make(map[string]int64, len(sources))
	workIDs := make(map[string]int64, len(sources))
	for _, source := range sources {
		var workID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO theory_source_works(
				library_id,canonical_key,title,work_type,authority_level,epistemic_status,copyright_scope,status
			) VALUES ($1,$2,$3,'handout',2,'source_text','internal_excerpt','reviewed')
			ON CONFLICT (library_id,canonical_key) DO UPDATE SET title=EXCLUDED.title,status='reviewed',update_time=now()
			RETURNING id
		`, libraryID, "enneagram-"+source.SourceID, source.DisplayName).Scan(&workID); err != nil {
			return nil, nil, fmt.Errorf("upsert source work %s: %w", source.SourceID, err)
		}
		workIDs[source.SourceID] = workID
		var fileID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM theory_source_files WHERE work_id=$1 AND sha256=$2 ORDER BY id LIMIT 1
		`, workID, source.SHA256).Scan(&fileID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO theory_source_files(
					work_id,relative_path,original_filename,file_format,mime_type,page_count,sha256,
					title_source,extraction_class,extraction_status,extraction_quality,ocr_text_uri,
					extractor_name,extractor_version
				) VALUES ($1,$2,$3,'pdf','application/pdf',$4,$5,'filename','image_dominant','extracted',1,$6,'tesseract','5.5.2')
				RETURNING id
			`, workID, "sources/"+source.SourceID+".pdf", source.DisplayName, source.PageCount,
				source.SHA256, "catalog://"+source.SourceID).Scan(&fileID)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("upsert source file %s: %w", source.SourceID, err)
		}
		fileIDs[source.SourceID] = fileID
	}
	return fileIDs, workIDs, nil
}

func collectPackagePages(packageValue Package) (map[string]SourcePage, error) {
	pages := make(map[string]SourcePage)
	for _, item := range sortedPackageItems(packageValue) {
		for _, page := range item.SourcePages {
			key := fmt.Sprintf("%s:%d", page.SourceID, page.PageNumber)
			if previous, exists := pages[key]; exists && previous != page {
				return nil, fmt.Errorf("conflicting source page metadata %s", key)
			}
			pages[key] = page
		}
	}
	return pages, nil
}

func ensureSourcePages(ctx context.Context, tx *sql.Tx, pages map[string]SourcePage, fileIDs map[string]int64, actorID int64) error {
	keys := make([]string, 0, len(pages))
	for key := range pages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		page := pages[key]
		fileID := fileIDs[page.SourceID]
		var existingHash, existingStatus string
		var existingType int
		err := tx.QueryRowContext(ctx, `
			SELECT ocr_text_hash,review_status,enneagram_type FROM theory_source_pages
			WHERE source_file_id=$1 AND page_number=$2
		`, fileID, page.PageNumber).Scan(&existingHash, &existingStatus, &existingType)
		if err == nil {
			if existingHash != page.OCRTextHash || existingStatus != "reviewed" || existingType != page.EnneagramType {
				return fmt.Errorf("source page metadata changed for %s", key)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO theory_source_pages(
				source_file_id,page_number,enneagram_type,ocr_text_uri,ocr_text_hash,
				review_status,reviewed_by,reviewed_at
			) VALUES ($1,$2,$3,$4,$5,'reviewed',$6,now())
		`, fileID, page.PageNumber, page.EnneagramType, page.OCRTextURI, page.OCRTextHash, actorID); err != nil {
			return fmt.Errorf("insert source page %s: %w", key, err)
		}
	}
	return nil
}

func stageItem(
	ctx context.Context,
	tx *sql.Tx,
	libraryID int64,
	packageValue Package,
	item Item,
	sortOrder int,
	workIDs map[string]int64,
	fileIDs map[string]int64,
	actorID int64,
) (int64, int64, error) {
	var version int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version),0)+1 FROM theory_cards WHERE library_id=$1 AND canonical_key=$2
	`, libraryID, item.ContentKey).Scan(&version); err != nil {
		return 0, 0, err
	}
	cardKind := "concept"
	if item.Dimension == "growth_practices" {
		cardKind = "practice"
	}
	aliases, _ := json.Marshal([]string{item.Dimension})
	var cardID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO theory_cards(
			library_id,canonical_key,canonical_name,aliases,domain,subdomain,card_kind,
			summary,definition,core_claim,applicable_context,non_applicable_context,
			epistemic_status,evidence_level,clinical_safety,authority_level,status,version,
			created_by,updated_by
		) VALUES ($1,$2,$3,$4::jsonb,'enneagram',$5,$6,$7,$7,$7,
			'已确认 main_type 的非临床人格反思','诊断、定型或从单次行为猜型',
			'author_interpretation','limited','caution',2,'draft',$8,$9,$9)
		RETURNING id
	`, libraryID, item.ContentKey, packageValue.Title+" / "+item.Dimension, aliases, item.Dimension,
		cardKind, item.Text, version, actorID).Scan(&cardID); err != nil {
		return 0, 0, fmt.Errorf("insert card %s: %w", item.ContentKey, err)
	}

	for _, page := range item.SourcePages {
		workID := workIDs[page.SourceID]
		fileID := fileIDs[page.SourceID]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO theory_card_sources(
				card_id,work_id,file_id,source_role,page_start,page_end,location_label,
				extraction_quality,quote_verified,verified_by,verified_at
			) VALUES ($1,$2,$3,'primary',$4,$4,$5,1,true,$6,now())
		`, cardID, workID, fileID, page.PageNumber,
			fmt.Sprintf("%s:p%d", page.SourceID, page.PageNumber), actorID); err != nil {
			return 0, 0, fmt.Errorf("insert card source %s: %w", item.ContentKey, err)
		}
	}

	var practiceID any
	chunkKind := "card"
	if item.Dimension == "growth_practices" {
		var id int64
		steps, _ := json.Marshal([]string{item.Text})
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO theory_practices(card_id,goal,estimated_minutes,steps,status,version)
			VALUES ($1,$2,10,$3::jsonb,'draft',$4) RETURNING id
		`, cardID, item.Text, steps, version).Scan(&id); err != nil {
			return 0, 0, fmt.Errorf("insert practice %s: %w", item.ContentKey, err)
		}
		practiceID = id
		chunkKind = "practice"
	}
	hash := sha256.Sum256([]byte(item.Text))
	tags := []string{"enneagram", item.Dimension}
	if packageValue.EnneagramType != nil {
		tags = append(tags, fmt.Sprintf("type-%02d", *packageValue.EnneagramType))
	}
	tagsJSON, _ := json.Marshal(tags)
	keywords, _ := json.Marshal([]string{packageValue.Title, item.Dimension})
	var chunkID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO theory_chunks(
			library_id,card_id,practice_id,chunk_key,chunk_kind,title,content,keywords,tags,
			authority_level,evidence_level,clinical_safety,token_count,content_hash,version,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,2,'limited','caution',$10,$11,$12,'enabled')
		RETURNING id
	`, libraryID, cardID, practiceID, item.ContentKey, chunkKind, packageValue.Title+" / "+item.Dimension,
		item.Text, keywords, tagsJSON, utf8.RuneCountInString(item.Text), hex.EncodeToString(hash[:]), version).Scan(&chunkID); err != nil {
		return 0, 0, fmt.Errorf("insert chunk %s: %w", item.ContentKey, err)
	}
	_ = sortOrder
	return cardID, chunkID, nil
}

func (s *Store) SubmitReview(ctx context.Context, importID, actorID int64) error {
	return s.transitionImport(ctx, importID, actorID, "draft", "in_review", "")
}

func (s *Store) Approve(ctx context.Context, importID, actorID int64, notes string) error {
	return s.transitionImport(ctx, importID, actorID, "in_review", "approved", notes)
}

func (s *Store) transitionImport(ctx context.Context, importID, actorID int64, from, to, notes string) error {
	if s == nil || s.db == nil || importID <= 0 || actorID <= 0 {
		return errors.New("invalid enneagram import transition")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE enneagram_catalog_imports SET status=$3,review_notes=$4,
			reviewed_by=CASE WHEN $3='approved' THEN $2 ELSE reviewed_by END,
			reviewed_at=CASE WHEN $3='approved' THEN now() ELSE reviewed_at END,update_time=now()
		WHERE id=$1 AND status=$5
	`, importID, actorID, to, strings.TrimSpace(notes), from)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("enneagram import state changed concurrently")
	}
	return nil
}

func (s *Store) Publish(ctx context.Context, importID, actorID int64) (PublishResult, error) {
	if s == nil || s.db == nil || s.theory == nil || importID <= 0 || actorID <= 0 {
		return PublishResult{}, errors.New("invalid enneagram publish request")
	}
	prepared, mappings, packageValue, nextVersion, err := s.preparePublish(ctx, importID, actorID)
	if err != nil {
		return PublishResult{}, err
	}
	release, err := s.theory.BuildRelease(ctx, theorystore.Release{
		LibraryID: prepared.LibraryID, Version: nextVersion, Status: theorystore.ReleaseStatusDraft,
		EmbeddingDimensions: 1536, RetrievalMode: theorystore.RetrievalLexicalOnly,
		IndexVersion: packageValue.ContentDigest,
	}, mappings)
	if err != nil {
		return PublishResult{}, fmt.Errorf("build enneagram release: %w", err)
	}
	if err := s.theory.ActivateRelease(ctx, prepared.LibraryID, release.ID, actorID); err != nil {
		return PublishResult{}, fmt.Errorf("activate enneagram release: %w", err)
	}
	if err := s.finalizePublish(ctx, prepared, packageValue, release.ID); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{ImportID: importID, LibraryID: prepared.LibraryID, LibraryKey: prepared.LibraryKey, ReleaseID: release.ID, Version: release.Version}, nil
}

type preparedImport struct {
	ImportID   int64
	LibraryID  int64
	LibraryKey string
}

func (s *Store) preparePublish(ctx context.Context, importID, actorID int64) (preparedImport, []theorystore.ReleaseMapping, Package, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	defer tx.Rollback()
	var prepared preparedImport
	var payload []byte
	var status string
	var currentVersion int
	if err := tx.QueryRowContext(ctx, `
		SELECT import.id,library.id,library.key,import.payload,import.status,library.current_version
		FROM enneagram_catalog_imports import
		JOIN theory_libraries library ON library.id=import.library_id
		WHERE import.id=$1 FOR UPDATE OF import,library
	`, importID).Scan(&prepared.ImportID, &prepared.LibraryID, &prepared.LibraryKey, &payload, &status, &currentVersion); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if status != "approved" {
		return preparedImport{}, nil, Package{}, 0, fmt.Errorf("enneagram import must be approved before publish")
	}
	var packageValue Package
	if err := json.Unmarshal(payload, &packageValue); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if _, _, err := bindingScope(packageValue); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	var unreviewed int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM enneagram_catalog_import_items item
		JOIN theory_card_sources source ON source.card_id=item.card_id
		LEFT JOIN theory_source_pages page ON page.source_file_id=source.file_id AND page.page_number=source.page_start
		WHERE item.import_id=$1 AND (page.id IS NULL OR page.review_status<>'reviewed')
	`, importID).Scan(&unreviewed); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if unreviewed != 0 {
		return preparedImport{}, nil, Package{}, 0, fmt.Errorf("enneagram import has unreviewed source pages")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_cards old SET status='superseded',update_time=now()
		FROM enneagram_catalog_import_items current
		JOIN theory_cards replacement ON replacement.id=current.card_id
		WHERE current.import_id=$1 AND old.library_id=$2 AND old.canonical_key=replacement.canonical_key
			AND old.status='published' AND old.id<>replacement.id
	`, importID, prepared.LibraryID); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_cards card SET status='published',reviewed_by=$2,reviewed_at=now(),published_at=now(),updated_by=$2,update_time=now()
		FROM enneagram_catalog_import_items item WHERE item.import_id=$1 AND card.id=item.card_id AND card.status='draft'
	`, importID, actorID); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE theory_practices practice SET status='published',update_time=now()
		FROM enneagram_catalog_import_items item WHERE item.import_id=$1 AND practice.card_id=item.card_id AND practice.status='draft'
	`, importID); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT card_id,chunk_id FROM enneagram_catalog_import_items WHERE import_id=$1 ORDER BY sort_order,content_key
	`, importID)
	if err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	var mappings []theorystore.ReleaseMapping
	for rows.Next() {
		var mapping theorystore.ReleaseMapping
		if err := rows.Scan(&mapping.CardID, &mapping.ChunkID); err != nil {
			rows.Close()
			return preparedImport{}, nil, Package{}, 0, err
		}
		mappings = append(mappings, mapping)
	}
	if err := rows.Close(); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	if len(mappings) == 0 {
		return preparedImport{}, nil, Package{}, 0, fmt.Errorf("enneagram import has no publishable items")
	}
	if err := tx.Commit(); err != nil {
		return preparedImport{}, nil, Package{}, 0, err
	}
	return prepared, mappings, packageValue, currentVersion + 1, nil
}

func (s *Store) finalizePublish(ctx context.Context, prepared preparedImport, packageValue Package, releaseID int64) (retErr error) {
	layer, personalityType, err := bindingScope(packageValue)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && retErr == nil {
			retErr = err
		}
	}()
	lockKey := fmt.Sprintf("app-chat-knowledge-binding:%s:%v", layer, personalityType)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_chat_knowledge_bindings SET status='disabled',update_time=now()
		WHERE layer_kind=$1 AND enneagram_type IS NOT DISTINCT FROM $2 AND status='enabled'
	`, layer, personalityType); err != nil {
		return err
	}
	var bindingID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM app_chat_knowledge_bindings
		WHERE layer_kind=$1 AND enneagram_type IS NOT DISTINCT FROM $2 AND theory_library_id=$3
		ORDER BY id DESC LIMIT 1
	`, layer, personalityType, prepared.LibraryID).Scan(&bindingID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO app_chat_knowledge_bindings(layer_kind,enneagram_type,theory_library_id,status)
			VALUES ($1,$2,$3,'enabled')
		`, layer, personalityType, prepared.LibraryID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE app_chat_knowledge_bindings SET status='enabled',update_time=now() WHERE id=$1`, bindingID)
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE enneagram_catalog_imports SET status='published',published_release_id=$2,published_at=now(),update_time=now()
		WHERE id=$1 AND status='approved'
	`, prepared.ImportID, releaseID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("enneagram import publish state changed concurrently")
	}
	return tx.Commit()
}

func (s *Store) Rollback(ctx context.Context, libraryKey string, targetVersion, actorID int) (PublishResult, error) {
	if s == nil || s.db == nil || s.theory == nil || strings.TrimSpace(libraryKey) == "" || targetVersion <= 0 || actorID <= 0 {
		return PublishResult{}, errors.New("invalid enneagram rollback request")
	}
	var libraryID, releaseID int64
	var currentVersion int
	if err := s.db.QueryRowContext(ctx, `
		SELECT library.id,release.id,library.current_version
		FROM theory_libraries library JOIN theory_library_releases release ON release.library_id=library.id
		WHERE library.key=$1 AND release.version=$2 AND release.status IN ('active','retired')
	`, libraryKey, targetVersion).Scan(&libraryID, &releaseID, &currentVersion); err != nil {
		return PublishResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT card_id,chunk_id FROM theory_release_cards WHERE release_id=$1 ORDER BY card_id,chunk_id`, releaseID)
	if err != nil {
		return PublishResult{}, err
	}
	var mappings []theorystore.ReleaseMapping
	for rows.Next() {
		var mapping theorystore.ReleaseMapping
		if err := rows.Scan(&mapping.CardID, &mapping.ChunkID); err != nil {
			rows.Close()
			return PublishResult{}, err
		}
		mappings = append(mappings, mapping)
	}
	if err := rows.Close(); err != nil {
		return PublishResult{}, err
	}
	release, err := s.theory.BuildRelease(ctx, theorystore.Release{
		LibraryID: libraryID, Version: currentVersion + 1, Status: theorystore.ReleaseStatusDraft,
		EmbeddingDimensions: 1536, RetrievalMode: theorystore.RetrievalLexicalOnly,
		IndexVersion: fmt.Sprintf("rollback-from-v%d", targetVersion),
	}, mappings)
	if err != nil {
		return PublishResult{}, err
	}
	if err := s.theory.ActivateRelease(ctx, libraryID, release.ID, int64(actorID)); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{LibraryID: libraryID, LibraryKey: libraryKey, ReleaseID: release.ID, Version: release.Version}, nil
}

func bindingScope(packageValue Package) (string, *int, error) {
	switch packageValue.Kind {
	case KindCore:
		if packageValue.LibraryID != "enneagram-core" || packageValue.EnneagramType != nil {
			return "", nil, fmt.Errorf("invalid core package identity")
		}
		return "theory", nil, nil
	case KindEnneagramType:
		if packageValue.EnneagramType == nil || *packageValue.EnneagramType < 1 || *packageValue.EnneagramType > 9 ||
			packageValue.LibraryID != libraryIDForType(*packageValue.EnneagramType) {
			return "", nil, fmt.Errorf("invalid enneagram type package identity")
		}
		value := *packageValue.EnneagramType
		return "enneagram_type", &value, nil
	default:
		return "", nil, fmt.Errorf("invalid package kind %q", packageValue.Kind)
	}
}
