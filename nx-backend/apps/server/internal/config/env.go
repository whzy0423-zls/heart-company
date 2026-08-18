package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/storage"
)

type Env struct {
	AdminPassword     string
	AdminUsername     string
	JWTSecret         string
	XinzhiliSecretKey string
	Port              int
	SiteConfig        string
	// AppEnv 当前运行环境标识（dev / staging / production），供 App 健康检查返回。
	AppEnv string
	// AppVersion 应用版本号，编译时注入或环境变量指定。
	AppVersion string
	AppRelease AppReleaseConfig
	// CORSAllowedOrigins 允许跨域访问 API 的 Origin 白名单；为空时 dev/test 允许任意 Origin，production 不回写 CORS。
	CORSAllowedOrigins []string
	// AdminConfig 后台品牌配置（名称/Logo/加载文案）JSON 文件路径。
	AdminConfig string
	// BuildScript 指向构建+发布官网的脚本绝对路径；为空则关闭自动构建。
	BuildScript string
	// BuildTimeout 单次构建超时（秒），<=0 时使用默认 600s。
	BuildTimeout int
	// DatabaseURL PostgreSQL 连接串，形如 postgres://user:pass@host:5432/db?sslmode=disable
	DatabaseURL string
	// ObjectUploader 允许测试或特殊部署注入自定义对象存储实现；为空时按 OSS_* 环境变量创建。
	ObjectUploader storage.ObjectUploader
	OSS            storage.OSSConfig
	ClassroomMedia ClassroomMediaConfig
	// UploadMaxBytes 单文件上传大小上限，单位 bytes；<=0 时默认 20 MiB。
	UploadDir       string
	UploadMaxBytes  int64
	UploadPublicURL string
	// PublicBaseURL 后端 server 对外可达的根地址（形如 https://api.example.com）。
	// 用于把本地存储产生的相对地址（/api/uploads、/api/upload-assets）补全为
	// 外部视频网关可拉取的绝对地址；外部网关链路不会从请求 Host/X-Forwarded-Host 推断。
	PublicBaseURL string
	// TrustedProxyCIDRs 显式可信反向代理网段；只有命中这些网段时才信任 X-Forwarded-For/X-Real-IP。
	TrustedProxyCIDRs      []string
	MiniMax                MiniMaxConfig
	MiniappChat            MiniappChatConfig
	WeChat                 WeChatConfig
	WxPay                  WxPayConfig
	Embedding              EmbeddingConfig
	SMS                    SMSConfig
	Video                  VideoConfig
	Image                  ImageConfig
	ASR                    ASRConfig
	JPush                  JPushConfig
	DBMaxOpenConns         int
	DBMaxIdleConns         int
	TTSMaxConcurrent       int
	XinzhiliMaxConnections int
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ClassroomMediaConfig controls private multipart uploads for teacher classroom media.
type ClassroomMediaConfig struct {
	Endpoint             string
	Bucket               string
	Region               string
	PartSizeBytes        int64
	MaxParts             int
	CredentialTTLSeconds int
	CoverURLTTLSeconds   int
	MaxVideoBytes        int64
	MaxAudioBytes        int64
}

// SMSConfig 短信发送配置。Provider 为空时非生产环境为 dev 模式；生产环境会 fail closed。
type SMSConfig struct {
	Provider                 string // aliyun | spug | "" (dev)
	APIKey                   string
	APISecret                string
	SignName                 string
	TemplateID               string
	SpugAPIBase              string
	SpugTemplateCode         string
	SpugTemplateName         string
	SpugTimeoutSeconds       int
	SpugReportToken          string
	SpugReportPath           string
	SpugReportChannel        string
	SpugReportTimeoutSeconds int
}

type MiniappChatConfig struct {
	RateLimitPerMinute int
	TimeoutSeconds     int
}

type WeChatConfig struct {
	AppID    string
	Secret   string
	LoginDev bool // true 或未配置 AppID/Secret 时启用本地登录回退
}

// WxPayConfig 微信支付 v3（JSAPI）配置。只有显式 Dev=true 时启用模拟支付。
type WxPayConfig struct {
	MchID            string // 商户号
	AppID            string // 小程序 AppID（下单/拉起用）
	APIv3Key         string // APIv3 密钥（回调解密用）
	SerialNo         string // 商户证书序列号
	PrivateKeyPath   string // 商户私钥 apiclient_key.pem 路径
	PlatformCertPath string // 微信支付平台证书 PEM 路径（回调验签用）
	NotifyURL        string // 支付回调地址（公网 HTTPS）
	ReportPriceCents int    // 深度报告单价（分）
	Dev              bool   // true 时走模拟支付；生产环境禁止开启
}

// EmbeddingConfig 向量化配置（用于 RAG 语义检索）。Provider 为空则关闭向量化。
type EmbeddingConfig struct {
	Provider  string // openai | minimax | "" (关闭)
	APIBase   string
	APIKey    string
	Model     string
	Dimension int
}

type MiniMaxConfig struct {
	Provider       string
	APIBase        string
	APIKey         string
	GroupID        string
	Model          string
	TimeoutSeconds int
	// SystemPrompt 可覆盖对话生成器的系统提示词；为空时使用内置默认。
	SystemPrompt string
}

// VideoConfig 视频生成网关配置（New API / OpenAI 兼容网关）。
// 视频生成为异步：创建任务返回 task_id，需轮询获取结果地址。
type VideoConfig struct {
	APIBase         string
	APIKey          string
	Mode            string
	Model           string
	ModelProfile    string
	GatewayContract GatewayContractConfig
	TimeoutSeconds  int
}

const (
	VideoGenerationModeDemo = "demo"
	VideoGenerationModePaid = "paid"
	paidVideoGenerationAck  = "ALLOW_PAID_VIDEO_GENERATION"
)

func videoGenerationMode(mode, acknowledgement string) string {
	if strings.EqualFold(strings.TrimSpace(mode), VideoGenerationModePaid) &&
		strings.TrimSpace(acknowledgement) == paidVideoGenerationAck {
		return VideoGenerationModePaid
	}
	return VideoGenerationModeDemo
}

// ImageConfig 文生图网关配置（gpt-image-2，OpenAI 兼容 / 中转代理）。
// 图像生成为同步：POST /v1/images/generations 直接返回 base64(b64_json)。
type ImageConfig struct {
	APIBase        string
	APIKey         string
	Model          string
	TimeoutSeconds int
}

// ASRConfig 语音转文字配置（OpenAI 兼容 / 中转代理）。
// 通过 POST /v1/audio/transcriptions 上传 multipart 音频并返回文本。
type ASRConfig struct {
	APIBase        string
	APIKey         string
	Model          string
	TimeoutSeconds int
}

// JPushConfig 极光推送配置。AppKey 为空时推送功能关闭（dev 模式仅写日志）。
type JPushConfig struct {
	AppKey       string
	MasterSecret string
}

var (
	ErrAppReleaseCertificateNotConfigured = errors.New("app release certificate SHA-256 is not configured")
	ErrInvalidAppReleaseCertificate       = errors.New("invalid app release certificate SHA-256")
)

type AppReleaseConfig struct {
	PackageName       string
	CertificateSHA256 string
}

func (c AppReleaseConfig) ExpectedCertificateSHA256() (string, error) {
	if strings.TrimSpace(c.CertificateSHA256) == "" {
		return "", ErrAppReleaseCertificateNotConfigured
	}

	normalized := strings.Map(func(r rune) rune {
		if r == ':' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, c.CertificateSHA256)
	if len(normalized) != 64 {
		return "", ErrInvalidAppReleaseCertificate
	}
	for _, r := range normalized {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", ErrInvalidAppReleaseCertificate
		}
	}
	return normalized, nil
}

func positiveIntEnv(key string, fallback int) int {
	v, err := strconv.Atoi(getenv(key, ""))
	if err != nil || v <= 0 {
		return fallback
	}
	max := 1000000
	if key == "CLASSROOM_MEDIA_PART_SIZE_MB" {
		max = 1024
	}
	if key == "CLASSROOM_MEDIA_MAX_PARTS" {
		max = 10000
	}
	if key == "CLASSROOM_MEDIA_CREDENTIAL_TTL_SECONDS" || key == "CLASSROOM_COVER_URL_TTL_SECONDS" {
		max = 86400
	}
	if v > max {
		return fallback
	}
	return v
}
func positiveInt64Env(key string, fallback int64) int64 {
	v, err := strconv.ParseInt(getenv(key, ""), 10, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	if key == "CLASSROOM_MEDIA_MAX_VIDEO_MB" && v > 4*1024*1024 {
		return fallback
	}
	if key == "CLASSROOM_MEDIA_MAX_AUDIO_MB" && v > 1024*1024 {
		return fallback
	}
	return v
}

func Load() Env {
	loadDotEnv()

	port, err := strconv.Atoi(getenv("PORT", "5320"))
	if err != nil {
		port = 5320
	}

	siteConfig, err := filepath.Abs(getenv("SITE_CONFIG_PATH", "../../../shared/site-config.json"))
	if err != nil {
		siteConfig = "../../../shared/site-config.json"
	}

	adminConfig, err := filepath.Abs(getenv("ADMIN_CONFIG_PATH", "../../../shared/admin-config.json"))
	if err != nil {
		adminConfig = "../../../shared/admin-config.json"
	}

	buildScript := getenv("BUILD_SCRIPT", "")
	if buildScript != "" {
		if abs, absErr := filepath.Abs(buildScript); absErr == nil {
			buildScript = abs
		}
	}

	buildTimeout, err := strconv.Atoi(getenv("BUILD_TIMEOUT_SECONDS", "600"))
	if err != nil {
		buildTimeout = 600
	}
	uploadMaxMB, err := strconv.Atoi(getenv("UPLOAD_MAX_MB", "20"))
	if err != nil || uploadMaxMB <= 0 {
		uploadMaxMB = 20
	}
	minimaxTimeout, err := strconv.Atoi(getenv("MINIMAX_TIMEOUT_SECONDS", "25"))
	if err != nil || minimaxTimeout <= 0 {
		minimaxTimeout = 25
	}
	miniappChatLimit, err := strconv.Atoi(getenv("MINIAPP_CHAT_RATE_LIMIT_PER_MINUTE", "12"))
	if err != nil || miniappChatLimit <= 0 {
		miniappChatLimit = 12
	}
	miniappChatTimeout, err := strconv.Atoi(getenv("MINIAPP_CHAT_TIMEOUT_SECONDS", "28"))
	if err != nil || miniappChatTimeout <= 0 {
		miniappChatTimeout = 28
	}
	dbMaxOpen, _ := strconv.Atoi(getenv("DB_MAX_OPEN_CONNS", "20"))
	if dbMaxOpen <= 0 {
		dbMaxOpen = 20
	}
	dbMaxIdle, _ := strconv.Atoi(getenv("DB_MAX_IDLE_CONNS", "5"))
	if dbMaxIdle < 0 || dbMaxIdle > dbMaxOpen {
		dbMaxIdle = minInt(5, dbMaxOpen)
	}
	ttsMaxConcurrent, _ := strconv.Atoi(getenv("XINZHILI_TTS_MAX_CONCURRENT", "8"))
	if ttsMaxConcurrent <= 0 {
		ttsMaxConcurrent = 8
	}
	maxRealtime, _ := strconv.Atoi(getenv("XINZHILI_MAX_CONNECTIONS", "50"))
	if maxRealtime <= 0 {
		maxRealtime = 50
	}

	ossPublicURL := getenv("OSS_PUBLIC_URL", "")
	uploadDir, err := filepath.Abs(getenv("UPLOAD_DIR", "../../../website-react/public/assets/uploads"))
	if err != nil {
		uploadDir = "../../../website-react/public/assets/uploads"
	}

	reportPrice, err := strconv.Atoi(getenv("WXPAY_REPORT_PRICE_CENTS", "990"))
	if err != nil || reportPrice <= 0 {
		reportPrice = 990 // 默认 ￥9.9
	}
	wxpay := WxPayConfig{
		MchID:            getenv("WXPAY_MCH_ID", ""),
		AppID:            getenv("WXPAY_APPID", getenv("WECHAT_APPID", "")),
		APIv3Key:         getenv("WXPAY_API_V3_KEY", ""),
		SerialNo:         getenv("WXPAY_SERIAL_NO", ""),
		PrivateKeyPath:   getenv("WXPAY_PRIVATE_KEY_PATH", ""),
		PlatformCertPath: getenv("WXPAY_PLATFORM_CERT_PATH", ""),
		NotifyURL:        getenv("WXPAY_NOTIFY_URL", ""),
		ReportPriceCents: reportPrice,
	}
	// 只有显式开启时启用 dev 模拟支付；非生产缺配置时 server 会自动 dev，生产缺配置会禁用支付功能。
	wxpay.Dev = getenv("WXPAY_DEV", "") == "true"

	embDim, err := strconv.Atoi(getenv("EMBEDDING_DIMENSION", "1536"))
	if err != nil || embDim <= 0 {
		embDim = 1536
	}
	embedding := EmbeddingConfig{
		Provider:  getenv("EMBEDDING_PROVIDER", ""),
		APIBase:   getenv("EMBEDDING_API_BASE", ""),
		APIKey:    getenv("EMBEDDING_API_KEY", ""),
		Model:     getenv("EMBEDDING_MODEL", ""),
		Dimension: embDim,
	}

	videoTimeout, err := strconv.Atoi(getenv("VIDEO_TIMEOUT_SECONDS", "120"))
	if err != nil || videoTimeout <= 0 {
		videoTimeout = 120
	}

	imageTimeout, err := strconv.Atoi(getenv("IMAGE_TIMEOUT_SECONDS", "120"))
	if err != nil || imageTimeout <= 0 {
		imageTimeout = 120
	}
	asrTimeout, err := strconv.Atoi(getenv("ASR_TIMEOUT_SECONDS", "60"))
	if err != nil || asrTimeout <= 0 {
		asrTimeout = 60
	}
	spugTimeout, err := strconv.Atoi(getenv("SPUG_PUSH_TIMEOUT_SECONDS", "10"))
	if err != nil || spugTimeout <= 0 {
		spugTimeout = 10
	}
	spugReportTimeout, err := strconv.Atoi(getenv("SPUG_PUSH_REPORT_TIMEOUT_SECONDS", "5"))
	if err != nil || spugReportTimeout <= 0 {
		spugReportTimeout = 5
	}

	appEnv := NormalizeAppEnv(getenv("APP_ENV", ""))

	classroomPartMB := positiveIntEnv("CLASSROOM_MEDIA_PART_SIZE_MB", 8)
	classroomMaxParts := positiveIntEnv("CLASSROOM_MEDIA_MAX_PARTS", 10000)
	classroomTTL := positiveIntEnv("CLASSROOM_MEDIA_CREDENTIAL_TTL_SECONDS", 900)
	classroomCoverTTL := positiveIntEnv("CLASSROOM_COVER_URL_TTL_SECONDS", 1800)
	classroomVideoMB := positiveInt64Env("CLASSROOM_MEDIA_MAX_VIDEO_MB", 4096)
	classroomAudioMB := positiveInt64Env("CLASSROOM_MEDIA_MAX_AUDIO_MB", 512)
	classroomMedia := ClassroomMediaConfig{Endpoint: getenv("CLASSROOM_MEDIA_ENDPOINT", getenv("OSS_ENDPOINT", "")), Bucket: getenv("CLASSROOM_MEDIA_BUCKET", getenv("OSS_BUCKET", "")), Region: getenv("CLASSROOM_MEDIA_REGION", getenv("OSS_REGION", "")), PartSizeBytes: int64(classroomPartMB) * 1024 * 1024, MaxParts: classroomMaxParts, CredentialTTLSeconds: classroomTTL, CoverURLTTLSeconds: classroomCoverTTL, MaxVideoBytes: classroomVideoMB * 1024 * 1024, MaxAudioBytes: classroomAudioMB * 1024 * 1024}

	return Env{
		AdminPassword: getenv("ADMIN_PASSWORD", "123456"),
		AdminUsername: getenv("ADMIN_USERNAME", "admin"),
		AppEnv:        appEnv,
		AppVersion:    getenv("APP_VERSION", "0.0.1"),
		AppRelease: AppReleaseConfig{
			PackageName:       strings.TrimSpace(getenv("APP_RELEASE_PACKAGE_NAME", "com.xinzhili.nine_xing_app")),
			CertificateSHA256: strings.TrimSpace(getenv("APP_RELEASE_CERT_SHA256", "")),
		},
		CORSAllowedOrigins: parseCSV(getenv("CORS_ALLOWED_ORIGINS", "")),
		XinzhiliSecretKey:  strings.TrimSpace(getenv("XINZHILI_SECRET_KEY", "")),
		JWTSecret:          getenv("JWT_SECRET", "nine-xing-dev-secret"),
		Port:               port,
		SiteConfig:         siteConfig,
		AdminConfig:        adminConfig,
		BuildScript:        buildScript,
		BuildTimeout:       buildTimeout,
		DatabaseURL:        getenv("DATABASE_URL", "postgres://nx:nx@localhost:5432/nx_admin?sslmode=disable"),
		ClassroomMedia:     classroomMedia,
		OSS: storage.OSSConfig{
			AccessKeyID:     getenv("OSS_ACCESS_KEY_ID", ""),
			AccessKeySecret: getenv("OSS_ACCESS_KEY_SECRET", ""),
			Bucket:          getenv("OSS_BUCKET", ""),
			Endpoint:        getenv("OSS_ENDPOINT", ""),
			PublicURL:       ossPublicURL,
			Region:          getenv("OSS_REGION", ""),
			Prefix:          getenv("OSS_PREFIX", "uploads"),
		},
		UploadDir:         uploadDir,
		UploadMaxBytes:    int64(uploadMaxMB) * 1024 * 1024,
		UploadPublicURL:   ossPublicURL,
		PublicBaseURL:     strings.TrimRight(strings.TrimSpace(getenv("PUBLIC_BASE_URL", "")), "/"),
		TrustedProxyCIDRs: parseCSV(getenv("TRUSTED_PROXY_CIDRS", "")),
		MiniMax: MiniMaxConfig{
			Provider:       getenv("CHAT_MODEL_PROVIDER", "openai-compatible"),
			APIBase:        getenv("MINIMAX_API_BASE", "https://api.minimaxi.com"),
			APIKey:         getenv("MINIMAX_API_KEY", ""),
			GroupID:        getenv("MINIMAX_GROUP_ID", ""),
			Model:          getenv("MINIMAX_MODEL", "abab6.5s-chat"),
			TimeoutSeconds: minimaxTimeout,
			SystemPrompt:   getenv("MINIMAX_SYSTEM_PROMPT", ""),
		},
		MiniappChat: MiniappChatConfig{
			RateLimitPerMinute: miniappChatLimit,
			TimeoutSeconds:     miniappChatTimeout,
		},
		DBMaxOpenConns: dbMaxOpen, DBMaxIdleConns: dbMaxIdle, TTSMaxConcurrent: ttsMaxConcurrent, XinzhiliMaxConnections: maxRealtime,
		WeChat: WeChatConfig{
			AppID:    getenv("WECHAT_APPID", ""),
			Secret:   getenv("WECHAT_SECRET", ""),
			LoginDev: getenv("WECHAT_LOGIN_DEV", "") == "true",
		},
		WxPay:     wxpay,
		Embedding: embedding,
		SMS: SMSConfig{
			Provider:                 getenv("SMS_PROVIDER", ""),
			APIKey:                   getenv("SMS_API_KEY", ""),
			APISecret:                getenv("SMS_API_SECRET", ""),
			SignName:                 getenv("SMS_SIGN_NAME", ""),
			TemplateID:               getenv("SMS_TEMPLATE_ID", ""),
			SpugAPIBase:              getenv("SPUG_PUSH_API_BASE", "https://push.spug.cc"),
			SpugTemplateCode:         getenv("SPUG_PUSH_TEMPLATE_CODE", ""),
			SpugTemplateName:         getenv("SPUG_PUSH_TEMPLATE_NAME", "芯之力"),
			SpugTimeoutSeconds:       spugTimeout,
			SpugReportToken:          getenv("SPUG_PUSH_REPORT_TOKEN", ""),
			SpugReportPath:           getenv("SPUG_PUSH_REPORT_PATH", "/send"),
			SpugReportChannel:        getenv("SPUG_PUSH_REPORT_CHANNEL", ""),
			SpugReportTimeoutSeconds: spugReportTimeout,
		},
		Video: VideoConfig{
			APIBase:         getenv("VIDEO_API_BASE", ""),
			APIKey:          getenv("VIDEO_API_KEY", ""),
			Mode:            videoGenerationMode(getenv("VIDEO_GENERATION_MODE", ""), getenv("VIDEO_PAID_GENERATION_ACK", "")),
			Model:           getenv("VIDEO_MODEL", "video-ds-2.0-fast"),
			ModelProfile:    strings.TrimSpace(getenv("VIDEO_MODEL_PROFILE", "")),
			GatewayContract: loadVideoGatewayContract(),
			TimeoutSeconds:  videoTimeout,
		},
		Image: ImageConfig{
			APIBase:        getenv("IMAGE_API_BASE", ""),
			APIKey:         getenv("IMAGE_API_KEY", ""),
			Model:          getenv("IMAGE_MODEL", "gpt-image-2"),
			TimeoutSeconds: imageTimeout,
		},
		ASR: ASRConfig{
			APIBase:        getenv("ASR_API_BASE", "https://api.siliconflow.cn"),
			APIKey:         getenv("ASR_API_KEY", ""),
			Model:          getenv("ASR_MODEL", "FunAudioLLM/SenseVoiceSmall"),
			TimeoutSeconds: asrTimeout,
		},
		JPush: JPushConfig{
			AppKey:       getenv("JPUSH_APP_KEY", ""),
			MasterSecret: getenv("JPUSH_MASTER_SECRET", ""),
		},
	}
}

func ValidateProduction(env Env) error {
	appEnv := NormalizeAppEnv(env.AppEnv)
	if err := validateAppEnv(appEnv); err != nil {
		return err
	}
	if appEnv != "production" {
		return nil
	}
	if weakSecret(env.JWTSecret) {
		return fmt.Errorf("production JWT_SECRET must be a strong random secret")
	}
	if weakPassword(env.AdminPassword) {
		return fmt.Errorf("production ADMIN_PASSWORD must be changed to a strong password")
	}
	if strings.Contains(env.DatabaseURL, "://nx:nx@") || databasePasswordWeak(env.DatabaseURL) {
		return fmt.Errorf("production DATABASE_URL/POSTGRES_PASSWORD must use strong non-placeholder credentials")
	}
	if env.WeChat.LoginDev {
		return fmt.Errorf("production WECHAT_LOGIN_DEV must be false")
	}
	if env.WxPay.Dev {
		return fmt.Errorf("production WXPAY_DEV must be false")
	}
	if strings.TrimSpace(env.Video.APIKey) != "" {
		publicBaseURL := strings.TrimSpace(env.PublicBaseURL)
		if publicBaseURL == "" {
			return fmt.Errorf("production PUBLIC_BASE_URL must be set when video gateway is enabled")
		}
		if !netguard.IsPublicHTTPURL(publicBaseURL) {
			return fmt.Errorf("production PUBLIC_BASE_URL must be a public http(s) URL when video gateway is enabled")
		}
	}
	if err := validateTrustedProxyCIDRs(env.TrustedProxyCIDRs); err != nil {
		return err
	}
	if err := validateProductionExternalAPIBases(env); err != nil {
		return err
	}
	return nil
}

func NormalizeAppEnv(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "":
		return ""
	case "local", "development":
		return "dev"
	case "prod":
		return "production"
	default:
		return value
	}
}

func IsProduction(raw string) bool {
	return NormalizeAppEnv(raw) == "production"
}

func validateAppEnv(value string) error {
	switch NormalizeAppEnv(value) {
	case "dev", "test", "staging", "production":
		return nil
	default:
		return fmt.Errorf("APP_ENV must be explicitly set to one of dev, test, staging, production")
	}
}

func validateProductionExternalAPIBases(env Env) error {
	if strings.TrimSpace(env.Video.APIKey) != "" && strings.TrimSpace(env.Video.APIBase) == "" {
		return fmt.Errorf("production VIDEO_API_BASE must be set when VIDEO_API_KEY is configured")
	}
	if strings.TrimSpace(env.Image.APIKey) != "" && strings.TrimSpace(env.Image.APIBase) == "" {
		return fmt.Errorf("production IMAGE_API_BASE must be set when IMAGE_API_KEY is configured")
	}
	for label, apiBase := range map[string]string{
		"ASR_API_BASE":       env.ASR.APIBase,
		"EMBEDDING_API_BASE": env.Embedding.APIBase,
		"IMAGE_API_BASE":     env.Image.APIBase,
		"MINIMAX_API_BASE":   env.MiniMax.APIBase,
		"VIDEO_API_BASE":     env.Video.APIBase,
	} {
		apiBase = strings.TrimSpace(apiBase)
		if apiBase == "" {
			continue
		}
		if !netguard.IsPublicHTTPURL(apiBase) {
			return fmt.Errorf("production %s must be a public http(s) URL", label)
		}
	}
	return nil
}

func validateTrustedProxyCIDRs(values []string) error {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := netip.ParsePrefix(value); err == nil {
			continue
		}
		if _, err := netip.ParseAddr(value); err != nil {
			return fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR/IP %q", value)
		}
	}
	return nil
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimRight(strings.TrimSpace(part), "/")
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func weakSecret(secret string) bool {
	secret = strings.TrimSpace(secret)
	return len(secret) < 32 ||
		secret == "nine-xing-dev-secret" ||
		strings.Contains(secret, "please-change")
}

func weakPassword(password string) bool {
	password = strings.TrimSpace(password)
	lower := strings.ToLower(password)
	return len(password) < 12 ||
		password == "123456" ||
		lower == "password" ||
		strings.Contains(lower, "please-change") ||
		strings.Contains(lower, "change-me") ||
		strings.Contains(lower, "changeme")
}

func databasePasswordWeak(databaseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil {
		return true
	}
	if u.Scheme == "" || u.Host == "" || u.User == nil {
		return true
	}
	password, ok := u.User.Password()
	if !ok {
		return true
	}
	return weakPassword(password)
}

func loadDotEnv() {
	if explicit := strings.TrimSpace(os.Getenv("ENV_FILE")); explicit != "" {
		loadDotEnvFile(explicit)
		return
	}
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			loadDotEnvFile(candidate)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func loadDotEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "export ") && !strings.Contains(line, "=") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
