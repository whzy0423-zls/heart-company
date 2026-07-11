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

func TestVideoGenerationModeFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		mode string
		ack  string
		want string
	}{
		{name: "default", want: "demo"},
		{name: "paid without acknowledgement", mode: "paid", want: "demo"},
		{name: "acknowledgement without paid mode", ack: "ALLOW_PAID_VIDEO_GENERATION", want: "demo"},
		{name: "misspelled acknowledgement", mode: "paid", ack: "allow", want: "demo"},
		{name: "unknown mode", mode: "production", ack: "ALLOW_PAID_VIDEO_GENERATION", want: "demo"},
		{name: "explicit paid", mode: "paid", ack: "ALLOW_PAID_VIDEO_GENERATION", want: "paid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
			t.Setenv("VIDEO_GENERATION_MODE", tc.mode)
			t.Setenv("VIDEO_PAID_GENERATION_ACK", tc.ack)

			if got := Load().Video.Mode; got != tc.want {
				t.Fatalf("video generation mode=%q, want %q", got, tc.want)
			}
		})
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

func TestLoadReadsSpugSMSConfig(t *testing.T) {
	t.Setenv("SMS_PROVIDER", "spug")
	t.Setenv("SPUG_PUSH_API_BASE", "https://push.spug.cc")
	t.Setenv("SPUG_PUSH_TEMPLATE_CODE", "tmpl123")
	t.Setenv("SPUG_PUSH_TEMPLATE_NAME", "芯之力")
	t.Setenv("SPUG_PUSH_TIMEOUT_SECONDS", "7")

	env := Load()

	if env.SMS.Provider != "spug" {
		t.Fatalf("expected spug provider, got %q", env.SMS.Provider)
	}
	if env.SMS.SpugAPIBase != "https://push.spug.cc" {
		t.Fatalf("expected spug api base, got %q", env.SMS.SpugAPIBase)
	}
	if env.SMS.SpugTemplateCode != "tmpl123" {
		t.Fatalf("expected spug template code, got %q", env.SMS.SpugTemplateCode)
	}
	if env.SMS.SpugTemplateName != "芯之力" {
		t.Fatalf("expected spug template name, got %q", env.SMS.SpugTemplateName)
	}
	if env.SMS.SpugTimeoutSeconds != 7 {
		t.Fatalf("expected spug timeout 7, got %d", env.SMS.SpugTimeoutSeconds)
	}
}

func TestLoadDefaultsASRToSiliconFlowFreeModel(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("ASR_API_BASE", "")
	t.Setenv("ASR_MODEL", "")
	t.Setenv("ASR_TIMEOUT_SECONDS", "")

	env := Load()

	if env.ASR.APIBase != "https://api.siliconflow.cn" {
		t.Fatalf("expected default SiliconFlow ASR base, got %q", env.ASR.APIBase)
	}
	if env.ASR.Model != "FunAudioLLM/SenseVoiceSmall" {
		t.Fatalf("expected default free SiliconFlow ASR model, got %q", env.ASR.Model)
	}
	if env.ASR.TimeoutSeconds != 60 {
		t.Fatalf("expected default ASR timeout 60, got %d", env.ASR.TimeoutSeconds)
	}
}

func TestLoadNormalizesAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", " Production ")

	env := Load()

	if env.AppEnv != "production" {
		t.Fatalf("expected normalized production APP_ENV, got %q", env.AppEnv)
	}
}

func TestValidateProductionTreatsProdAliasAsProduction(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "123456",
		AppEnv:        "prod",
		DatabaseURL:   "postgres://nx:nx@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "nine-xing-dev-secret",
	})
	if err == nil {
		t.Fatal("expected prod APP_ENV alias to run production validation")
	}
}

func TestValidateProductionRejectsUnknownAppEnv(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "prd",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err == nil {
		t.Fatal("expected unknown APP_ENV to be rejected")
	}
}

func TestValidateProductionRejectsMissingAppEnv(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err == nil {
		t.Fatal("expected missing APP_ENV to be rejected")
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

func TestValidateProductionRejectsDevFallbacks(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "123456",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx:nx@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "nine-xing-dev-secret",
	})
	if err == nil {
		t.Fatal("expected production validation to reject weak defaults")
	}
}

func TestValidateProductionAllowsMissingOptionalIntegrations(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	})
	if err != nil {
		t.Fatalf("expected production core config to pass without optional integrations, got %v", err)
	}
}

func TestValidateProductionRejectsExplicitDevIntegrations(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	withWeChatDev := base
	withWeChatDev.WeChat.LoginDev = true
	if err := ValidateProduction(withWeChatDev); err == nil {
		t.Fatal("expected production validation to reject WECHAT_LOGIN_DEV")
	}

	withWxPayDev := base
	withWxPayDev.WxPay.Dev = true
	if err := ValidateProduction(withWxPayDev); err == nil {
		t.Fatal("expected production validation to reject WXPAY_DEV")
	}
}

