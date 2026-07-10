package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDefaultsVideoGatewayContract(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_MODEL_PROFILE", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", "")

	env := Load()

	if env.Video.GatewayContract.Name != "legacy_flat_v1" {
		t.Fatalf("got %#v", env.Video.GatewayContract)
	}
	if env.Video.GatewayContract.Version != "1" {
		t.Fatal("expected contract version 1")
	}
}

func TestLoadVideoGatewayContractFailsClosedForIncompleteIdentity(t *testing.T) {
	cases := []struct {
		name     string
		contract string
		version  string
		body     string
	}{
		{
			name: "body without name",
			body: `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
		{
			name:     "name without version",
			contract: "configured_contract",
			body:     `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
		{
			name:    "version without name",
			version: "2",
			body:    `{"duration":{"name":"seconds","valueType":"int"}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
			t.Setenv("VIDEO_MODEL_PROFILE", "")
			t.Setenv("VIDEO_GATEWAY_CONTRACT", tc.contract)
			t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", tc.version)
			t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", tc.body)

			env := Load()

			if !reflect.DeepEqual(env.Video.GatewayContract, GatewayContractConfig{}) {
				t.Fatalf("expected incomplete explicit contract to fail closed, got %#v", env.Video.GatewayContract)
			}
		})
	}
}

func TestLoadParsesVideoGatewayContract(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_MODEL_PROFILE", " standard ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT", " seedance2_configured_v1 ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", " 7 ")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", `{
		"name":"ignored-json-name",
		"version":"ignored-json-version",
		"declaredModes":["reference","edit","extend"],
		"duration":{"name":"content.duration","valueType":"int","valueMap":{"smart":"-1"}},
		"aspectRatio":{"name":"content.aspect_ratio","valueType":"string"},
		"resolution":{"name":"content.resolution","valueType":"string","valueMap":{"1080P":"1080p"}},
		"generateAudio":{"name":"content.generate_audio","valueType":"bool"},
		"taskMode":{"name":"content.task_mode","valueType":"string"},
		"references":{
			"mode":"content_items",
			"imageField":"image_url",
			"videoField":"video_url",
			"audioField":"audio_url",
			"roleFields":{
				"reference_image":"reference_image",
				"first_frame":"first_frame",
				"last_frame":"last_frame",
				"reference_video":"reference_video",
				"reference_audio":"reference_audio",
				"edit_target":"edit_target",
				"extend_target":"extend_target"
			},
			"supportsRoles":["reference_image","first_frame","last_frame","reference_video","reference_audio","edit_target","extend_target"],
			"requiresTargetFirst":true
		},
		"limits":{"maxImages":9,"maxVideos":3,"maxAudios":3,"maxVideoSecondsTotal":15,"maxAudioSecondsTotal":15},
		"idempotency":{"header":"X-Request-Key"},
		"reconciliation":{
			"lookupByRequestKey":true,
			"method":"GET",
			"pathTemplate":"/v1/videos/by-request/{requestKey}",
			"taskIdPaths":["data.task_id","task_id"],
			"statusPaths":["data.status","status"]
		}
	}`)

	env := Load()

	want := GatewayContractConfig{
		Name:          "seedance2_configured_v1",
		Version:       "7",
		DeclaredModes: []string{"reference", "edit", "extend"},
		Duration: FieldEncoding{
			Name:      "content.duration",
			ValueType: "int",
			ValueMap:  map[string]string{"smart": "-1"},
		},
		AspectRatio: FieldEncoding{Name: "content.aspect_ratio", ValueType: "string"},
		Resolution: FieldEncoding{
			Name:      "content.resolution",
			ValueType: "string",
			ValueMap:  map[string]string{"1080P": "1080p"},
		},
		GenerateAudio: FieldEncoding{Name: "content.generate_audio", ValueType: "bool"},
		TaskMode:      FieldEncoding{Name: "content.task_mode", ValueType: "string"},
		References: ReferenceEncoding{
			Mode:       "content_items",
			ImageField: "image_url",
			VideoField: "video_url",
			AudioField: "audio_url",
			RoleFields: map[string]string{
				"reference_image": "reference_image",
				"first_frame":     "first_frame",
				"last_frame":      "last_frame",
				"reference_video": "reference_video",
				"reference_audio": "reference_audio",
				"edit_target":     "edit_target",
				"extend_target":   "extend_target",
			},
			SupportsRoles:       []string{"reference_image", "first_frame", "last_frame", "reference_video", "reference_audio", "edit_target", "extend_target"},
			RequiresTargetFirst: true,
		},
		Limits: MediaLimits{
			MaxImages:            9,
			MaxVideos:            3,
			MaxAudios:            3,
			MaxVideoSecondsTotal: 15,
			MaxAudioSecondsTotal: 15,
		},
		Idempotency: IdempotencyContract{Header: "X-Request-Key"},
		Reconciliation: ReconciliationContract{
			LookupByRequestKey: true,
			Method:             "GET",
			PathTemplate:       "/v1/videos/by-request/{requestKey}",
			TaskIDPaths:        []string{"data.task_id", "task_id"},
			StatusPaths:        []string{"data.status", "status"},
		},
	}
	if env.Video.ModelProfile != "standard" {
		t.Fatalf("expected trimmed model profile, got %q", env.Video.ModelProfile)
	}
	if !reflect.DeepEqual(env.Video.GatewayContract, want) {
		t.Fatalf("unexpected gateway contract:\n got: %#v\nwant: %#v", env.Video.GatewayContract, want)
	}
}

func TestLoadFailsClosedForUnsafeVideoGatewayContractJSON(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("VIDEO_GATEWAY_CONTRACT", "custom_contract")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "2")
	t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", `{
		"duration":{"name":"content[0]","valueType":"int"},
		"resolution":{"name":"resolution","valueType":"string"}
	}`)

	env := Load()

	if env.Video.GatewayContract.Name != "custom_contract" || env.Video.GatewayContract.Version != "2" {
		t.Fatalf("expected selected contract identity to remain visible, got %#v", env.Video.GatewayContract)
	}
	if env.Video.GatewayContract.Duration.Name != "" || env.Video.GatewayContract.Resolution.Name != "" {
		t.Fatalf("expected unsafe configured fields to fail closed, got %#v", env.Video.GatewayContract)
	}
}

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
