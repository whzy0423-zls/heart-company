package theorystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"nine-xing/nx-backend/apps/server/internal/theorypackage"
)

var (
	ErrDatabaseURLMissing     = errors.New("theory database URL is not configured")
	ErrActivationBlocked      = errors.New("theory package activation is blocked")
	ErrPackageConflict        = errors.New("theory package conflicts with database state")
	ErrActorInvalid           = errors.New("theory package actor is not an active database user")
	ErrReviewerRole           = errors.New("reviewer does not have the required active role")
	ErrReviewsIncomplete      = errors.New("three database reviews are incomplete")
	ErrActorSeparation        = errors.New("promotion actor must be separate from all reviewers")
	ErrImportedContentChanged = errors.New("staged package content was modified")
)

type ReviewType string

const (
	ReviewSourceVerification ReviewType = "source-verification"
	ReviewTheory             ReviewType = "theory-review"
	ReviewSafety             ReviewType = "safety-review"
)

var reviewRoles = map[ReviewType]string{
	ReviewSourceVerification: "theory_source_reviewer",
	ReviewTheory:             "theory_content_reviewer",
	ReviewSafety:             "theory_safety_reviewer",
}

type ReviewReceipt struct {
	PackageID     string     `json:"packageId"`
	ContentDigest string     `json:"contentDigest"`
	ReviewType    ReviewType `json:"reviewType"`
	ReviewerID    int64      `json:"reviewerId"`
	NoOp          bool       `json:"noOp"`
}

type PromotionReceipt struct {
	PackageID      string        `json:"packageId"`
	ContentDigest  string        `json:"contentDigest"`
	ReleaseID      int64         `json:"releaseId"`
	ReleaseVersion int           `json:"releaseVersion"`
	ReleaseStatus  ReleaseStatus `json:"releaseStatus"`
	CardCount      int           `json:"cardCount"`
	ChunkCount     int           `json:"chunkCount"`
	NoOp           bool          `json:"noOp"`
}

type PackagePlan struct {
	PackageID     string `json:"packageId"`
	ContentDigest string `json:"contentDigest"`
	PackageDigest string `json:"packageDigest"`
	Sources       int    `json:"sources"`
	Cards         int    `json:"cards"`
	Practices     int    `json:"practices"`
	Operation     string `json:"operation"`
	WriteAllowed  bool   `json:"writeAllowed"`
	NoOp          bool   `json:"noOp"`
}

type PackageSyncer struct {
	db                    *sql.DB
	libraryKey            string
	beforePromotionCommit func(*sql.Tx) error
}

func NewPackageSyncer(db *sql.DB) *PackageSyncer {
	return &PackageSyncer{db: db, libraryKey: "xinzhili"}
}

func TheoryDatabaseURL(getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := strings.TrimSpace(getenv("THEORY_DATABASE_URL")); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(getenv("DATABASE_URL")); value != "" {
		return value, nil
	}
	return "", ErrDatabaseURLMissing
}

func RedactDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	return &redactedDatabaseError{cause: err}
}

type redactedDatabaseError struct{ cause error }

func (e *redactedDatabaseError) Error() string {
	return "database operation failed; inspect server-side database logs"
}
func (e *redactedDatabaseError) Unwrap() error { return e.cause }

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}

func databaseFailure(operation string, err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return fmt.Errorf("%s (SQLSTATE %s): %w", operation, postgresError.Code, RedactDatabaseError(err))
	}
	return fmt.Errorf("%s: %w", operation, RedactDatabaseError(err))
}

func ValidatePackage(root string) (PackagePlan, error) {
	report, err := theorypackage.Validate(root)
	if err != nil {
		return PackagePlan{}, err
	}
	payload, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return PackagePlan{}, fmt.Errorf("read package manifest: %w", err)
	}
	var manifest struct {
		Counts struct {
			Sources   int `json:"sources"`
			Cards     int `json:"cards"`
			Practices int `json:"practices"`
		} `json:"counts"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return PackagePlan{}, fmt.Errorf("decode package manifest: %w", err)
	}
	return PackagePlan{
		PackageID: report.PackageID, ContentDigest: report.ContentDigest, PackageDigest: report.PackageDigest,
		Sources: manifest.Counts.Sources, Cards: manifest.Counts.Cards, Practices: manifest.Counts.Practices,
		Operation: "validate", WriteAllowed: false,
	}, nil
}

func (s *PackageSyncer) Plan(ctx context.Context, root string) (PackagePlan, error) {
	plan, err := ValidatePackage(root)
	if err != nil {
		return PackagePlan{}, err
	}
	plan.Operation = "stage"
	if s == nil || s.db == nil {
		return PackagePlan{}, errors.New("plan package: database is unavailable")
	}
	var digest, databaseName, libraryKey string
	err = s.db.QueryRowContext(ctx, `SELECT content_digest, target_database, l.key FROM theory_package_imports i JOIN theory_libraries l ON l.id=i.library_id WHERE package_id=$1`, plan.PackageID).Scan(&digest, &databaseName, &libraryKey)
	if err == nil {
		if digest != plan.ContentDigest || libraryKey != s.libraryKey {
			return PackagePlan{}, fmt.Errorf("plan package: %w", ErrPackageConflict)
		}
		var currentDatabase string
		if err := s.db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&currentDatabase); err != nil {
			return PackagePlan{}, RedactDatabaseError(err)
		}
		if databaseName != currentDatabase {
			return PackagePlan{}, fmt.Errorf("plan package: target database mismatch: %w", ErrPackageConflict)
		}
		plan.NoOp = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	return plan, nil
}

type packageManifest struct {
	SchemaVersion string `json:"schemaVersion"`
	PackageID     string `json:"packageId"`
	ContentDigest string `json:"contentDigest"`
	PackageDigest string `json:"packageDigest"`
	RoundID       string `json:"roundId"`
	Sources       []struct {
		SourceID        string         `json:"sourceId"`
		SourceSHA256    string         `json:"sourceSha256"`
		RelativePath    string         `json:"relativePath"`
		Format          string         `json:"format"`
		ExtractionRoute string         `json:"extractionRoute"`
		Attribution     map[string]any `json:"attribution"`
	} `json:"sources"`
}

type packageEvidence struct {
	SourceID        string         `json:"sourceId"`
	ExtractionRoute string         `json:"extractionRoute"`
	Locator         map[string]any `json:"locator"`
}

type packageCard struct {
	CanonicalKey    string          `json:"canonicalKey"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Definition      string          `json:"definition"`
	Domain          string          `json:"domain"`
	EpistemicStatus string          `json:"epistemicStatus"`
	EvidenceLevel   string          `json:"evidenceLevel"`
	AuthorityLevel  int             `json:"authorityLevel"`
	Safety          json.RawMessage `json:"safety"`
	PrimaryEvidence packageEvidence `json:"primaryEvidence"`
}

