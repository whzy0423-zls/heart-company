// Package image 接入 OpenAI 兼容 / 中转代理网关的文生图能力（gpt-image-2）。
//
// 与 video 包不同，图像生成是同步的：POST /v1/images/generations 直接
// 返回 base64(b64_json)。本包据此只暴露一步 Generate：调用网关拿到图片
// 字节后经 uploadasset 落库，返回可公网访问的资产地址供上层注册到资产库。
package image

import (
	"bytes"
	"context"
	"encoding/base64"
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

const defaultModel = "gpt-image-2"
const defaultSize = "1024x1024"

// allowedSizes 网关支持的图片尺寸，默认 1024x1024。
var allowedSizes = map[string]bool{
	"1024x1024": true,
	"1024x1536": true,
	"1536x1024": true,
	"auto":      true,
}

type Store struct {
	client       *Client
	uploads      *uploadasset.Store
	uploader     storage.ObjectUploader
	defaultModel string
}

// GenerateInput 文生图请求参数。
type GenerateInput struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size"`
}

// Result 文生图结果：落库后的资产地址与元信息。
type Result struct {
	AssetID     int64  `json:"assetId"`
	ContentType string `json:"contentType"`
	Model       string `json:"model"`
	ObjectURL   string `json:"objectUrl"`
	PreviewURL  string `json:"previewUrl"`
	Prompt      string `json:"prompt"`
	Size        string `json:"size"`
	URL         string `json:"url"`
}

func NewStore(uploads *uploadasset.Store, cfg config.ImageConfig, uploaders ...storage.ObjectUploader) *Store {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	var uploader storage.ObjectUploader
	if len(uploaders) > 0 {
		uploader = uploaders[0]
	}
	return &Store{
		client:       NewClient(cfg),
		uploads:      uploads,
		uploader:     uploader,
		defaultModel: model,
	}
}

// Generate 同步调用网关生成图片，下载字节后经 uploadasset 落库。
func (s *Store) Generate(ctx context.Context, input GenerateInput) (Result, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Result{}, fmt.Errorf("请输入提示词")
	}
	if len([]rune(prompt)) > 2000 {
		return Result{}, fmt.Errorf("提示词不能超过 2000 个字")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = s.defaultModel
	}
	size := strings.TrimSpace(input.Size)
	if size == "" {
		size = defaultSize
	}
	if !allowedSizes[size] {
		return Result{}, fmt.Errorf("图片尺寸只能是 1024x1024、1024x1536、1536x1024 或 auto")
	}

	data, contentType, err := s.client.Generate(ctx, model, prompt, size)
	if err != nil {
		return Result{}, err
	}

	ext := "png"
	if strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg") {
		ext = "jpg"
	} else if strings.Contains(contentType, "webp") {
		ext = "webp"
	}
	name := fmt.Sprintf("image-%s.%s", time.Now().Format("20060102150405"), ext)
	var objectKey string
	var objectURL string
	if s.uploader != nil {
		uploaded, err := s.uploader.Upload(ctx, storage.UploadInput{
			ContentType: contentType,
			Dir:         "image/generated",
			Filename:    name,
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return Result{}, err
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
	asset, err := s.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        data,
		Dir:         "image/generated",
		Name:        name,
		ObjectKey:   objectKey,
		ObjectURL:   objectURL,
		Size:        int64(len(data)),
	})
	if err != nil {
		return Result{}, err
	}
	previewURL := "/api/upload-assets/" + fmt.Sprint(asset.ID)
	resultURL := previewURL
	if isPublicHTTPURL(asset.ObjectURL) {
		resultURL = asset.ObjectURL
	}
	return Result{
		AssetID:     asset.ID,
		ContentType: contentType,
		Model:       model,
		ObjectURL:   asset.ObjectURL,
		PreviewURL:  previewURL,
		Prompt:      prompt,
		Size:        size,
		URL:         resultURL,
	}, nil
}

func isPublicHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}

// Client 是 OpenAI 兼容文生图网关的最小 HTTP 客户端。
type Client struct {
	apiBase string
	apiKey  string
	client  *http.Client
}

func NewClient(cfg config.ImageConfig) *Client {
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
		return fmt.Errorf("文生图网关未配置 IMAGE_API_BASE")
	}
	if c.apiKey == "" {
		return fmt.Errorf("文生图网关未配置 IMAGE_API_KEY")
	}
	return nil
}

// Generate 调用 POST /v1/images/generations 同步生成单张图片，
// 返回解码后的图片字节与 content-type。gpt-image 系列固定返回 b64_json。
func (c *Client) Generate(ctx context.Context, model, prompt, size string) ([]byte, string, error) {
	if err := c.ensureReady(); err != nil {
		return nil, "", err
	}
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}
	raw, _ := json.Marshal(body)
	reqURL, err := url.Parse(c.apiBase + "/v1/images/generations")
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	payload, err := c.doJSON(req)
	if err != nil {
		return nil, "", err
	}
	return decodeImage(payload)
}

// decodeImage 从 {data:[{b64_json:...}]} 中提取并解码首张图片。
func decodeImage(payload map[string]any) ([]byte, string, error) {
	dataArr, ok := payload["data"].([]any)
	if !ok || len(dataArr) == 0 {
		if msg := findString(payload, "error.message", "error", "message"); msg != "" {
			return nil, "", fmt.Errorf("文生图网关返回错误: %s", msg)
		}
		return nil, "", fmt.Errorf("文生图网关未返回图片数据")
	}
	first, ok := dataArr[0].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("文生图网关返回格式异常")
	}
	b64, _ := first["b64_json"].(string)
	if strings.TrimSpace(b64) == "" {
		return nil, "", fmt.Errorf("文生图网关未返回 b64_json 图片数据")
	}
	bin, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, "", fmt.Errorf("解码图片数据失败: %w", err)
	}
	return bin, sniffContentType(bin), nil
}

// sniffContentType 通过魔数判断图片类型，默认 image/png。
func sniffContentType(data []byte) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return "image/png"
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
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("文生图网关返回 HTTP %d: %s", resp.StatusCode, compactBody(raw))
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

// findString 按点分路径在嵌套 map 中查找字符串值。
func findString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		parts := strings.Split(path, ".")
		var current any = payload
		ok := true
		for _, part := range parts {
			m, isMap := current.(map[string]any)
			if !isMap {
				ok = false
				break
			}
			current, ok = m[part]
			if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		if s, isStr := current.(string); isStr && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func compactBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
