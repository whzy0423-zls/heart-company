package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type Env struct {
	AdminPassword string
	AdminUsername string
	JWTSecret     string
	Port          int
	SiteConfig    string
	// AppEnv 当前运行环境标识（dev / staging / production），供 App 健康检查返回。
	AppEnv string
	// AppVersion 应用版本号，编译时注入或环境变量指定。
	AppVersion string
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
	// UploadMaxBytes 单文件上传大小上限，单位 bytes；<=0 时默认 20 MiB。
	UploadDir       string
	UploadMaxBytes  int64
	UploadPublicURL string
	// PublicBaseURL 后端 server 对外可达的根地址（形如 https://api.example.com）。
	// 用于把本地存储产生的相对地址（/api/uploads、/api/upload-assets）补全为
	// 外部视频网关可拉取的绝对地址；为空时回退到当前请求推断的 scheme://host。
	PublicBaseURL string
	MiniMax       MiniMaxConfig
	MiniappChat   MiniappChatConfig
	WeChat        WeChatConfig
	WxPay         WxPayConfig
	Embedding     EmbeddingConfig
	SMS           SMSConfig
	Video         VideoConfig
	Image         ImageConfig
}

// SMSConfig 短信发送配置。Provider 为空时为 dev 模式（不实际发送，验证码写日志/返回响应）。
type SMSConfig struct {
	Provider   string // aliyun | "" (dev)
	APIKey     string
	APISecret  string
	SignName   string
	TemplateID string
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
	NotifyURL        string // 支付回调地址（公网 HTTPS）
	ReportPriceCents int    // 深度报告单价（分）
	Dev              bool   // true 或配置不全时走模拟支付
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
	APIBase        string
	APIKey         string
	Model          string
	TimeoutSeconds int
}

// ImageConfig 文生图网关配置（gpt-image-2，OpenAI 兼容 / 中转代理）。
// 图像生成为同步：POST /v1/images/generations 直接返回 base64(b64_json)。
type ImageConfig struct {
	APIBase        string
	APIKey         string
	Model          string
	TimeoutSeconds int
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
		NotifyURL:        getenv("WXPAY_NOTIFY_URL", ""),
		ReportPriceCents: reportPrice,
	}
	// 只有显式开启时启用 dev 模拟支付；非生产缺配置时 server 会自动 dev，生产缺配置会启动失败。
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

	return Env{
		AdminPassword: getenv("ADMIN_PASSWORD", "123456"),
		AdminUsername: getenv("ADMIN_USERNAME", "admin"),
		AppEnv:        getenv("APP_ENV", "dev"),
		AppVersion:    getenv("APP_VERSION", "0.0.1"),
		JWTSecret:     getenv("JWT_SECRET", "nine-xing-dev-secret"),
		Port:          port,
		SiteConfig:    siteConfig,
		AdminConfig:   adminConfig,
		BuildScript:   buildScript,
		BuildTimeout:  buildTimeout,
		DatabaseURL:   getenv("DATABASE_URL", "postgres://nx:nx@localhost:5432/nx_admin?sslmode=disable"),
		OSS: storage.OSSConfig{
			AccessKeyID:     getenv("OSS_ACCESS_KEY_ID", ""),
			AccessKeySecret: getenv("OSS_ACCESS_KEY_SECRET", ""),
			Bucket:          getenv("OSS_BUCKET", ""),
			Endpoint:        getenv("OSS_ENDPOINT", ""),
			PublicURL:       ossPublicURL,
			Region:          getenv("OSS_REGION", ""),
			Prefix:          getenv("OSS_PREFIX", "uploads"),
		},
		UploadDir:       uploadDir,
		UploadMaxBytes:  int64(uploadMaxMB) * 1024 * 1024,
		UploadPublicURL: ossPublicURL,
		PublicBaseURL:   strings.TrimRight(strings.TrimSpace(getenv("PUBLIC_BASE_URL", "")), "/"),
		MiniMax: MiniMaxConfig{
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
		WeChat: WeChatConfig{
			AppID:    getenv("WECHAT_APPID", ""),
			Secret:   getenv("WECHAT_SECRET", ""),
			LoginDev: getenv("WECHAT_LOGIN_DEV", "") == "true",
		},
		WxPay:     wxpay,
		Embedding: embedding,
		SMS: SMSConfig{
			Provider:   getenv("SMS_PROVIDER", ""),
			APIKey:     getenv("SMS_API_KEY", ""),
			APISecret:  getenv("SMS_API_SECRET", ""),
			SignName:   getenv("SMS_SIGN_NAME", ""),
			TemplateID: getenv("SMS_TEMPLATE_ID", ""),
		},
		Video: VideoConfig{
			APIBase:        getenv("VIDEO_API_BASE", "https://zz1cc.cc.cd"),
			APIKey:         getenv("VIDEO_API_KEY", ""),
			Model:          getenv("VIDEO_MODEL", "video-ds-2.0-fast"),
			TimeoutSeconds: videoTimeout,
		},
		Image: ImageConfig{
			APIBase:        getenv("IMAGE_API_BASE", "https://zz1cc.cc.cd"),
			APIKey:         getenv("IMAGE_API_KEY", ""),
			Model:          getenv("IMAGE_MODEL", "gpt-image-2"),
			TimeoutSeconds: imageTimeout,
		},
	}
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
