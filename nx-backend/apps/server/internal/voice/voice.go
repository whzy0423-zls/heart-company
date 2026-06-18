package voice

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

type Store struct {
	client  *MiniMaxClient
	db      *sql.DB
	uploads *uploadasset.Store
}

type Profile struct {
	CreateTime    time.Time `json:"createTime"`
	ID            string    `json:"id"`
	LastError     string    `json:"lastError"`
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	Remark        string    `json:"remark"`
	SampleAssetID string    `json:"sampleAssetId"`
	SampleName    string    `json:"sampleName"`
	SampleURL     string    `json:"sampleUrl"`
	Status        string    `json:"status"`
	UpdateTime    time.Time `json:"updateTime"`
	VoiceID       string    `json:"voiceId"`
}

type Generation struct {
	AudioAssetID string    `json:"audioAssetId"`
	AudioURL     string    `json:"audioUrl"`
	CreateTime   time.Time `json:"createTime"`
	ErrorMessage string    `json:"errorMessage"`
	ID           string    `json:"id"`
	Model        string    `json:"model"`
	ProfileID    string    `json:"profileId"`
	Provider     string    `json:"provider"`
	Status       string    `json:"status"`
	Text         string    `json:"text"`
	VoiceID      string    `json:"voiceId"`
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type CreateProfileInput struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Remark        string `json:"remark"`
	SampleAssetID string `json:"sampleAssetId"`
	SampleName    string `json:"sampleName"`
	SampleURL     string `json:"sampleUrl"`
	VoiceID       string `json:"voiceId"`
}

type GenerateInput struct {
	Model     string `json:"model"`
	ProfileID string `json:"profileId"`
	Text      string `json:"text"`
	VoiceID   string `json:"voiceId"`
}

func NewStore(database *sql.DB, uploads *uploadasset.Store, cfg config.MiniMaxConfig) *Store {
	return &Store{
		client:  NewMiniMaxClient(cfg),
		db:      database,
		uploads: uploads,
	}
}

func (s *Store) ListProfiles(ctx context.Context, query url.Values) (PageResult[Profile], error) {
	page, pageSize := pagination(query)
	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR voice_id ILIKE $%d)", len(args), len(args)))
	}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM voice_profiles WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Profile]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, name, provider, voice_id, COALESCE(sample_asset_id::text,''), sample_url, sample_name,
		        status, remark, last_error, create_time, update_time
		   FROM voice_profiles
		  WHERE `+condition+`
		  ORDER BY create_time DESC
		  LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Profile]{}, err
	}
	defer rows.Close()

	items := []Profile{}
	for rows.Next() {
		var item Profile
		if err := rows.Scan(&item.ID, &item.Name, &item.Provider, &item.VoiceID, &item.SampleAssetID, &item.SampleURL, &item.SampleName, &item.Status, &item.Remark, &item.LastError, &item.CreateTime, &item.UpdateTime); err != nil {
			return PageResult[Profile]{}, err
		}
		items = append(items, item)
	}
	return PageResult[Profile]{Items: items, Total: total}, rows.Err()
}

func (s *Store) CreateProfile(ctx context.Context, input CreateProfileInput) (Profile, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Profile{}, fmt.Errorf("请输入人声名称")
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "minimax"
	}
	sampleID, err := parseOptionalID(input.SampleAssetID)
	if err != nil || sampleID == 0 {
		return Profile{}, fmt.Errorf("请先上传音频样本")
	}
	voiceID := strings.TrimSpace(input.VoiceID)
	if voiceID == "" {
		voiceID = "nx_voice_" + randomID(10)
	}

	var id string
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO voice_profiles (name, provider, voice_id, sample_asset_id, sample_url, sample_name, status, remark)
		 VALUES ($1,$2,$3,$4,$5,$6,'draft',$7)
		 RETURNING id::text`,
		name, provider, voiceID, sampleID, strings.TrimSpace(input.SampleURL), strings.TrimSpace(input.SampleName), strings.TrimSpace(input.Remark),
	).Scan(&id)
	if err != nil {
		return Profile{}, err
	}
	profile, err := s.CloneProfile(ctx, id)
	if err != nil {
		saved, findErr := s.Profile(ctx, id)
		if findErr == nil {
			return saved, nil
		}
		return Profile{}, err
	}
	return profile, nil
}

func (s *Store) CloneProfile(ctx context.Context, id string) (Profile, error) {
	profile, err := s.Profile(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	assetID, err := parseOptionalID(profile.SampleAssetID)
	if err != nil || assetID == 0 {
		return Profile{}, fmt.Errorf("音频样本不存在")
	}
	asset, err := s.uploads.Find(ctx, assetID)
	if err != nil {
		return Profile{}, fmt.Errorf("读取音频样本失败: %w", err)
	}
	if err := s.setProfileStatus(ctx, id, "cloning", ""); err != nil {
		return Profile{}, err
	}
	fileID, err := s.client.UploadCloneAudio(ctx, asset.Name, asset.ContentType, asset.Data)
	if err != nil {
		_ = s.setProfileStatus(ctx, id, "failed", err.Error())
		return Profile{}, err
	}
	voiceID := strings.TrimSpace(profile.VoiceID)
	if voiceID == "" {
		voiceID = "nx_voice_" + randomID(10)
	}
	if err := s.client.CloneVoice(ctx, fileID, voiceID); err != nil {
		_ = s.setProfileStatus(ctx, id, "failed", err.Error())
		return Profile{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE voice_profiles
		    SET voice_id=$1, status='ready', last_error='', update_time=now()
		  WHERE id=$2`,
		voiceID, id,
	); err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, id)
}

