// Package mindquote 管理「成长心语」：分组(mind_groups) + 心语(mind_quotes)。
// 后台 CRUD 编辑，官网公开只读展示。每条心语含简短文案(title)与完整原文(content)。
package mindquote

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

const queryTimeout = 10 * time.Second

// Store 基于 PostgreSQL 的心语存储。
type Store struct {
	db *sql.DB
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
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

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// ---------------- 分组 ----------------

// Group 是后台分组记录（含该组心语数量）。
type Group struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Intro      string `json:"intro"`
	Sort       int    `json:"sort"`
	Status     string `json:"status"`
	QuoteCount int    `json:"quoteCount"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

// ListGroups 返回全部分组（按 sort），含每组心语数量。
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT g.id, g.name, g.intro, g.sort, g.status, g.create_time, g.update_time,
		        (SELECT count(*) FROM mind_quotes q WHERE q.group_id = g.id) AS quote_count
		 FROM mind_groups g
		 ORDER BY g.sort ASC, g.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Group{}
	for rows.Next() {
		var g Group
		var id int64
		var ct, ut time.Time
		if err := rows.Scan(&id, &g.Name, &g.Intro, &g.Sort, &g.Status, &ct, &ut, &g.QuoteCount); err != nil {
			return nil, err
		}
		g.ID = strconv.FormatInt(id, 10)
		g.CreateTime = formatTime(ct)
		g.UpdateTime = formatTime(ut)
		items = append(items, g)
	}
	return items, rows.Err()
}

// SaveGroup 新增或更新分组（ID 为空则新增）。
func (s *Store) SaveGroup(ctx context.Context, input Group) (Group, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Group{}, errors.New("分组名称不能为空")
	}
	status := input.Status
	if status == "" {
		status = "enabled"
	}

	if input.ID == "" {
		var id int64
		var ct, ut time.Time
		if err := s.db.QueryRowContext(c,
			`INSERT INTO mind_groups (name, intro, sort, status)
			 VALUES ($1,$2,$3,$4) RETURNING id, create_time, update_time`,
			name, input.Intro, input.Sort, status,
		).Scan(&id, &ct, &ut); err != nil {
			return Group{}, err
		}
		input.ID = strconv.FormatInt(id, 10)
		input.Name = name
		input.Status = status
		input.CreateTime = formatTime(ct)
		input.UpdateTime = formatTime(ut)
		return input, nil
	}

	gid, err := strconv.ParseInt(input.ID, 10, 64)
	if err != nil {
		return Group{}, errors.New("invalid group id")
	}
	var ut time.Time
	if err := s.db.QueryRowContext(c,
		`UPDATE mind_groups SET name=$1, intro=$2, sort=$3, status=$4, update_time=now()
		 WHERE id=$5 RETURNING update_time`,
		name, input.Intro, input.Sort, status, gid,
	).Scan(&ut); err != nil {
		return Group{}, err
	}
	input.Name = name
	input.Status = status
	input.UpdateTime = formatTime(ut)
	return input, nil
}

// DeleteGroup 删除分组；其下心语的 group_id 由外键 ON DELETE SET NULL 自动置空。
func (s *Store) DeleteGroup(ctx context.Context, id string) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM mind_groups WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------------- 心语 ----------------

// Quote 是后台心语记录。
type Quote struct {
	ID         string `json:"id"`
	GroupID    string `json:"groupId"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Prompt     string `json:"prompt"`
	Sort       int    `json:"sort"`
	Status     string `json:"status"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

func groupIDValue(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return id
}

func scanQuote(rows interface {
	Scan(dest ...any) error
}) (Quote, error) {
	var q Quote
	var id int64
	var gid sql.NullInt64
	var ct, ut time.Time
	if err := rows.Scan(&id, &gid, &q.Title, &q.Content, &q.Prompt, &q.Sort, &q.Status, &ct, &ut); err != nil {
		return Quote{}, err
	}
	q.ID = strconv.FormatInt(id, 10)
	if gid.Valid {
		q.GroupID = strconv.FormatInt(gid.Int64, 10)
	}
	q.CreateTime = formatTime(ct)
	q.UpdateTime = formatTime(ut)
	return q, nil
}

const quoteColumns = `id, group_id, title, content, prompt, sort, status, create_time, update_time`

// ListQuotes 分页返回心语（可按 groupId、keyword 过滤）。
func (s *Store) ListQuotes(ctx context.Context, query map[string]string) (PageResult[Quote], error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if gid := strings.TrimSpace(query["groupId"]); gid != "" {
		if gid == "0" || gid == "none" {
			where = append(where, "group_id IS NULL")
		} else {
			args = append(args, gid)
			where = append(where, "group_id = $"+strconv.Itoa(len(args)))
		}
	}
	if kw := strings.TrimSpace(query["keyword"]); kw != "" {
		args = append(args, "%"+kw+"%")
		where = append(where, "(title ILIKE $"+strconv.Itoa(len(args))+" OR content ILIKE $"+strconv.Itoa(len(args))+")")
	}
	if st := strings.TrimSpace(query["status"]); st != "" {
		args = append(args, st)
		where = append(where, "status = $"+strconv.Itoa(len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM mind_quotes WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Quote]{}, err
	}

	page, pageSize := pageParams(query)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(c,
		"SELECT "+quoteColumns+" FROM mind_quotes WHERE "+cond+
			" ORDER BY sort ASC, id ASC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return PageResult[Quote]{}, err
	}
	defer rows.Close()
	items := []Quote{}
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return PageResult[Quote]{}, err
		}
		items = append(items, q)
	}
	return PageResult[Quote]{Items: items, Total: total}, rows.Err()
}

// GetQuote 取单条心语。
func (s *Store) GetQuote(ctx context.Context, id string) (Quote, bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	row := s.db.QueryRowContext(c, "SELECT "+quoteColumns+" FROM mind_quotes WHERE id=$1", id)
	q, err := scanQuote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Quote{}, false, nil
		}
		return Quote{}, false, err
	}
	return q, true, nil
}

// SaveQuote 新增或更新心语（ID 为空则新增）。
func (s *Store) SaveQuote(ctx context.Context, input Quote) (Quote, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return Quote{}, errors.New("简短文案不能为空")
	}
	status := input.Status
	if status == "" {
		status = "enabled"
	}
	gid := groupIDValue(input.GroupID)

	if input.ID == "" {
		row := s.db.QueryRowContext(c,
			`INSERT INTO mind_quotes (group_id, title, content, prompt, sort, status)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+quoteColumns,
			gid, title, input.Content, input.Prompt, input.Sort, status)
		return scanQuote(row)
	}

	qid, err := strconv.ParseInt(input.ID, 10, 64)
	if err != nil {
		return Quote{}, errors.New("invalid quote id")
	}
	row := s.db.QueryRowContext(c,
		`UPDATE mind_quotes SET group_id=$1, title=$2, content=$3, prompt=$4, sort=$5, status=$6, update_time=now()
		 WHERE id=$7 RETURNING `+quoteColumns,
		gid, title, input.Content, input.Prompt, input.Sort, status, qid)
	return scanQuote(row)
}

