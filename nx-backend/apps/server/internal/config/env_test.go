package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMiniappChatDefaults(t *testing.T) {
	t.Setenv("MINIAPP_CHAT_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("MINIAPP_CHAT_TIMEOUT_SECONDS", "")

	env := Load()

	if env.MiniappChat.RateLimitPerMinute != 12 {
		t.Fatalf("expected default chat rate limit 12, got %d", env.MiniappChat.RateLimitPerMinute)
	}
	if env.MiniappChat.TimeoutSeconds != 28 {
		t.Fatalf("expected default chat timeout 28, got %d", env.MiniappChat.TimeoutSeconds)
	}
}

func TestLoadMiniappChatOverrides(t *testing.T) {
	t.Setenv("MINIAPP_CHAT_RATE_LIMIT_PER_MINUTE", "5")
	t.Setenv("MINIAPP_CHAT_TIMEOUT_SECONDS", "18")

	env := Load()

	if env.MiniappChat.RateLimitPerMinute != 5 {
		t.Fatalf("expected configured chat rate limit 5, got %d", env.MiniappChat.RateLimitPerMinute)
	}
	if env.MiniappChat.TimeoutSeconds != 18 {
		t.Fatalf("expected configured chat timeout 18, got %d", env.MiniappChat.TimeoutSeconds)
	}
}

func TestLoadReadsDotEnvFromParentDirectory(t *testing.T) {
	for _, key := range []string{"ENV_FILE", "OSS_BUCKET", "OSS_PUBLIC_URL", "OSS_REGION", "OSS_ACCESS_KEY_ID", "OSS_ACCESS_KEY_SECRET"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("OSS_BUCKET=test-bucket\nOSS_PUBLIC_URL=https://cdn.example.com\nOSS_REGION=cn-test\nOSS_ACCESS_KEY_ID=ak\nOSS_ACCESS_KEY_SECRET=sk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	env := Load()

	if env.OSS.Bucket != "test-bucket" || env.OSS.PublicURL != "https://cdn.example.com" {
		t.Fatalf("expected OSS config from parent .env, got %+v", env.OSS)
	}
}
