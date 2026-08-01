package xinzhili

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestVoiceConfigDraftEncryptsAliyunAPIKey(t *testing.T) {
	codec := mustVoiceSecretCodec(t, 0x44)
	prepared, err := prepareVoiceConfigForPersist(TTSConfig{
		Provider: TTSProviderAliyunCosyVoice,
		Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
		APIKey:   "dashscope-key-1234",
		GroupID:  "workspace-1",
		Model:    "cosyvoice-v3.5-plus",
		Voice:    "clone-voice-id",
		Format:   "mp3",
	}, voiceConfigRecord{}, codec)
	if err != nil {
		t.Fatalf("prepareVoiceConfigForPersist: %v", err)
	}
	if prepared.APIKeyCiphertext == "" || strings.Contains(prepared.APIKeyCiphertext, "dashscope-key-1234") {
		t.Fatalf("api key not encrypted correctly: %q", prepared.APIKeyCiphertext)
	}
	if prepared.APIKeySuffix != "1234" {
		t.Fatalf("suffix=%q", prepared.APIKeySuffix)
	}
	if prepared.Config.APIKey != "" {
		t.Fatalf("persisted public config leaked api key: %#v", prepared.Config)
	}
}

func TestVoiceConfigDraftRequiresSecretKeyOnlyForAliyunSecretWrites(t *testing.T) {
	_, err := prepareVoiceConfigForPersist(TTSConfig{
		Provider: TTSProviderAliyunCosyVoice,
		Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
		APIKey:   "dashscope-key",
		GroupID:  "workspace-1",
		Model:    "cosyvoice-v3.5-plus",
		Voice:    "clone-voice-id",
		Format:   "mp3",
	}, voiceConfigRecord{}, nil)
	if !errors.Is(err, ErrVoiceSecretKeyInvalid) {
		t.Fatalf("err=%v want ErrVoiceSecretKeyInvalid", err)
	}

	prepared, err := prepareVoiceConfigForPersist(TTSConfig{
		Provider: TTSProviderOpenAICompatible,
		Endpoint: "https://voice.example.com/v1",
		APIKey:   "openai-style-key",
		Model:    "tts-1",
		Voice:    "alloy",
		Format:   "mp3",
	}, voiceConfigRecord{}, nil)
	if err != nil {
		t.Fatalf("openai compatible should not require XINZHILI_SECRET_KEY: %v", err)
	}
	if prepared.APIKeyCiphertext != "" || prepared.Config.APIKey != "openai-style-key" {
		t.Fatalf("legacy/openai provider should keep old storage semantics: %+v", prepared)
	}
}

func TestVoiceConfigDraftPreservesEncryptedAliyunKeyWhenIncomingKeyEmpty(t *testing.T) {
	codec := mustVoiceSecretCodec(t, 0x55)
	first, err := prepareVoiceConfigForPersist(TTSConfig{
		Provider: TTSProviderAliyunCosyVoice,
		Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
		APIKey:   "dashscope-key-9876",
		GroupID:  "workspace-1",
		Model:    "cosyvoice-v3.5-plus",
		Voice:    "clone-voice-id",
		Format:   "mp3",
	}, voiceConfigRecord{}, codec)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := prepareVoiceConfigForPersist(TTSConfig{
		Provider: TTSProviderAliyunCosyVoice,
		Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
		GroupID:  "workspace-1",
		Model:    "cosyvoice-v3.5-plus",
		Voice:    "clone-voice-v2",
		Format:   "mp3",
	}, first, nil)
	if err != nil {
		t.Fatalf("preserve encrypted key should not require codec: %v", err)
	}
	if updated.APIKeyCiphertext != first.APIKeyCiphertext || updated.APIKeySuffix != "9876" {
		t.Fatalf("encrypted key not preserved: first=%+v updated=%+v", first, updated)
	}
}

func mustVoiceSecretCodec(t *testing.T, fill byte) *VoiceSecretCodec {
	t.Helper()
	codec, err := NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return codec
}
