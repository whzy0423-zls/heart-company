package xinzhili

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type VoiceConfigStatus string

const (
	VoiceConfigStatusDraft    VoiceConfigStatus = "draft"
	VoiceConfigStatusActive   VoiceConfigStatus = "active"
	VoiceConfigStatusInactive VoiceConfigStatus = "inactive"
	VoiceConfigStatusArchived VoiceConfigStatus = "archived"
)

type VoiceConfig struct {
	ID           int64             `json:"id"`
	Version      int64             `json:"version"`
	Status       VoiceConfigStatus `json:"status"`
	TTS          TTSConfig         `json:"tts"`
	APIKeySet    bool              `json:"apiKeySet"`
	APIKeySuffix string            `json:"apiKeySuffix"`
	CreateTime   time.Time         `json:"createTime"`
	UpdateTime   time.Time         `json:"updateTime"`
}

type VoiceCleanupJob struct {
	ID            int64     `json:"id"`
	ConfigVersion int64     `json:"configVersion"`
	RemoteVoiceID string    `json:"remoteVoiceId"`
	Provider      string    `json:"provider"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"lastError"`
	ScheduledAt   time.Time `json:"scheduledAt"`
	CreateTime    time.Time `json:"createTime"`
	UpdateTime    time.Time `json:"updateTime"`
}

type voiceConfigRecord struct {
	ID               int64
	Version          int64
	Status           VoiceConfigStatus
	Provider         string
	Endpoint         string
	GroupID          string
	Model            string
	Voice            string
	Format           string
	APIKeyCiphertext string
	APIKeySuffix     string
	Config           TTSConfig
	CreateTime       time.Time
	UpdateTime       time.Time
}

type VoiceConfigStore struct {
	DB    *sql.DB
	Codec *VoiceSecretCodec
}

func NewVoiceConfigStore(db *sql.DB, codec *VoiceSecretCodec) *VoiceConfigStore {
	return &VoiceConfigStore{DB: db, Codec: codec}
}

func (s *VoiceConfigStore) ReadActive(ctx context.Context) (VoiceConfig, bool, error) {
	return s.readByStatus(ctx, VoiceConfigStatusActive)
}

func (s *VoiceConfigStore) ReadDraft(ctx context.Context) (VoiceConfig, bool, error) {
	return s.readByStatus(ctx, VoiceConfigStatusDraft)
}