type packagePractice struct {
	CanonicalKey                     string          `json:"canonicalKey"`
	Title                            string          `json:"title"`
	Purpose                          string          `json:"purpose"`
	Steps                            json.RawMessage `json:"steps"`
	StopConditions                   json.RawMessage `json:"stopConditions"`
	ProfessionalEscalationConditions json.RawMessage `json:"professionalEscalationConditions"`
	Safety                           json.RawMessage `json:"safety"`
	PrimaryEvidence                  packageEvidence `json:"primaryEvidence"`
}

type packageRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type packagePreview struct {
	CanonicalKey string `json:"canonicalKey"`
	SourceKind   string `json:"sourceKind"`
	Text         string `json:"text"`
	ContentHash  string `json:"contentHash"`
}

type stagedPackage struct {
	Manifest  packageManifest   `json:"manifest"`
	Cards     []packageCard     `json:"cards"`
	Practices []packagePractice `json:"practices"`
	Relations []packageRelation `json:"relations"`
	Previews  []packagePreview  `json:"previews"`
}

type canonicalDatabaseSnapshot struct {
	SchemaVersion string          `json:"schemaVersion"`
	Cards         json.RawMessage `json:"cards"`
	Practices     json.RawMessage `json:"practices"`
	SourceWorks   json.RawMessage `json:"sourceWorks"`
	SourceFiles   json.RawMessage `json:"sourceFiles"`
	CardSources   json.RawMessage `json:"cardSources"`
	Relations     json.RawMessage `json:"relations"`
}

type databaseFingerprint struct {
	SchemaVersion string                    `json:"schemaVersion"`
	SHA256        string                    `json:"sha256"`
	PayloadSHA256 string                    `json:"payloadSha256"`
	Snapshot      canonicalDatabaseSnapshot `json:"snapshot"`
}

func (s *PackageSyncer) Stage(ctx context.Context, root string, actorID int64) (PackagePlan, error) {
	for attempt := 0; attempt < 3; attempt++ {
		plan, err := s.stageOnce(ctx, root, actorID)
		if err == nil || !isSerializationFailure(err) {
			return plan, err
		}
		if ctx.Err() != nil {
			return PackagePlan{}, ctx.Err()
		}
	}
	return PackagePlan{}, databaseFailure("stage package retry exhausted", errors.New("serialization retry exhausted"))
}

