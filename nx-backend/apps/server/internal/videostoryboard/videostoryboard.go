// Package videostoryboard manages Seedance storyboard design jobs.
package videostoryboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

type Shot struct {
	Action         string   `json:"action"`
	Assets         []string `json:"assets"`
	Audio          string   `json:"audio"`
	Camera         string   `json:"camera"`
	Characters     []string `json:"characters"`
	Composition    string   `json:"composition"`
	Dialogue       string   `json:"dialogue"`
	Duration       float64  `json:"duration"`
	Index          int      `json:"index"`
	Lighting       string   `json:"lighting"`
	Scene          string   `json:"scene"`
	SeedancePrompt string   `json:"seedancePrompt"`
	Title          string   `json:"title"`
}

type Job struct {
	AnalysisJobID string   `json:"analysisJobId"`
	CreateTime    string   `json:"createTime"`
	ErrorMessage  string   `json:"errorMessage"`
	GlobalPrompt  string   `json:"globalPrompt"`
	ID            string   `json:"id"`
	RawResult     string   `json:"rawResult"`
	Shots         []Shot   `json:"shots"`
	Status        string   `json:"status"`
	StyleGuide    []string `json:"styleGuide"`
	Theme         string   `json:"theme"`
	Title         string   `json:"title"`
	UpdateTime    string   `json:"updateTime"`
}

type CreateInput struct {
	AnalysisJobID string `json:"analysisJobId"`
	Theme         string `json:"theme"`
	Title         string `json:"title"`
}

type UpdateInput struct {
	GlobalPrompt string   `json:"globalPrompt"`
	Shots        []Shot   `json:"shots"`
	StyleGuide   []string `json:"styleGuide"`
	Theme        string   `json:"theme"`
	Title        string   `json:"title"`
}

type Result struct {
	GlobalPrompt string   `json:"globalPrompt"`
	RawResult    string   `json:"rawResult"`
	Shots        []Shot   `json:"shots"`
	StyleGuide   []string `json:"styleGuide"`
	Title        string   `json:"title"`
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func (s *Store) Create(ctx context.Context, input CreateInput) (Job, error) {
	analysisID, err := parseRequiredID(input.AnalysisJobID)
	if err != nil {
		return Job{}, fmt.Errorf("请选择已完成的视频分析记录")
	}
	theme := strings.TrimSpace(input.Theme)
	if theme == "" {
		return Job{}, fmt.Errorf("请输入分镜主题")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = theme
	}
	var id string
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO video_storyboards (analysis_job_id, title, theme, status)
		 VALUES ($1,$2,$3,'queued')
		 RETURNING id::text`,
		analysisID, title, theme,
	).Scan(&id)
	if err != nil {
		return Job{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) List(ctx context.Context, query url.Values) (PageResult[Job], error) {
	page, pageSize := pagination(query)
	where := []string{"1=1"}
	args := []any{}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR theme ILIKE $%d)", len(args), len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM video_storyboards WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Job]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, COALESCE(analysis_job_id::text,''), title, theme, status,
		        style_guide, global_prompt, shots, raw_result, error_message, create_time, update_time
		   FROM video_storyboards
		  WHERE `+condition+`
		  ORDER BY create_time DESC
		  LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Job]{}, err
	}
	defer rows.Close()

	items := []Job{}
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return PageResult[Job]{}, err
		}
		items = append(items, item)
	}
	return PageResult[Job]{Items: items, Total: total}, rows.Err()
}

func (s *Store) Find(ctx context.Context, id string) (Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id::text, COALESCE(analysis_job_id::text,''), title, theme, status,
		        style_guide, global_prompt, shots, raw_result, error_message, create_time, update_time
		   FROM video_storyboards WHERE id=$1`, id,
	)
	return scanJob(row)
}

func (s *Store) Update(ctx context.Context, id string, input UpdateInput) (Job, error) {
	title := strings.TrimSpace(input.Title)
	theme := strings.TrimSpace(input.Theme)
	if title == "" {
		return Job{}, fmt.Errorf("请输入分镜标题")
	}
	if theme == "" {
		return Job{}, fmt.Errorf("请输入分镜主题")
	}
	styleGuide, _ := json.Marshal(cleanList(input.StyleGuide))
	shots, _ := json.Marshal(cleanShots(input.Shots))
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET title=$1, theme=$2, style_guide=$3::jsonb, global_prompt=$4, shots=$5::jsonb, update_time=now()
		  WHERE id=$6`,
		title, theme, string(styleGuide), strings.TrimSpace(input.GlobalPrompt), string(shots), id,
	)
	if err != nil {
		return Job{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) MarkRunning(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET status='running', error_message='', update_time=now()
		  WHERE id=$1 AND status IN ('queued','failed')`, id)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("分镜任务不可运行或已被其他进程处理")
	}
	return nil
}

