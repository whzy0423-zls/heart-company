// Package video 接入 New API / OpenAI 兼容网关的视频生成能力（即梦 2.0）。
//
// 与 voice 包不同，视频生成是异步的：创建任务仅返回 task_id 与初始状态，
// 需轮询拉取最终结果。本包据此拆分为两步：
//   - Generate：调用网关创建任务，落库一行 status='queued' 并记录 task_id；
//   - Refresh：按 id 轮询网关，完成后下载视频字节经 uploadasset 落库并回填。
package video

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

// 终态集合：处于这些状态时无需再次轮询网关。
var terminalStatuses = map[string]bool{
	"completed": true,
	"succeeded": true,
	"failed":    true,
}

type Store struct {
	client       *Client
	db           *sql.DB
	uploads      *uploadasset.Store
	uploader     storage.ObjectUploader
	defaultModel string
}

type Generation struct {
	AspectRatio  string  `json:"aspectRatio"`
	CreateTime   string  `json:"createTime"`
	Duration     float64 `json:"duration"`
	ErrorMessage string  `json:"errorMessage"`
	FPS          float64 `json:"fps"`
	Height       int     `json:"height"`
	ID           string  `json:"id"`
	ImageURL     string  `json:"imageUrl"`
	Model        string  `json:"model"`
	Prompt       string  `json:"prompt"`
	Provider     string  `json:"provider"`
	Seconds      int     `json:"seconds"`
	Status       string  `json:"status"`
	TaskID       string  `json:"taskId"`
	UpdateTime   string  `json:"updateTime"`
	VideoAssetID string  `json:"videoAssetId"`
	VideoURL     string  `json:"videoUrl"`
	Width        int     `json:"width"`
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type GenerateInput struct {
	AspectRatio string   `json:"aspectRatio"`
	Audios      []string `json:"audios"`   // 参考音频 URL
	ImageURL    string   `json:"imageUrl"` // 兼容旧字段：单张参考图地址
	Images      []string `json:"images"`   // 参考图片地址（上传到文件桶后的可公网访问 URL）
	Videos      []string `json:"videos"`   // 参考视频地址（上传到文件桶后的可公网访问 URL）
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Seconds     int      `json:"seconds"`
}

// 网关支持的视频时长（秒），默认 15s。
var allowedSeconds = map[int]bool{5: true, 10: true, 15: true}

var allowedAspectRatios = map[string]bool{"16:9": true, "9:16": true, "1:1": true}

const defaultSeconds = 15
const defaultAspectRatio = "16:9"

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

// cleanURLs 去除空白、丢弃空串并按出现顺序去重，
// 用于整理上传到文件桶后回传的参考图片/视频地址。
func cleanURLs(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func NewStore(database *sql.DB, uploads *uploadasset.Store, cfg config.VideoConfig, uploaders ...storage.ObjectUploader) *Store {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "video-ds-2.0-fast"
	}
	var uploader storage.ObjectUploader
	if len(uploaders) > 0 {
		uploader = uploaders[0]
	}
	return &Store{
		client:       NewClient(cfg),
		db:           database,
		uploads:      uploads,
		uploader:     uploader,
		defaultModel: model,
	}
}

// Generate 创建一个视频生成任务：调用网关拿到 task_id 后落库 'queued' 行。
// 不在此阻塞等待结果——前端按返回的 id 调 Refresh 轮询。
func (s *Store) Generate(ctx context.Context, input GenerateInput) (Generation, error) {
	prompt := strings.TrimSpace(input.Prompt)

	// 收集参考图片：兼容旧的单字段 ImageURL，并入 images 数组后去重去空。
	images := cleanURLs(append([]string{input.ImageURL}, input.Images...))
	videos := cleanURLs(input.Videos)
	audios := cleanURLs(input.Audios)

	if prompt == "" && len(images) == 0 && len(videos) == 0 && len(audios) == 0 {
		return Generation{}, fmt.Errorf("请输入提示词或提供参考图片/视频/音频")
	}
	if len([]rune(prompt)) > 2000 {
		return Generation{}, fmt.Errorf("提示词不能超过 2000 个字")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = s.defaultModel
	}

	seconds := input.Seconds
	if seconds == 0 {
		seconds = defaultSeconds
	}
	if !allowedSeconds[seconds] {
		return Generation{}, fmt.Errorf("视频时长只能是 5、10 或 15 秒")
	}
	aspectRatio := strings.TrimSpace(input.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = defaultAspectRatio
	}
	if !allowedAspectRatios[aspectRatio] {
		return Generation{}, fmt.Errorf("视频画幅仅支持 16:9、9:16 或 1:1")
	}

	// image_url 列仅用于后台列表展示首帧，取第一张参考图。
	var imageURL string
	if len(images) > 0 {
		imageURL = images[0]
	}

	task, err := s.client.CreateTask(ctx, model, prompt, images, videos, audios, seconds, aspectRatio)
	if err != nil {
		var id string
		_ = s.db.QueryRowContext(ctx,
			`INSERT INTO video_generations (provider, model, prompt, image_url, seconds, aspect_ratio, status, error_message)
				 VALUES ('newapi',$1,$2,$3,$4,$5,'failed',$6)
				 RETURNING id::text`,
			model, prompt, imageURL, seconds, aspectRatio, err.Error(),
		).Scan(&id)
		return Generation{}, err
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return Generation{}, fmt.Errorf("视频网关创建成功但未返回 task_id，请检查上游响应")
	}

	status := normalizeStatus(task.Status)
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_generations (provider, model, prompt, image_url, task_id, seconds, aspect_ratio, status)
			 VALUES ('newapi',$1,$2,$3,$4,$5,$6,$7)
			 RETURNING id::text`,
		model, prompt, imageURL, task.TaskID, seconds, aspectRatio, status,
	).Scan(&id); err != nil {
		return Generation{}, err
	}
	return s.Generation(ctx, id)
}

// Refresh 轮询单条记录的网关状态。终态直接返回；进行中则查询网关，
// 完成后下载视频落库并回填资产与元数据。
func (s *Store) Refresh(ctx context.Context, id string) (Generation, error) {
	item, err := s.Generation(ctx, id)
	if err != nil {
		return Generation{}, err
	}
	if shouldSkipRefresh(item) {
		return item, nil
	}

	task, err := s.client.QueryTask(ctx, item.TaskID, item.Seconds)
	if err != nil {
		return item, nil // 轮询失败不改写状态，下次再试
	}
	status := normalizeStatus(task.Status)

	switch status {
	case "completed", "succeeded":
		data, contentType, err := s.client.DownloadTaskContent(ctx, item.TaskID)
		if err != nil {
			if strings.TrimSpace(task.URL) == "" {
				return item, nil
			}
			data, contentType, err = s.client.Download(ctx, task.URL)
			if err != nil {
				_ = s.markFailed(ctx, id, "视频已生成但下载失败，请稍后重试")
				return s.Generation(ctx, id)
			}
		}
		asset, err := s.createUploadAsset(ctx, data, contentType, "video/generated", fmt.Sprintf("video-%s.mp4", time.Now().Format("20060102150405")))
		if err != nil {
			return item, err
		}
		if !isPublicHTTPURL(asset.ObjectURL) {
			_ = s.markFailed(ctx, id, "视频已生成，但没有文件桶公网地址，请配置 OSS_PUBLIC_URL/文件桶公网访问后重试")
			return s.Generation(ctx, id)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE video_generations
			    SET status='completed', video_asset_id=$1, video_url=$2,
			        duration=$3, fps=$4, width=$5, height=$6, error_message='', update_time=now()
			  WHERE id=$7`,
			asset.ID, asset.ObjectURL, task.Duration, task.FPS, task.Width, task.Height, id,
		); err != nil {
			return Generation{}, err
		}
	case "failed":
		_ = s.markFailed(ctx, id, task.ErrorMessage)
	default:
		if status != item.Status {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE video_generations SET status=$1, error_message='', update_time=now() WHERE id=$2`,
				status, id,
			)
		}
	}
	return s.Generation(ctx, id)
}

func (s *Store) createUploadAsset(ctx context.Context, data []byte, contentType string, dir string, name string) (uploadasset.Asset, error) {
	var objectKey string
	var objectURL string
	if s.uploader != nil {
		uploaded, err := s.uploader.Upload(ctx, storage.UploadInput{
			ContentType: contentType,
			Dir:         dir,
			Filename:    name,
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return uploadasset.Asset{}, err
		}
		objectKey = uploaded.Key
		objectURL = uploaded.URL
		if strings.TrimSpace(uploaded.Name) != "" {
			name = uploaded.Name
		}
		if strings.TrimSpace(uploaded.ContentType) != "" {
			contentType = uploaded.ContentType
		}
	}
	return s.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        data,
		Dir:         dir,
		Name:        name,
		ObjectKey:   objectKey,
		ObjectURL:   objectURL,
		Size:        int64(len(data)),
	})
}

func (s *Store) markFailed(ctx context.Context, id string, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "视频生成失败"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE video_generations SET status='failed', error_message=$1, update_time=now() WHERE id=$2`,
		message, id,
	)
	return err
}

