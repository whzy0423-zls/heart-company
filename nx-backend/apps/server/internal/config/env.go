package config

import (
	"os"
	"path/filepath"
	"strconv"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type Env struct {
	AdminPassword string
	AdminUsername string
	JWTSecret     string
	Port          int
	SiteConfig    string
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
	MiniMax         MiniMaxConfig
}

type MiniMaxConfig struct {
	APIBase string
	APIKey  string
	GroupID string
}

func Load() Env {
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

	ossPublicURL := getenv("OSS_PUBLIC_URL", "")
	uploadDir, err := filepath.Abs(getenv("UPLOAD_DIR", "../../../website-react/public/assets/uploads"))
	if err != nil {
		uploadDir = "../../../website-react/public/assets/uploads"
	}

	return Env{
		AdminPassword: getenv("ADMIN_PASSWORD", "123456"),
		AdminUsername: getenv("ADMIN_USERNAME", "admin"),
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
		MiniMax: MiniMaxConfig{
			APIBase: getenv("MINIMAX_API_BASE", "https://api.minimaxi.com"),
			APIKey:  getenv("MINIMAX_API_KEY", ""),
			GroupID: getenv("MINIMAX_GROUP_ID", ""),
		},
	}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
