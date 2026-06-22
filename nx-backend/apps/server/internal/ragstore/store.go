// Package ragstore manages manually curated knowledge used by miniapp RAG chat.
package ragstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	SourceManual   = "manual"
	queryTimeout   = 10 * time.Second
)

type Store struct {
	db *sql.DB
}

type Document struct {
	CreateTime string   `json:"createTime"`
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Status     string   `json:"status"`
	Tags       []string `json:"tags"`
	Title      string   `json:"title"`
	UpdateTime string   `json:"updateTime"`
	Content    string   `json:"content"`
	Sort       int      `json:"sort"`
}

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, queryTimeout)
}

func NormalizeDocument(input Document) (Document, error) {
	doc := input
	doc.Title = truncateRunes(strings.TrimSpace(doc.Title), 120)
	doc.Content = truncateRunes(strings.TrimSpace(doc.Content), 8000)
	doc.Source = strings.TrimSpace(doc.Source)
	doc.Status = strings.TrimSpace(doc.Status)
	doc.Tags = normalizeTags(doc.Tags)

	if doc.Title == "" {
		return Document{}, errors.New("请输入知识标题")
	}
	if doc.Content == "" {
		return Document{}, errors.New("请输入知识内容")
	}
	if doc.Status == "" {
		doc.Status = StatusEnabled
	}
	if doc.Status != StatusEnabled && doc.Status != StatusDisabled {
		return Document{}, errors.New("知识状态只能是 enabled 或 disabled")
	}
	if doc.Source == "" {
		doc.Source = SourceManual
	}
	return doc, nil
}

func ToRAGDocuments(items []Document) []rag.Document {
	docs := make([]rag.Document, 0, len(items))
	for _, item := range items {
		doc, err := NormalizeDocument(item)
		if err != nil || doc.Status != StatusEnabled || strings.TrimSpace(doc.ID) == "" {
			continue
		}
		docs = append(docs, rag.Document{
			ID:      "kb-" + doc.ID,
			Title:   doc.Title,
			Content: doc.Content,
			Tags:    doc.Tags,
		})
	}
	return docs
}

func (s *Store) EnabledDocuments(ctx context.Context) ([]rag.Document, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()

	rows, err := s.db.QueryContext(c,
		`SELECT id::text, title, content, tags, status, source, sort, create_time, update_time
		   FROM rag_documents
		  WHERE status=$1
		  ORDER BY sort ASC, update_time DESC, id DESC
		  LIMIT 200`,
		StatusEnabled,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	return ToRAGDocuments(items), nil
}

func (s *Store) ListDocuments(ctx context.Context, query map[string]string) (PageResult[Document], error) {
	if s == nil || s.db == nil {
		return PageResult[Document]{Items: []Document{}}, nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query["keyword"]); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR content ILIKE $%d)", len(args), len(args)))
	}
	if status := strings.TrimSpace(query["status"]); status != "" && status != "all" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM rag_documents WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Document]{}, err
	}

	page, pageSize := pageParams(query)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(c,
		`SELECT id::text, title, content, tags, status, source, sort, create_time, update_time
		   FROM rag_documents
		  WHERE `+condition+`
		  ORDER BY sort ASC, update_time DESC, id DESC
		  LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Document]{}, err
	}
	defer rows.Close()

	items, err := scanDocuments(rows)
	if err != nil {
		return PageResult[Document]{}, err
	}
	return PageResult[Document]{Items: items, Total: total}, nil
}

func (s *Store) SaveDocument(ctx context.Context, input Document) (Document, error) {
	if s == nil || s.db == nil {
		return Document{}, errors.New("knowledge database is not configured")
	}
	doc, err := NormalizeDocument(input)
	if err != nil {
		return Document{}, err
	}
	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return Document{}, err
	}

	c, cancel := s.ctx(ctx)
	defer cancel()
	if strings.TrimSpace(doc.ID) == "" {
		return scanDocument(s.db.QueryRowContext(c,
			`INSERT INTO rag_documents (title, content, tags, status, source, sort)
			 VALUES ($1,$2,$3::jsonb,$4,$5,$6)
			 RETURNING id::text, title, content, tags, status, source, sort, create_time, update_time`,
			doc.Title, doc.Content, string(tagsJSON), doc.Status, doc.Source, doc.Sort,
		))
	}

	if _, err := strconv.ParseInt(doc.ID, 10, 64); err != nil {
		return Document{}, errors.New("invalid knowledge id")
	}
	return scanDocument(s.db.QueryRowContext(c,
		`UPDATE rag_documents
		    SET title=$1, content=$2, tags=$3::jsonb, status=$4, source=$5, sort=$6, update_time=now()
		  WHERE id=$7
		  RETURNING id::text, title, content, tags, status, source, sort, create_time, update_time`,
		doc.Title, doc.Content, string(tagsJSON), doc.Status, doc.Source, doc.Sort, doc.ID,
	))
}

func (s *Store) DeleteDocument(ctx context.Context, id string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err != nil {
		return false, errors.New("invalid knowledge id")
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM rag_documents WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanDocuments(rows *sql.Rows) ([]Document, error) {
	items := []Document{}
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, doc)
	}
	return items, rows.Err()
}

type documentScanner interface {
	Scan(dest ...any) error
}

func scanDocument(scanner documentScanner) (Document, error) {
	var doc Document
	var tagsRaw []byte
	var createTime, updateTime time.Time
	err := scanner.Scan(
		&doc.ID,
		&doc.Title,
		&doc.Content,
		&tagsRaw,
		&doc.Status,
		&doc.Source,
		&doc.Sort,
		&createTime,
		&updateTime,
	)
	if err != nil {
		return Document{}, err
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &doc.Tags)
	}
	doc.CreateTime = formatTime(createTime)
	doc.UpdateTime = formatTime(updateTime)
	return doc, nil
}

func pageParams(query map[string]string) (int, int) {
	page, _ := strconv.Atoi(query["page"])
	pageSize, _ := strconv.Atoi(query["pageSize"])
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func normalizeTags(tags []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = truncateRunes(strings.TrimSpace(tag), 40)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
		if len(result) >= 12 {
			break
		}
	}
	return result
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func truncateRunes(text string, max int) string {
	if max <= 0 || utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max])
}