func (s *PackageSyncer) stageOnce(ctx context.Context, root string, actorID int64) (PackagePlan, error) {
	plan, err := ValidatePackage(root)
	if err != nil {
		return PackagePlan{}, err
	}
	if s == nil || s.db == nil {
		return PackagePlan{}, errors.New("stage package: database is unavailable")
	}
	if actorID <= 0 {
		return PackagePlan{}, ErrActorInvalid
	}
	pkg, payload, _, err := loadStagedPackage(root, plan)
	if err != nil {
		return PackagePlan{}, err
	}
	// Close the validation/read race: all files and digests must still validate after loading.
	after, err := ValidatePackage(root)
	if err != nil || after.ContentDigest != plan.ContentDigest || after.PackageDigest != plan.PackageDigest {
		return PackagePlan{}, fmt.Errorf("stage package changed while reading: %w", ErrPackageConflict)
	}
	payloadSHA256, err := canonicalJSONSHA256(payload)
	if err != nil {
		return PackagePlan{}, fmt.Errorf("hash staged payload: %w", err)
	}
	payloadReceiptSHA256 := payloadContractReceiptSHA256(plan.PackageID, plan.ContentDigest, plan.PackageDigest, payloadSHA256)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "nine-xing:theory-package:"+s.libraryKey); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	var actorExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status=1)`, actorID).Scan(&actorExists); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	if !actorExists {
		return PackagePlan{}, ErrActorInvalid
	}
	var currentDatabase string
	if err := tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&currentDatabase); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	var existingDigest, existingDatabase, existingLibrary string
	err = tx.QueryRowContext(ctx, `SELECT i.content_digest, i.target_database, l.key FROM theory_package_imports i JOIN theory_libraries l ON l.id=i.library_id WHERE i.package_id=$1 FOR UPDATE OF i`, plan.PackageID).Scan(&existingDigest, &existingDatabase, &existingLibrary)
	if err == nil {
		if existingDigest != plan.ContentDigest || existingDatabase != currentDatabase || existingLibrary != s.libraryKey {
			return PackagePlan{}, fmt.Errorf("stage package identity mismatch: %w", ErrPackageConflict)
		}
		plan.Operation, plan.NoOp = "stage", true
		if err := tx.Commit(); err != nil {
			return PackagePlan{}, databaseFailure("commit idempotent stage", err)
		}
		return plan, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PackagePlan{}, RedactDatabaseError(err)
	}

	var libraryID int64
	var currentVersion int
	err = tx.QueryRowContext(ctx, `SELECT id, current_version FROM theory_libraries WHERE key=$1 FOR UPDATE`, s.libraryKey).Scan(&libraryID, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO theory_libraries(key,name,description,status,created_by,updated_by) VALUES($1,'芯之力理论库','经三审后发布的芯之力理论卡与实践卡','draft',$2,$2) RETURNING id,current_version`, s.libraryKey, actorID).Scan(&libraryID, &currentVersion)
	}
	if err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	desiredVersion := desiredReleaseVersion(pkg.Manifest.RoundID)
	if currentVersion > desiredVersion {
		return PackagePlan{}, fmt.Errorf("stage package desired version %d is behind active version %d: %w", desiredVersion, currentVersion, ErrPackageConflict)
	}
	keys := make([]string, 0, len(pkg.Cards)+len(pkg.Practices))
	for _, card := range pkg.Cards {
		keys = append(keys, card.CanonicalKey)
	}
	for _, practice := range pkg.Practices {
		keys = append(keys, practice.CanonicalKey)
	}
	var collision bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM theory_cards WHERE canonical_key=ANY($1) AND library_id<>$2)`, keys, libraryID).Scan(&collision); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	if collision {
		return PackagePlan{}, fmt.Errorf("stage package card key belongs to another library: %w", ErrPackageConflict)
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM theory_cards WHERE canonical_key=ANY($1) AND library_id=$2)`, keys, libraryID).Scan(&collision); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	if collision {
		return PackagePlan{}, fmt.Errorf("stage package would overwrite existing cards: %w", ErrPackageConflict)
	}
	workIDs := map[string]int64{}
	fileIDs := map[string]int64{}
	for _, source := range pkg.Manifest.Sources {
		var workID int64
		workType := "book"
		if source.Attribution["materialType"] == "course_translation_material" {
			workType = "course"
		}
		title := strings.TrimSuffix(path.Base(source.RelativePath), path.Ext(source.RelativePath))
		metadata, _ := json.Marshal(map[string]any{"sourceId": source.SourceID, "attribution": source.Attribution})
		if err := tx.QueryRowContext(ctx, `INSERT INTO theory_source_works(library_id,canonical_key,title,work_type,authority_level,epistemic_status,copyright_scope,metadata,status) VALUES($1,$2,$3,$4,3,$5,'metadata_only',$6::jsonb,'registered') RETURNING id`, libraryID, source.SourceID, title, workType, mapEpistemic(workType), metadata).Scan(&workID); err != nil {
			return PackagePlan{}, RedactDatabaseError(err)
		}
		workIDs[source.SourceID] = workID
		class, quality := "text_rich", 1.0
		if strings.Contains(source.ExtractionRoute, "ocr") {
			class, quality = "image_dominant", 0.8
		}
		var fileID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO theory_source_files(work_id,relative_path,original_filename,file_format,mime_type,sha256,title_source,extraction_class,extraction_status,extraction_quality,metadata) VALUES($1,$2,$3,$4,$5,$6,'filename',$7,'review_required',$8,'{}'::jsonb) RETURNING id`, workID, source.RelativePath, path.Base(source.RelativePath), source.Format, mimeFor(source.Format), source.SourceSHA256, class, quality).Scan(&fileID); err != nil {
			return PackagePlan{}, RedactDatabaseError(err)
		}
		fileIDs[source.SourceID] = fileID
	}
	cardIDs := map[string]int64{}
	for _, card := range pkg.Cards {
		cardID, err := insertPackageCard(ctx, tx, libraryID, actorID, card.CanonicalKey, card.Title, card.Summary, card.Definition, card.Domain, "concept", card.EpistemicStatus, card.EvidenceLevel, card.AuthorityLevel, card.Safety)
		if err != nil {
			return PackagePlan{}, err
		}
		cardIDs[card.CanonicalKey] = cardID
		if err := insertPackageSource(ctx, tx, cardID, workIDs[card.PrimaryEvidence.SourceID], fileIDs[card.PrimaryEvidence.SourceID], card.PrimaryEvidence); err != nil {
			return PackagePlan{}, err
		}
	}
	for _, practice := range pkg.Practices {
		cardID, err := insertPackageCard(ctx, tx, libraryID, actorID, practice.CanonicalKey, practice.Title, practice.Purpose, practice.Purpose, "practice", "practice", "practice_framework", "experiential", 3, practice.Safety)
		if err != nil {
			return PackagePlan{}, err
		}
		cardIDs[practice.CanonicalKey] = cardID
		if err := insertPackageSource(ctx, tx, cardID, workIDs[practice.PrimaryEvidence.SourceID], fileIDs[practice.PrimaryEvidence.SourceID], practice.PrimaryEvidence); err != nil {
			return PackagePlan{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO theory_practices(card_id,goal,steps,reflection_prompts,expected_feedback,stop_conditions,professional_escalation,contraindications,status,version) VALUES($1,$2,$3::jsonb,'[]'::jsonb,'[]'::jsonb,$4::jsonb,$5::jsonb,'不替代诊断、治疗或危机处置','draft',1)`, cardID, practice.Purpose, practice.Steps, practice.StopConditions, practice.ProfessionalEscalationConditions); err != nil {
			return PackagePlan{}, RedactDatabaseError(err)
		}
	}
	for _, relation := range pkg.Relations {
		fromID, fromOK := cardIDs[relation.From]
		toID, toOK := cardIDs[relation.To]
		if !fromOK || !toOK {
			return PackagePlan{}, fmt.Errorf("stage relation references unknown card: %w", ErrPackageConflict)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO theory_card_relations(from_card_id,to_card_id,relation_type,note,confidence,status,created_by) VALUES($1,$2,$3,'数据包显式语义关系',1,'draft',$4)`, fromID, toID, relation.Type, actorID); err != nil {
			return PackagePlan{}, RedactDatabaseError(err)
		}
	}
	fingerprints, err := databaseFingerprintJSON(ctx, tx, libraryID, payloadSHA256)
	if err != nil {
		return PackagePlan{}, err
	}
	var importID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_package_imports(package_id,content_digest,package_digest,schema_version,library_id,target_database,desired_release_version,payload,payload_sha256,payload_receipt_sha256,object_fingerprints,staged_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11::jsonb,$12) RETURNING id`, plan.PackageID, plan.ContentDigest, plan.PackageDigest, pkg.Manifest.SchemaVersion, libraryID, currentDatabase, desiredVersion, payload, payloadSHA256, payloadReceiptSHA256, fingerprints, actorID).Scan(&importID); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	if importID <= 0 {
		return PackagePlan{}, errors.New("stage package: invalid import receipt")
	}
	if err := tx.Commit(); err != nil {
		return PackagePlan{}, RedactDatabaseError(err)
	}
	plan.Operation, plan.WriteAllowed = "stage", true
	return plan, nil
}

