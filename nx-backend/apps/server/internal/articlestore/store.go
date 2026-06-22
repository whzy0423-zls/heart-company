// Package articlestore manages reading articles edited in the admin and
// rendered on the reading H5. Article bodies are stored as Markdown text.
package articlestore

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
)

const (
	StatusPublished = "published"
	StatusDraft     = "draft"
	queryTimeout    = 10 * time.Second
)

// Article is the full record used by the admin CRUD endpoints.
type Article struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Cover       string   `json:"cover"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	Sort        int      `json:"sort"`
	ViewCount   int64    `json:"viewCount"`
	VoiceKey    string   `json:"voiceKey"`
	AudioURL    string   `json:"audioUrl"`
	AudioStatus string   `json:"audioStatus"`
	AudioError  string   `json:"audioError"`
	PublishTime string   `json:"publishTime"`
	CreateTime  string   `json:"createTime"`
	UpdateTime  string   `json:"updateTime"`
}

// PublicArticle is the trimmed shape served to the H5 list (no full content).
type PublicArticle struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Cover       string   `json:"cover"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	ViewCount   int64    `json:"viewCount"`
	HasAudio    bool     `json:"hasAudio"`
	PublishTime string   `json:"publishTime"`
}

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type Store struct {
	db         *sql.DB
	tts        TTSClient
	assets     AssetCreator
	voices     VoiceResolver
	audioModel string
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database, audioModel: "speech-02-hd"}
}

func (s *Store) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, queryTimeout)
}

// NormalizeArticle trims and bounds fields before persistence.
func NormalizeArticle(input Article) (Article, error) {
	doc := input
	doc.Title = truncateRunes(strings.TrimSpace(doc.Title), 200)
	doc.Summary = truncateRunes(strings.TrimSpace(doc.Summary), 500)
	doc.Cover = strings.TrimSpace(doc.Cover)
	doc.Author = truncateRunes(strings.TrimSpace(doc.Author), 80)
	doc.Category = truncateRunes(strings.TrimSpace(doc.Category), 60)
	doc.Content = strings.TrimSpace(doc.Content)
	doc.VoiceKey = strings.TrimSpace(doc.VoiceKey)
	doc.Tags = normalizeTags(doc.Tags)

	if doc.Title == "" {
		return Article{}, errors.New("文章标题不能为空")
	}
	if doc.Content == "" {
		return Article{}, errors.New("文章正文不能为空")
	}
	if utf8.RuneCountInString(doc.Content) > 50_000 {
		return Article{}, errors.New("正文太长，请控制在 5 万字以内")
	}
	if doc.Status != StatusDraft {
		doc.Status = StatusPublished
	}
	return doc, nil
}

const adminColumns = `id::text, title, summary, cover, author, category, content, tags, status, sort, view_count, voice_key, audio_url, audio_status, audio_error, publish_time, create_time, update_time`

// ListArticles powers the admin table with keyword/status/category filters.
func (s *Store) ListArticles(ctx context.Context, query map[string]string) (PageResult[Article], error) {
	if s == nil || s.db == nil {
		return PageResult[Article]{Items: []Article{}}, nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query["keyword"]); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR summary ILIKE $%d OR content ILIKE $%d)", len(args), len(args), len(args)))
	}
	if status := strings.TrimSpace(query["status"]); status != "" && status != "all" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if category := strings.TrimSpace(query["category"]); category != "" {
		args = append(args, category)
		where = append(where, fmt.Sprintf("category=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM articles WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Article]{}, err
	}

	page, pageSize := pageParams(query)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(c,
		`SELECT `+adminColumns+`
		   FROM articles
		  WHERE `+condition+`
		  ORDER BY sort ASC, publish_time DESC, id DESC
		  LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Article]{}, err
	}
	defer rows.Close()

	items := []Article{}
	for rows.Next() {
		doc, err := scanArticle(rows)
		if err != nil {
			return PageResult[Article]{}, err
		}
		items = append(items, doc)
	}
	return PageResult[Article]{Items: items, Total: total}, rows.Err()
}

// GetArticle returns the full record by id (admin).
func (s *Store) GetArticle(ctx context.Context, id string) (Article, bool, error) {
	if s == nil || s.db == nil {
		return Article{}, false, nil
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err != nil {
		return Article{}, false, errors.New("invalid article id")
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	doc, err := scanArticle(s.db.QueryRowContext(c,
		`SELECT `+adminColumns+` FROM articles WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Article{}, false, nil
	}
	if err != nil {
		return Article{}, false, err
	}
	return doc, true, nil
}

func (s *Store) SaveArticle(ctx context.Context, input Article) (Article, error) {
	if s == nil || s.db == nil {
		return Article{}, errors.New("article database is not configured")
	}
	doc, err := NormalizeArticle(input)
	if err != nil {
		return Article{}, err
	}
	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return Article{}, err
	}

	c, cancel := s.ctx(ctx)
	defer cancel()
	if strings.TrimSpace(doc.ID) == "" {
		return scanArticle(s.db.QueryRowContext(c,
			`INSERT INTO articles (title, summary, cover, author, category, content, tags, status, sort, voice_key)
			 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10)
			 RETURNING `+adminColumns,
			doc.Title, doc.Summary, doc.Cover, doc.Author, doc.Category, doc.Content, string(tagsJSON), doc.Status, doc.Sort, doc.VoiceKey,
		))
	}

	if _, err := strconv.ParseInt(doc.ID, 10, 64); err != nil {
		return Article{}, errors.New("invalid article id")
	}
	// 正文或音色变化时，作废已缓存的听书音频，提示需要重新生成。
	return scanArticle(s.db.QueryRowContext(c,
		`UPDATE articles
		    SET title=$1, summary=$2, cover=$3, author=$4, category=$5, content=$6,
		        tags=$7::jsonb, status=$8, sort=$9, voice_key=$10, update_time=now(),
		        audio_status = CASE WHEN content <> $6 OR voice_key <> $10 THEN 'none' ELSE audio_status END,
		        audio_url    = CASE WHEN content <> $6 OR voice_key <> $10 THEN '' ELSE audio_url END,
		        audio_asset_id = CASE WHEN content <> $6 OR voice_key <> $10 THEN NULL ELSE audio_asset_id END,
		        audio_error  = CASE WHEN content <> $6 OR voice_key <> $10 THEN '' ELSE audio_error END
		  WHERE id=$11
		  RETURNING `+adminColumns,
		doc.Title, doc.Summary, doc.Cover, doc.Author, doc.Category, doc.Content, string(tagsJSON), doc.Status, doc.Sort, doc.VoiceKey, doc.ID,
	))
}

