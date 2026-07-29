package voice

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/testutil"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCopyProfileToBailianStatusDecision(t *testing.T) {
	tests := []struct {
		status string
		want   bailianCopyAction
	}{
		{status: "ready", want: returnExistingBailianCopy},
		{status: "cloning", want: returnExistingBailianCopy},
		{status: "draft", want: cloneExistingBailianCopy},
		{status: "failed", want: cloneExistingBailianCopy},
		{status: "unexpected", want: returnExistingBailianCopy},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := bailianCopyActionForStatus(tt.status); got != tt.want {
				t.Fatalf("status %q action = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestCopyProfileToBailianPlanRejectsNonMiniMaxSource(t *testing.T) {
	_, err := buildBailianCopyPlan(Profile{Provider: ProviderBailian, SampleAssetID: "9"})
	if err == nil || !strings.Contains(err.Error(), "MiniMax") {
		t.Fatalf("err=%v, want MiniMax source validation error", err)
	}
}

func TestCopyProfileToBailianPlanRejectsMissingSample(t *testing.T) {
	_, err := buildBailianCopyPlan(Profile{Provider: ProviderMiniMax})
	if err == nil || !strings.Contains(err.Error(), "音频样本") {
		t.Fatalf("err=%v, want missing sample validation error", err)
	}
}

func TestCopyProfileToBailianPlanCopiesBailianFields(t *testing.T) {
	plan, err := buildBailianCopyPlan(Profile{
		Name:          "韩老师",
		Provider:      ProviderMiniMax,
		Remark:        "原始备注",
		SampleAssetID: "42",
		SampleName:    "teacher.mp3",
		SampleURL:     "/api/upload-assets/42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "韩老师（百炼）" || plan.Provider != ProviderBailian || plan.SampleAssetID != 42 || plan.SampleName != "teacher.mp3" || plan.SampleURL != "/api/upload-assets/42" || plan.Remark != "原始备注" {
		t.Fatalf("unexpected copy plan: %+v", plan)
	}
}

func TestCopyProfileToBailianReusesMiniMaxSampleAndPreservesSource(t *testing.T) {
	store, database, cleanup := newVoiceProfileCopyTestStore(t)
	defer cleanup()

	var cloneCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cloneCalls++
		if r.URL.Path != bailianGenerationPath {
			t.Fatalf("clone path = %q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		input := payload["input"].(map[string]any)
		if input["audio_url"] != "https://cdn.example.com/voice/sample.mp3" {
			t.Fatalf("clone must reuse the source object URL, input=%+v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"voice_id": "bailian-voice"}})
	}))
	defer upstream.Close()
	store.bailian = NewBailianClient(BailianConfig{APIBase: upstream.URL, APIKey: "test-key", TargetModel: defaultBailianTargetModel})
	store.bailian.client = upstream.Client()

	asset := createVoiceProfileCopySample(t, store)
	sourceID := insertVoiceProfileCopySource(t, database, asset.ID, "ready")

	copy, err := store.CopyProfileToBailian(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("CopyProfileToBailian: %v", err)
	}
	if copy.Provider != ProviderBailian || copy.Status != "ready" || copy.VoiceID != "bailian-voice" {
		t.Fatalf("unexpected copied profile: %+v", copy)
	}
	if copy.SampleAssetID != stringInt(asset.ID) || copy.SampleURL != "/api/upload-assets/"+stringInt(asset.ID) || copy.SampleName != "sample.mp3" {
		t.Fatalf("copied profile did not preserve sample fields: %+v", copy)
	}
	if !strings.Contains(copy.Name, "（百炼）") {
		t.Fatalf("copy name = %q", copy.Name)
	}
	if cloneCalls != 1 {
		t.Fatalf("clone calls = %d, want 1", cloneCalls)
	}

	var sourceProvider, sourceStatus string
	var sourceAssetID int64
	if err := database.QueryRow(`SELECT provider, status, sample_asset_id FROM voice_profiles WHERE id=$1`, sourceID).Scan(&sourceProvider, &sourceStatus, &sourceAssetID); err != nil {
		t.Fatal(err)
	}
	if sourceProvider != ProviderMiniMax || sourceStatus != "ready" || sourceAssetID != asset.ID {
		t.Fatalf("source profile was modified: provider=%q status=%q sample=%d", sourceProvider, sourceStatus, sourceAssetID)
	}
}

func TestCopyProfileToBailianReturnsExistingReadyOrCloningProfileForSameSample(t *testing.T) {
	store, database, cleanup := newVoiceProfileCopyTestStore(t)
	defer cleanup()
	for _, status := range []string{"ready", "cloning"} {
		t.Run(status, func(t *testing.T) {
			asset := createVoiceProfileCopySample(t, store)
			sourceID := insertVoiceProfileCopySource(t, database, asset.ID, "ready")
			existingID := insertVoiceProfileCopyBailian(t, database, asset.ID, status)

			copy, err := store.CopyProfileToBailian(context.Background(), sourceID)
			if err != nil {
				t.Fatalf("CopyProfileToBailian: %v", err)
			}
			if copy.ID != existingID || copy.Provider != ProviderBailian || copy.Status != status {
				t.Fatalf("expected %s existing Bailian profile, got %+v", status, copy)
			}
			var count int
			if err := database.QueryRow(`SELECT count(*) FROM voice_profiles WHERE provider=$1 AND sample_asset_id=$2`, ProviderBailian, asset.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("Bailian profiles for sample = %d, want 1", count)
			}
		})
	}
}

func TestCopyProfileToBailianRetriesExistingFailedProfile(t *testing.T) {
	store, database, cleanup := newVoiceProfileCopyTestStore(t)
	defer cleanup()
	asset := createVoiceProfileCopySample(t, store)
	sourceID := insertVoiceProfileCopySource(t, database, asset.ID, "ready")
	existingID := insertVoiceProfileCopyBailian(t, database, asset.ID, "failed")

	var cloneCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cloneCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"voice_id": "retry-voice"}})
	}))
	defer upstream.Close()
	store.bailian = NewBailianClient(BailianConfig{APIBase: upstream.URL, APIKey: "test-key", TargetModel: defaultBailianTargetModel})
	store.bailian.client = upstream.Client()

	copy, err := store.CopyProfileToBailian(context.Background(), sourceID)
	if err != nil {
		t.Fatalf("CopyProfileToBailian: %v", err)
	}
	if copy.ID != existingID || copy.Status != "ready" || copy.VoiceID != "retry-voice" {
		t.Fatalf("failed copy was not retried in place: %+v", copy)
	}
	if cloneCalls != 1 {
		t.Fatalf("clone calls = %d, want 1", cloneCalls)
	}
}

