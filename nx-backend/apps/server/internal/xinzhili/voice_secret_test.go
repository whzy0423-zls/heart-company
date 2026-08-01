package xinzhili

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestVoiceSecretEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	codec, err := NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewVoiceSecretCodec: %v", err)
	}

	ciphertext, err := codec.Encrypt("dashscope-secret-key")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == "" {
		t.Fatal("ciphertext should not be empty")
	}
	if strings.Contains(ciphertext, "dashscope-secret-key") {
		t.Fatalf("ciphertext leaked plaintext: %q", ciphertext)
	}

	plaintext, err := codec.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plaintext != "dashscope-secret-key" {
		t.Fatalf("plaintext=%q", plaintext)
	}
}

func TestVoiceSecretRejectsInvalidMasterKey(t *testing.T) {
	for name, key := range map[string]string{
		"missing":      "",
		"not base64":   "not-base64",
		"wrong length": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewVoiceSecretCodec(key)
			if !errors.Is(err, ErrVoiceSecretKeyInvalid) {
				t.Fatalf("err=%v want ErrVoiceSecretKeyInvalid", err)
			}
		})
	}
}

func TestVoiceSecretWrongKeyFails(t *testing.T) {
	codecA, err := NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	codecB, err := NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := codecA.Encrypt("tts-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codecB.Decrypt(ciphertext); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestVoiceSecretRejectsMalformedCiphertext(t *testing.T) {
	codec, err := NewVoiceSecretCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	for _, ciphertext := range []string{"", "v1:not-base64", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := codec.Decrypt(ciphertext); !errors.Is(err, ErrVoiceSecretCiphertextInvalid) {
			t.Fatalf("Decrypt(%q) err=%v want ErrVoiceSecretCiphertextInvalid", ciphertext, err)
		}
	}
}
