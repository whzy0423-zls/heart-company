package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/enneagramcatalog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const enneagramLibraryPathPrefix = "/api/enneagram-library/types/"

type enneagramLibraryAdminService interface {
	Overview(context.Context) ([]enneagramTypeSummary, error)
	Detail(context.Context, int) (enneagramTypeDetail, error)
	SaveDraft(context.Context, int, enneagramDraftInput, int64) (enneagramTypeDetail, error)
	SubmitReview(context.Context, int, int64) error
	Approve(context.Context, int, int64, string) error
	Preview(context.Context, int, string) (enneagramPreview, error)
	Publish(context.Context, int, int64) (enneagramPublishResult, error)
	Versions(context.Context, int) ([]enneagramVersion, error)
	Rollback(context.Context, int, int, int64) (enneagramPublishResult, error)
}

type enneagramLibraryAdminStore struct {
	db      *sql.DB
	catalog *enneagramcatalog.Store
}

func newEnneagramLibraryAdminStore(database *sql.DB) *enneagramLibraryAdminStore {
	return &enneagramLibraryAdminStore{db: database, catalog: enneagramcatalog.NewStore(database)}
}

type enneagramTypeSummary struct {
	Type           int        `json:"type"`
	LibraryID      int64      `json:"libraryId"`
	LibraryKey     string     `json:"libraryKey"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	ImportID       int64      `json:"importId"`
	ItemCount      int        `json:"itemCount"`
	CurrentVersion int        `json:"currentVersion"`
	ActiveRelease  *int64     `json:"activeReleaseId"`
	UpdatedAt      *time.Time `json:"updatedAt"`
}

type enneagramTypeItem struct {
	ContentKey     string                    `json:"contentKey"`
	Dimension      string                    `json:"dimension"`
	Text           string                    `json:"text"`
	ProvenanceKind string                    `json:"provenanceKind"`
	SourcePages    []enneagramSourcePageView `json:"sourcePages"`
}

type enneagramSourcePageView struct {
	SourceID           string `json:"sourceId"`
	PageNumber         int    `json:"pageNumber"`
	EnneagramType      int    `json:"enneagramType"`
	OCRTextHash        string `json:"ocrTextHash"`
	OCRStatus          string `json:"ocrStatus"`
	ManualReviewStatus string `json:"manualReviewStatus"`
}

type enneagramTypeDetail struct {
	Summary       enneagramTypeSummary `json:"summary"`
	SourceChapter string               `json:"sourceChapter"`
	ContentDigest string               `json:"contentDigest"`
	ReviewNotes   string               `json:"reviewNotes"`
	Items         []enneagramTypeItem  `json:"items"`
}

type enneagramDraftItemInput struct {
	ContentKey string `json:"contentKey"`
	Text       string `json:"text"`
}

type enneagramDraftInput struct {
	Title         string                    `json:"title"`
	SourceChapter string                    `json:"sourceChapter"`
	ContentDigest string                    `json:"contentDigest"`
	Items         []enneagramDraftItemInput `json:"items"`
}

type enneagramPreviewHit struct {
	ContentKey string  `json:"contentKey"`
	Dimension  string  `json:"dimension"`
	Text       string  `json:"text"`
	Score      float64 `json:"score"`
}

type enneagramPreview struct {
	Type  int                   `json:"type"`
	Query string                `json:"query"`
	Hits  []enneagramPreviewHit `json:"hits"`
}

type enneagramPublishResult struct {
	ImportID   int64  `json:"importId"`
	LibraryID  int64  `json:"libraryId"`
	LibraryKey string `json:"libraryKey"`
	ReleaseID  int64  `json:"releaseId"`
	Version    int    `json:"version"`
}

type enneagramVersion struct {
	ReleaseID    int64      `json:"releaseId"`
	Version      int        `json:"version"`
	Status       string     `json:"status"`
	CardCount    int        `json:"cardCount"`
	ChunkCount   int        `json:"chunkCount"`
	IndexVersion string     `json:"indexVersion"`
	ActivatedAt  *time.Time `json:"activatedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func (s *enneagramLibraryAdminStore) Overview(ctx context.Context) ([]enneagramTypeSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("九型人格库服务未配置")
	}
	result := make([]enneagramTypeSummary, 9)
	for number := 1; number <= 9; number++ {
		result[number-1] = enneagramTypeSummary{Type: number, LibraryKey: enneagramLibraryKey(number), Title: fmt.Sprintf("%d号人格", number), Status: "missing"}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (import.enneagram_type)
			import.enneagram_type,library.id,library.key,import.title,import.status,import.id,
			(SELECT count(*) FROM enneagram_catalog_import_items item WHERE item.import_id=import.id),
			library.current_version,release.id,import.update_time
		FROM enneagram_catalog_imports import
		JOIN theory_libraries library ON library.id=import.library_id
		LEFT JOIN theory_library_releases release ON release.library_id=library.id AND release.status='active'
		WHERE import.kind='enneagram_type'
		ORDER BY import.enneagram_type,import.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item enneagramTypeSummary
		if err := rows.Scan(&item.Type, &item.LibraryID, &item.LibraryKey, &item.Title, &item.Status, &item.ImportID, &item.ItemCount, &item.CurrentVersion, &item.ActiveRelease, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if item.Type >= 1 && item.Type <= 9 {
			result[item.Type-1] = item
		}
	}
	return result, rows.Err()
}

