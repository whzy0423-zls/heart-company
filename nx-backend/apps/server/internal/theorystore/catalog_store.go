package theorystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrStoreUnavailable       = errors.New("theory store unavailable")
	ErrInvalidSHA256          = errors.New("invalid sha256")
	ErrWorkNotFound           = errors.New("source work not found")
	ErrFileNotFound           = errors.New("source file not found")
	ErrDuplicateSelf          = errors.New("source file cannot duplicate itself")
	ErrDuplicateHashMismatch  = errors.New("duplicate source files have different sha256")
	ErrDuplicateCycle         = errors.New("duplicate source file cycle")
	ErrDuplicateCrossLibrary  = errors.New("duplicate source files belong to different libraries")
	ErrCanonicalWorkSelf      = errors.New("source work cannot be its own canonical work")
	ErrCatalogScopeChanged    = errors.New("source catalog library scope changed")
	ErrInvalidExtractionState = errors.New("invalid extraction status update")
)

const storeOperationTimeout = 10 * time.Second

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) RegisterWork(parent context.Context, work SourceWork) (SourceWork, error) {
	if err := s.available(); err != nil {
		return SourceWork{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	normalizeWork(&work)
	if err := ValidateSourceWork(work); err != nil {
		return SourceWork{}, fmt.Errorf("register source work: %w", err)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO theory_source_works (
			library_id, canonical_key, title, original_title, authors, editors, translators,
			publisher, published_year, edition, isbn, work_type, authority_level,
			epistemic_status, copyright_scope, canonical_work_id, metadata, status
		) VALUES (
			$1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb,
			$8, $9, $10, $11, $12, $13, $14, $15, $16, $17::jsonb, $18
		)
		ON CONFLICT (library_id, canonical_key) DO UPDATE SET
			title = EXCLUDED.title,
			original_title = EXCLUDED.original_title,
			authors = EXCLUDED.authors,
			editors = EXCLUDED.editors,
			translators = EXCLUDED.translators,
			publisher = EXCLUDED.publisher,
			published_year = EXCLUDED.published_year,
			edition = EXCLUDED.edition,
			isbn = EXCLUDED.isbn,
			work_type = EXCLUDED.work_type,
			authority_level = EXCLUDED.authority_level,
			epistemic_status = EXCLUDED.epistemic_status,
			copyright_scope = EXCLUDED.copyright_scope,
			canonical_work_id = EXCLUDED.canonical_work_id,
			metadata = EXCLUDED.metadata,
			status = EXCLUDED.status,
			update_time = now()
		WHERE theory_source_works.id IS DISTINCT FROM EXCLUDED.canonical_work_id
		RETURNING id, library_id, canonical_key, title, original_title, authors, editors, translators,
			publisher, published_year, edition, isbn, work_type, authority_level, epistemic_status,
			copyright_scope, canonical_work_id, metadata, status, create_time, update_time`,
		work.LibraryID, work.CanonicalKey, work.Title, work.OriginalTitle, jsonArgument(work.Authors, `[]`),
		jsonArgument(work.Editors, `[]`), jsonArgument(work.Translators, `[]`), work.Publisher,
		work.PublishedYear, work.Edition, work.ISBN, work.WorkType, work.AuthorityLevel,
		work.EpistemicStatus, work.CopyrightScope, work.CanonicalWorkID, jsonArgument(work.Metadata, `{}`), work.Status,
	)
	registered, err := scanWork(row)
	if errors.Is(err, sql.ErrNoRows) && work.CanonicalWorkID != nil {
		return SourceWork{}, fmt.Errorf("register source work: %w", ErrCanonicalWorkSelf)
	}
	if err != nil {
		return SourceWork{}, fmt.Errorf("register source work: %w", err)
	}
	return registered, nil
}

func (s *Store) RegisterFile(parent context.Context, file SourceFile) (SourceFile, error) {
	if err := s.available(); err != nil {
		return SourceFile{}, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	normalizeFile(&file)
	if err := ValidateSourceFile(file); err != nil {
		if !sha256Pattern.MatchString(file.SHA256) {
			return SourceFile{}, fmt.Errorf("register source file: %w: %v", ErrInvalidSHA256, err)
		}
		return SourceFile{}, fmt.Errorf("register source file: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SourceFile{}, fmt.Errorf("register source file: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var libraryID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT library_id
		FROM theory_source_works
		WHERE id = $1`, file.WorkID).Scan(&libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceFile{}, fmt.Errorf("register source file work %d: %w", file.WorkID, ErrWorkNotFound)
		}
		return SourceFile{}, fmt.Errorf("register source file: find work: %w", err)
	}

	libraryScope := []int64{libraryID}
	var initialTargetLibraryID int64
	if file.DuplicateOfFileID != nil {
		var targetID int64
		var targetSHA string
		err := tx.QueryRowContext(ctx, `
			SELECT file.id, work.library_id, file.sha256
			FROM theory_source_files file
			JOIN theory_source_works work ON work.id = file.work_id
			WHERE file.id = $1`, *file.DuplicateOfFileID).Scan(&targetID, &initialTargetLibraryID, &targetSHA)
		if errors.Is(err, sql.ErrNoRows) {
			return SourceFile{}, fmt.Errorf("register source file duplicate target %d: %w", *file.DuplicateOfFileID, ErrFileNotFound)
		}
		if err != nil {
			return SourceFile{}, fmt.Errorf("register source file: find duplicate target: %w", err)
		}
		libraryScope = append(libraryScope, initialTargetLibraryID)
	}
	if err := lockTheoryLibraries(ctx, tx, libraryScope...); err != nil {
		return SourceFile{}, fmt.Errorf("register source file: lock libraries: %w", err)
	}

	var lockedLibraryID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT library_id
		FROM theory_source_works
		WHERE id = $1
		FOR SHARE`, file.WorkID).Scan(&lockedLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceFile{}, fmt.Errorf("register source file work %d: %w", file.WorkID, ErrWorkNotFound)
		}
		return SourceFile{}, fmt.Errorf("register source file: lock work: %w", err)
	}
	if lockedLibraryID != libraryID {
		return SourceFile{}, fmt.Errorf("register source file: %w", ErrCatalogScopeChanged)
	}

	if file.DuplicateOfFileID != nil {
		var targetID, targetLibraryID int64
		var targetSHA string
		err := tx.QueryRowContext(ctx, `
			SELECT file.id, work.library_id, file.sha256
			FROM theory_source_files file
			JOIN theory_source_works work ON work.id = file.work_id
			WHERE file.id = $1
			FOR UPDATE OF file, work`, *file.DuplicateOfFileID).Scan(&targetID, &targetLibraryID, &targetSHA)
		if errors.Is(err, sql.ErrNoRows) {
			return SourceFile{}, fmt.Errorf("register source file duplicate target %d: %w", *file.DuplicateOfFileID, ErrFileNotFound)
		}
		if err != nil {
			return SourceFile{}, fmt.Errorf("register source file: lock duplicate target: %w", err)
		}
		if targetLibraryID != initialTargetLibraryID {
			return SourceFile{}, fmt.Errorf("register source file: %w", ErrCatalogScopeChanged)
		}
		if targetLibraryID != lockedLibraryID {
			return SourceFile{}, fmt.Errorf("register source file: %w", ErrDuplicateCrossLibrary)
		}
		if targetSHA != file.SHA256 {
			return SourceFile{}, fmt.Errorf("register source file: %w", ErrDuplicateHashMismatch)
		}
	}

	registered, err := scanFile(tx.QueryRowContext(ctx, `
		INSERT INTO theory_source_files (
			work_id, relative_path, original_filename, file_format, mime_type, byte_size, page_count,
			sha256, duplicate_of_file_id, title_source, extraction_class, extraction_status,
			extraction_quality, extracted_text_uri, ocr_text_uri, extractor_name, extractor_version,
			error_code, error_message, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20::jsonb
		)
		RETURNING id, work_id, relative_path, original_filename, file_format, mime_type, byte_size,
			page_count, sha256, duplicate_of_file_id, title_source, extraction_class, extraction_status,
			extraction_quality, extracted_text_uri, ocr_text_uri, extractor_name, extractor_version,
			error_code, error_message, metadata, create_time, update_time`,
		file.WorkID, file.RelativePath, file.OriginalFilename, file.FileFormat, file.MIMEType, file.ByteSize,
		file.PageCount, file.SHA256, file.DuplicateOfFileID, file.TitleSource, file.ExtractionClass,
		file.ExtractionStatus, file.ExtractionQuality, file.ExtractedTextURI, file.OCRTextURI,
		file.ExtractorName, file.ExtractorVersion, file.ErrorCode, file.ErrorMessage, jsonArgument(file.Metadata, `{}`),
	))
	if err != nil {
		return SourceFile{}, fmt.Errorf("register source file: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SourceFile{}, fmt.Errorf("register source file: commit: %w", err)
	}
	return registered, nil
}

func (s *Store) FindFileBySHA256(parent context.Context, libraryID int64, sha256 string) (SourceFile, bool, error) {
	if err := s.available(); err != nil {
		return SourceFile{}, false, err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	sha256 = strings.TrimSpace(sha256)
	if libraryID <= 0 {
		return SourceFile{}, false, fmt.Errorf("find source file: library id must be positive")
	}
	if !sha256Pattern.MatchString(sha256) {
		return SourceFile{}, false, fmt.Errorf("find source file: %w", ErrInvalidSHA256)
	}
	file, err := scanFile(s.db.QueryRowContext(ctx, `
		SELECT file.id, file.work_id, file.relative_path, file.original_filename, file.file_format,
			file.mime_type, file.byte_size, file.page_count, file.sha256, file.duplicate_of_file_id,
			file.title_source, file.extraction_class, file.extraction_status, file.extraction_quality,
			file.extracted_text_uri, file.ocr_text_uri, file.extractor_name, file.extractor_version,
			file.error_code, file.error_message, file.metadata, file.create_time, file.update_time
		FROM theory_source_files file
		JOIN theory_source_works work ON work.id = file.work_id
		WHERE work.library_id = $1 AND file.sha256 = $2
		ORDER BY file.duplicate_of_file_id IS NULL DESC, file.id ASC
		LIMIT 1`, libraryID, sha256))
	if errors.Is(err, sql.ErrNoRows) {
		return SourceFile{}, false, nil
	}
	if err != nil {
		return SourceFile{}, false, fmt.Errorf("find source file: %w", err)
	}
	return file, true, nil
}

func (s *Store) MarkDuplicate(parent context.Context, fileID, duplicateOfFileID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark duplicate: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if fileID <= 0 || duplicateOfFileID <= 0 {
		return fmt.Errorf("mark duplicate: file ids must be positive")
	}
	if fileID == duplicateOfFileID {
		return fmt.Errorf("mark duplicate: %w", ErrDuplicateSelf)
	}

	discovered, err := loadDuplicateFiles(ctx, tx, fileID, duplicateOfFileID, false)
	if err != nil {
		return fmt.Errorf("mark duplicate: discover files: %w", err)
	}
	source, sourceFound := discovered[fileID]
	target, targetFound := discovered[duplicateOfFileID]
	if !sourceFound || !targetFound {
		return fmt.Errorf("mark duplicate: %w", ErrFileNotFound)
	}
	if err := lockTheoryLibraries(ctx, tx, source.libraryID, target.libraryID); err != nil {
		return fmt.Errorf("mark duplicate: lock libraries: %w", err)
	}

	locked, err := loadDuplicateFiles(ctx, tx, fileID, duplicateOfFileID, true)
	if err != nil {
		return fmt.Errorf("mark duplicate: lock files: %w", err)
	}
	lockedSource, sourceFound := locked[fileID]
	lockedTarget, targetFound := locked[duplicateOfFileID]
	if !sourceFound || !targetFound {
		return fmt.Errorf("mark duplicate: %w", ErrFileNotFound)
	}
	if lockedSource.libraryID != source.libraryID || lockedTarget.libraryID != target.libraryID {
		return fmt.Errorf("mark duplicate: %w", ErrCatalogScopeChanged)
	}
	if lockedSource.libraryID != lockedTarget.libraryID {
		return fmt.Errorf("mark duplicate: %w", ErrDuplicateCrossLibrary)
	}
	if lockedSource.sha256 != lockedTarget.sha256 {
		return fmt.Errorf("mark duplicate: %w", ErrDuplicateHashMismatch)
	}

	var cycle bool
	if err := tx.QueryRowContext(ctx, `
		WITH RECURSIVE duplicate_chain(id, duplicate_of_file_id) AS (
			SELECT id, duplicate_of_file_id FROM theory_source_files WHERE id = $1
			UNION
			SELECT file.id, file.duplicate_of_file_id
			FROM theory_source_files file
			JOIN duplicate_chain chain ON file.id = chain.duplicate_of_file_id
		)
		SELECT EXISTS(SELECT 1 FROM duplicate_chain WHERE id = $2)`, duplicateOfFileID, fileID).Scan(&cycle); err != nil {
		return fmt.Errorf("mark duplicate: inspect duplicate chain: %w", err)
	}
	if cycle {
		return fmt.Errorf("mark duplicate: %w", ErrDuplicateCycle)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE theory_source_files
		SET duplicate_of_file_id = $1, update_time = now()
		WHERE id = $2`, duplicateOfFileID, fileID)
	if err != nil {
		return fmt.Errorf("mark duplicate: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark duplicate: rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("mark duplicate: %w", ErrFileNotFound)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark duplicate: commit: %w", err)
	}
	return nil
}

