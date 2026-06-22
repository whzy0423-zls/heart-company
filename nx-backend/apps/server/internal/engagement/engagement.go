package engagement

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

func (s *Store) Messages(ctx context.Context, values url.Values) (PageResult[Message], error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	where := []string{"1=1"}
	args := []any{}
	if typ := strings.TrimSpace(values.Get("type")); typ != "" {
		args = append(args, typ)
		where = append(where, "type=$"+strconv.Itoa(len(args)))
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
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM messages WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Message]{}, err
	}
	page, pageSize := pageParams(values)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		`SELECT id, type, title, content, business_id, business_type, target_path, is_read, create_time
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
		if err := rows.Scan(&id, &item.Type, &item.Title, &item.Content, &item.BusinessID, &item.BusinessType, &item.TargetPath, &item.IsRead, &createTime); err != nil {
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
		_, err := s.db.ExecContext(c, `UPDATE messages SET is_read=$1`, read)
		return err
	}
	args := []any{read}
	placeholders := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		args = append(args, id)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	if len(placeholders) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(c, `UPDATE messages SET is_read=$1 WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
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
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