func (s *VoiceConfigStore) SaveDraft(ctx context.Context, incoming TTSConfig, expectedVersion int64) (VoiceConfig, error) {
	if s == nil || s.DB == nil {
		return VoiceConfig{}, errors.New("数据库未初始化，无法保存芯之力音色配置")
	}
	c, cancel := configContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(c, nil)
	if err != nil {
		return VoiceConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockVoiceConfig(c, tx); err != nil {
		return VoiceConfig{}, err
	}
	current, found, err := readVoiceConfigRecordTx(c, tx, VoiceConfigStatusDraft)
	if err != nil {
		return VoiceConfig{}, err
	}
	if !found {
		current, found, err = readVoiceConfigRecordTx(c, tx, VoiceConfigStatusActive)
		if err != nil {
			return VoiceConfig{}, err
		}
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return VoiceConfig{}, ErrConfigConflict
	}
	prepared, err := prepareVoiceConfigForPersist(incoming, current, s.Codec)
	if err != nil {
		return VoiceConfig{}, err
	}
	nextVersion, err := nextVoiceConfigVersion(c, tx)
	if err != nil {
		return VoiceConfig{}, err
	}
	if _, err := tx.ExecContext(c, `UPDATE app_xinzhili_voice_configs SET status='archived', update_time=now() WHERE status='draft'`); err != nil {
		return VoiceConfig{}, err
	}
	prepared.Version, prepared.Status = nextVersion, VoiceConfigStatusDraft
	inserted, err := insertVoiceConfigRecordTx(c, tx, prepared)
	if err != nil {
		return VoiceConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return VoiceConfig{}, err
	}
	return inserted.toConfig(s.Codec)
}

func (s *VoiceConfigStore) Activate(ctx context.Context, expectedVersion int64) (VoiceConfig, error) {
	if s == nil || s.DB == nil {
		return VoiceConfig{}, errors.New("数据库未初始化，无法启用芯之力音色配置")
	}
	c, cancel := configContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(c, nil)
	if err != nil {
		return VoiceConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockVoiceConfig(c, tx); err != nil {
		return VoiceConfig{}, err
	}
	draft, found, err := readVoiceConfigRecordTx(c, tx, VoiceConfigStatusDraft)
	if err != nil {
		return VoiceConfig{}, err
	}
	if !found || draft.Version != expectedVersion {
		return VoiceConfig{}, ErrConfigConflict
	}
	if _, err := tx.ExecContext(c, `UPDATE app_xinzhili_voice_configs SET status='inactive', update_time=now() WHERE status='active'`); err != nil {
		return VoiceConfig{}, err
	}
	var active voiceConfigRecord
	if err := tx.QueryRowContext(c, `UPDATE app_xinzhili_voice_configs SET status='active', update_time=now() WHERE version=$1 AND status='draft' RETURNING `+voiceConfigColumns, expectedVersion).Scan(voiceConfigScanDest(&active)...); err != nil {
		return VoiceConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return VoiceConfig{}, err
	}
	return active.toConfig(s.Codec)
}

func (s *VoiceConfigStore) Deactivate(ctx context.Context, expectedVersion int64) error {
	if s == nil || s.DB == nil {
		return errors.New("数据库未初始化，无法停用芯之力音色配置")
	}
	c, cancel := configContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockVoiceConfig(c, tx); err != nil {
		return err
	}
	active, found, err := readVoiceConfigRecordTx(c, tx, VoiceConfigStatusActive)
	if err != nil {
		return err
	}
	if !found || active.Version != expectedVersion {
		return ErrConfigConflict
	}
	result, err := tx.ExecContext(c, `UPDATE app_xinzhili_voice_configs SET status='inactive', update_time=now() WHERE version=$1 AND status='active'`, expectedVersion)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ErrConfigConflict
	}
	return tx.Commit()
}

func (s *VoiceConfigStore) Restore(ctx context.Context, version, expectedVersion int64) (VoiceConfig, error) {
	if s == nil || s.DB == nil {
		return VoiceConfig{}, errors.New("数据库未初始化，无法恢复芯之力音色配置")
	}
	if version <= 0 {
		return VoiceConfig{}, ErrConfigConflict
	}
	c, cancel := configContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(c, nil)
	if err != nil {
		return VoiceConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockVoiceConfig(c, tx); err != nil {
		return VoiceConfig{}, err
	}
	current, found, err := readVoiceConfigRecordTx(c, tx, VoiceConfigStatusDraft)
	if err != nil {
		return VoiceConfig{}, err
	}
	if !found {
		current, found, err = readVoiceConfigRecordTx(c, tx, VoiceConfigStatusActive)
		if err != nil {
			return VoiceConfig{}, err
		}
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return VoiceConfig{}, ErrConfigConflict
	}
	source, err := readVoiceConfigRecordByVersionTx(c, tx, version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VoiceConfig{}, ErrConfigConflict
		}
		return VoiceConfig{}, err
	}
	nextVersion, err := nextVoiceConfigVersion(c, tx)
	if err != nil {
		return VoiceConfig{}, err
	}
	if _, err := tx.ExecContext(c, `UPDATE app_xinzhili_voice_configs SET status='archived', update_time=now() WHERE status='draft'`); err != nil {
		return VoiceConfig{}, err
	}
	source.ID, source.Version, source.Status = 0, nextVersion, VoiceConfigStatusDraft
	inserted, err := insertVoiceConfigRecordTx(c, tx, source)
	if err != nil {
		return VoiceConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return VoiceConfig{}, err
	}
	return inserted.toConfig(s.Codec)
}

func (s *VoiceConfigStore) ScheduleRemoteDelete(ctx context.Context, configVersion int64, provider, remoteVoiceID string) (VoiceCleanupJob, error) {
	if s == nil || s.DB == nil {
		return VoiceCleanupJob{}, errors.New("数据库未初始化，无法创建芯之力音色清理任务")
	}
	c, cancel := configContext(ctx)
	defer cancel()
	var job VoiceCleanupJob
	err := s.DB.QueryRowContext(c, `INSERT INTO app_xinzhili_voice_cleanup_jobs(config_version, provider, remote_voice_id, status, scheduled_at)
		VALUES ($1,$2,$3,'pending',now()) RETURNING id, config_version, remote_voice_id, provider, status, attempts, last_error, scheduled_at, create_time, update_time`,
		configVersion, strings.TrimSpace(provider), strings.TrimSpace(remoteVoiceID)).Scan(&job.ID, &job.ConfigVersion, &job.RemoteVoiceID, &job.Provider, &job.Status, &job.Attempts, &job.LastError, &job.ScheduledAt, &job.CreateTime, &job.UpdateTime)
	return job, err
}

func prepareVoiceConfigForPersist(incoming TTSConfig, current voiceConfigRecord, codec *VoiceSecretCodec) (voiceConfigRecord, error) {
	normalized, err := normalizeVoiceTTS(incoming)
	if err != nil {
		return voiceConfigRecord{}, err
	}
	record := voiceConfigRecord{
		Provider: normalized.Provider,
		Endpoint: normalized.Endpoint,
		GroupID:  normalized.GroupID,
		Model:    normalized.Model,
		Voice:    normalized.Voice,
		Format:   normalized.Format,
		Config:   normalized,
	}
	if normalized.Provider == TTSProviderAliyunCosyVoice {
		record.Config.APIKey = ""
		if normalized.APIKey != "" {
			if codec == nil {
				return voiceConfigRecord{}, ErrVoiceSecretKeyInvalid
			}
			ciphertext, err := codec.Encrypt(normalized.APIKey)
			if err != nil {
				return voiceConfigRecord{}, err
			}
			record.APIKeyCiphertext = ciphertext
			record.APIKeySuffix = secretSuffix(normalized.APIKey)
			return record, nil
		}
		if current.Provider == TTSProviderAliyunCosyVoice && current.APIKeyCiphertext != "" {
			record.APIKeyCiphertext = current.APIKeyCiphertext
			record.APIKeySuffix = current.APIKeySuffix
			return record, nil
		}
		return voiceConfigRecord{}, errors.New("阿里 CosyVoice TTS API Key 不能为空")
	}
	record.APIKeySuffix = secretSuffix(normalized.APIKey)
	return record, nil
}

func normalizeVoiceTTS(cfg TTSConfig) (TTSConfig, error) {
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.GroupID = strings.TrimSpace(cfg.GroupID)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.Voice = strings.TrimSpace(cfg.Voice)
	cfg.Format = strings.TrimSpace(cfg.Format)
	if cfg.Format == "" {
		cfg.Format = "mp3"
	}
	if cfg.Provider == TTSProviderOpenAICompatible {
		cfg.GroupID = ""
	}
	if cfg.Provider != TTSProviderOpenAICompatible && cfg.Provider != TTSProviderMiniMax && cfg.Provider != TTSProviderAliyunCosyVoice {
		return TTSConfig{}, errors.New("TTS provider 仅支持 openai-compatible、minimax 或 aliyun-cosyvoice")
	}
	if cfg.Endpoint == "" || cfg.Model == "" || cfg.Voice == "" {
		return TTSConfig{}, errors.New("TTS endpoint、模型和音色不能为空")
	}
	if cfg.Provider == TTSProviderMiniMax && cfg.GroupID == "" {
		return TTSConfig{}, errors.New("MiniMax TTS 必须配置 GroupID")
	}
	if cfg.Provider == TTSProviderAliyunCosyVoice && cfg.GroupID == "" {
		return TTSConfig{}, errors.New("阿里 CosyVoice TTS 必须配置业务空间")
	}
	if cfg.Format != "mp3" {
		return TTSConfig{}, errors.New("TTS format 必须为 mp3")
	}
	if cfg.Provider == TTSProviderAliyunCosyVoice {
		if err := validateEndpoint(cfg.Endpoint, "wss", "https", "ws"); err != nil {
			return TTSConfig{}, fmt.Errorf("TTS endpoint: %w", err)
		}
		return cfg, nil
	}
	if err := validateEndpoint(cfg.Endpoint, "https"); err != nil {
		return TTSConfig{}, fmt.Errorf("TTS endpoint: %w", err)
	}
	return cfg, nil
}

func (r voiceConfigRecord) toConfig(codec *VoiceSecretCodec) (VoiceConfig, error) {
	cfg := r.Config
	if cfg.Provider == "" {
		cfg = TTSConfig{Provider: r.Provider, Endpoint: r.Endpoint, GroupID: r.GroupID, Model: r.Model, Voice: r.Voice, Format: r.Format}
	}
	if cfg.Provider == TTSProviderAliyunCosyVoice && r.APIKeyCiphertext != "" {
		if codec == nil {
			return VoiceConfig{}, ErrVoiceSecretKeyInvalid
		}
		plaintext, err := codec.Decrypt(r.APIKeyCiphertext)
		if err != nil {
			return VoiceConfig{}, err
		}
		cfg.APIKey = plaintext
	}
	return VoiceConfig{ID: r.ID, Version: r.Version, Status: r.Status, TTS: cfg, APIKeySet: cfg.APIKey != "" || r.APIKeyCiphertext != "", APIKeySuffix: r.APIKeySuffix, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime}, nil
}

func secretSuffix(secret string) string {
	runes := []rune(strings.TrimSpace(secret))
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}

const voiceConfigColumns = `id, version, status, provider, endpoint, group_id, model, voice, format, api_key_ciphertext, api_key_suffix, config, create_time, update_time`

func lockVoiceConfig(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:xinzhili-voice-config', 0))`)
	return err
}

func (s *VoiceConfigStore) readByStatus(ctx context.Context, status VoiceConfigStatus) (VoiceConfig, bool, error) {
	if s == nil || s.DB == nil {
		return VoiceConfig{}, false, nil
	}
	c, cancel := configContext(ctx)
	defer cancel()
	var record voiceConfigRecord
	query := `SELECT ` + voiceConfigColumns + ` FROM app_xinzhili_voice_configs WHERE status=$1 ORDER BY version DESC LIMIT 1`
	if err := s.DB.QueryRowContext(c, query, string(status)).Scan(voiceConfigScanDest(&record)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VoiceConfig{}, false, nil
		}
		return VoiceConfig{}, false, err
	}
	cfg, err := record.toConfig(s.Codec)
	if err != nil {
		return VoiceConfig{}, false, err
	}
	return cfg, true, nil
}

