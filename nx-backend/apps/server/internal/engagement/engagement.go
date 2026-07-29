package engagement

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/realip"
)

type Store struct {
	db *sql.DB
}

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type Message struct {
	BusinessID   string `json:"businessId"`
	BusinessType string `json:"businessType"`
	Content      string `json:"content"`
	CreateTime   string `json:"createTime"`
	ID           string `json:"id"`
	IsRead       bool   `json:"isRead"`
	TargetPath   string `json:"targetPath"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	Platform     string `json:"platform"`
	EventKey     string `json:"eventKey"`
}

type UnreadSummary struct {
	Count       int                 `json:"count"`
	LatestID    string              `json:"latestId"`
	Items       []UnreadSummaryItem `json:"items"`
	HasMore     bool                `json:"hasMore"`
	NextAfterID string              `json:"nextAfterId"`
}

type UnreadSummaryItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Platform     string `json:"platform"`
	EventKey     string `json:"eventKey"`
	BusinessType string `json:"businessType"`
	TargetPath   string `json:"targetPath"`
	CreateTime   string `json:"createTime"`
}

type GameResultInput struct {
	Centers    any            `json:"centers"`
	Gender     string         `json:"gender"`
	ResultType int            `json:"resultType"`
	Score      map[string]any `json:"score"`
	SecondType int            `json:"secondType"`
	VisitorID  string         `json:"visitorId"`
}

type GameResult struct {
	CreateTime string `json:"createTime"`
	Gender     string `json:"gender"`
	ID         string `json:"id"`
	ResultType int    `json:"resultType"`
	SecondType int    `json:"secondType"`
	VisitorID  string `json:"visitorId"`
}

type GameOverview struct {
	CenterItems     []NameValue      `json:"centerItems"`
	GenderItems     []NameValue      `json:"genderItems"`
	Total           int              `json:"total"`
	TypeGenderItems []TypeGenderItem `json:"typeGenderItems"`
	TypeItems       []NameValue      `json:"typeItems"`
}

type NameValue struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type TypeGenderItem struct {
	Female  int    `json:"female"`
	Male    int    `json:"male"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Unknown int    `json:"unknown"`
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func buildMessageWhere(values url.Values) (string, []any, error) {
	where := []string{"1=1"}
	args := []any{}
	if typ := strings.TrimSpace(values.Get("type")); typ != "" {
		args = append(args, typ)
		where = append(where, "type=$"+strconv.Itoa(len(args)))
	}
	if businessType := strings.TrimSpace(values.Get("businessType")); businessType != "" {
		args = append(args, businessType)
		where = append(where, "business_type=$"+strconv.Itoa(len(args)))
	}
	if platform := strings.TrimSpace(values.Get("platform")); platform != "" {
		if platform != "website" && platform != "miniapp" && platform != "system" {
			return "", nil, fmt.Errorf("invalid platform")
		}
		args = append(args, platform)
		where = append(where, "platform=$"+strconv.Itoa(len(args)))
	}
	if read := strings.TrimSpace(values.Get("read")); read != "" {
		args = append(args, read == "true" || read == "1")
		where = append(where, "is_read=$"+strconv.Itoa(len(args)))
	}
	if keyword := strings.TrimSpace(values.Get("keyword")); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		index := strconv.Itoa(len(args))
		where = append(where, "(lower(title) LIKE $"+index+" OR lower(content) LIKE $"+index+")")
	}
	return strings.Join(where, " AND "), args, nil
}

func (s *Store) Messages(ctx context.Context, values url.Values) (PageResult[Message], error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cond, args, err := buildMessageWhere(values)
	if err != nil {
		return PageResult[Message]{}, err
	}
	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM messages WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Message]{}, err
	}
	page, pageSize := pageParams(values)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		`SELECT id, type, title, content, business_id, business_type, target_path, is_read, create_time, platform, event_key
		 FROM messages WHERE `+cond+`
		 ORDER BY create_time DESC, id DESC
		 LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Message]{}, err
	}
	defer rows.Close()
	items := []Message{}
	for rows.Next() {
		var item Message
		var id int64
		var createTime time.Time
		if err := rows.Scan(&id, &item.Type, &item.Title, &item.Content, &item.BusinessID, &item.BusinessType, &item.TargetPath, &item.IsRead, &createTime, &item.Platform, &item.EventKey); err != nil {
			return PageResult[Message]{}, err
		}
		item.ID = strconv.FormatInt(id, 10)
		item.CreateTime = formatTime(createTime)
		items = append(items, item)
	}
	return PageResult[Message]{Items: items, Total: total}, rows.Err()
}

func (s *Store) MarkMessages(ctx context.Context, ids []string, read bool) error {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if len(ids) == 0 {
		if !read {
			return fmt.Errorf("empty ids can only mark all read")
		}
		_, err := s.db.ExecContext(c, `UPDATE messages SET is_read=true WHERE is_read=false`)
		return err
	}
	args := []any{read}
	placeholders := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		parsed, err := parsePositiveDecimalID(id)
		if err != nil {
			return err
		}
		args = append(args, parsed)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	if len(placeholders) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(c, `UPDATE messages SET is_read=$1 WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