func shouldSkipRefresh(item Generation) bool {
	if strings.TrimSpace(item.TaskID) == "" {
		return true
	}
	if terminalStatuses[item.Status] && strings.TrimSpace(item.VideoURL) != "" {
		return true
	}
	return false
}

func isPublicHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isPrivateOrLocalIP(ip)
	}
	return true
}

func isPrivateOrLocalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (s *Store) ListGenerations(ctx context.Context, query url.Values) (PageResult[Generation], error) {
	page, pageSize := pagination(query)
	condition, args := generationListCondition(query)

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM video_generations WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Generation]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
			        COALESCE(video_asset_id::text,''), video_url, duration, fps, width, height,
		        status, error_message, create_time, update_time
		   FROM video_generations
		  WHERE `+condition+`
		  ORDER BY create_time DESC
		  LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Generation]{}, err
	}
	defer rows.Close()
	items := []Generation{}
	for rows.Next() {
		var item Generation
		var createTime, updateTime time.Time
		if err := rows.Scan(&item.ID, &item.Provider, &item.Model, &item.Prompt, &item.ImageURL, &item.TaskID, &item.Seconds, &item.AspectRatio,
			&item.VideoAssetID, &item.VideoURL, &item.Duration, &item.FPS, &item.Width, &item.Height,
			&item.Status, &item.ErrorMessage, &createTime, &updateTime); err != nil {
			return PageResult[Generation]{}, err
		}
		item.CreateTime = formatTime(createTime)
		item.UpdateTime = formatTime(updateTime)
		items = append(items, item)
	}
	return PageResult[Generation]{Items: items, Total: total}, rows.Err()
}

