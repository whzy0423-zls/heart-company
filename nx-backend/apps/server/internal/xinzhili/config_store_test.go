package xinzhili

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const xinzhiliConfigKey = "xinzhili_model_config"

var (
	errConfigStoreTestDatabaseUnavailable = errors.New("set a localhost TEST_DATABASE_URL to run xinzhili config store integration tests")
	errConfigStoreTestDatabaseRequired    = errors.New("CI requires TEST_DATABASE_URL for xinzhili config store integration tests")
)

func configStoreTestDSN(getenv func(string) string) (string, error) {
	if dsn := strings.TrimSpace(getenv("TEST_DATABASE_URL")); dsn != "" {
		return dsn, nil
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return "", errConfigStoreTestDatabaseRequired
	}
	return "", errConfigStoreTestDatabaseUnavailable
}

func TestConfigStoreFirstWriteAndRead(t *testing.T) {
	database := openConfigStoreTestDB(t)
	cfg := validConfig()

	created, err := UpdateConfig(context.Background(), database, cfg, 0)
	if err != nil {
		t.Fatalf("first UpdateConfig: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("version=%d want=1", created.Version)
	}
	got, found, err := ReadConfig(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("ReadConfig found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Fatalf("read=%#v created=%#v", got, created)
	}
}

func TestConfigStoreReadAppliesDefaultsToOlderStoredConfig(t *testing.T) {
	database := openConfigStoreTestDB(t)
	cfg := validConfig()
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, xinzhiliConfigKey, string(body)); err != nil {
		t.Fatal(err)
	}

	got, found, err := ReadConfig(context.Background(), database)
	if err != nil || !found {
		t.Fatalf("ReadConfig found=%v err=%v", found, err)
	}
	if got.Timing.PartialStableMs != 120 || got.Timing.NormalEndSilenceMs != 500 || got.Timing.MaxProactivePrompts != 2 {
		t.Fatalf("stored timing defaults not applied: %+v", got.Timing)
	}
}

func TestConfigStoreReadRejectsSemanticallyInvalidStoredConfig(t *testing.T) {
	database := openConfigStoreTestDB(t)
	invalid := validConfig()
	invalid.EnabledModes = []Mode{ModeComfort}
	body, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, xinzhiliConfigKey, string(body)); err != nil {
		t.Fatal(err)
	}

	if _, found, err := ReadConfig(context.Background(), database); err == nil || found {
		t.Fatalf("invalid stored config found=%v err=%v, want found=false with error", found, err)
	}
}