func newVoiceProfileCopyTestStore(t *testing.T) (*Store, *sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run voice profile copy integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrapVoiceProfileCopyTables(database); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	store := NewStore(database, uploadasset.NewStore(database), config.MiniMaxConfig{})
	return store, database, func() { _ = database.Close() }
}

func bootstrapVoiceProfileCopyTables(database *sql.DB) error {
	_, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS upload_assets (
			id BIGSERIAL PRIMARY KEY,
			key TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			dir TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT '',
			size BIGINT NOT NULL DEFAULT 0,
			data BYTEA,
			object_key TEXT NOT NULL DEFAULT '',
			object_url TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS voice_profiles (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT 'minimax',
			voice_id TEXT NOT NULL DEFAULT '',
			sample_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
			sample_url TEXT NOT NULL DEFAULT '',
			sample_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			remark TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
			update_time TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	return err
}

func createVoiceProfileCopySample(t *testing.T, store *Store) uploadasset.Asset {
	t.Helper()
	asset, err := store.uploads.Create(context.Background(), uploadasset.CreateInput{
		ContentType: "audio/mpeg",
		Data:        []byte("sample-audio"),
		Dir:         "voice/sample",
		Name:        "sample.mp3",
		ObjectURL:   "https://cdn.example.com/voice/sample.mp3",
		Size:        int64(len("sample-audio")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM voice_profiles WHERE sample_asset_id=$1`, asset.ID)
		_, _ = store.db.Exec(`DELETE FROM upload_assets WHERE id=$1`, asset.ID)
	})
	return asset
}

func insertVoiceProfileCopySource(t *testing.T, database *sql.DB, assetID int64, status string) string {
	t.Helper()
	var id string
	err := database.QueryRow(
		`INSERT INTO voice_profiles (name, provider, voice_id, sample_asset_id, sample_url, sample_name, status)
		 VALUES ('原 MiniMax 音色',$1,'minimax-voice',$2,$3,'sample.mp3',$4)
		 RETURNING id::text`,
		ProviderMiniMax, assetID, "/api/upload-assets/"+stringInt(assetID), status,
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertVoiceProfileCopyBailian(t *testing.T, database *sql.DB, assetID int64, status string) string {
	t.Helper()
	var id string
	err := database.QueryRow(
		`INSERT INTO voice_profiles (name, provider, voice_id, sample_asset_id, sample_url, sample_name, status)
		 VALUES ('已有百炼音色',$1,'existing-bailian-voice',$2,$3,'sample.mp3',$4)
		 RETURNING id::text`,
		ProviderBailian, assetID, "/api/upload-assets/"+stringInt(assetID), status,
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func stringInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