func loadStagedPackage(root string, plan PackagePlan) (stagedPackage, []byte, []byte, error) {
	var pkg stagedPackage
	if err := readPackageJSON(root, "manifest.json", &pkg.Manifest); err != nil {
		return pkg, nil, nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "cards"))
	if err != nil {
		return pkg, nil, nil, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var card packageCard
		if err := readPackageJSON(root, filepath.Join("cards", entry.Name()), &card); err != nil {
			return pkg, nil, nil, err
		}
		pkg.Cards = append(pkg.Cards, card)
	}
	entries, err = os.ReadDir(filepath.Join(root, "practices"))
	if err != nil {
		return pkg, nil, nil, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var practice packagePractice
		if err := readPackageJSON(root, filepath.Join("practices", entry.Name()), &practice); err != nil {
			return pkg, nil, nil, err
		}
		pkg.Practices = append(pkg.Practices, practice)
	}
	var relations struct {
		Relations []packageRelation `json:"relations"`
	}
	if err := readPackageJSON(root, "relations.json", &relations); err != nil {
		return pkg, nil, nil, err
	}
	pkg.Relations = relations.Relations
	entries, err = os.ReadDir(filepath.Join(root, "chunk-previews"))
	if err != nil {
		return pkg, nil, nil, err
	}
	fingerprintMap := map[string]string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var preview packagePreview
		if err := readPackageJSON(root, filepath.Join("chunk-previews", entry.Name()), &preview); err != nil {
			return pkg, nil, nil, err
		}
		sum := sha256.Sum256([]byte(preview.Text))
		if fmt.Sprintf("%x", sum) != preview.ContentHash {
			return pkg, nil, nil, fmt.Errorf("preview digest mismatch: %w", ErrPackageConflict)
		}
		pkg.Previews = append(pkg.Previews, preview)
		fingerprintMap[preview.CanonicalKey] = preview.ContentHash
	}
	sort.Slice(pkg.Cards, func(i, j int) bool { return pkg.Cards[i].CanonicalKey < pkg.Cards[j].CanonicalKey })
	sort.Slice(pkg.Practices, func(i, j int) bool { return pkg.Practices[i].CanonicalKey < pkg.Practices[j].CanonicalKey })
	sort.Slice(pkg.Previews, func(i, j int) bool { return pkg.Previews[i].CanonicalKey < pkg.Previews[j].CanonicalKey })
	if len(pkg.Cards) != plan.Cards || len(pkg.Practices) != plan.Practices || len(pkg.Manifest.Sources) != plan.Sources || len(pkg.Previews) != plan.Cards+plan.Practices {
		return pkg, nil, nil, fmt.Errorf("package object count mismatch: %w", ErrPackageConflict)
	}
	payload, err := json.Marshal(pkg)
	if err != nil {
		return pkg, nil, nil, err
	}
	fingerprints, err := json.Marshal(fingerprintMap)
	return pkg, payload, fingerprints, err
}

func readPackageJSON(root, name string, target any) error {
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("decode package object %s: %w", name, err)
	}
	return nil
}
func desiredReleaseVersion(roundID string) int {
	value := strings.TrimPrefix(roundID, "round-")
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 1
	}
	return n
}
func mapEpistemic(workType string) string {
	if workType == "course" {
		return "course_adaptation"
	}
	return "author_interpretation"
}
func mapCardEpistemic(value string) string {
	switch value {
	case "course_adaptation":
		return value
	case "traditional_symbolism":
		return value
	case "evidence_informed":
		return value
	default:
		return "author_interpretation"
	}
}
func mapEvidence(value string) string {
	switch value {
	case "experiential":
		return value
	case "mixed":
		return "moderate"
	case "textual":
		return "limited"
	default:
		return "unknown"
	}
}
func mimeFor(format string) string {
	switch format {
	case "pdf":
		return "application/pdf"
	case "epub":
		return "application/epub+zip"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return "application/octet-stream"
}
func insertPackageCard(ctx context.Context, tx *sql.Tx, libraryID, actorID int64, key, title, summary, definition, domain, kind, epistemic, evidence string, authority int, safety json.RawMessage) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO theory_cards(library_id,canonical_key,canonical_name,domain,card_kind,summary,definition,core_claim,applicable_context,non_applicable_context,shadow_or_risk,epistemic_status,evidence_level,clinical_safety,controversy_notes,cultural_context,authority_level,language,status,version,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$6,'一般自我反思与教育场景','不用于诊断、治疗、危机替代或强迫参与',$8,$9,$10,'caution','需经来源、理论与安全三审',$11,$12,'zh-CN','draft',1,$13,$13) RETURNING id`, libraryID, key, title, domain, kind, summary, definition, string(safety), mapCardEpistemic(epistemic), mapEvidence(evidence), string(safety), authority, actorID).Scan(&id)
	if err != nil {
		return 0, RedactDatabaseError(err)
	}
	return id, nil
}
func insertPackageSource(ctx context.Context, tx *sql.Tx, cardID, workID, fileID int64, evidence packageEvidence) error {
	if workID <= 0 || fileID <= 0 {
		return fmt.Errorf("source mapping missing: %w", ErrPackageConflict)
	}
	locator, _ := json.Marshal(evidence.Locator)
	var page any
	if raw, ok := evidence.Locator["page"].(float64); ok && raw >= 1 {
		page = int(raw)
	}
	quality := 1.0
	if strings.Contains(evidence.ExtractionRoute, "ocr") {
		quality = .8
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO theory_card_sources(card_id,work_id,file_id,source_role,page_start,page_end,location_label,quotation,interpretation_note,extraction_quality,quote_verified) VALUES($1,$2,$3,'primary',$4,$4,$5,'','数据包为原创提炼，定位仅用于人工来源核验',$6,false)`, cardID, workID, fileID, page, string(locator), quality)
	if err != nil {
		return RedactDatabaseError(err)
	}
	return nil
}

func databaseFingerprintJSON(ctx context.Context, tx *sql.Tx, libraryID int64, payloadSHA256 string) ([]byte, error) {
	snapshot, err := loadCanonicalDatabaseSnapshot(ctx, tx, libraryID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode database snapshot: %w", err)
	}
	digest := sha256.Sum256(payload)
	fingerprint := databaseFingerprint{SchemaVersion: "xinzhili.database-snapshot.v1", SHA256: fmt.Sprintf("%x", digest), PayloadSHA256: payloadSHA256, Snapshot: snapshot}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("encode database fingerprint: %w", err)
	}
	return encoded, nil
}

