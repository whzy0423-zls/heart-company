package xinzhili

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	configKey          = "xinzhili_model_config"
	configStoreTimeout = 10 * time.Second
)

// ReadConfig reads the realtime voice configuration from its independent
// site_configs key. It never consults the legacy model_config record.
func ReadConfig(ctx context.Context, db *sql.DB) (Config, bool, error) {
	if db == nil {
		return Config{}, false, nil
	}
	c, cancel := configContext(ctx)
	defer cancel()

	var raw []byte
	if err := db.QueryRowContext(c, `SELECT config FROM site_configs WHERE key=$1`, configKey).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	cfg, err := decodeStoredConfig(raw)
	if err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

// UpdateConfig applies a compare-and-swap update under a transaction-scoped
// advisory lock. The fixed lock also serializes two concurrent first writes.
func UpdateConfig(ctx context.Context, db *sql.DB, incoming Config, expectedVersion int64) (Config, error) {
	if db == nil {
		return Config{}, errors.New("数据库未初始化，无法保存芯之力配置")
	}
	c, cancel := configContext(ctx)
	defer cancel()

	tx, err := db.BeginTx(c, nil)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:xinzhili-model-config', 0))`); err != nil {
		return Config{}, err
	}

	current, found, err := readConfigTx(c, tx)
	if err != nil {
		return Config{}, err
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return Config{}, ErrConfigConflict
	}

	merged := MergeIncoming(current, incoming)
	normalized, err := merged.WithDefaults()
	if err != nil {
		return Config{}, err
	}
	if found {
		normalized.Version = current.Version + 1
	} else {
		normalized.Version = 1
	}
	normalized.ClearASRKey = false
	normalized.ClearTTSKey = false
	body, err := json.Marshal(normalized)
	if err != nil {
		return Config{}, err
	}

	if found {
		_, err = tx.ExecContext(c, `UPDATE site_configs SET config=$1::jsonb, update_time=now() WHERE key=$2`, string(body), configKey)
	} else {
		_, err = tx.ExecContext(c, `INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, configKey, string(body))
	}
	if err != nil {
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, err
	}
	return normalized, nil
}

// MergeIncoming applies secret update semantics used by the store. Empty
// incoming secrets preserve current values; explicit clear markers erase them.
func MergeIncoming(current, incoming Config) Config {
	incoming.RealtimeASR.APIKey = strings.TrimSpace(incoming.RealtimeASR.APIKey)
	incoming.TTS.APIKey = strings.TrimSpace(incoming.TTS.APIKey)
	if incoming.ClearASRKey {
		incoming.RealtimeASR.APIKey = ""
	} else if incoming.RealtimeASR.APIKey == "" {
		incoming.RealtimeASR.APIKey = current.RealtimeASR.APIKey
	}
	if incoming.ClearTTSKey {
		incoming.TTS.APIKey = ""
	} else if incoming.TTS.APIKey == "" {
		incoming.TTS.APIKey = current.TTS.APIKey
	}
	incoming.ClearASRKey = false
	incoming.ClearTTSKey = false
	return incoming
}

func readConfigTx(ctx context.Context, tx *sql.Tx) (Config, bool, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, configKey).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	cfg, err := decodeStoredConfig(raw)
	if err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func decodeStoredConfig(raw []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	// Clear markers are request-only controls. Even if malformed or manually
	// written JSON contains them, reading stored state must never execute them.
	cfg.ClearASRKey = false
	cfg.ClearTTSKey = false
	normalized, err := cfg.WithDefaults()
	if err != nil {
		return Config{}, err
	}
	if err := normalized.Validate(); err != nil {
		return Config{}, err
	}
	return normalized, nil
}

func configContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, configStoreTimeout)
}