func TestConfigStoreUpdateRejectsSemanticallyInvalidStoredConfig(t *testing.T) {
	database := openConfigStoreTestDB(t)
	invalid := validConfig()
	invalid.RealtimeASR.Provider = "invalid-provider"
	invalid.Version = 1
	body, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO site_configs (key, config, update_time) VALUES ($1, $2::jsonb, now())`, xinzhiliConfigKey, string(body)); err != nil {
		t.Fatal(err)
	}

	if _, err := UpdateConfig(context.Background(), database, validConfig(), 1); err == nil {
		t.Fatal("UpdateConfig should reject invalid current stored config")
	}
}

func TestConfigStoreRejectsVersionConflict(t *testing.T) {
	database := openConfigStoreTestDB(t)
	created, err := UpdateConfig(context.Background(), database, validConfig(), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UpdateConfig(context.Background(), database, created, 0)
	if !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("err=%v want ErrConfigConflict", err)
	}
	updated, err := UpdateConfig(context.Background(), database, created, created.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != created.Version+1 {
		t.Fatalf("version=%d want=%d", updated.Version, created.Version+1)
	}
}

func TestConfigStorePreservesAndExplicitlyClearsSecrets(t *testing.T) {
	database := openConfigStoreTestDB(t)
	created, err := UpdateConfig(context.Background(), database, validConfig(), 0)
	if err != nil {
		t.Fatal(err)
	}

	incoming := created
	incoming.RealtimeASR.APIKey = ""
	incoming.TTS.APIKey = ""
	preserved, err := UpdateConfig(context.Background(), database, incoming, created.Version)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.RealtimeASR.APIKey != "asr-secret" || preserved.TTS.APIKey != "tts-secret" {
		t.Fatalf("empty secrets should preserve stored values: %#v", preserved)
	}

	incoming = preserved
	incoming.Enabled = false
	incoming.ClearASRKey = true
	incoming.ClearTTSKey = true
	cleared, err := UpdateConfig(context.Background(), database, incoming, preserved.Version)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.RealtimeASR.APIKey != "" || cleared.TTS.APIKey != "" {
		t.Fatalf("explicit clear failed: %#v", cleared)
	}
	if cleared.ClearASRKey || cleared.ClearTTSKey {
		t.Fatal("clear markers must not be persisted or returned")
	}

	var persisted map[string]any
	var raw []byte
	if err := database.QueryRow(`SELECT config FROM site_configs WHERE key=$1`, xinzhiliConfigKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if _, exists := persisted["clearAsrKey"]; exists {
		t.Fatal("clearAsrKey leaked into persisted JSON")
	}
	if _, exists := persisted["clearTtsKey"]; exists {
		t.Fatal("clearTtsKey leaked into persisted JSON")
	}
}

func TestConfigStoreClearSecretFailsWhileEnabled(t *testing.T) {
	database := openConfigStoreTestDB(t)
	created, err := UpdateConfig(context.Background(), database, validConfig(), 0)
	if err != nil {
		t.Fatal(err)
	}
	created.ClearASRKey = true
	if _, err := UpdateConfig(context.Background(), database, created, created.Version); err == nil {
		t.Fatal("enabled config must reject clearing a required ASR key")
	}
	stored, found, err := ReadConfig(context.Background(), database)
	if err != nil || !found || stored.Version != created.Version || stored.RealtimeASR.APIKey != "asr-secret" {
		t.Fatalf("failed update changed stored config: found=%v err=%v stored=%#v", found, err, stored)
	}
}

func TestConfigStoreLegacyUntouched(t *testing.T) {
	database := openConfigStoreTestDB(t)
	legacy := map[string]any{
		"chat":   map[string]any{"apiBase": "https://legacy.example.com", "apiKey": "legacy-secret"},
		"assist": map[string]any{"enabled": true, "systemPrompt": "legacy prompt"},
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO site_configs (key, config, update_time) VALUES ('model_config', $1::jsonb, now())`, string(body)); err != nil {
		t.Fatal(err)
	}

	if _, err := UpdateConfig(context.Background(), database, validConfig(), 0); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := database.QueryRow(`SELECT config FROM site_configs WHERE key='model_config'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("legacy model_config changed: got=%#v want=%#v", got, legacy)
	}
}

func TestConfigStoreConcurrentFirstWriteOnlyOneSucceeds(t *testing.T) {
	database := openConfigStoreTestDB(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := UpdateConfig(context.Background(), database, validConfig(), 0)
			errs <- err
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConfigConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	stored, found, err := ReadConfig(context.Background(), database)
	if err != nil || !found || stored.Version != 1 {
		t.Fatalf("stored found=%v err=%v config=%#v", found, err, stored)
	}
}

func TestConfigStoreTestDSNGuard(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantDSN string
		wantErr error
	}{
		{name: "local without database skips", env: map[string]string{}, wantErr: errConfigStoreTestDatabaseUnavailable},
		{name: "CI without database fails", env: map[string]string{"CI": "true"}, wantErr: errConfigStoreTestDatabaseRequired},
		{name: "configured database", env: map[string]string{"TEST_DATABASE_URL": "postgres://localhost/example_test"}, wantDSN: "postgres://localhost/example_test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			got, err := configStoreTestDSN(getenv)
			if got != tt.wantDSN || !errors.Is(err, tt.wantErr) {
				t.Fatalf("dsn=%q err=%v wantDSN=%q wantErr=%v", got, err, tt.wantDSN, tt.wantErr)
			}
		})
	}
}

func openConfigStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, err := configStoreTestDSN(os.Getenv)
	if errors.Is(err, errConfigStoreTestDatabaseUnavailable) {
		t.Skip(err.Error())
	}
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	host := parsed.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		t.Fatalf("TEST_DATABASE_URL must point to localhost, got host %q", host)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatalf("TEST_DATABASE_URL must use PostgreSQL, got scheme %q", parsed.Scheme)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("TEST_DATABASE_URL must name an isolated test database, got %q", databaseName)
	}
	if parsed.Query().Has("dbname") || parsed.Query().Has("database") {
		t.Fatal("TEST_DATABASE_URL must not override the database name in query parameters")
	}

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("ping test database: %v", err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS site_configs (
		key TEXT PRIMARY KEY,
		config JSONB NOT NULL,
		update_time TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		_ = database.Close()
		t.Fatalf("ensure site_configs: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM site_configs WHERE key IN ($1, 'model_config')`, xinzhiliConfigKey); err != nil {
		_ = database.Close()
		t.Fatalf("clean config fixtures: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM site_configs WHERE key IN ($1, 'model_config')`, xinzhiliConfigKey)
		_ = database.Close()
	})
	return database
}