func (s *Store) RecoverRunningAsFailed(ctx context.Context, message string) (int64, error) {
	if strings.TrimSpace(message) == "" {
		message = "服务重启或任务超时，分镜任务已中止，请重试"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET status='failed', error_message=$1, update_time=now()
		  WHERE status='running'`, message)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) QueuedIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text
		   FROM video_storyboards
		  WHERE status='queued'
		  ORDER BY create_time ASC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) Retry(ctx context.Context, id string) (Job, error) {
	current, err := s.Find(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if current.Status != "failed" {
		return Job{}, fmt.Errorf("只有失败的分镜任务可以重试")
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET status='queued',
		        style_guide='[]'::jsonb,
		        global_prompt='',
		        shots='[]'::jsonb,
		        raw_result='',
		        error_message='',
		        update_time=now()
		  WHERE id=$1`, id)
	if err != nil {
		return Job{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) Complete(ctx context.Context, id string, result Result) error {
	styleGuide, _ := json.Marshal(cleanList(result.StyleGuide))
	shots, _ := json.Marshal(cleanShots(result.Shots))
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET status='completed', title=$1, style_guide=$2::jsonb, global_prompt=$3,
		        shots=$4::jsonb, raw_result=$5, error_message='', update_time=now()
		  WHERE id=$6`,
		strings.TrimSpace(result.Title), string(styleGuide), strings.TrimSpace(result.GlobalPrompt),
		string(shots), strings.TrimSpace(result.RawResult), id,
	)
	return err
}

func (s *Store) Fail(ctx context.Context, id string, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "分镜设计失败"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_storyboards
		    SET status='failed', error_message=$1, update_time=now()
		  WHERE id=$2`, message, id)
	return err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM video_storyboards WHERE id=$1`, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(row scanner) (Job, error) {
	var item Job
	var styleRaw, shotsRaw []byte
	var createTime, updateTime time.Time
	if err := row.Scan(&item.ID, &item.AnalysisJobID, &item.Title, &item.Theme, &item.Status, &styleRaw, &item.GlobalPrompt, &shotsRaw, &item.RawResult, &item.ErrorMessage, &createTime, &updateTime); err != nil {
		return Job{}, err
	}
	item.StyleGuide = parseList(styleRaw)
	item.Shots = parseShots(shotsRaw)
	item.CreateTime = formatTime(createTime)
	item.UpdateTime = formatTime(updateTime)
	return item, nil
}

func parseList(raw []byte) []string {
	var values []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return cleanList(values)
}

func parseShots(raw []byte) []Shot {
	var values []Shot
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return cleanShots(values)
}

func cleanList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cleanShots(values []Shot) []Shot {
	out := []Shot{}
	for index, shot := range values {
		shot.Title = strings.TrimSpace(shot.Title)
		shot.Scene = strings.TrimSpace(shot.Scene)
		shot.Action = strings.TrimSpace(shot.Action)
		shot.Camera = strings.TrimSpace(shot.Camera)
		shot.Composition = strings.TrimSpace(shot.Composition)
		shot.Lighting = strings.TrimSpace(shot.Lighting)
		shot.Audio = strings.TrimSpace(shot.Audio)
		shot.Dialogue = strings.TrimSpace(shot.Dialogue)
		shot.SeedancePrompt = strings.TrimSpace(shot.SeedancePrompt)
		shot.Characters = cleanList(shot.Characters)
		shot.Assets = cleanList(shot.Assets)
		if shot.Index <= 0 {
			shot.Index = index + 1
		}
		if shot.Duration < 0 {
			shot.Duration = 0
		}
		if shot.Title == "" && shot.Scene == "" && shot.Action == "" && shot.SeedancePrompt == "" {
			continue
		}
		out = append(out, shot)
	}
	return out
}

func pagination(query url.Values) (int, int) {
	page := 1
	pageSize := 10
	if v := strings.TrimSpace(query.Get("page")); v != "" {
		_, _ = fmt.Sscan(v, &page)
	}
	if v := strings.TrimSpace(query.Get("pageSize")); v != "" {
		_, _ = fmt.Sscan(v, &pageSize)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

func parseRequiredID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty id")
	}
	var id int64
	if _, err := fmt.Sscan(value, &id); err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}