func (s *Store) UnreadSummary(ctx context.Context, afterID int64, afterText string, limit int) (UnreadSummary, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := UnreadSummary{Items: []UnreadSummaryItem{}, NextAfterID: afterText}
	if err := s.db.QueryRowContext(c, `SELECT count(*) FILTER (WHERE is_read=false), COALESCE(max(id), 0) FROM messages`).Scan(&result.Count, &result.LatestID); err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(c, `SELECT id,title,content,platform,event_key,business_type,target_path,create_time FROM messages WHERE is_read=false AND id>$1 ORDER BY id ASC LIMIT $2`, afterID, limit+1)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item UnreadSummaryItem
		var id int64
		var content string
		var createTime time.Time
		if err := rows.Scan(&id, &item.Title, &content, &item.Platform, &item.EventKey, &item.BusinessType, &item.TargetPath, &createTime); err != nil {
			return result, err
		}
		item.ID = strconv.FormatInt(id, 10)
		item.Summary = messageSummary(content)
		item.CreateTime = formatTime(createTime)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > limit {
		result.HasMore = true
		result.Items = result.Items[:limit]
	}
	if len(result.Items) > 0 {
		result.NextAfterID = result.Items[len(result.Items)-1].ID
	}
	return result, nil
}

func messageSummary(content string) string {
	content = strings.TrimSpace(content)
	sensitive := regexp.MustCompile(`(?i)(openid|unionid)(["']?\s*[:=]\s*["']?)[^,\s\}"']+`)
	content = sensitive.ReplaceAllString(content, "$1$2***")
	phone := regexp.MustCompile(`(手机号|手机|电话)([：:=]?\s*)[0-9+()\- ]{6,}`)
	content = phone.ReplaceAllString(content, "$1$2***")
	content = strings.Join(strings.Fields(content), " ")
	runes := []rune(content)
	if len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return content
}

func parsePositiveDecimalID(value string) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("invalid message id")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid message id")
		}
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid message id")
	}
	return id, nil
}

func ParsePositiveMessageID(value string) (int64, error) {
	return parsePositiveDecimalID(strings.TrimSpace(value))
}

func (s *Store) TrackGameResult(ctx context.Context, input GameResultInput, r *http.Request) (GameResult, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	score, _ := json.Marshal(input.Score)
	centers, _ := json.Marshal(input.Centers)
	if string(score) == "null" {
		score = []byte("{}")
	}
	if string(centers) == "null" {
		centers = []byte("[]")
	}
	var result GameResult
	var createTime time.Time
	err := s.db.QueryRowContext(c,
		`INSERT INTO game_results (visitor_id, gender, result_type, second_type, score, centers, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8)
		 RETURNING id::text, visitor_id, gender, result_type, second_type, create_time`,
		truncate(input.VisitorID, 128),
		truncate(input.Gender, 32),
		input.ResultType,
		input.SecondType,
		string(score),
		string(centers),
		clientIP(r),
		truncate(r.UserAgent(), 512),
	).Scan(&result.ID, &result.VisitorID, &result.Gender, &result.ResultType, &result.SecondType, &createTime)
	result.CreateTime = formatTime(createTime)
	return result, err
}

func (s *Store) GameOverview(ctx context.Context) (GameOverview, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var result GameOverview
	if err := s.db.QueryRowContext(c, `SELECT count(*) FROM game_results`).Scan(&result.Total); err != nil {
		return result, err
	}
	typeItems, err := queryNameValues(c, s.db, `SELECT result_type::text, count(*) FROM game_results GROUP BY result_type ORDER BY result_type`)
	if err != nil {
		return result, err
	}
	result.TypeItems = typeItems
	genderItems, err := queryNameValues(c, s.db, `
		SELECT COALESCE(NULLIF(gender, ''), 'unknown'), count(*)
		FROM game_results
		GROUP BY COALESCE(NULLIF(gender, ''), 'unknown')
		ORDER BY count(*) DESC`)
	if err != nil {
		return result, err
	}
	result.GenderItems = genderItems
	typeGenderItems, err := queryTypeGenderItems(c, s.db)
	if err != nil {
		return result, err
	}
	result.TypeGenderItems = typeGenderItems
	centerItems, err := queryNameValues(c, s.db, `
		WITH center_items AS (
			SELECT COALESCE(
				NULLIF(item->>'name', ''),
				CASE item->>'key'
					WHEN 'gut' THEN '本能中心'
					WHEN 'heart' THEN '情感中心'
					WHEN 'head' THEN '思维中心'
				END
			) AS name
			FROM game_results, jsonb_array_elements(centers) item
		)
		SELECT name, count(*)
		FROM center_items
		WHERE name IS NOT NULL
		GROUP BY name
		ORDER BY count(*) DESC`)
	if err != nil {
		return result, err
	}
	result.CenterItems = centerItems
	return result, nil
}

func queryTypeGenderItems(ctx context.Context, db *sql.DB) ([]TypeGenderItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT result_type::text,
		       count(*),
		       count(*) FILTER (WHERE gender = 'male'),
		       count(*) FILTER (WHERE gender = 'female'),
		       count(*) FILTER (WHERE gender IS NULL OR gender = '' OR gender NOT IN ('male', 'female'))
		FROM game_results
		GROUP BY result_type
		ORDER BY result_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TypeGenderItem{}
	for rows.Next() {
		var item TypeGenderItem
		if err := rows.Scan(&item.Name, &item.Total, &item.Male, &item.Female, &item.Unknown); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func queryNameValues(ctx context.Context, db *sql.DB, query string) ([]NameValue, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NameValue{}
	for rows.Next() {
		var item NameValue
		if err := rows.Scan(&item.Name, &item.Value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func pageParams(values url.Values) (int, int) {
	page, _ := strconv.Atoi(values.Get("page"))
	pageSize, _ := strconv.Atoi(values.Get("pageSize"))
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func clientIP(r *http.Request) string {
	return realip.RemoteAddr(r)
}
