package publicknowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

var (
	ErrValidation = errors.New("invalid source selection")
	ErrNotFound   = errors.New("source not found")
)

const MaxStatusIDs = 200
const PreviewLimit = 10

type Source struct {
	ID             string    `json:"id"`
	DatasetID      string    `json:"datasetId"`
	SourceKind     string    `json:"sourceKind"`
	SourceRecordID int64     `json:"sourceRecordId"`
	Title          string    `json:"title"`
	Category       string    `json:"category"`
	Categories     []string  `json:"categories"`
	FileFormat     string    `json:"fileFormat"`
	ExtractStatus  string    `json:"extractStatus"`
	SourceChunks   int       `json:"sourceChunks"`
	TextChars      int64     `json:"textChars"`
	Enabled        bool      `json:"enabled"`
	QualityStatus  string    `json:"qualityStatus"`
	ImportedChunks int       `json:"importedChunks"`
	CreateTime     time.Time `json:"createTime"`
	UpdateTime     time.Time `json:"updateTime"`
}

type Category struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ListResult struct {
	Items      []Source   `json:"items"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	Categories []Category `json:"categories"`
}

type Chunk struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	Locator       map[string]any `json:"locator"`
	ImportBatchID string         `json:"importBatchId"`
}

type PreviewResult struct {
	Items []Chunk `json:"items"`
	Total int     `json:"total"`
}

type ListParams struct {
	Keyword       string
	Category      string
	Enabled       *bool
	QualityStatus string
	DatasetID     string
	Page          int
	PageSize      int
}

func ParseListParams(values url.Values) (ListParams, error) {
	p := ListParams{Keyword: strings.TrimSpace(values.Get("keyword")), Category: strings.TrimSpace(values.Get("category")), QualityStatus: values.Get("qualityStatus"), DatasetID: strings.TrimSpace(values.Get("datasetId")), Page: 1, PageSize: 20}
	for name, dest := range map[string]*int{"page": &p.Page, "pageSize": &p.PageSize} {
		if value := values.Get(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return p, fmt.Errorf("%w: %s must be a positive integer", ErrValidation, name)
			}
			*dest = parsed
		}
	}
	if p.Page > 1000000 || p.PageSize > 100 || utf8.RuneCountInString(p.Keyword) > 200 || utf8.RuneCountInString(p.Category) > 200 || len(p.DatasetID) > 128 {
		return p, fmt.Errorf("%w: query exceeds bounds", ErrValidation)
	}
	if value := values.Get("enabled"); value != "" {
		if value != "true" && value != "false" {
			return p, fmt.Errorf("%w: enabled must be true or false", ErrValidation)
		}
		enabled := value == "true"
		p.Enabled = &enabled
	}
	if p.QualityStatus != "" && p.QualityStatus != "pending" && p.QualityStatus != "ready" && p.QualityStatus != "needs_review" {
		return p, fmt.Errorf("%w: invalid qualityStatus", ErrValidation)
	}
	return p, nil
}

func ValidateStatusIDs(ids []string) error {
	if len(ids) == 0 || len(ids) > MaxStatusIDs {
		return fmt.Errorf("%w: select 1 to %d sources", ErrValidation, MaxStatusIDs)
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 256 || id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n") || seen[id] {
			return fmt.Errorf("%w: invalid or duplicate source id", ErrValidation)
		}
		seen[id] = true
	}
	return nil
}

type Store struct{ db *sql.DB }

func NewStore(database *sql.DB) *Store { return &Store{db: database} }

func (s *Store) List(ctx context.Context, p ListParams) (ListResult, error) {
	result := ListResult{Items: []Source{}, Categories: []Category{}, Page: p.Page, PageSize: p.PageSize}
	where, args := " WHERE true", []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where += " AND " + fmt.Sprintf(clause, len(args))
	}
	if p.Keyword != "" {
		add("title ILIKE $%d ESCAPE '\\'", "%"+escapeLike(p.Keyword)+"%")
	}
	if p.Category != "" {
		args = append(args, p.Category)
		where += fmt.Sprintf(` AND (category = $%d OR EXISTS (
			SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(metadata->'categories') = 'array' THEN metadata->'categories' ELSE '[]'::jsonb END) c
			WHERE (CASE WHEN jsonb_typeof(c) = 'string' THEN c #>> '{}' ELSE c->>'name' END) = $%d))`, len(args), len(args))
	}
	if p.Enabled != nil {
		add("enabled = $%d", *p.Enabled)
	}
	if p.QualityStatus != "" {
		add("quality_status = $%d", p.QualityStatus)
	}
	if p.DatasetID != "" {
		add("dataset_id = $%d", p.DatasetID)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM public_knowledge_sources"+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, p.PageSize, (p.Page-1)*p.PageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT id,dataset_id,source_kind,source_record_id,title,category,file_format,extract_status,
		source_chunks,text_chars,enabled,quality_status,imported_chunks,metadata,create_time,update_time
		FROM public_knowledge_sources`+where+fmt.Sprintf(" ORDER BY category,title,id LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item Source
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.SourceKind, &item.SourceRecordID, &item.Title, &item.Category, &item.FileFormat, &item.ExtractStatus, &item.SourceChunks, &item.TextChars, &item.Enabled, &item.QualityStatus, &item.ImportedChunks, &metadata, &item.CreateTime, &item.UpdateTime); err != nil {
			rows.Close()
			return result, err
		}
		item.Categories = sourceCategories(item.Category, metadata)
		result.Items = append(result.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	categoryWhere, categoryArgs := "", []any{}
	if p.DatasetID != "" {
		categoryWhere, categoryArgs = " WHERE dataset_id = $1", []any{p.DatasetID}
	}
	rows, err = s.db.QueryContext(ctx, `SELECT names.name,count(*) FROM public_knowledge_sources s CROSS JOIN LATERAL (
		SELECT DISTINCT name FROM (
			SELECT s.category AS name UNION ALL
			SELECT CASE WHEN jsonb_typeof(c) = 'string' THEN c #>> '{}' ELSE c->>'name' END
			FROM jsonb_array_elements(CASE WHEN jsonb_typeof(s.metadata->'categories') = 'array' THEN s.metadata->'categories' ELSE '[]'::jsonb END) c
		) all_names WHERE name <> ''
	) names`+categoryWhere+` GROUP BY names.name ORDER BY names.name`, categoryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.Name, &category.Count); err != nil {
			return result, err
		}
		result.Categories = append(result.Categories, category)
	}
	return result, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(s)
}

func sourceCategories(primary string, raw []byte) []string {
	result := []string{}
	if primary != "" {
		result = append(result, primary)
	}
	var metadata struct {
		Categories []json.RawMessage `json:"categories"`
	}
	_ = json.Unmarshal(raw, &metadata)
	for _, value := range metadata.Categories {
		var category string
		if err := json.Unmarshal(value, &category); err != nil {
			var entry struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(value, &entry)
			category = entry.Name
		}
		if category == "" || len(category) > 600 {
			continue
		}
		duplicate := false
		for _, existing := range result {
			duplicate = duplicate || existing == category
		}
		if !duplicate {
			result = append(result, category)
		}
	}
	return result
}

func (s *Store) SetEnabled(ctx context.Context, ids []string, enabled bool) (int, error) {
	if err := ValidateStatusIDs(ids); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	placeholders, args := make([]string, len(ids)), make([]any, len(ids))
	for i, id := range ids {
		placeholders[i], args[i] = fmt.Sprintf("$%d", i+1), id
	}
	selection := strings.Join(placeholders, ",")
	rows, err := tx.QueryContext(ctx, "SELECT id FROM public_knowledge_sources WHERE id IN ("+selection+") ORDER BY id FOR UPDATE", args...)
	if err != nil {
		return 0, err
	}
	count := 0
	for rows.Next() {
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if count != len(ids) {
		return 0, ErrNotFound
	}
	args = append(args, enabled)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE public_knowledge_sources SET enabled=$%d,update_time=now() WHERE id IN (%s)", len(args), selection), args...); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) Preview(ctx context.Context, id string) (PreviewResult, error) {
	result := PreviewResult{Items: []Chunk{}}
	if err := ValidateStatusIDs([]string{id}); err != nil {
		return result, err
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM public_knowledge_sources WHERE id=$1)", id).Scan(&exists); err != nil {
		return result, err
	}
	if !exists {
		return result, ErrNotFound
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM knowledge_documents WHERE public_source_id=$1", id).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,left(content,2000),locator,COALESCE(import_batch_id,'')
		FROM knowledge_documents WHERE public_source_id=$1 ORDER BY id LIMIT $2`, id, PreviewLimit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Chunk
		var locator []byte
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &locator, &item.ImportBatchID); err != nil {
			return result, err
		}
		item.Locator = SafeLocator(locator)
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func SafeLocator(raw json.RawMessage) map[string]any {
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	result := map[string]any{}
	for _, key := range []string{"chapter", "chapter_title", "chapterTitle", "page", "page_start", "page_end", "pageStart", "pageEnd", "chunk_id", "chunkId", "source_chunk_id", "sourceChunkId", "original_chunk_id", "originalChunkId", "chunk_index", "chunkIndex", "part", "section", "section_title", "section_id", "sqlite_chunk_id", "segment_index"} {
		if value, exists := values[key]; exists {
			switch v := value.(type) {
			case string:
				if len(v) <= 600 {
					result[key] = v
				}
			case float64:
				result[key] = v
			}
		}
	}
	return result
}

// LexicalTokens matches the importer: adjacent Han bigrams and ASCII words/numbers.
func LexicalTokens(text string) []string {
	result, seen := []string{}, map[string]bool{}
	add := func(token string) {
		if token != "" && !seen[token] && len(result) < 64 {
			seen[token] = true
			result = append(result, token)
		}
	}
	var previous rune
	hanRun := 0
	var word strings.Builder
	flush := func() { add(strings.ToLower(word.String())); word.Reset() }
	flushHan := func() {
		if hanRun == 1 {
			add(string(previous))
		}
		previous, hanRun = 0, 0
	}
	count := 0
	for _, r := range text {
		count++
		if count > 1024 || len(result) >= 64 {
			break
		}
		if r >= '\u3400' && r <= '\u4dbf' || r >= '\u4e00' && r <= '\u9fff' {
			flush()
			if previous != 0 {
				add(string([]rune{previous, r}))
			}
			previous = r
			hanRun++
		} else {
			flushHan()
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				if word.Len() < 64 {
					word.WriteRune(r)
				}
			} else {
				flush()
			}
		}
	}
	flushHan()
	flush()
	return result
}

func (s *Store) Search(ctx context.Context, question string, topK int) ([]rag.Document, error) {
	result := []rag.Document{}
	tokens := LexicalTokens(question)
	if len(tokens) == 0 || topK <= 0 {
		return result, nil
	}
	if topK > 100 {
		topK = 100
	}
	for i, token := range tokens {
		tokens[i] = "'" + token + "'"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.title,d.content FROM knowledge_documents d
		WHERE d.library_kind='public' AND d.release_id IS NULL AND d.public_source_id IS NOT NULL AND d.safety_level <= 0
		AND EXISTS(SELECT 1 FROM public_knowledge_sources s WHERE s.id=d.public_source_id AND s.enabled)
		AND COALESCE(d.public_search_vector,to_tsvector('simple',d.search_text)) @@ to_tsquery('simple',$1)
		ORDER BY ts_rank(COALESCE(d.public_search_vector,to_tsvector('simple',d.search_text)),to_tsquery('simple',$1)) DESC,d.id LIMIT $2`, strings.Join(tokens, " | "), topK)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item rag.Document
		if err := rows.Scan(&item.ID, &item.Title, &item.Content); err != nil {
			return result, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