func TestValidateProductionRejectsVideoGatewayWithoutPublicBaseURL(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err == nil {
		t.Fatal("expected production validation to require PUBLIC_BASE_URL when video gateway is enabled")
	}
}

func TestValidateProductionRequiresExplicitMediaAPIBaseWhenKeyConfigured(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		PublicBaseURL: "https://api.example.com",
	}

	withVideoKey := base
	withVideoKey.Video.APIKey = "video-key"
	if err := ValidateProduction(withVideoKey); err == nil {
		t.Fatal("expected production validation to require VIDEO_API_BASE when VIDEO_API_KEY is set")
	}

	withImageKey := base
	withImageKey.Image.APIKey = "image-key"
	if err := ValidateProduction(withImageKey); err == nil {
		t.Fatal("expected production validation to require IMAGE_API_BASE when IMAGE_API_KEY is set")
	}
}

func TestValidateProductionRejectsPrivatePublicBaseURLWhenVideoGatewayEnabled(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		PublicBaseURL: "http://127.0.0.1:8080",
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err == nil {
		t.Fatal("expected production validation to reject private PUBLIC_BASE_URL when video gateway is enabled")
	}
}

func TestValidateProductionRejectsInvalidTrustedProxyCIDRs(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword:     "a-strong-admin-password",
		AppEnv:            "production",
		DatabaseURL:       "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:         "12345678901234567890123456789012",
		TrustedProxyCIDRs: []string{"not-a-cidr"},
	})
	if err == nil {
		t.Fatal("expected production validation to reject invalid TRUSTED_PROXY_CIDRS")
	}
}

func TestValidateProductionRejectsPlaceholderAdminAndDatabasePasswords(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	withPlaceholderAdmin := base
	withPlaceholderAdmin.AdminPassword = "change-me-to-a-strong-password"
	if err := ValidateProduction(withPlaceholderAdmin); err == nil {
		t.Fatal("expected production validation to reject placeholder ADMIN_PASSWORD")
	}

	withPlaceholderDB := base
	withPlaceholderDB.DatabaseURL = "postgres://nx_app:change-me-too@db:5432/nx_admin?sslmode=disable"
	if err := ValidateProduction(withPlaceholderDB); err == nil {
		t.Fatal("expected production validation to reject placeholder POSTGRES_PASSWORD in DATABASE_URL")
	}
}

func TestValidateProductionRejectsPrivateExternalAPIBases(t *testing.T) {
	base := Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
	}

	for name, mutate := range map[string]func(*Env){
		"MINIMAX_API_BASE": func(env *Env) { env.MiniMax.APIBase = "http://127.0.0.1:8000" },
		"VIDEO_API_BASE":   func(env *Env) { env.Video.APIBase = "http://10.0.0.2:8000" },
		"IMAGE_API_BASE":   func(env *Env) { env.Image.APIBase = "http://localhost:8000" },
		"ASR_API_BASE":     func(env *Env) { env.ASR.APIBase = "http://192.168.1.2:8000" },
		"EMBEDDING_API_BASE": func(env *Env) {
			env.Embedding.APIBase = "http://169.254.169.254/latest"
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := base
			mutate(&env)
			if err := ValidateProduction(env); err == nil {
				t.Fatalf("expected production validation to reject private %s", name)
			}
		})
	}
}

func TestValidateProductionAcceptsCompleteOptionalConfig(t *testing.T) {
	err := ValidateProduction(Env{
		AdminPassword: "a-strong-admin-password",
		AppEnv:        "production",
		DatabaseURL:   "postgres://nx_app:a-strong-database-password@db:5432/nx_admin?sslmode=disable",
		JWTSecret:     "12345678901234567890123456789012",
		SMS: SMSConfig{
			Provider:  "aliyun",
			APIKey:    "ak",
			APISecret: "sk",
		},
		WeChat: WeChatConfig{
			AppID:  "wx-appid",
			Secret: "wx-secret",
		},
		WxPay: WxPayConfig{
			MchID:            "mch",
			AppID:            "wx-appid",
			APIv3Key:         "12345678901234567890123456789012",
			SerialNo:         "serial",
			PrivateKeyPath:   "/run/secrets/apiclient_key.pem",
			PlatformCertPath: "/run/secrets/wechatpay_platform.pem",
			NotifyURL:        "https://api.example.com/api/pay/notify",
		},
		PublicBaseURL:     "https://api.example.com",
		TrustedProxyCIDRs: []string{"127.0.0.1", "10.0.0.0/24"},
		Video: VideoConfig{
			APIBase: "https://video.example.com",
			APIKey:  "video-key",
		},
	})
	if err != nil {
		t.Fatalf("expected complete production config to pass, got %v", err)
	}
}