func loadCanonicalDatabaseSnapshot(ctx context.Context, tx *sql.Tx, libraryID int64) (canonicalDatabaseSnapshot, error) {
	snapshot := canonicalDatabaseSnapshot{SchemaVersion: "xinzhili.database-snapshot.v1"}
	queries := []struct {
		target      *json.RawMessage
		name, query string
	}{
		{&snapshot.Cards, "cards", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'canonical_key'),'[]'::jsonb) FROM (SELECT to_jsonb(c)-ARRAY['id','library_id','status','reviewed_by','reviewed_at','published_at','created_by','updated_by','create_time','update_time'] AS item FROM theory_cards c WHERE c.library_id=$1) rows`},
		{&snapshot.Practices, "practices", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'card_canonical_key'),'[]'::jsonb) FROM (SELECT (to_jsonb(p)-ARRAY['id','card_id','status','create_time','update_time'])||jsonb_build_object('card_canonical_key',c.canonical_key) AS item FROM theory_practices p JOIN theory_cards c ON c.id=p.card_id WHERE c.library_id=$1) rows`},
		{&snapshot.SourceWorks, "source works", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'canonical_key'),'[]'::jsonb) FROM (SELECT (to_jsonb(w)-ARRAY['id','library_id','canonical_work_id','status','create_time','update_time'])||jsonb_build_object('canonical_work_key',canonical.canonical_key) AS item FROM theory_source_works w LEFT JOIN theory_source_works canonical ON canonical.id=w.canonical_work_id WHERE w.library_id=$1) rows`},
		{&snapshot.SourceFiles, "source files", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'work_canonical_key',item->>'relative_path',item->>'sha256'),'[]'::jsonb) FROM (SELECT (to_jsonb(f)-ARRAY['id','work_id','duplicate_of_file_id','create_time','update_time'])||jsonb_build_object('work_canonical_key',w.canonical_key,'duplicate_sha256',duplicate.sha256) AS item FROM theory_source_files f JOIN theory_source_works w ON w.id=f.work_id LEFT JOIN theory_source_files duplicate ON duplicate.id=f.duplicate_of_file_id WHERE w.library_id=$1) rows`},
		{&snapshot.CardSources, "card sources", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'card_canonical_key',item->>'work_canonical_key',item->>'file_sha256',item->>'location_label'),'[]'::jsonb) FROM (SELECT (to_jsonb(s)-ARRAY['id','card_id','work_id','file_id','verified_by','verified_at','create_time','update_time'])||jsonb_build_object('card_canonical_key',c.canonical_key,'work_canonical_key',w.canonical_key,'file_sha256',f.sha256) AS item FROM theory_card_sources s JOIN theory_cards c ON c.id=s.card_id JOIN theory_source_works w ON w.id=s.work_id LEFT JOIN theory_source_files f ON f.id=s.file_id WHERE c.library_id=$1) rows`},
		{&snapshot.Relations, "relations", `SELECT COALESCE(jsonb_agg(item ORDER BY item->>'from_canonical_key',item->>'to_canonical_key',item->>'relation_type'),'[]'::jsonb) FROM (SELECT (to_jsonb(r)-ARRAY['id','from_card_id','to_card_id','status','created_by','reviewed_by','create_time','update_time'])||jsonb_build_object('from_canonical_key',source.canonical_key,'to_canonical_key',target.canonical_key) AS item FROM theory_card_relations r JOIN theory_cards source ON source.id=r.from_card_id JOIN theory_cards target ON target.id=r.to_card_id WHERE source.library_id=$1) rows`},
	}
	for _, query := range queries {
		var raw []byte
		if err := tx.QueryRowContext(ctx, query.query, libraryID).Scan(&raw); err != nil {
			return canonicalDatabaseSnapshot{}, databaseFailure("snapshot "+query.name, err)
		}
		if !json.Valid(raw) {
			return canonicalDatabaseSnapshot{}, fmt.Errorf("snapshot %s returned invalid JSON", query.name)
		}
		*query.target = append((*query.target)[:0], raw...)
	}
	return snapshot, nil
}

func verifyDatabaseFingerprint(ctx context.Context, tx *sql.Tx, libraryID int64, encoded []byte, payloadSHA256 string) error {
	var expected databaseFingerprint
	if err := decodeStrictJSON(encoded, &expected); err != nil {
		return fmt.Errorf("decode stored database fingerprint: %v: %w", err, ErrPackageConflict)
	}
	if expected.SchemaVersion != "xinzhili.database-snapshot.v1" || expected.Snapshot.SchemaVersion != expected.SchemaVersion || !isLowerSHA256(expected.SHA256) || !isLowerSHA256(expected.PayloadSHA256) || expected.PayloadSHA256 != payloadSHA256 || !completeSnapshotContract(expected.Snapshot) {
		return fmt.Errorf("stored database fingerprint contract invalid: %w", ErrPackageConflict)
	}
	expectedPayload, err := json.Marshal(expected.Snapshot)
	if err != nil {
		return fmt.Errorf("encode expected database snapshot: %w", err)
	}
	expectedDigest := sha256.Sum256(expectedPayload)
	if fmt.Sprintf("%x", expectedDigest) != expected.SHA256 {
		return fmt.Errorf("stored database fingerprint digest invalid: %w", ErrPackageConflict)
	}
	current, err := loadCanonicalDatabaseSnapshot(ctx, tx, libraryID)
	if err != nil {
		return err
	}
	currentPayload, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("encode current database snapshot: %w", err)
	}
	currentDigest := sha256.Sum256(currentPayload)
	if fmt.Sprintf("%x", currentDigest) != expected.SHA256 || !jsonBytesEqual(currentPayload, expectedPayload) {
		return ErrImportedContentChanged
	}
	return nil
}

func decodeStrictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func completeSnapshotContract(snapshot canonicalDatabaseSnapshot) bool {
	for _, raw := range []json.RawMessage{snapshot.Cards, snapshot.Practices, snapshot.SourceWorks, snapshot.SourceFiles, snapshot.CardSources, snapshot.Relations} {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' || !json.Valid(trimmed) {
			return false
		}
	}
	return true
}

func canonicalJSONSHA256(payload []byte) (string, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", digest), nil
}

func payloadContractReceiptSHA256(packageID, contentDigest, packageDigest, payloadSHA256 string) string {
	receipt, _ := json.Marshal(struct {
		PackageID     string `json:"packageId"`
		ContentDigest string `json:"contentDigest"`
		PackageDigest string `json:"packageDigest"`
		PayloadSHA256 string `json:"payloadSha256"`
	}{packageID, contentDigest, packageDigest, payloadSHA256})
	digest := sha256.Sum256(receipt)
	return fmt.Sprintf("%x", digest)
}

func jsonBytesEqual(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func (s *PackageSyncer) RecordReview(ctx context.Context, packageID string, reviewType ReviewType, reviewerID int64, notes string) (ReviewReceipt, error) {
	requiredRole, ok := reviewRoles[reviewType]
	if !ok || strings.TrimSpace(packageID) == "" || reviewerID <= 0 {
		return ReviewReceipt{}, ErrReviewerRole
	}
	if s == nil || s.db == nil {
		return ReviewReceipt{}, errors.New("record review: database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "nine-xing:theory-package:"+s.libraryKey); err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	var importID int64
	var digest, state, databaseName, libraryKey string
	if err := tx.QueryRowContext(ctx, `SELECT i.id,i.content_digest,i.state,i.target_database,l.key FROM theory_package_imports i JOIN theory_libraries l ON l.id=i.library_id WHERE i.package_id=$1 FOR UPDATE OF i`, packageID).Scan(&importID, &digest, &state, &databaseName, &libraryKey); err != nil {
		return ReviewReceipt{}, fmt.Errorf("record review: package not staged: %w", ErrPackageConflict)
	}
	var currentDatabase string
	if err := tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&currentDatabase); err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	if state != "staged" || databaseName != currentDatabase || libraryKey != s.libraryKey {
		return ReviewReceipt{}, fmt.Errorf("record review scope mismatch: %w", ErrPackageConflict)
	}
	var roleOK bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE u.id=$1 AND u.status=1 AND r.status=1 AND r.code=$2)`, reviewerID, requiredRole).Scan(&roleOK); err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	if !roleOK {
		return ReviewReceipt{}, ErrReviewerRole
	}
	var existingReviewer int64
	err = tx.QueryRowContext(ctx, `SELECT reviewer_user_id FROM theory_package_reviews WHERE import_id=$1 AND review_type=$2 AND content_digest=$3`, importID, reviewType, digest).Scan(&existingReviewer)
	if err == nil {
		if existingReviewer != reviewerID {
			return ReviewReceipt{}, fmt.Errorf("review already recorded by another user: %w", ErrPackageConflict)
		}
		if err := tx.Commit(); err != nil {
			return ReviewReceipt{}, RedactDatabaseError(err)
		}
		return ReviewReceipt{PackageID: packageID, ContentDigest: digest, ReviewType: reviewType, ReviewerID: reviewerID, NoOp: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO theory_package_reviews(import_id,review_type,content_digest,decision,reviewer_user_id,reviewer_role,notes) VALUES($1,$2,$3,'approved',$4,$5,$6)`, importID, reviewType, digest, reviewerID, requiredRole, strings.TrimSpace(notes)); err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	if err := tx.Commit(); err != nil {
		return ReviewReceipt{}, RedactDatabaseError(err)
	}
	return ReviewReceipt{PackageID: packageID, ContentDigest: digest, ReviewType: reviewType, ReviewerID: reviewerID}, nil
}

func (s *PackageSyncer) Promote(ctx context.Context, packageID string, actorID int64) (PromotionReceipt, error) {
	for attempt := 0; attempt < 3; attempt++ {
		receipt, err := s.promoteOnce(ctx, packageID, actorID)
		if err == nil || !isSerializationFailure(err) {
			return receipt, err
		}
		if ctx.Err() != nil {
			return PromotionReceipt{}, ctx.Err()
		}
	}
	return PromotionReceipt{}, databaseFailure("promote package retry exhausted", errors.New("serialization retry exhausted"))
}

func (s *PackageSyncer) promoteOnce(ctx context.Context, packageID string, actorID int64) (PromotionReceipt, error) {
	if s == nil || s.db == nil {
		return PromotionReceipt{}, errors.New("promote package: database is unavailable")
	}
	if actorID <= 0 {
		return PromotionReceipt{}, ErrActorInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "nine-xing:theory-package:"+s.libraryKey); err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	var importID, libraryID int64
	var digest, packageDigest, schemaVersion, state, targetDatabase, libraryKey string
	var desiredVersion int
	var storedPayloadSHA256, storedPayloadReceiptSHA256 string
	var payload, fingerprints []byte
	if err := tx.QueryRowContext(ctx, `SELECT i.id,i.library_id,i.content_digest,i.package_digest,i.schema_version,i.state,i.target_database,l.key,i.desired_release_version,i.payload,i.payload_sha256,i.payload_receipt_sha256,i.object_fingerprints FROM theory_package_imports i JOIN theory_libraries l ON l.id=i.library_id WHERE i.package_id=$1 FOR UPDATE OF i`, packageID).Scan(&importID, &libraryID, &digest, &packageDigest, &schemaVersion, &state, &targetDatabase, &libraryKey, &desiredVersion, &payload, &storedPayloadSHA256, &storedPayloadReceiptSHA256, &fingerprints); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromotionReceipt{}, fmt.Errorf("promote package not staged: %w", ErrPackageConflict)
		}
		return PromotionReceipt{}, databaseFailure("lock staged package", err)
	}
	var currentDatabase string
	if err := tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&currentDatabase); err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	if targetDatabase != currentDatabase || libraryKey != s.libraryKey {
		return PromotionReceipt{}, fmt.Errorf("promote package scope mismatch: %w", ErrPackageConflict)
	}
	var actorOK bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status=1)`, actorID).Scan(&actorOK); err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	if !actorOK {
		return PromotionReceipt{}, ErrActorInvalid
	}
	computedPayloadSHA256, err := canonicalJSONSHA256(payload)
	if err != nil {
		return PromotionReceipt{}, fmt.Errorf("canonicalize staged payload: %w", ErrPackageConflict)
	}
	if computedPayloadSHA256 != storedPayloadSHA256 || payloadContractReceiptSHA256(packageID, digest, packageDigest, storedPayloadSHA256) != storedPayloadReceiptSHA256 {
		return PromotionReceipt{}, fmt.Errorf("staged payload receipt mismatch: %w", ErrPackageConflict)
	}
	var pkg stagedPackage
	if err := json.Unmarshal(payload, &pkg); err != nil {
		return PromotionReceipt{}, fmt.Errorf("decode staged payload: %w", ErrPackageConflict)
	}
	if pkg.Manifest.PackageID != packageID || pkg.Manifest.ContentDigest != digest || pkg.Manifest.PackageDigest != packageDigest || pkg.Manifest.SchemaVersion != schemaVersion {
		return PromotionReceipt{}, fmt.Errorf("staged package digest contract mismatch: %w", ErrPackageConflict)
	}
	reviewers := map[ReviewType]int64{}
	reviewerRoles := map[ReviewType]string{}
	rows, err := tx.QueryContext(ctx, `SELECT review_type,reviewer_user_id,reviewer_role FROM theory_package_reviews WHERE import_id=$1 AND content_digest=$2 AND decision='approved'`, importID, digest)
	if err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	for rows.Next() {
		var kind ReviewType
		var id int64
		var role string
		if err := rows.Scan(&kind, &id, &role); err != nil {
			rows.Close()
			return PromotionReceipt{}, databaseFailure("verify card sources", err)
		}
		reviewers[kind] = id
		reviewerRoles[kind] = role
	}
	if err := rows.Close(); err != nil {
		return PromotionReceipt{}, databaseFailure("publish cards", err)
	}
	if len(reviewers) != 3 {
		return PromotionReceipt{}, ErrReviewsIncomplete
	}
	seen := map[int64]bool{}
	for kind := range reviewRoles {
		id, ok := reviewers[kind]
		requiredRole := reviewRoles[kind]
		if !ok || reviewerRoles[kind] != requiredRole {
			return PromotionReceipt{}, ErrReviewsIncomplete
		}
		var liveRole bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE u.id=$1 AND u.status=1 AND r.status=1 AND r.code=$2)`, id, requiredRole).Scan(&liveRole); err != nil {
			return PromotionReceipt{}, RedactDatabaseError(err)
		}
		if !liveRole {
			return PromotionReceipt{}, ErrReviewsIncomplete
		}
		if id == actorID {
			return PromotionReceipt{}, ErrActorSeparation
		}
		if seen[id] {
			return PromotionReceipt{}, ErrActorSeparation
		}
		seen[id] = true
	}
	if err := verifyDatabaseFingerprint(ctx, tx, libraryID, fingerprints, storedPayloadSHA256); err != nil {
		return PromotionReceipt{}, err
	}
	if err := verifyPackageWorkflowState(ctx, tx, libraryID, state, reviewers); err != nil {
		return PromotionReceipt{}, err
	}
	var currentVersion, maxRelease, maxActive int
	if err := tx.QueryRowContext(ctx, `SELECT current_version FROM theory_libraries WHERE id=$1 FOR UPDATE`, libraryID).Scan(&currentVersion); err != nil {
		return PromotionReceipt{}, databaseFailure("publish practices", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0),COALESCE(max(version) FILTER(WHERE status='active'),0) FROM theory_library_releases WHERE library_id=$1`, libraryID).Scan(&maxRelease, &maxActive); err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	if currentVersion > desiredVersion || maxRelease > desiredVersion || maxActive > desiredVersion {
		return PromotionReceipt{}, fmt.Errorf("release version %d conflicts with current/max/active %d/%d/%d: %w", desiredVersion, currentVersion, maxRelease, maxActive, ErrPackageConflict)
	}
	var prior PromotionReceipt
	err = tx.QueryRowContext(ctx, `SELECT p.release_id,r.version,r.status,r.card_count,r.chunk_count FROM theory_package_promotions p JOIN theory_library_releases r ON r.id=p.release_id WHERE p.import_id=$1 AND p.content_digest=$2`, importID, digest).Scan(&prior.ReleaseID, &prior.ReleaseVersion, &prior.ReleaseStatus, &prior.CardCount, &prior.ChunkCount)
	if err == nil {
		if state != "promoted" || prior.ReleaseVersion != desiredVersion || maxRelease != desiredVersion || (prior.ReleaseStatus != ReleaseStatusReady && prior.ReleaseStatus != ReleaseStatusActive) {
			return PromotionReceipt{}, fmt.Errorf("promotion receipt conflicts with package/version state: %w", ErrPackageConflict)
		}
		if err := verifyReadyRelease(ctx, tx, prior.ReleaseID, pkg); err != nil {
			return PromotionReceipt{}, err
		}
		prior.PackageID, prior.ContentDigest, prior.NoOp = packageID, digest, true
		if err := tx.Commit(); err != nil {
			return PromotionReceipt{}, databaseFailure("commit idempotent promotion", err)
		}
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	if state != "staged" {
		return PromotionReceipt{}, fmt.Errorf("package state %s: %w", state, ErrPackageConflict)
	}
	if currentVersion >= desiredVersion || maxRelease >= desiredVersion {
		return PromotionReceipt{}, fmt.Errorf("release version %d is not monotonic after %d/%d: %w", desiredVersion, currentVersion, maxRelease, ErrPackageConflict)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_source_works SET status='reviewed',update_time=now() WHERE library_id=$1`, libraryID); err != nil {
		return PromotionReceipt{}, databaseFailure("publish source works", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_card_sources s SET verified_by=$2,verified_at=now(),update_time=now() FROM theory_cards c WHERE s.card_id=c.id AND c.library_id=$1`, libraryID, reviewers[ReviewSourceVerification]); err != nil {
		return PromotionReceipt{}, databaseFailure("verify card sources", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE theory_cards SET status='published',reviewed_by=$2,reviewed_at=now(),published_at=now(),updated_by=$3,update_time=now() WHERE library_id=$1 AND status='draft' AND version=1`, libraryID, reviewers[ReviewTheory], actorID)
	if err != nil {
		return PromotionReceipt{}, databaseFailure("publish cards", err)
	}
	affected, _ := result.RowsAffected()
	if affected != int64(len(pkg.Cards)+len(pkg.Practices)) {
		return PromotionReceipt{}, fmt.Errorf("publish card count %d: %w", affected, ErrImportedContentChanged)
	}
	result, err = tx.ExecContext(ctx, `UPDATE theory_practices p SET status='published',update_time=now() FROM theory_cards c WHERE p.card_id=c.id AND c.library_id=$1 AND p.status='draft' AND p.version=1`, libraryID)
	if err != nil {
		return PromotionReceipt{}, databaseFailure("publish practices", err)
	}
	affected, _ = result.RowsAffected()
	if affected != int64(len(pkg.Practices)) {
		return PromotionReceipt{}, fmt.Errorf("publish practice count %d: %w", affected, ErrImportedContentChanged)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_card_relations r SET status='published',reviewed_by=$2,update_time=now() FROM theory_cards c WHERE r.from_card_id=c.id AND c.library_id=$1 AND r.status='draft'`, libraryID, reviewers[ReviewTheory]); err != nil {
		return PromotionReceipt{}, databaseFailure("publish relations", err)
	}
	cardIDs := map[string]int64{}
	rows, err = tx.QueryContext(ctx, `SELECT canonical_key,id FROM theory_cards WHERE library_id=$1 AND status='published'`, libraryID)
	if err != nil {
		return PromotionReceipt{}, RedactDatabaseError(err)
	}
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			rows.Close()
			return PromotionReceipt{}, databaseFailure("publish relations", err)
		}
		cardIDs[key] = id
	}
	rows.Close()
	chunkIDs := map[string]int64{}
	for _, preview := range pkg.Previews {
		cardID := cardIDs[preview.CanonicalKey]
		if cardID <= 0 {
			return PromotionReceipt{}, fmt.Errorf("preview card missing: %w", ErrImportedContentChanged)
		}
		var practiceID *int64
		if preview.SourceKind == "practice" {
			var id int64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM theory_practices WHERE card_id=$1 AND status='published'`, cardID).Scan(&id); err != nil {
				return PromotionReceipt{}, databaseFailure("load published practice", err)
			}
			practiceID = &id
		}
		var title, authority, evidence, safety string
		var authorityLevel int
		if err := tx.QueryRowContext(ctx, `SELECT canonical_name,authority_level,evidence_level,clinical_safety FROM theory_cards WHERE id=$1`, cardID).Scan(&title, &authorityLevel, &evidence, &safety); err != nil {
			return PromotionReceipt{}, databaseFailure("insert formal chunk", err)
		}
		authority = strconv.Itoa(authorityLevel)
		_ = authority
		keywords, _ := json.Marshal([]string{preview.CanonicalKey, title})
		var chunkID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO theory_chunks(library_id,card_id,practice_id,chunk_key,chunk_kind,title,content,keywords,tags,authority_level,evidence_level,clinical_safety,token_count,content_hash,version,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,'["xinzhili","round-001"]'::jsonb,$9,$10,$11,$12,$13,1,'enabled') RETURNING id`, libraryID, cardID, practiceID, preview.CanonicalKey, mapChunkKind(practiceID), title, preview.Text, keywords, authorityLevel, evidence, safety, len([]rune(preview.Text)), preview.ContentHash).Scan(&chunkID); err != nil {
			return PromotionReceipt{}, databaseFailure("insert formal chunk", err)
		}
		chunkIDs[preview.CanonicalKey] = chunkID
	}
	var releaseID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_library_releases(library_id,version,status,retrieval_mode,index_version,card_count,chunk_count) VALUES($1,$2,'building','lexical_only',$3,0,0) RETURNING id`, libraryID, desiredVersion, digest).Scan(&releaseID); err != nil {
		return PromotionReceipt{}, databaseFailure("create release", err)
	}
	for _, preview := range pkg.Previews {
		if _, err := tx.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES($1,$2,$3)`, releaseID, cardIDs[preview.CanonicalKey], chunkIDs[preview.CanonicalKey]); err != nil {
			return PromotionReceipt{}, databaseFailure("map release chunk", err)
		}
	}
	cardCount, chunkCount := len(cardIDs), len(chunkIDs)
	if cardCount != 52 || chunkCount != 52 {
		return PromotionReceipt{}, fmt.Errorf("ready release incomplete: %w", ErrImportedContentChanged)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='ready',card_count=$2,chunk_count=$3,update_time=now() WHERE id=$1 AND status='building'`, releaseID, cardCount, chunkCount); err != nil {
		return PromotionReceipt{}, databaseFailure("finalize release", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO theory_package_promotions(import_id,content_digest,release_id,promoted_by) VALUES($1,$2,$3,$4)`, importID, digest, releaseID, actorID); err != nil {
		return PromotionReceipt{}, databaseFailure("write promotion receipt", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_package_imports SET state='promoted',promoted_at=now(),update_time=now() WHERE id=$1 AND state='staged'`, importID); err != nil {
		return PromotionReceipt{}, databaseFailure("mark package promoted", err)
	}
	if s.beforePromotionCommit != nil {
		if err := s.beforePromotionCommit(tx); err != nil {
			return PromotionReceipt{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PromotionReceipt{}, databaseFailure("commit promotion", err)
	}
	return PromotionReceipt{PackageID: packageID, ContentDigest: digest, ReleaseID: releaseID, ReleaseVersion: desiredVersion, ReleaseStatus: ReleaseStatusReady, CardCount: cardCount, ChunkCount: chunkCount}, nil
}

func verifyReadyRelease(ctx context.Context, tx *sql.Tx, releaseID int64, pkg stagedPackage) error {
	var mappings int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM theory_release_cards WHERE release_id=$1`, releaseID).Scan(&mappings); err != nil {
		return RedactDatabaseError(err)
	}
	if mappings != len(pkg.Previews) {
		return ErrImportedContentChanged
	}
	for _, preview := range pkg.Previews {
		var content, hash, status, key string
		var version int
		err := tx.QueryRowContext(ctx, `SELECT ch.content,ch.content_hash,ch.status,ch.version,c.canonical_key FROM theory_release_cards m JOIN theory_chunks ch ON ch.id=m.chunk_id JOIN theory_cards c ON c.id=m.card_id WHERE m.release_id=$1 AND ch.chunk_key=$2`, releaseID, preview.CanonicalKey).Scan(&content, &hash, &status, &version, &key)
		if err != nil {
			return ErrImportedContentChanged
		}
		if content != preview.Text || hash != preview.ContentHash || status != "enabled" || version != 1 || key != preview.CanonicalKey {
			return fmt.Errorf("%w: ready chunk %s fingerprint mismatch", ErrImportedContentChanged, preview.CanonicalKey)
		}
	}
	return nil
}

