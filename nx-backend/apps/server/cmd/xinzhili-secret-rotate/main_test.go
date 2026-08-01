package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestRunSecretRotateDryRunValidatesKeysWithoutDatabaseURL(t *testing.T) {
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	out := &bytes.Buffer{}
	err := runSecretRotate([]string{"--old-key-env", "OLD_KEY", "--new-key-env", "NEW_KEY", "--dry-run"}, func(key string) string {
		return map[string]string{"OLD_KEY": oldKey, "NEW_KEY": newKey}[key]
	}, out)
	if err != nil {
		t.Fatalf("runSecretRotate: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run") || !strings.Contains(out.String(), "密钥校验通过") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunSecretRotateRejectsInvalidKey(t *testing.T) {
	err := runSecretRotate([]string{"--old-key-env", "OLD_KEY", "--new-key-env", "NEW_KEY", "--dry-run"}, func(key string) string {
		return map[string]string{"OLD_KEY": "bad", "NEW_KEY": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))}[key]
	}, &bytes.Buffer{})
	if !errors.Is(err, errSecretRotateInvalidKey) {
		t.Fatalf("err=%v want errSecretRotateInvalidKey", err)
	}
}