func (s *Store) DeleteArticle(ctx context.Context, id string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err != nil {
		return false, errors.New("invalid article id")
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM articles WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PublicList serves published articles to the H5 list (no full content).
func (s *Store) PublicList(ctx context.Context, query map[string]string) (PageResult[PublicArticle], error) {
	if s == nil || s.db == nil {
		return PageResult[PublicArticle]{Items: []PublicArticle{}}, nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"status=$1"}
	args := []any{StatusPublished}
	if keyword := strings.TrimSpace(query["keyword"]); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR summary ILIKE $%d)", len(args), len(args)))
	}
	if category := strings.TrimSpace(query["category"]); category != "" {
		args = append(args, category)
		where = append(where, fmt.Sprintf("category=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM articles WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[PublicArticle]{}, err
	}

	page, pageSize := pageParams(query)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(c,
		`SELECT id::text, title, summary, cover, author, category, tags, view_count,
		        (audio_status='ready' AND audio_url <> '') AS has_audio, publish_time
		   FROM articles
		  WHERE `+condition+`
		  ORDER BY sort ASC, publish_time DESC, id DESC
		  LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[PublicArticle]{}, err
	}
	defer rows.Close()

	items := []PublicArticle{}
	for rows.Next() {
		var doc PublicArticle
		var tagsRaw []byte
		var publishTime time.Time
		if err := rows.Scan(&doc.ID, &doc.Title, &doc.Summary, &doc.Cover, &doc.Author,
			&doc.Category, &tagsRaw, &doc.ViewCount, &doc.HasAudio, &publishTime); err != nil {
			return PageResult[PublicArticle]{}, err
		}
		if len(tagsRaw) > 0 {
			_ = json.Unmarshal(tagsRaw, &doc.Tags)
		}
		doc.PublishTime = formatTime(publishTime)
		items = append(items, doc)
	}
	return PageResult[PublicArticle]{Items: items, Total: total}, rows.Err()
}

// PublicDetail returns one published article with full content and bumps view_count.
func (s *Store) PublicDetail(ctx context.Context, id string) (Article, bool, error) {
	if s == nil || s.db == nil {
		return Article{}, false, nil
	}
	if _, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err != nil {
		return Article{}, false, errors.New("invalid article id")
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	doc, err := scanArticle(s.db.QueryRowContext(c,
		`UPDATE articles SET view_count = view_count + 1
		  WHERE id=$1 AND status=$2
		  RETURNING `+adminColumns, id, StatusPublished))
	if errors.Is(err, sql.ErrNoRows) {
		return Article{}, false, nil
	}
	if err != nil {
		return Article{}, false, err
	}
	return doc, true, nil
}

// Categories lists distinct non-empty categories among published articles.
func (s *Store) Categories(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return []string{}, nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT DISTINCT category FROM articles
		  WHERE status=$1 AND category <> ''
		  ORDER BY category`, StatusPublished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := []string{}
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

type articleScanner interface {
	Scan(dest ...any) error
}

func scanArticle(scanner articleScanner) (Article, error) {
	var doc Article
	var tagsRaw []byte
	var publishTime, createTime, updateTime time.Time
	err := scanner.Scan(
		&doc.ID, &doc.Title, &doc.Summary, &doc.Cover, &doc.Author, &doc.Category,
		&doc.Content, &tagsRaw, &doc.Status, &doc.Sort, &doc.ViewCount,
		&doc.VoiceKey, &doc.AudioURL, &doc.AudioStatus, &doc.AudioError,
		&publishTime, &createTime, &updateTime,
	)
	if err != nil {
		return Article{}, err
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &doc.Tags)
	}
	doc.PublishTime = formatTime(publishTime)
	doc.CreateTime = formatTime(createTime)
	doc.UpdateTime = formatTime(updateTime)
	if doc.Tags == nil {
		doc.Tags = []string{}
	}
	if doc.AudioStatus == "" {
		doc.AudioStatus = "none"
	}
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