func (s *Store) Profile(ctx context.Context, id string) (Profile, error) {
	var item Profile
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text, name, provider, voice_id, COALESCE(sample_asset_id::text,''), sample_url, sample_name,
		        status, remark, last_error, create_time, update_time
		   FROM voice_profiles
		  WHERE id=$1`,
		id,
	).Scan(&item.ID, &item.Name, &item.Provider, &item.VoiceID, &item.SampleAssetID, &item.SampleURL, &item.SampleName, &item.Status, &item.Remark, &item.LastError, &item.CreateTime, &item.UpdateTime)
	return item, err
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM voice_profiles WHERE id=$1`, id)
	return err
}

func (s *Store) Generate(ctx context.Context, input GenerateInput) (Generation, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return Generation{}, fmt.Errorf("请输入测试文本")
	}
	if len([]rune(text)) > 1000 {
		return Generation{}, fmt.Errorf("测试文本不能超过 1000 个字")
	}
	profile, err := s.Profile(ctx, input.ProfileID)
	if err != nil {
		return Generation{}, fmt.Errorf("请选择可用音色")
	}
	if profile.Status != "ready" {
		return Generation{}, fmt.Errorf("当前音色还未克隆完成")
	}
	voiceID := strings.TrimSpace(input.VoiceID)
	if voiceID == "" {
		voiceID = profile.VoiceID
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "speech-02-hd"
	}
	audio, contentType, err := s.client.TextToAudio(ctx, model, voiceID, text)
	if err != nil {
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO voice_generations (profile_id, provider, voice_id, text, model, status, error_message)
			 VALUES ($1,$2,$3,$4,$5,'failed',$6)`,
			input.ProfileID, profile.Provider, voiceID, text, model, err.Error(),
		)
		return Generation{}, err
	}
	asset, err := s.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        audio,
		Dir:         "voice/generated",
		Name:        fmt.Sprintf("voice-%s.mp3", time.Now().Format("20060102150405")),
		Size:        int64(len(audio)),
	})
	if err != nil {
		return Generation{}, err
	}
	audioURL := "/api/upload-assets/" + fmt.Sprint(asset.ID)
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO voice_generations (profile_id, provider, voice_id, text, model, audio_asset_id, audio_url, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'success')
		 RETURNING id::text`,
		input.ProfileID, profile.Provider, voiceID, text, model, asset.ID, audioURL,
	).Scan(&id); err != nil {
		return Generation{}, err
	}
	return s.Generation(ctx, id)
}

