// Package videoanalysis manages asynchronous video analysis jobs.
package videoanalysis

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

type Job struct {
	Assets         []string `json:"assets"`
	AudioSummary   string   `json:"audioSummary"`
	Characters     []string `json:"characters"`
	CreateTime     string   `json:"createTime"`
	ErrorMessage   string   `json:"errorMessage"`
	HasSpeech      bool     `json:"hasSpeech"`
	ID             string   `json:"id"`
	RawResult      string   `json:"rawResult"`
	Scenes         []string `json:"scenes"`
	SeedancePrompt string   `json:"seedancePrompt"`
	SpeechKeywords []string `json:"speechKeywords"`
	SpeechOutline  []string `json:"speechOutline"`
	SpeechTopics   []string `json:"speechTopics"`
	Status         string   `json:"status"`
	UpdateTime     string   `json:"updateTime"`
	VideoAssetID   string   `json:"videoAssetId"`
	VideoName      string   `json:"videoName"`
	VideoURL       string   `json:"videoUrl"`
}

type CreateInput struct {
	VideoAssetID string `json:"videoAssetId"`
	VideoName    string `json:"videoName"`
	VideoURL     string `json:"videoUrl"`
}

type Result struct {
	Assets         []string `json:"assets"`
	AudioSummary   string   `json:"audioSummary"`
	Characters     []string `json:"characters"`
	HasSpeech      bool     `json:"hasSpeech"`
	RawResult      string   `json:"rawResult"`
	Scenes         []string `json:"scenes"`
	SeedancePrompt string   `json:"seedancePrompt"`
	SpeechKeywords []string `json:"speechKeywords"`
	SpeechOutline  []string `json:"speechOutline"`
	SpeechTopics   []string `json:"speechTopics"`
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
	videoURL := strings.TrimSpace(input.VideoURL)
	if videoURL == "" {
		return Job{}, fmt.Errorf("请先上传视频")
	}
	videoName := strings.TrimSpace(input.VideoName)
	if videoName == "" {
		videoName = "未命名视频"
	}
	assetID, err := parseOptionalID(input.VideoAssetID)
	if err != nil {
		return Job{}, fmt.Errorf("视频资产标识无效")
	}

	var id string
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO video_analysis_jobs (video_asset_id, video_url, video_name, status)
		 VALUES ($1,$2,$3,'queued')
		 RETURNING id::text`,
		nullInt64(assetID), videoURL, videoName,
	).Scan(&id)
	if err != nil {
		return Job{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) List(ctx context.Context, query url.Values) (PageResult[Job], error) {
	page, pageSize := pagination(query)
	args := []any{}
	where := []string{"1=1"}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM video_analysis_jobs WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Job]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, COALESCE(video_asset_id::text,''), video_url, video_name, status,
		        scenes, characters, assets, has_speech, audio_summary, speech_topics, speech_keywords, speech_outline,
		        seedance_prompt, raw_result, error_message, create_time, update_time
		   FROM video_analysis_jobs
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
		`SELECT id::text, COALESCE(video_asset_id::text,''), video_url, video_name, status,
		        scenes, characters, assets, has_speech, audio_summary, speech_topics, speech_keywords, speech_outline,
		        seedance_prompt, raw_result, error_message, create_time, update_time
		   FROM video_analysis_jobs WHERE id=$1`, id,
	)
	return scanJob(row)
}

func (s *Store) MarkRunning(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
		    SET status='running', error_message='', update_time=now()
		  WHERE id=$1 AND status IN ('queued','failed')`, id)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("视频分析任务不可运行或已被其他进程处理")
	}
	return nil
}

func (s *Store) RecoverRunningAsFailed(ctx context.Context, message string) (int64, error) {
	if strings.TrimSpace(message) == "" {
		message = "服务重启或任务超时，视频分析任务已中止，请重试"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
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
		   FROM video_analysis_jobs
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
		return Job{}, fmt.Errorf("只有失败的视频分析任务可以重试")
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
		    SET status='queued',
		        scenes='[]'::jsonb,
		        characters='[]'::jsonb,
		        assets='[]'::jsonb,
		        has_speech=false,
		        audio_summary='',
		        speech_topics='[]'::jsonb,
		        speech_keywords='[]'::jsonb,
		        speech_outline='[]'::jsonb,
		        seedance_prompt='',
		        raw_result='',
		        error_message='',
		        update_time=now()
		  WHERE id=$1`, id)
	if err != nil {
		return Job{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) UpdateVideoURL(ctx context.Context, id string, videoURL string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
		    SET video_url=$1, update_time=now()
		  WHERE id=$2`, strings.TrimSpace(videoURL), id)
	return err
}

func (s *Store) Complete(ctx context.Context, id string, result Result) error {
	scenes, _ := json.Marshal(cleanList(result.Scenes))
	characters, _ := json.Marshal(cleanList(result.Characters))
	assets, _ := json.Marshal(cleanList(result.Assets))
	speechTopics, _ := json.Marshal(cleanList(result.SpeechTopics))
	speechKeywords, _ := json.Marshal(cleanList(result.SpeechKeywords))
	speechOutline, _ := json.Marshal(cleanList(result.SpeechOutline))
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
		    SET status='completed', scenes=$1::jsonb, characters=$2::jsonb, assets=$3::jsonb,
		        has_speech=$4, audio_summary=$5, speech_topics=$6::jsonb, speech_keywords=$7::jsonb,
		        speech_outline=$8::jsonb, seedance_prompt=$9, raw_result=$10, error_message='', update_time=now()
		  WHERE id=$11`,
		string(scenes), string(characters), string(assets), result.HasSpeech, strings.TrimSpace(result.AudioSummary),
		string(speechTopics), string(speechKeywords), string(speechOutline),
		strings.TrimSpace(result.SeedancePrompt), strings.TrimSpace(result.RawResult), id,
	)
	return err
}

func (s *Store) Fail(ctx context.Context, id string, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "视频分析失败"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_analysis_jobs
		    SET status='failed', error_message=$1, update_time=now()
		  WHERE id=$2`, message, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(row scanner) (Job, error) {
	var item Job
	var scenesRaw, charactersRaw, assetsRaw, speechTopicsRaw, speechKeywordsRaw, speechOutlineRaw []byte
	var createTime, updateTime time.Time
	if err := row.Scan(&item.ID, &item.VideoAssetID, &item.VideoURL, &item.VideoName, &item.Status, &scenesRaw, &charactersRaw, &assetsRaw, &item.HasSpeech, &item.AudioSummary, &speechTopicsRaw, &speechKeywordsRaw, &speechOutlineRaw, &item.SeedancePrompt, &item.RawResult, &item.ErrorMessage, &createTime, &updateTime); err != nil {
		return Job{}, err
	}
	item.Scenes = parseList(scenesRaw)
	item.Characters = parseList(charactersRaw)
	item.Assets = parseList(assetsRaw)
	item.AudioSummary = strings.TrimSpace(item.AudioSummary)
	item.SpeechTopics = parseList(speechTopicsRaw)
	item.SpeechKeywords = parseList(speechKeywordsRaw)
	item.SpeechOutline = parseList(speechOutlineRaw)
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

func parseOptionalID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	var id int64
	_, err := fmt.Sscan(value, &id)
	return id, err
}

func nullInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
