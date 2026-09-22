// Package voicebroadcastconfig owns the global configuration used by the
// optional conversation voice-broadcast channel. Secrets are encrypted at
// rest and are deliberately kept out of the public View type.
package voicebroadcastconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	DefaultProvider    = "aliyun-bailian"
	DefaultEndpoint    = "https://dashscope.aliyuncs.com/api/v1"
	DefaultRegion      = "cn-beijing"
	DefaultModel       = "qwen3-tts-instruct-flash"
	DefaultFemaleVoice = "Cherry"
	maxConfigString    = 512
	maxAPIKeyRunes     = 4096
	configRowID        = 1
)

var (
	ErrConflict            = errors.New("voice broadcast configuration version conflict")
	ErrDatabaseUnavailable = errors.New("voice broadcast configuration database is not initialized")
	ErrSecretUnavailable   = errors.New("voice broadcast secret encryption is not configured")
)

// Voice is an administrator-selectable Bailian voice.
type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Health is the last test-synthesis result. Message is an administrator-safe
// category/message and must never contain an API key or raw provider body.
type Health struct {
	Status        string    `json:"status"`
	OK            bool      `json:"ok"`
	LatencyMS     int64     `json:"latencyMs"`
	ErrorCategory string    `json:"errorCategory,omitempty"`
	Message       string    `json:"message,omitempty"`
	TestedAt      time.Time `json:"testedAt,omitempty"`
}

// Config contains the runtime secret as well as persistence metadata. APIKey
// and APIKeyCiphertext are intentionally omitted from JSON serialization.
type Config struct {
	Version          int64     `json:"version"`
	Enabled          bool      `json:"enabled"`
	Provider         string    `json:"provider"`
	Region           string    `json:"region"`
	WorkspaceID      string    `json:"workspaceId"`
	Model            string    `json:"model"`
	DefaultVoice     string    `json:"defaultVoice"`
	CurrentVoice     string    `json:"currentVoice"`
	Voices           []Voice   `json:"voices"`
	Health           Health    `json:"health"`
	UpdatedAt        time.Time `json:"updatedAt,omitempty"`
	APIKey           string    `json:"-"`
	APIKeySet        bool      `json:"-"`
	APIKeySuffix     string    `json:"-"`
	APIKeyCiphertext string    `json:"-"`
}

// View is the safe admin/API representation. It contains only a masked key
// state and can be serialized without a secret leak.
type View struct {
	Version      int64   `json:"version"`
	Enabled      bool    `json:"enabled"`
	Provider     string  `json:"provider"`
	Region       string  `json:"region"`
	WorkspaceID  string  `json:"workspaceId"`
	Model        string  `json:"model"`
	DefaultVoice string  `json:"defaultVoice"`
	CurrentVoice string  `json:"currentVoice"`
	Voices       []Voice `json:"voices"`
	APIKeySet    bool    `json:"apiKeySet"`
	APIKeySuffix string  `json:"apiKeySuffix,omitempty"`
	Health       Health  `json:"health"`
	UpdatedAt    string  `json:"updatedAt,omitempty"`
}

// UpdateInput is the full editable configuration. Empty APIKey preserves the
// existing encrypted key unless ClearAPIKey is true.
type UpdateInput struct {
	Enabled      *bool   `json:"enabled"`
	Provider     string  `json:"provider"`
	Region       string  `json:"region"`
	WorkspaceID  string  `json:"workspaceId"`
	Model        string  `json:"model"`
	DefaultVoice string  `json:"defaultVoice"`
	CurrentVoice string  `json:"currentVoice"`
	Voices       []Voice `json:"voices"`
	APIKey       string  `json:"apiKey"`
	ClearAPIKey  bool    `json:"clearApiKey"`
}

// DefaultConfig returns the disabled, first-run configuration. The default
// female voice is Cherry, which is the standard Qwen Bailian female voice.
func DefaultConfig() Config {
	voices := defaultVoices()
	return Config{
		Provider:     DefaultProvider,
		Region:       DefaultRegion,
		Model:        DefaultModel,
		DefaultVoice: DefaultFemaleVoice,
		CurrentVoice: DefaultFemaleVoice,
		Voices:       voices,
		Health:       Health{Status: "unknown", Message: "尚未测试"},
	}
}

func defaultVoices() []Voice {
	return []Voice{
		{ID: "Cherry", Name: "Cherry（默认女声）", Provider: DefaultProvider, Model: DefaultModel},
		{ID: "Serena", Name: "Serena", Provider: DefaultProvider, Model: DefaultModel},
		{ID: "Ethan", Name: "Ethan", Provider: DefaultProvider, Model: DefaultModel},
	}
}