func (s *Store) ListGenerations(ctx context.Context, query url.Values) (PageResult[Generation], error) {
	page, pageSize := pagination(query)
	args := []any{}
	where := []string{"1=1"}
	if profileID := strings.TrimSpace(query.Get("profileId")); profileID != "" {
		args = append(args, profileID)
		where = append(where, fmt.Sprintf("profile_id=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM voice_generations WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Generation]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, COALESCE(profile_id::text,''), provider, voice_id, text, model, COALESCE(audio_asset_id::text,''),
		        audio_url, status, error_message, create_time
		   FROM voice_generations
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
		if err := rows.Scan(&item.ID, &item.ProfileID, &item.Provider, &item.VoiceID, &item.Text, &item.Model, &item.AudioAssetID, &item.AudioURL, &item.Status, &item.ErrorMessage, &item.CreateTime); err != nil {
			return PageResult[Generation]{}, err
		}
		items = append(items, item)
	}
	return PageResult[Generation]{Items: items, Total: total}, rows.Err()
}

func (s *Store) Generation(ctx context.Context, id string) (Generation, error) {
	var item Generation
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text, COALESCE(profile_id::text,''), provider, voice_id, text, model, COALESCE(audio_asset_id::text,''),
		        audio_url, status, error_message, create_time
		   FROM voice_generations
		  WHERE id=$1`,
		id,
	).Scan(&item.ID, &item.ProfileID, &item.Provider, &item.VoiceID, &item.Text, &item.Model, &item.AudioAssetID, &item.AudioURL, &item.Status, &item.ErrorMessage, &item.CreateTime)
	return item, err
}

func (s *Store) setProfileStatus(ctx context.Context, id string, status string, lastError string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE voice_profiles SET status=$1, last_error=$2, update_time=now() WHERE id=$3`,
		status, lastError, id,
	)
	return err
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

func parseOptionalID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	var id int64
	_, err := fmt.Sscan(value, &id)
	return id, err
}

func randomID(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func parseInt64(value string) (int64, bool) {
	var id int64
	if _, err := fmt.Sscan(strings.TrimSpace(value), &id); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

type MiniMaxClient struct {
	apiBase string
	apiKey  string
	client  *http.Client
	groupID string
}

func NewMiniMaxClient(cfg config.MiniMaxConfig) *MiniMaxClient {
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.minimax.io"
	}
	return &MiniMaxClient{
		apiBase: apiBase,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  &http.Client{Timeout: 120 * time.Second},
		groupID: strings.TrimSpace(cfg.GroupID),
	}
}

func (c *MiniMaxClient) UploadCloneAudio(ctx context.Context, filename string, contentType string, data []byte) (string, error) {
	if err := c.ensureReady(); err != nil {
		return "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("purpose", "voice_clone")
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/files/upload"), &body)
	if err != nil {
		return "", err
	}
	c.auth(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	payload, err := c.doJSON(req)
	if err != nil {
		return "", err
	}
	fileID := findString(payload, "file.file_id", "file.id", "file_id", "id")
	if fileID == "" {
		return "", fmt.Errorf("MiniMax 上传成功但未返回 file_id")
	}
	return fileID, nil
}

func (c *MiniMaxClient) CloneVoice(ctx context.Context, fileID string, voiceID string) error {
	if err := c.ensureReady(); err != nil {
		return err
	}
	body := map[string]any{
		"voice_id": voiceID,
	}
	if numericFileID, ok := parseInt64(fileID); ok {
		body["file_id"] = numericFileID
	} else {
		body["file_id"] = fileID
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/voice_clone"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	_, err = c.doJSON(req)
	return err
}

func (c *MiniMaxClient) TextToAudio(ctx context.Context, model string, voiceID string, text string) ([]byte, string, error) {
	if err := c.ensureReady(); err != nil {
		return nil, "", err
	}
	body := map[string]any{
		"audio_setting": map[string]any{
			"bitrate":     128000,
			"channel":     1,
			"format":      "mp3",
			"sample_rate": 32_000,
		},
		"model":  model,
		"stream": false,
		"text":   text,
		"voice_setting": map[string]any{
			"pitch":    0,
			"speed":    1,
			"voice_id": voiceID,
			"vol":      1,
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/t2a_v2"), bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	result, err := c.doJSON(req)
	if err != nil {
		return nil, "", err
	}
	audioValue := findString(result, "data.audio", "audio", "data.audio_url", "audio_url")
	if audioValue == "" {
		return nil, "", fmt.Errorf("MiniMax 未返回音频数据")
	}
	if strings.HasPrefix(audioValue, "http://") || strings.HasPrefix(audioValue, "https://") {
		return c.download(ctx, audioValue)
	}
	if decoded, err := hex.DecodeString(audioValue); err == nil && len(decoded) > 0 {
		return decoded, "audio/mpeg", nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(audioValue); err == nil && len(decoded) > 0 {
		return decoded, "audio/mpeg", nil
	}
	return nil, "", fmt.Errorf("MiniMax 音频格式无法识别")
}

func (c *MiniMaxClient) ensureReady() error {
	if c.apiKey == "" {
		return fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}
	return nil
}

func (c *MiniMaxClient) endpoint(path string) string {
	endpoint := c.apiBase + path
	if c.groupID == "" {
		return endpoint
	}
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + "GroupId=" + url.QueryEscape(c.groupID)
}

func (c *MiniMaxClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func (c *MiniMaxClient) doJSON(req *http.Request) (map[string]any, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	var payload map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MiniMax 请求失败(%d): %s", resp.StatusCode, compactBody(raw))
	}
	if err := minimaxBaseError(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *MiniMaxClient) download(ctx context.Context, audioURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("下载 MiniMax 音频失败(%d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return data, contentType, nil
}

func minimaxBaseError(payload map[string]any) error {
	if len(payload) == 0 {
		return nil
	}
	base, ok := payload["base_resp"].(map[string]any)
	if !ok {
		return nil
	}
	code := fmt.Sprint(base["status_code"])
	if code == "" || code == "0" || code == "<nil>" {
		return nil
	}
	message := fmt.Sprint(base["status_msg"])
	if message == "" || message == "<nil>" {
		message = "MiniMax 调用失败"
	}
	return errors.New(message)
}

func findString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		current := any(payload)
		for _, part := range strings.Split(path, ".") {
			m, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = m[part]
		}
		if value, ok := current.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		switch value := current.(type) {
		case float64:
			if value != 0 {
				return fmt.Sprintf("%.0f", value)
			}
		case json.Number:
			return value.String()
		}
	}
	return ""
}

func compactBody(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len([]rune(text)) > 500 {
		return string([]rune(text)[:500])
	}
	return text
}