func (s *Store) UpdateExtractionStatus(parent context.Context, fileID int64, status ExtractionStatus, quality float64, errorCode, errorMessage string) error {
	if err := s.available(); err != nil {
		return err
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	errorCode = strings.TrimSpace(errorCode)
	errorMessage = strings.TrimSpace(errorMessage)
	if fileID <= 0 || !validExtractionStatus(status) || math.IsNaN(quality) || math.IsInf(quality, 0) || quality < 0 || quality > 1 {
		return fmt.Errorf("update extraction status: %w", ErrInvalidExtractionState)
	}
	if status == ExtractionStatusFailed {
		if errorCode == "" || errorMessage == "" {
			return fmt.Errorf("update extraction status: %w: failed status requires error code and message", ErrInvalidExtractionState)
		}
	} else {
		errorCode = ""
		errorMessage = ""
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update extraction status: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var libraryID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT work.library_id
		FROM theory_source_files file
		JOIN theory_source_works work ON work.id = file.work_id
		WHERE file.id = $1`, fileID).Scan(&libraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("update extraction status: %w", ErrFileNotFound)
		}
		return fmt.Errorf("update extraction status: discover file: %w", err)
	}
	if err := lockTheoryLibraries(ctx, tx, libraryID); err != nil {
		return fmt.Errorf("update extraction status: lock library: %w", err)
	}

	var lockedLibraryID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT work.library_id
		FROM theory_source_files file
		JOIN theory_source_works work ON work.id = file.work_id
		WHERE file.id = $1
		FOR UPDATE OF file, work`, fileID).Scan(&lockedLibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("update extraction status: %w", ErrFileNotFound)
		}
		return fmt.Errorf("update extraction status: lock file: %w", err)
	}
	if lockedLibraryID != libraryID {
		return fmt.Errorf("update extraction status: %w", ErrCatalogScopeChanged)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE theory_source_files
		SET extraction_status = $1, extraction_quality = $2, error_code = $3, error_message = $4, update_time = now()
		WHERE id = $5`, status, quality, errorCode, errorMessage, fileID)
	if err != nil {
		return fmt.Errorf("update extraction status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update extraction status: rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("update extraction status: %w", ErrFileNotFound)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update extraction status: commit: %w", err)
	}
	return nil
}

func (s *Store) available() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("catalog operation: %w", ErrStoreUnavailable)
	}
	return nil
}

func storeContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, storeOperationTimeout)
}

func normalizeWork(work *SourceWork) {
	work.CanonicalKey = strings.TrimSpace(work.CanonicalKey)
	work.Title = strings.TrimSpace(work.Title)
	work.OriginalTitle = strings.TrimSpace(work.OriginalTitle)
	work.Publisher = strings.TrimSpace(work.Publisher)
	work.Edition = strings.TrimSpace(work.Edition)
	work.ISBN = strings.TrimSpace(work.ISBN)
	work.Authors = normalizedJSON(work.Authors, `[]`)
	work.Editors = normalizedJSON(work.Editors, `[]`)
	work.Translators = normalizedJSON(work.Translators, `[]`)
	work.Metadata = normalizedJSON(work.Metadata, `{}`)
}

func normalizeFile(file *SourceFile) {
	file.RelativePath = strings.TrimSpace(file.RelativePath)
	file.OriginalFilename = strings.TrimSpace(file.OriginalFilename)
	file.FileFormat = strings.TrimSpace(file.FileFormat)
	file.MIMEType = strings.TrimSpace(file.MIMEType)
	file.SHA256 = strings.TrimSpace(file.SHA256)
	file.ExtractedTextURI = strings.TrimSpace(file.ExtractedTextURI)
	file.OCRTextURI = strings.TrimSpace(file.OCRTextURI)
	file.ExtractorName = strings.TrimSpace(file.ExtractorName)
	file.ExtractorVersion = strings.TrimSpace(file.ExtractorVersion)
	file.ErrorCode = strings.TrimSpace(file.ErrorCode)
	file.ErrorMessage = strings.TrimSpace(file.ErrorMessage)
	file.Metadata = normalizedJSON(file.Metadata, `{}`)
}

func normalizedJSON(value json.RawMessage, fallback string) json.RawMessage {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		trimmed = fallback
	}
	return json.RawMessage(trimmed)
}

func jsonArgument(value json.RawMessage, fallback string) string {
	return string(normalizedJSON(value, fallback))
}

func lockTheoryLibraries(ctx context.Context, tx *sql.Tx, libraryIDs ...int64) error {
	var row *sql.Row
	switch len(libraryIDs) {
	case 1:
		row = tx.QueryRowContext(ctx, `SELECT lock_theory_libraries(ARRAY[$1]::BIGINT[])`, libraryIDs[0])
	case 2:
		row = tx.QueryRowContext(ctx, `SELECT lock_theory_libraries(ARRAY[$1, $2]::BIGINT[])`, libraryIDs[0], libraryIDs[1])
	default:
		return fmt.Errorf("library lock scope must contain one or two ids")
	}
	var result any
	return row.Scan(&result)
}

type duplicateFileState struct {
	workID, libraryID int64
	sha256            string
}

func loadDuplicateFiles(ctx context.Context, tx *sql.Tx, fileID, duplicateOfFileID int64, lock bool) (map[int64]duplicateFileState, error) {
	query := `
		SELECT file.id, file.work_id, work.library_id, file.sha256
		FROM theory_source_files file
		JOIN theory_source_works work ON work.id = file.work_id
		WHERE file.id IN ($1, $2)
		ORDER BY file.id`
	if lock {
		query += ` FOR UPDATE OF file, work`
	}
	rows, err := tx.QueryContext(ctx, query, fileID, duplicateOfFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make(map[int64]duplicateFileState, 2)
	for rows.Next() {
		var id int64
		var item duplicateFileState
		if err := rows.Scan(&id, &item.workID, &item.libraryID, &item.sha256); err != nil {
			return nil, err
		}
		files[id] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWork(row rowScanner) (SourceWork, error) {
	var work SourceWork
	err := row.Scan(
		&work.ID, &work.LibraryID, &work.CanonicalKey, &work.Title, &work.OriginalTitle,
		&work.Authors, &work.Editors, &work.Translators, &work.Publisher, &work.PublishedYear,
		&work.Edition, &work.ISBN, &work.WorkType, &work.AuthorityLevel, &work.EpistemicStatus,
		&work.CopyrightScope, &work.CanonicalWorkID, &work.Metadata, &work.Status,
		&work.CreateTime, &work.UpdateTime,
	)
	return work, err
}

func scanFile(row rowScanner) (SourceFile, error) {
	var file SourceFile
	err := row.Scan(
		&file.ID, &file.WorkID, &file.RelativePath, &file.OriginalFilename, &file.FileFormat,
		&file.MIMEType, &file.ByteSize, &file.PageCount, &file.SHA256, &file.DuplicateOfFileID,
		&file.TitleSource, &file.ExtractionClass, &file.ExtractionStatus, &file.ExtractionQuality,
		&file.ExtractedTextURI, &file.OCRTextURI, &file.ExtractorName, &file.ExtractorVersion,
		&file.ErrorCode, &file.ErrorMessage, &file.Metadata, &file.CreateTime, &file.UpdateTime,
	)
	return file, err
}