func verifyPackageWorkflowState(ctx context.Context, tx *sql.Tx, libraryID int64, state string, reviewers map[ReviewType]int64) error {
	expectCard, expectPractice, expectRelation, expectWork := "draft", "draft", "draft", "registered"
	if state == "promoted" {
		expectCard, expectPractice, expectRelation, expectWork = "published", "published", "published", "reviewed"
	} else if state != "staged" {
		return fmt.Errorf("package workflow state %s invalid: %w", state, ErrPackageConflict)
	}
	checks := []struct {
		name, query string
		want        int
		args        []any
	}{
		{"cards", `SELECT count(*) FROM theory_cards WHERE library_id=$1 AND status=$2`, 52, []any{libraryID, expectCard}},
		{"practices", `SELECT count(*) FROM theory_practices p JOIN theory_cards c ON c.id=p.card_id WHERE c.library_id=$1 AND p.status=$2`, 12, []any{libraryID, expectPractice}},
		{"relations", `SELECT count(*) FROM theory_card_relations r JOIN theory_cards c ON c.id=r.from_card_id WHERE c.library_id=$1 AND r.status=$2`, 19, []any{libraryID, expectRelation}},
		{"source works", `SELECT count(*) FROM theory_source_works WHERE library_id=$1 AND status=$2`, 24, []any{libraryID, expectWork}},
	}
	for _, check := range checks {
		var count int
		if err := tx.QueryRowContext(ctx, check.query, check.args...).Scan(&count); err != nil {
			return databaseFailure("verify workflow "+check.name, err)
		}
		if count != check.want {
			return fmt.Errorf("%w: workflow %s count %d", ErrImportedContentChanged, check.name, count)
		}
	}
	var verified int
	if state == "staged" {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM theory_card_sources s JOIN theory_cards c ON c.id=s.card_id WHERE c.library_id=$1 AND s.verified_by IS NULL AND s.verified_at IS NULL`, libraryID).Scan(&verified); err != nil {
			return RedactDatabaseError(err)
		}
	} else {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM theory_card_sources s JOIN theory_cards c ON c.id=s.card_id WHERE c.library_id=$1 AND s.verified_by=$2 AND s.verified_at IS NOT NULL`, libraryID, reviewers[ReviewSourceVerification]).Scan(&verified); err != nil {
			return RedactDatabaseError(err)
		}
	}
	if verified != 52 {
		return fmt.Errorf("%w: workflow card source verification count %d", ErrImportedContentChanged, verified)
	}
	return nil
}
func mapChunkKind(practiceID *int64) string {
	if practiceID != nil {
		return "practice"
	}
	return "card"
}

func ActivatePackage(context.Context, *sql.DB, string, int64) error {
	return fmt.Errorf("%w: safety evaluation is not_runnable_for_activation; milestone B retrieval integration and milestone C session safety integration are incomplete", ErrActivationBlocked)
}