// BuildView strips all runtime and persistence secret fields.
func BuildView(cfg Config, found bool) View {
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg = mergeDefaults(cfg)
	}
	voices := append([]Voice(nil), cfg.Voices...)
	if len(voices) == 0 {
		voices = defaultVoices()
	}
	view := View{
		Version:      cfg.Version,
		Enabled:      cfg.Enabled,
		Provider:     cfg.Provider,
		Region:       cfg.Region,
		WorkspaceID:  cfg.WorkspaceID,
		Model:        cfg.Model,
		DefaultVoice: cfg.DefaultVoice,
		CurrentVoice: cfg.CurrentVoice,
		Voices:       voices,
		APIKeySet:    cfg.APIKeySet || strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(cfg.APIKeyCiphertext) != "",
		APIKeySuffix: cfg.APIKeySuffix,
		Health:       cfg.Health,
	}
	if !found && view.Health.Status == "" {
		view.Health = DefaultConfig().Health
	}
	if !cfg.UpdatedAt.IsZero() {
		view.UpdatedAt = cfg.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return view
}

// Store persists one global configuration row.
type Store struct {
	DB    *sql.DB
	Codec *xinzhili.VoiceSecretCodec
}

func NewStore(db *sql.DB, codec *xinzhili.VoiceSecretCodec) *Store {
	return &Store{DB: db, Codec: codec}
}

// ReadConfig is a small loader for runtime callers that do not need to keep a
// Store on Server. It returns defaults when the singleton row has not been
// created yet.
func ReadConfig(ctx context.Context, db *sql.DB, codec *xinzhili.VoiceSecretCodec) (Config, bool, error) {
	return NewStore(db, codec).Read(ctx)
}

