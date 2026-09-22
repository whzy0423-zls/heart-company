package voicebroadcastconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestDefaultConfigUsesBailianDefaultFemaleVoice(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Fatal("voice broadcast must be disabled by default")
	}
	if cfg.Provider != DefaultProvider || cfg.Model != DefaultModel {
		t.Fatalf("defaults provider=%q model=%q", cfg.Provider, cfg.Model)
	}
	if cfg.DefaultVoice != DefaultFemaleVoice || cfg.CurrentVoice != DefaultFemaleVoice {
		t.Fatalf("defaults defaultVoice=%q currentVoice=%q", cfg.DefaultVoice, cfg.CurrentVoice)
	}
	if len(cfg.Voices) == 0 || cfg.Voices[0].ID != DefaultFemaleVoice {
		t.Fatalf("default voices=%+v", cfg.Voices)
	}
}

func TestNormalizeUpdatePreservesSecretAndUsesCAS(t *testing.T) {
	codec := mustTestCodec(t)
	current := Config{
		Version:          4,
		Enabled:          false,
		Provider:         DefaultProvider,
		Region:           "cn-beijing",
		WorkspaceID:      "workspace-old",
		Model:            DefaultModel,
		DefaultVoice:     DefaultFemaleVoice,
		CurrentVoice:     DefaultFemaleVoice,
		APIKeyCiphertext: mustEncrypt(t, codec, "sk-old-secret"),
		APIKeySuffix:     "cret",
	}
	updated, err := prepareUpdate(UpdateInput{
		Enabled:      boolPtr(true),
		Provider:     DefaultProvider,
		Region:       "cn-shanghai",
		WorkspaceID:  "workspace-new",
		Model:        DefaultModel,
		CurrentVoice: "Serena",
	}, current, codec)
	if err != nil {
		t.Fatal(err)
	}
	if updated.APIKeyCiphertext != current.APIKeyCiphertext || updated.APIKeySuffix != current.APIKeySuffix {
		t.Fatalf("empty key should preserve ciphertext: old=%+v new=%+v", current, updated)
	}
	if updated.APIKey != "" {
		t.Fatal("persisted config must not contain plaintext API key")
	}
	if updated.CurrentVoice != "Serena" || updated.WorkspaceID != "workspace-new" {
		t.Fatalf("updated metadata=%+v", updated)
	}
}

func TestPrepareUpdateEncryptsNewKeyAndRejectsMissingCodec(t *testing.T) {
	input := UpdateInput{
		APIKey:       " sk-new-secret ",
		Provider:     DefaultProvider,
		Region:       "cn-beijing",
		Model:        DefaultModel,
		CurrentVoice: DefaultFemaleVoice,
	}
	prepared, err := prepareUpdate(input, Config{}, mustTestCodec(t))
	if err != nil {
		t.Fatal(err)
	}
	if prepared.APIKeyCiphertext == "" || strings.Contains(prepared.APIKeyCiphertext, "sk-new-secret") {
		t.Fatalf("secret was not encrypted: %q", prepared.APIKeyCiphertext)
	}
	if prepared.APIKeySuffix != "cret" {
		t.Fatalf("suffix=%q", prepared.APIKeySuffix)
	}
	if _, err := prepareUpdate(input, Config{}, nil); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("err=%v want ErrSecretUnavailable", err)
	}
}

func TestBuildViewNeverReturnsPlaintextSecret(t *testing.T) {
	cfg := Config{
		Version:          2,
		Enabled:          true,
		Provider:         DefaultProvider,
		Region:           "cn-beijing",
		Model:            DefaultModel,
		DefaultVoice:     DefaultFemaleVoice,
		CurrentVoice:     DefaultFemaleVoice,
		APIKey:           "sk-plain-secret",
		APIKeyCiphertext: "v1:encrypted",
		APIKeySuffix:     "cret",
	}
	view := BuildView(cfg, true)
	if !view.APIKeySet || view.APIKeySuffix != "cret" {
		t.Fatalf("view=%+v", view)
	}
	if strings.Contains(string(mustJSON(t, view)), "sk-plain-secret") {
		t.Fatal("view leaked plaintext API key")
	}
}

func TestSecretSuffixMasksShortSecrets(t *testing.T) {
	for _, test := range []struct {
		secret string
		want   string
	}{
		{secret: "", want: ""},
		{secret: "a", want: "*"},
		{secret: "abcd", want: "****"},
		{secret: "abcde", want: "bcde"},
	} {
		if got := secretSuffix(test.secret); got != test.want {
			t.Errorf("secretSuffix(%q)=%q want %q", test.secret, got, test.want)
		}
	}
}

func TestStoreNilDatabaseReturnsSafeDefaults(t *testing.T) {
	store := NewStore(nil, mustTestCodec(t))
	cfg, found, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found || !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatalf("found=%v cfg=%+v", found, cfg)
	}
	_, err = store.Update(context.Background(), UpdateInput{}, 0)
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("update err=%v", err)
	}
}

func mustTestCodec(t *testing.T) *xinzhili.VoiceSecretCodec {
	t.Helper()
	codec, err := xinzhili.NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return codec
}

func mustEncrypt(t *testing.T, codec *xinzhili.VoiceSecretCodec, secret string) string {
	t.Helper()
	ciphertext, err := codec.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	return ciphertext
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func boolPtr(value bool) *bool { return &value }
