// Package bailianconfig persists the shared DashScope/Bailian credential used
// by realtime ASR, Qwen TTS, and Qwen voice cloning. It is intentionally
// independent from xinzhili_model_config so changing a credential cannot
// overwrite a concurrently edited realtime configuration.
package bailianconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	ConfigKey = "bailian_shared_credentials"
	timeout   = 10 * time.Second
)

var ErrConflict = errors.New("bailian credentials version conflict")

// Config is the versioned shared Bailian credential stored in site_configs.
type Config struct {
	Version int64  `json:"version"`
	APIKey  string `json:"apiKey"`
}

// Read returns the shared credential and whether its record has ever been
// saved. A found empty APIKey is meaningful: it deliberately disables legacy
// credential fallback.
func Read(ctx context.Context, db *sql.DB) (Config, bool, error) {
	if db == nil {
		return Config{}, false, nil
	}
	c, cancel := withTimeout(ctx)
	defer cancel()

	var raw []byte
	if err := db.QueryRowContext(c, `SELECT config FROM site_configs WHERE key=$1`, ConfigKey).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	config, err := decode(raw)
	if err != nil {
		return Config{}, false, err
	}
	return config, true, nil
}

// Update applies a version-checked shared credential update. Empty API keys
// preserve an existing key unless clearAPIKey is true. Before the first save an
// empty non-clear update is a no-op, preserving the ability to fall back to a
// legacy Bailian configuration.
func Update(ctx context.Context, db *sql.DB, apiKey string, expectedVersion int64, clearAPIKey bool) (Config, error) {
	if db == nil {
		return Config{}, errors.New("database is not initialized")
	}
	c, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := db.BeginTx(c, nil)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:bailian-shared-credentials', 0))`); err != nil {
		return Config{}, err
	}

	current, found, err := readTx(c, tx)
	if err != nil {
		return Config{}, err
	}
	if (!found && expectedVersion != 0) || (found && current.Version != expectedVersion) {
		return Config{}, ErrConflict
	}

	apiKey = strings.TrimSpace(apiKey)
	if !found && apiKey == "" && !clearAPIKey {
		return Config{}, nil
	}

	next := Config{APIKey: apiKey}
	if found {
		next.Version = current.Version + 1
		if !clearAPIKey && next.APIKey == "" {
			next.APIKey = current.APIKey
		}
	} else {
		next.Version = 1
	}
	if clearAPIKey {
		next.APIKey = ""
	}

	body, err := json.Marshal(next)
	if err != nil {
		return Config{}, err
	}
	if found {
		_, err = tx.ExecContext(c, `UPDATE site_configs SET config=$1::jsonb, update_time=now() WHERE key=$2`, string(body), ConfigKey)
	} else {
		_, err = tx.ExecContext(c, `INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, ConfigKey, string(body))
	}
	if err != nil {
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, err
	}
	return next, nil
}

func readTx(ctx context.Context, tx *sql.Tx) (Config, bool, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT config FROM site_configs WHERE key=$1`, ConfigKey).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	config, err := decode(raw)
	if err != nil {
		return Config{}, false, err
	}
	return config, true, nil
}

func decode(raw []byte) (Config, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return Config{}, err
	}
	if object == nil {
		return Config{}, errors.New("stored bailian credentials must be a JSON object")
	}
	if _, exists := object["version"]; !exists {
		return Config{}, errors.New("stored bailian credentials version is required")
	}
	var config Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return Config{}, err
	}
	if config.Version < 1 {
		return Config{}, errors.New("stored bailian credentials version must be at least 1")
	}
	config.APIKey = strings.TrimSpace(config.APIKey)
	return config, nil
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}