func (s *Store) Read(ctx context.Context) (Config, bool, error) {
	if s == nil || s.DB == nil {
		return DefaultConfig(), false, nil
	}
	var row persistedRow
	err := s.DB.QueryRowContext(ctx, persistedSelectSQL, configRowID).Scan(rowScanDest(&row)...)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConfig(), false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	cfg, err := row.toConfig(s.Codec)
	if err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func (s *Store) Update(ctx context.Context, input UpdateInput, expectedVersion int64) (Config, error) {
	if s == nil || s.DB == nil {
		return Config{}, ErrDatabaseUnavailable
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:app-voice-broadcast-config',0))`); err != nil {
		return Config{}, err
	}
	currentRow, found, err := readRowTx(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	current := DefaultConfig()
	if found {
		current, err = currentRow.toConfig(s.Codec)
		if err != nil {
			return Config{}, err
		}
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return Config{}, ErrConflict
	}
	prepared, err := prepareUpdate(input, current, s.Codec)
	if err != nil {
		return Config{}, err
	}
	prepared.Version = current.Version + 1
	prepared.UpdatedAt = time.Now().UTC()
	if err := writeRowTx(ctx, tx, prepared, found); err != nil {
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, err
	}
	return prepared, nil
}

// RecordHealth stores the latest test result using the same optimistic version
// check as configuration updates. This prevents a stale test from overwriting
// a newer voice selection.
func (s *Store) RecordHealth(ctx context.Context, expectedVersion int64, health Health) (Config, error) {
	if s == nil || s.DB == nil {
		return Config{}, ErrDatabaseUnavailable
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:app-voice-broadcast-config',0))`); err != nil {
		return Config{}, err
	}
	row, found, err := readRowTx(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	current := DefaultConfig()
	if found {
		current, err = row.toConfig(s.Codec)
		if err != nil {
			return Config{}, err
		}
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return Config{}, ErrConflict
	}
	current.Version++
	current.Health = normalizeHealth(health)
	current.UpdatedAt = time.Now().UTC()
	if err := writeRowTx(ctx, tx, current, found); err != nil {
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, err
	}
	return current, nil
}

type persistedRow struct {
	ID               int64
	Version          int64
	Enabled          bool
	Provider         string
	Region           string
	WorkspaceID      string
	Model            string
	DefaultVoice     string
	CurrentVoice     string
	Voices           []byte
	APIKeyCiphertext string
	APIKeySuffix     string
	Health           []byte
	UpdatedAt        time.Time
}

const persistedSelectSQL = `SELECT id,version,enabled,provider,region,workspace_id,model,default_voice,current_voice,voices,api_key_ciphertext,api_key_suffix,health,update_time FROM app_voice_broadcast_configs WHERE id=$1`

func rowScanDest(row *persistedRow) []any {
	return []any{&row.ID, &row.Version, &row.Enabled, &row.Provider, &row.Region, &row.WorkspaceID, &row.Model, &row.DefaultVoice, &row.CurrentVoice, &row.Voices, &row.APIKeyCiphertext, &row.APIKeySuffix, &row.Health, &row.UpdatedAt}
}

func (r persistedRow) toConfig(codec *xinzhili.VoiceSecretCodec) (Config, error) {
	cfg := Config{
		Version:          r.Version,
		Enabled:          r.Enabled,
		Provider:         r.Provider,
		Region:           r.Region,
		WorkspaceID:      r.WorkspaceID,
		Model:            r.Model,
		DefaultVoice:     r.DefaultVoice,
		CurrentVoice:     r.CurrentVoice,
		APIKeyCiphertext: r.APIKeyCiphertext,
		APIKeySuffix:     r.APIKeySuffix,
		APIKeySet:        strings.TrimSpace(r.APIKeyCiphertext) != "",
		UpdatedAt:        r.UpdatedAt,
	}
	if len(r.Voices) > 0 {
		if err := json.Unmarshal(r.Voices, &cfg.Voices); err != nil {
			return Config{}, fmt.Errorf("decode voice broadcast voices: %w", err)
		}
	}
	if len(r.Health) > 0 {
		if err := json.Unmarshal(r.Health, &cfg.Health); err != nil {
			return Config{}, fmt.Errorf("decode voice broadcast health: %w", err)
		}
	}
	cfg = mergeDefaults(cfg)
	if strings.TrimSpace(r.APIKeyCiphertext) != "" {
		if codec == nil {
			return cfg, nil
		}
		plaintext, err := codec.Decrypt(r.APIKeyCiphertext)
		if err != nil {
			return Config{}, fmt.Errorf("decrypt voice broadcast API key: %w", err)
		}
		cfg.APIKey = plaintext
		cfg.APIKeySet = strings.TrimSpace(plaintext) != ""
	}
	return cfg, nil
}

func readRowTx(ctx context.Context, tx *sql.Tx) (persistedRow, bool, error) {
	var row persistedRow
	err := tx.QueryRowContext(ctx, persistedSelectSQL, configRowID).Scan(rowScanDest(&row)...)
	if errors.Is(err, sql.ErrNoRows) {
		return persistedRow{}, false, nil
	}
	if err != nil {
		return persistedRow{}, false, err
	}
	return row, true, nil
}

func writeRowTx(ctx context.Context, tx *sql.Tx, cfg Config, found bool) error {
	voices, err := json.Marshal(normalizeVoices(cfg.Voices, cfg.DefaultVoice))
	if err != nil {
		return err
	}
	health, err := json.Marshal(normalizeHealth(cfg.Health))
	if err != nil {
		return err
	}
	args := []any{configRowID, cfg.Version, cfg.Enabled, cfg.Provider, cfg.Region, cfg.WorkspaceID, cfg.Model, cfg.DefaultVoice, cfg.CurrentVoice, string(voices), cfg.APIKeyCiphertext, cfg.APIKeySuffix, string(health), cfg.UpdatedAt}
	if found {
		_, err = tx.ExecContext(ctx, `UPDATE app_voice_broadcast_configs SET version=$2,enabled=$3,provider=$4,region=$5,workspace_id=$6,model=$7,default_voice=$8,current_voice=$9,voices=$10::jsonb,api_key_ciphertext=$11,api_key_suffix=$12,health=$13::jsonb,update_time=$14 WHERE id=$1`, args...)
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_voice_broadcast_configs(id,version,enabled,provider,region,workspace_id,model,default_voice,current_voice,voices,api_key_ciphertext,api_key_suffix,health,create_time,update_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13::jsonb,now(),$14)`, args...)
	return err
}

func prepareUpdate(input UpdateInput, current Config, codec *xinzhili.VoiceSecretCodec) (Config, error) {
	cfg := mergeDefaults(current)
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if value := strings.TrimSpace(input.Provider); value != "" {
		cfg.Provider = value
	}
	if value := strings.TrimSpace(input.Region); value != "" {
		cfg.Region = value
	}
	if value := strings.TrimSpace(input.WorkspaceID); value != "" {
		cfg.WorkspaceID = value
	}
	if value := strings.TrimSpace(input.Model); value != "" {
		cfg.Model = value
	}
	if value := strings.TrimSpace(input.DefaultVoice); value != "" {
		cfg.DefaultVoice = value
	}
	if value := strings.TrimSpace(input.CurrentVoice); value != "" {
		cfg.CurrentVoice = value
	}
	if input.Voices != nil {
		cfg.Voices = append([]Voice(nil), input.Voices...)
	}
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	if cfg.Provider == "bailian" {
		cfg.Provider = DefaultProvider
	}
	if cfg.Provider != DefaultProvider {
		return Config{}, errors.New("语音播报仅支持阿里百炼")
	}
	for field, value := range map[string]string{"region": cfg.Region, "workspaceId": cfg.WorkspaceID, "model": cfg.Model, "defaultVoice": cfg.DefaultVoice, "currentVoice": cfg.CurrentVoice} {
		if utf8.RuneCountInString(value) > maxConfigString {
			return Config{}, fmt.Errorf("%s 长度超出限制", field)
		}
	}
	if utf8.RuneCountInString(strings.TrimSpace(input.APIKey)) > maxAPIKeyRunes {
		return Config{}, errors.New("API Key 长度超出限制")
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if input.ClearAPIKey {
		cfg.APIKey, cfg.APIKeyCiphertext, cfg.APIKeySuffix, cfg.APIKeySet = "", "", "", false
	} else if apiKey != "" {
		if codec == nil {
			return Config{}, ErrSecretUnavailable
		}
		ciphertext, err := codec.Encrypt(apiKey)
		if err != nil {
			return Config{}, err
		}
		cfg.APIKey, cfg.APIKeyCiphertext, cfg.APIKeySuffix, cfg.APIKeySet = "", ciphertext, secretSuffix(apiKey), true
	} else {
		cfg.APIKey = ""
	}
	cfg.Voices = normalizeVoices(cfg.Voices, cfg.DefaultVoice)
	if !containsVoice(cfg.Voices, cfg.CurrentVoice) {
		cfg.Voices = append(cfg.Voices, Voice{ID: cfg.CurrentVoice, Name: cfg.CurrentVoice, Provider: cfg.Provider, Model: cfg.Model})
	}
	cfg.Health = normalizeHealth(cfg.Health)
	return cfg, nil
}

func mergeDefaults(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.Provider == "" {
		cfg.Provider = defaults.Provider
	}
	if cfg.Region == "" {
		cfg.Region = defaults.Region
	}
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.DefaultVoice == "" {
		cfg.DefaultVoice = defaults.DefaultVoice
	}
	if cfg.CurrentVoice == "" {
		cfg.CurrentVoice = cfg.DefaultVoice
	}
	if len(cfg.Voices) == 0 {
		cfg.Voices = defaults.Voices
	}
	if cfg.Health.Status == "" {
		cfg.Health = defaults.Health
	}
	cfg.Voices = normalizeVoices(cfg.Voices, cfg.DefaultVoice)
	return cfg
}

func normalizeVoices(input []Voice, fallback string) []Voice {
	if len(input) == 0 {
		input = defaultVoices()
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]Voice, 0, len(input))
	for _, voice := range input {
		voice.ID = strings.TrimSpace(voice.ID)
		if voice.ID == "" {
			continue
		}
		if _, exists := seen[voice.ID]; exists {
			continue
		}
		seen[voice.ID] = struct{}{}
		voice.Name = strings.TrimSpace(voice.Name)
		if voice.Name == "" {
			voice.Name = voice.ID
		}
		if strings.TrimSpace(voice.Provider) == "" {
			voice.Provider = DefaultProvider
		}
		if strings.TrimSpace(voice.Model) == "" {
			voice.Model = DefaultModel
		}
		result = append(result, voice)
	}
	if len(result) == 0 {
		result = []Voice{{ID: fallback, Name: fallback, Provider: DefaultProvider, Model: DefaultModel}}
	}
	return result
}

func normalizeHealth(health Health) Health {
	switch health.Status {
	case "unknown", "ok", "error", "not_configured":
	default:
		health.Status = "unknown"
	}
	if health.TestedAt.IsZero() && health.Status == "unknown" && health.Message == "" {
		health.Message = "尚未测试"
	}
	if len(health.Message) > maxConfigString {
		health.Message = health.Message[:maxConfigString]
	}
	if health.LatencyMS < 0 {
		health.LatencyMS = 0
	}
	return health
}

func containsVoice(voices []Voice, id string) bool {
	for _, voice := range voices {
		if voice.ID == id {
			return true
		}
	}
	return false
}

func secretSuffix(secret string) string {
	runes := []rune(strings.TrimSpace(secret))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[len(runes)-4:])
}