func readVoiceConfigRecordTx(ctx context.Context, tx *sql.Tx, status VoiceConfigStatus) (voiceConfigRecord, bool, error) {
	var record voiceConfigRecord
	query := `SELECT ` + voiceConfigColumns + ` FROM app_xinzhili_voice_configs WHERE status=$1 ORDER BY version DESC LIMIT 1 FOR UPDATE`
	if err := tx.QueryRowContext(ctx, query, string(status)).Scan(voiceConfigScanDest(&record)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return voiceConfigRecord{}, false, nil
		}
		return voiceConfigRecord{}, false, err
	}
	return record, true, nil
}

func readVoiceConfigRecordByVersionTx(ctx context.Context, tx *sql.Tx, version int64) (voiceConfigRecord, error) {
	var record voiceConfigRecord
	query := `SELECT ` + voiceConfigColumns + ` FROM app_xinzhili_voice_configs WHERE version=$1 FOR UPDATE`
	if err := tx.QueryRowContext(ctx, query, version).Scan(voiceConfigScanDest(&record)...); err != nil {
		return voiceConfigRecord{}, err
	}
	return record, nil
}

func nextVoiceConfigVersion(ctx context.Context, tx *sql.Tx) (int64, error) {
	var maxVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version), 0) FROM app_xinzhili_voice_configs`).Scan(&maxVersion); err != nil {
		return 0, err
	}
	return maxVersion + 1, nil
}

func insertVoiceConfigRecordTx(ctx context.Context, tx *sql.Tx, record voiceConfigRecord) (voiceConfigRecord, error) {
	body, err := json.Marshal(record.Config)
	if err != nil {
		return voiceConfigRecord{}, err
	}
	var inserted voiceConfigRecord
	query := `INSERT INTO app_xinzhili_voice_configs(version, status, provider, endpoint, group_id, model, voice, format, api_key_ciphertext, api_key_suffix, config)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb) RETURNING ` + voiceConfigColumns
	err = tx.QueryRowContext(ctx, query,
		record.Version, string(record.Status), record.Provider, record.Endpoint, record.GroupID, record.Model, record.Voice, record.Format, record.APIKeyCiphertext, record.APIKeySuffix, string(body),
	).Scan(voiceConfigScanDest(&inserted)...)
	return inserted, err
}

func voiceConfigScanDest(record *voiceConfigRecord) []any {
	var raw []byte
	return []any{&record.ID, &record.Version, &record.Status, &record.Provider, &record.Endpoint, &record.GroupID, &record.Model, &record.Voice, &record.Format, &record.APIKeyCiphertext, &record.APIKeySuffix, voiceConfigJSONScanner{target: &record.Config, raw: &raw}, &record.CreateTime, &record.UpdateTime}
}

type voiceConfigJSONScanner struct {
	target *TTSConfig
	raw    *[]byte
}

func (s voiceConfigJSONScanner) Scan(value any) error {
	if s.target == nil {
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case nil:
		data = []byte(`{}`)
	default:
		return fmt.Errorf("unsupported voice config json type %T", value)
	}
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	if s.raw != nil {
		*s.raw = append((*s.raw)[:0], data...)
	}
	return json.Unmarshal(data, s.target)
}