func (s *enneagramLibraryAdminStore) Detail(ctx context.Context, number int) (enneagramTypeDetail, error) {
	if s == nil || s.db == nil || !validEnneagramType(number) {
		return enneagramTypeDetail{}, errors.New("人格型号无效")
	}
	var detail enneagramTypeDetail
	var payload []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT library.id,library.key,import.title,import.status,import.id,
			(SELECT count(*) FROM enneagram_catalog_import_items item WHERE item.import_id=import.id),
			library.current_version,release.id,import.update_time,import.source_chapter,
			import.content_digest,import.review_notes,import.payload
		FROM enneagram_catalog_imports import
		JOIN theory_libraries library ON library.id=import.library_id
		LEFT JOIN theory_library_releases release ON release.library_id=library.id AND release.status='active'
		WHERE import.kind='enneagram_type' AND import.enneagram_type=$1
		ORDER BY import.id DESC LIMIT 1`, number).Scan(
		&detail.Summary.LibraryID, &detail.Summary.LibraryKey, &detail.Summary.Title,
		&detail.Summary.Status, &detail.Summary.ImportID, &detail.Summary.ItemCount,
		&detail.Summary.CurrentVersion, &detail.Summary.ActiveRelease, &detail.Summary.UpdatedAt,
		&detail.SourceChapter, &detail.ContentDigest, &detail.ReviewNotes, &payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return enneagramTypeDetail{}, errors.New("该型号知识库尚未导入")
	}
	if err != nil {
		return enneagramTypeDetail{}, err
	}
	detail.Summary.Type = number
	var packageValue enneagramcatalog.Package
	if err := json.Unmarshal(payload, &packageValue); err != nil {
		return enneagramTypeDetail{}, fmt.Errorf("读取九型目录内容: %w", err)
	}
	for _, dimension := range enneagramcatalog.RequiredDimensions {
		for _, item := range packageValue.Dimensions[dimension] {
			pages := make([]enneagramSourcePageView, 0, len(item.SourcePages))
			for _, page := range item.SourcePages {
				pages = append(pages, enneagramSourcePageView{
					SourceID: page.SourceID, PageNumber: page.PageNumber, EnneagramType: page.EnneagramType,
					OCRTextHash: page.OCRTextHash, OCRStatus: page.OCRStatus, ManualReviewStatus: page.ManualReviewStatus,
				})
			}
			detail.Items = append(detail.Items, enneagramTypeItem{
				ContentKey: item.ContentKey, Dimension: item.Dimension, Text: item.Text,
				ProvenanceKind: item.ProvenanceKind, SourcePages: pages,
			})
		}
	}
	if detail.Items == nil {
		detail.Items = []enneagramTypeItem{}
	}
	return detail, nil
}

func (s *enneagramLibraryAdminStore) SaveDraft(ctx context.Context, number int, input enneagramDraftInput, actorID int64) (enneagramTypeDetail, error) {
	if s == nil || s.db == nil || !validEnneagramType(number) || actorID <= 0 {
		return enneagramTypeDetail{}, errors.New("草稿保存参数无效")
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.SourceChapter) == "" || len(input.Items) == 0 {
		return enneagramTypeDetail{}, errors.New("标题、来源章节和条目正文均不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return enneagramTypeDetail{}, err
	}
	defer tx.Rollback()
	var importID, libraryID int64
	var currentDigest string
	var payload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT import.id,import.library_id,import.content_digest,import.payload
		FROM enneagram_catalog_imports import
		WHERE import.kind='enneagram_type' AND import.enneagram_type=$1
		ORDER BY import.id DESC LIMIT 1 FOR UPDATE`, number).Scan(&importID, &libraryID, &currentDigest, &payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return enneagramTypeDetail{}, errors.New("该型号知识库尚未导入")
		}
		return enneagramTypeDetail{}, err
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM enneagram_catalog_imports WHERE id=$1`, importID).Scan(&status); err != nil {
		return enneagramTypeDetail{}, err
	}
	if status != "draft" {
		return enneagramTypeDetail{}, errors.New("只有草稿状态的目录可以编辑")
	}
	if strings.TrimSpace(input.ContentDigest) != "" && input.ContentDigest != currentDigest {
		return enneagramTypeDetail{}, errors.New("草稿已被其他管理员更新，请刷新后重试")
	}
	var packageValue enneagramcatalog.Package
	if err := json.Unmarshal(payload, &packageValue); err != nil {
		return enneagramTypeDetail{}, err
	}
	provided := make(map[string]string, len(input.Items))
	dimensionsByKey := make(map[string]string)
	for _, item := range input.Items {
		key, text := strings.TrimSpace(item.ContentKey), strings.TrimSpace(item.Text)
		if key == "" || text == "" {
			return enneagramTypeDetail{}, errors.New("条目标识和正文不能为空")
		}
		if _, duplicate := provided[key]; duplicate {
			return enneagramTypeDetail{}, fmt.Errorf("条目标识重复: %s", key)
		}
		provided[key] = text
	}
	matched := 0
	for _, dimension := range enneagramcatalog.RequiredDimensions {
		items := packageValue.Dimensions[dimension]
		for index := range items {
			dimensionsByKey[items[index].ContentKey] = dimension
			text, exists := provided[items[index].ContentKey]
			if !exists {
				continue
			}
			items[index].Text = text
			matched++
		}
		packageValue.Dimensions[dimension] = items
	}
	if matched != len(provided) {
		return enneagramTypeDetail{}, errors.New("草稿包含不属于当前型号的条目")
	}
	packageValue.Title = strings.TrimSpace(input.Title)
	packageValue.SourceChapter = strings.TrimSpace(input.SourceChapter)
	digest, err := enneagramcatalog.ContentDigest(packageValue)
	if err != nil {
		return enneagramTypeDetail{}, err
	}
	packageValue.ContentDigest = digest
	updatedPayload, err := json.Marshal(packageValue)
	if err != nil {
		return enneagramTypeDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE enneagram_catalog_imports
		SET title=$2,source_chapter=$3,content_digest=$4,payload=$5::jsonb,update_time=now()
		WHERE id=$1 AND status='draft'`, importID, packageValue.Title, packageValue.SourceChapter, digest, updatedPayload); err != nil {
		return enneagramTypeDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_libraries SET name=$2,updated_by=$3,update_time=now() WHERE id=$1`, libraryID, packageValue.Title, actorID); err != nil {
		return enneagramTypeDetail{}, err
	}
	for contentKey, text := range provided {
		hash := sha256.Sum256([]byte(text))
		canonicalName := packageValue.Title + " / " + dimensionsByKey[contentKey]
		result, err := tx.ExecContext(ctx, `
			UPDATE theory_cards card SET canonical_name=$3,summary=$4,definition=$4,core_claim=$4,updated_by=$5,update_time=now()
			FROM enneagram_catalog_import_items item
			WHERE item.import_id=$1 AND item.content_key=$2 AND card.id=item.card_id`, importID, contentKey, canonicalName, text, actorID)
		if err != nil {
			return enneagramTypeDetail{}, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return enneagramTypeDetail{}, fmt.Errorf("草稿条目不存在: %s", contentKey)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE theory_chunks chunk SET content=$3,content_hash=$4,token_count=$5,update_time=now()
			FROM enneagram_catalog_import_items item
			WHERE item.import_id=$1 AND item.content_key=$2 AND chunk.id=item.chunk_id`, importID, contentKey, text, hex.EncodeToString(hash[:]), utf8.RuneCountInString(text)); err != nil {
			return enneagramTypeDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE theory_practices practice SET goal=$3,steps=jsonb_build_array($3::text),update_time=now()
			FROM enneagram_catalog_import_items item
			WHERE item.import_id=$1 AND item.content_key=$2 AND practice.card_id=item.card_id`, importID, contentKey, text); err != nil {
			return enneagramTypeDetail{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return enneagramTypeDetail{}, err
	}
	return s.Detail(ctx, number)
}

func (s *enneagramLibraryAdminStore) SubmitReview(ctx context.Context, number int, actorID int64) error {
	importID, err := s.latestImportID(ctx, number, "draft")
	if err != nil {
		return err
	}
	return s.catalog.SubmitReview(ctx, importID, actorID)
}

func (s *enneagramLibraryAdminStore) Approve(ctx context.Context, number int, actorID int64, notes string) error {
	importID, err := s.latestImportID(ctx, number, "in_review")
	if err != nil {
		return err
	}
	return s.catalog.Approve(ctx, importID, actorID, notes)
}

func (s *enneagramLibraryAdminStore) Preview(ctx context.Context, number int, query string) (enneagramPreview, error) {
	detail, err := s.Detail(ctx, number)
	if err != nil {
		return enneagramPreview{}, err
	}
	query = strings.TrimSpace(query)
	preview := enneagramPreview{Type: number, Query: query, Hits: []enneagramPreviewHit{}}
	for _, item := range detail.Items {
		score := previewScore(query, item.Text, item.ContentKey)
		if query != "" && score == 0 {
			continue
		}
		preview.Hits = append(preview.Hits, enneagramPreviewHit{ContentKey: item.ContentKey, Dimension: item.Dimension, Text: item.Text, Score: score})
	}
	sort.SliceStable(preview.Hits, func(i, j int) bool { return preview.Hits[i].Score > preview.Hits[j].Score })
	if len(preview.Hits) > 8 {
		preview.Hits = preview.Hits[:8]
	}
	return preview, nil
}

func (s *enneagramLibraryAdminStore) Publish(ctx context.Context, number int, actorID int64) (enneagramPublishResult, error) {
	importID, err := s.latestImportID(ctx, number, "approved")
	if err != nil {
		return enneagramPublishResult{}, err
	}
	result, err := s.catalog.Publish(ctx, importID, actorID)
	if err != nil {
		return enneagramPublishResult{}, err
	}
	return publishResultView(result), nil
}

func (s *enneagramLibraryAdminStore) Versions(ctx context.Context, number int) ([]enneagramVersion, error) {
	if s == nil || s.db == nil || !validEnneagramType(number) {
		return nil, errors.New("人格型号无效")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT release.id,release.version,release.status,release.card_count,release.chunk_count,
			release.index_version,release.activated_at,release.create_time
		FROM theory_library_releases release
		JOIN theory_libraries library ON library.id=release.library_id
		WHERE library.key=$1 ORDER BY release.version DESC`, enneagramLibraryKey(number))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []enneagramVersion{}
	for rows.Next() {
		var version enneagramVersion
		if err := rows.Scan(&version.ReleaseID, &version.Version, &version.Status, &version.CardCount, &version.ChunkCount, &version.IndexVersion, &version.ActivatedAt, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *enneagramLibraryAdminStore) Rollback(ctx context.Context, number, targetVersion int, actorID int64) (enneagramPublishResult, error) {
	if !validEnneagramType(number) || targetVersion <= 0 || actorID <= 0 || actorID > int64(^uint(0)>>1) {
		return enneagramPublishResult{}, errors.New("回滚参数无效")
	}
	result, err := s.catalog.Rollback(ctx, enneagramLibraryKey(number), targetVersion, int(actorID))
	if err != nil {
		return enneagramPublishResult{}, err
	}
	return publishResultView(result), nil
}

func (s *enneagramLibraryAdminStore) latestImportID(ctx context.Context, number int, status string) (int64, error) {
	if s == nil || s.db == nil || !validEnneagramType(number) {
		return 0, errors.New("人格型号无效")
	}
	var importID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM enneagram_catalog_imports
		WHERE kind='enneagram_type' AND enneagram_type=$1 AND status=$2
		ORDER BY id DESC LIMIT 1`, number, status).Scan(&importID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("该型号没有%s状态的目录", status)
	}
	return importID, err
}

func publishResultView(result enneagramcatalog.PublishResult) enneagramPublishResult {
	return enneagramPublishResult{ImportID: result.ImportID, LibraryID: result.LibraryID, LibraryKey: result.LibraryKey, ReleaseID: result.ReleaseID, Version: result.Version}
}

func previewScore(query, text, key string) float64 {
	if query == "" {
		return 1
	}
	query = strings.ToLower(query)
	haystack := strings.ToLower(key + " " + text)
	if strings.Contains(haystack, query) {
		return 1
	}
	matched := 0
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(strings.Fields(query)))
}

func enneagramLibraryKey(number int) string { return fmt.Sprintf("enneagram-type-%02d", number) }
func validEnneagramType(number int) bool    { return number >= 1 && number <= 9 }

func parseEnneagramLibraryPath(path string) (number int, action string, ok bool) {
	if !strings.HasPrefix(path, enneagramLibraryPathPrefix) {
		return 0, "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, enneagramLibraryPathPrefix), "/"), "/")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, "", false
	}
	number, err := strconv.Atoi(parts[0])
	if err != nil || !validEnneagramType(number) {
		return 0, "", false
	}
	if len(parts) == 2 {
		action = parts[1]
	}
	return number, action, true
}

func registerEnneagramLibraryAdminRoutes(mux *http.ServeMux, permission func(string, http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/enneagram-library/types", permission("App:EnneagramLibrary:View", s.enneagramLibraryOverview))
	mux.HandleFunc(enneagramLibraryPathPrefix, func(w http.ResponseWriter, r *http.Request) {
		_, action, ok := parseEnneagramLibraryPath(r.URL.Path)
		if !ok {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
			return
		}
		code := "App:EnneagramLibrary:View"
		switch action {
		case "draft", "submit":
			code = "App:EnneagramLibrary:Edit"
		case "approve":
			code = "App:EnneagramLibrary:Review"
		case "publish", "rollback":
			code = "App:EnneagramLibrary:Publish"
		}
		permission(code, s.enneagramLibraryTypeAction)(w, r)
	})
}

func (s *Server) enneagramLibraryOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	items, err := s.enneagramAdmin.Overview(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, items)
}

func (s *Server) enneagramLibraryTypeAction(w http.ResponseWriter, r *http.Request) {
	number, action, ok := parseEnneagramLibraryPath(r.URL.Path)
	if !ok {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	actorID := userFromRequest(r).ID
	var result any
	var err error
	switch action {
	case "":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err = s.enneagramAdmin.Detail(r.Context(), number)
	case "draft":
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var input enneagramDraftInput
		if err = decodeAdminJSON(r, &input); err == nil {
			result, err = s.enneagramAdmin.SaveDraft(r.Context(), number, input, actorID)
		}
	case "submit":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		err = s.enneagramAdmin.SubmitReview(r.Context(), number, actorID)
		result = map[string]any{"type": number, "status": "in_review"}
	case "approve":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var input struct {
			Notes string `json:"notes"`
		}
		if err = decodeOptionalAdminJSON(r, &input); err == nil {
			err = s.enneagramAdmin.Approve(r.Context(), number, actorID, input.Notes)
		}
		result = map[string]any{"type": number, "status": "approved"}
	case "preview":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var input struct {
			Query string `json:"query"`
		}
		if err = decodeOptionalAdminJSON(r, &input); err == nil {
			result, err = s.enneagramAdmin.Preview(r.Context(), number, input.Query)
		}
	case "publish":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		result, err = s.enneagramAdmin.Publish(r.Context(), number, actorID)
	case "versions":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err = s.enneagramAdmin.Versions(r.Context(), number)
	case "rollback":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var input struct {
			Version int `json:"version"`
		}
		if err = decodeAdminJSON(r, &input); err == nil {
			result, err = s.enneagramAdmin.Rollback(r.Context(), number, input.Version, actorID)
		}
	default:
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if action != "" && action != "preview" && action != "versions" {
		s.recordAdminAudit(r, auditlog.Entry{Action: "enneagram_library." + action, TargetType: "enneagram_type", TargetID: strconv.Itoa(number), Summary: fmt.Sprintf("九型人格库 %d 号执行 %s", number, action)})
	}
	httpx.OK(w, result)
}

func decodeAdminJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, (2<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("请求 JSON 无效")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("请求 JSON 只能包含一个对象")
	}
	return nil
}

func decodeOptionalAdminJSON(r *http.Request, target any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, (2<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return errors.New("请求 JSON 无效")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("请求 JSON 只能包含一个对象")
	}
	return nil
}

func methodNotAllowed(w http.ResponseWriter) {
	httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
}