// DeleteQuote 删除心语。
func (s *Store) DeleteQuote(ctx context.Context, id string) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM mind_quotes WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------------- 公开只读（官网）----------------

// PublicQuote 官网列表用的轻量心语（不含原文）。
type PublicQuote struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// PublicGroup 官网用：分组 + 其下心语简短文案。
type PublicGroup struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Intro  string        `json:"intro"`
	Quotes []PublicQuote `json:"quotes"`
}

// PublicGroups 返回启用的分组及其启用的心语（轻量），供官网一次拉全。
func (s *Store) PublicGroups(ctx context.Context) ([]PublicGroup, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	groupRows, err := s.db.QueryContext(c,
		`SELECT id, name, intro FROM mind_groups WHERE status='enabled' ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer groupRows.Close()

	groups := []PublicGroup{}
	index := map[string]int{}
	for groupRows.Next() {
		var g PublicGroup
		var id int64
		if err := groupRows.Scan(&id, &g.Name, &g.Intro); err != nil {
			return nil, err
		}
		g.ID = strconv.FormatInt(id, 10)
		g.Quotes = []PublicQuote{}
		index[g.ID] = len(groups)
		groups = append(groups, g)
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}

	allGroup := PublicGroup{
		ID:     "all",
		Name:   "全部心语",
		Intro:  "从 PDF《老韩语录·九型成长心语》整理出的首批心语，后续可在后台归入脑、心、腹分组。",
		Quotes: []PublicQuote{},
	}

	quoteRows, err := s.db.QueryContext(c,
		`SELECT id, group_id, title FROM mind_quotes
		 WHERE status='enabled'
		 ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer quoteRows.Close()
	for quoteRows.Next() {
		var id int64
		var gid sql.NullInt64
		var title string
		if err := quoteRows.Scan(&id, &gid, &title); err != nil {
			return nil, err
		}
		item := PublicQuote{
			ID:    strconv.FormatInt(id, 10),
			Title: title,
		}
		allGroup.Quotes = append(allGroup.Quotes, item)
		if !gid.Valid {
			continue
		}
		key := strconv.FormatInt(gid.Int64, 10)
		if pos, ok := index[key]; ok {
			groups[pos].Quotes = append(groups[pos].Quotes, PublicQuote{
				ID:    item.ID,
				Title: item.Title,
			})
		}
	}
	if err := quoteRows.Err(); err != nil {
		return nil, err
	}
	if len(allGroup.Quotes) > 0 {
		groups = append([]PublicGroup{allGroup}, groups...)
	}
	return groups, nil
}

// PublicDetail 返回单条启用心语的完整原文（官网详情页）。
func (s *Store) PublicDetail(ctx context.Context, id string) (Quote, bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	row := s.db.QueryRowContext(c,
		"SELECT "+quoteColumns+" FROM mind_quotes WHERE id=$1 AND status='enabled'", id)
	q, err := scanQuote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Quote{}, false, nil
		}
		return Quote{}, false, err
	}
	return q, true, nil
}

// CountQuotes 用于 seed 判断是否已有数据。
func (s *Store) CountQuotes(ctx context.Context) (int, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var n int
	err := s.db.QueryRowContext(c, `SELECT count(*) FROM mind_quotes`).Scan(&n)
	return n, err
}