func generationListCondition(query url.Values) (string, []any) {
	args := []any{}
	where := []string{"task_id <> ''"}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("prompt ILIKE $%d", len(args)))
	}
	return strings.Join(where, " AND "), args
}

func (s *Store) Generation(ctx context.Context, id string) (Generation, error) {
	var item Generation
	var createTime, updateTime time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text, provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
			        COALESCE(video_asset_id::text,''), video_url, duration, fps, width, height,
		        status, error_message, create_time, update_time
		   FROM video_generations
		  WHERE id=$1`,
		id,
	).Scan(&item.ID, &item.Provider, &item.Model, &item.Prompt, &item.ImageURL, &item.TaskID, &item.Seconds, &item.AspectRatio,
		&item.VideoAssetID, &item.VideoURL, &item.Duration, &item.FPS, &item.Width, &item.Height,
		&item.Status, &item.ErrorMessage, &createTime, &updateTime)
	if err != nil {
		return Generation{}, err
	}
	item.CreateTime = formatTime(createTime)
	item.UpdateTime = formatTime(updateTime)
	return item, nil
}

func normalizeStatus(raw string) string {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "", "unknown":
		return "queued"
	case "in_progress", "processing", "running":
		return "in_progress"
	case "queued", "pending", "submitted":
		return "queued"
	case "completed", "succeeded", "success":
		if status == "success" {
			return "succeeded"
		}
		return status
	case "failed", "error":
		return "failed"
	default:
		return status
	}
}

func pagination(query url.Values) (int, int) {
	page := 1
	pageSize := 20
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
		pageSize = 20
	}
	return page, pageSize
}

// Client 是 New API / OpenAI 兼容视频网关的最小 HTTP 客户端。
type Client struct {
	apiBase string
	apiKey  string
	client  *http.Client
}

// TaskResult 归一化网关创建/查询任务返回的字段。
type TaskResult struct {
	TaskID       string
	Status       string
	URL          string
	Duration     float64
	FPS          float64
	Width        int
	Height       int
	ErrorMessage string
}

func NewClient(cfg config.VideoConfig) *Client {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		apiBase: strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DisableKeepAlives: true,
				Proxy:             http.ProxyFromEnvironment,
			},
		},
	}
}

func (c *Client) ensureReady() error {
	if c == nil || c.apiBase == "" {
		return fmt.Errorf("视频网关未配置 VIDEO_API_BASE")
	}
	if c.apiKey == "" {
		return fmt.Errorf("视频网关未配置 VIDEO_API_KEY")
	}
	return nil
}

func (c *Client) endpoint(path string) string {
	return c.apiBase + path
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// CreateTask 调用 POST /v1/videos 创建任务。
// images/videos 为已上传到文件桶、可公网访问的参考素材地址。
func (c *Client) CreateTask(ctx context.Context, model, prompt string, images, videos, audios []string, seconds int, aspectRatio string) (TaskResult, error) {
	if err := c.ensureReady(); err != nil {
		return TaskResult{}, err
	}
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" {
		aspectRatio = defaultAspectRatio
	}
	body := map[string]any{"model": model, "prompt": prompt, "seconds": fmt.Sprint(seconds), "aspect_ratio": aspectRatio}
	if len(images) > 0 {
		body["images"] = images
	}
	if len(videos) > 0 {
		body["videos"] = videos
	}
	if len(audios) > 0 {
		body["audios"] = audios
	}
	raw, _ := json.Marshal(body)
	reqURL, err := url.Parse(c.endpoint("/v1/videos"))
	if err != nil {
		return TaskResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(raw))
	if err != nil {
		return TaskResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)

	payload, err := c.doJSON(req)
	if err != nil {
		return TaskResult{}, err
	}
	return parseTask(payload), nil
}

// QueryTask 调用 GET /v1/videos/{task_id} 轮询任务。
func (c *Client) QueryTask(ctx context.Context, taskID string, seconds int) (TaskResult, error) {
	if err := c.ensureReady(); err != nil {
		return TaskResult{}, err
	}
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	endpoint := c.endpoint("/v1/videos/" + url.PathEscape(taskID))
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return TaskResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		reqURL.String(), nil)
	if err != nil {
		return TaskResult{}, err
	}
	c.auth(req)

	payload, err := c.doJSON(req)
	if err != nil {
		return TaskResult{}, err
	}
	return parseTask(payload), nil
}

// DownloadTaskContent 调用 GET /v1/videos/{task_id}/content 下载最终视频内容。
func (c *Client) DownloadTaskContent(ctx context.Context, taskID string) ([]byte, string, error) {
	if err := c.ensureReady(); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.endpoint("/v1/videos/"+url.PathEscape(taskID)+"/content"), nil)
	if err != nil {
		return nil, "", err
	}
	c.auth(req)
	return c.download(req)
}

// Download 拉取最终视频字节，限制 200MB 防止内存失控。
func (c *Client) Download(ctx context.Context, fileURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", err
	}
	return c.download(req)
}

func (c *Client) download(req *http.Request) ([]byte, string, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("下载视频失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024*1024))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "video/mp4"
	}
	return data, contentType, nil
}

func (c *Client) doJSON(req *http.Request) (map[string]any, error) {
	payload, err := c.doJSONOnce(req)
	if err == nil || !isRetryableNetworkError(err) {
		return payload, err
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		retry.Body = body
	}
	return c.doJSONOnce(retry)
}

func (c *Client) doJSONOnce(req *http.Request) (map[string]any, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("视频网关返回 HTTP %d: %s", resp.StatusCode, compactBody(raw))
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析网关响应失败: %w", err)
	}
	return payload, nil
}

func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && !netErr.Timeout()
}

// parseTask 从网关返回的嵌套 JSON 中提取归一化字段。
func parseTask(payload map[string]any) TaskResult {
	result := TaskResult{
		TaskID:       findString(payload, "task_id", "id", "data.task_id", "data.id"),
		Status:       findString(payload, "status", "data.status"),
		URL:          findString(payload, "url", "video_url", "data.url", "data.video_url", "output.url"),
		ErrorMessage: findString(payload, "error.message", "error", "message", "data.error.message"),
		Duration:     findFloat(payload, "metadata.duration", "duration", "data.metadata.duration"),
		FPS:          findFloat(payload, "metadata.fps", "fps", "data.metadata.fps"),
		Width:        int(findFloat(payload, "metadata.width", "width", "data.metadata.width")),
		Height:       int(findFloat(payload, "metadata.height", "height", "data.metadata.height")),
	}
	if result.Status == "" && strings.TrimSpace(result.URL) != "" && findFloat(payload, "progress", "data.progress") >= 100 {
		result.Status = "completed"
	}
	if result.Width == 0 || result.Height == 0 {
		width, height := parseSize(findString(payload, "size", "metadata.size", "data.size", "data.metadata.size"))
		if result.Width == 0 {
			result.Width = width
		}
		if result.Height == 0 {
			result.Height = height
		}
	}
	return result
}

func parseSize(raw string) (int, int) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), "x")
	if len(parts) != 2 {
		return 0, 0
	}
	var width, height int
	_, _ = fmt.Sscan(strings.TrimSpace(parts[0]), &width)
	_, _ = fmt.Sscan(strings.TrimSpace(parts[1]), &height)
	return width, height
}

// findString 按点分路径在嵌套 map 中查找字符串值。
func findString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if v, ok := lookup(payload, path); ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return t
				}
			case float64:
				return fmt.Sprintf("%v", t)
			case json.Number:
				return t.String()
			}
		}
	}
	return ""
}

// findFloat 按点分路径在嵌套 map 中查找数值。
func findFloat(payload map[string]any, paths ...string) float64 {
	for _, path := range paths {
		if v, ok := lookup(payload, path); ok {
			switch t := v.(type) {
			case float64:
				return t
			case json.Number:
				if f, err := t.Float64(); err == nil {
					return f
				}
			case string:
				var f float64
				if _, err := fmt.Sscan(t, &f); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func lookup(payload map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = payload
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func compactBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
