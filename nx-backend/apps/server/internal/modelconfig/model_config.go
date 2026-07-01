// Package modelconfig 负责把"对话模型(MiniMax)"与"视频模型"的可配置参数
// （接口地址 / 密钥 / 模型名 / GroupID）持久化到 site_configs KV 表，
// 并在运行时与环境变量基线合并，供 server 重建对应客户端使用。
//
// 设计要点：
//   - 复用既有的 site_configs(key, config jsonb, update_time) 表，key 固定为 "model_config"。
//   - DB 中仅存"覆盖值"：任何为空的字段都会回退到环境变量基线（env），
//     这样首次部署无需写库即可工作，后台保存后才落库覆盖。
//   - 密钥永不回显：HTTP 层负责脱敏，本包只负责存储与合并。
package modelconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
)

const configKey = "model_config"

// DefaultAnalysisModel 是 MiniMax 官方多模态视频理解默认模型。
const DefaultAnalysisModel = "MiniMax-M3"

// DefaultAnalysisTimeoutSeconds 给视频理解预留更长响应时间。
const DefaultAnalysisTimeoutSeconds = 180

// ChatConfig 对话模型（MiniMax 兼容）可配置项。
type ChatConfig struct {
	APIBase string `json:"apiBase"`
	APIKey  string `json:"apiKey"`
	GroupID string `json:"groupId"`
	Model   string `json:"model"`
}

// VideoConfig 视频模型（New API / OpenAI 兼容网关）可配置项。
type VideoConfig struct {
	APIBase string `json:"apiBase"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

// ImageConfig 文生图模型（gpt-image-2，OpenAI 兼容 / 中转代理）可配置项。
type ImageConfig struct {
	APIBase string `json:"apiBase"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

// AnalysisConfig 视频分析模型（MiniMax/多模态兼容）可配置项。
type AnalysisConfig struct {
	APIBase string `json:"apiBase"`
	APIKey  string `json:"apiKey"`
	GroupID string `json:"groupId"`
	Model   string `json:"model"`
}

// AssistConfig 控制"芯之力专属模型"在聊天里的智能辅助行为：
//   - Enabled：是否启用 AI 辅助作答（关闭后聊天仅走资料检索/固定兜底）。
//     用 *bool 区分"未设置(默认启用)"与"显式关闭(false)"，避免 DB 旧记录被反序列化成 false 而误关。
//   - SystemPrompt：覆盖系统提示词；为空时使用内置默认。
type AssistConfig struct {
	Enabled      *bool  `json:"enabled,omitempty"`
	SystemPrompt string `json:"systemPrompt"`
}

// Config 模型配置覆盖值集合，整体作为一条 JSON 落库。
type Config struct {
	Chat     ChatConfig     `json:"chat"`
	Video    VideoConfig    `json:"video"`
	Image    ImageConfig    `json:"image"`
	Analysis AnalysisConfig `json:"analysis"`
	Assist   AssistConfig   `json:"assist"`
}

// AssistEnabled 解析 AI 辅助开关：未设置时默认启用。
func (c Config) AssistEnabled() bool {
	if c.Assist.Enabled == nil {
		return true
	}
	return *c.Assist.Enabled
}

// ReadStore 从 DB 读取覆盖配置。第二个返回值表示是否已存在记录；
// 当 db 为空或无记录时返回零值 Config 且 found=false。
func ReadStore(ctx context.Context, db *sql.DB) (Config, bool, error) {
	if db == nil {
		return Config{}, false, nil
	}

	c, cancel := context.WithTimeout(ctxOrBackground(ctx), 10*time.Second)
	defer cancel()

	var raw []byte
	err := db.QueryRowContext(c, `SELECT config FROM site_configs WHERE key=$1`, configKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, false, err
	}
	return cfg.trimmed(), true, nil
}

// UpsertStore 将覆盖配置写入 DB（key=model_config）。
func UpsertStore(ctx context.Context, db *sql.DB, cfg Config) error {
	if db == nil {
		return errors.New("数据库未初始化，无法保存模型配置")
	}

	c, cancel := context.WithTimeout(ctxOrBackground(ctx), 10*time.Second)
	defer cancel()

	body, err := json.Marshal(cfg.trimmed())
	if err != nil {
		return err
	}
	_, err = db.ExecContext(c,
		`INSERT INTO site_configs (key, config, update_time)
		 VALUES ($1, $2::jsonb, now())
		 ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()`,
		configKey, string(body),
	)
	return err
}

// ApplyChat 把覆盖值叠加到环境变量基线上，空字段回退到 base。
func (c Config) ApplyChat(base config.MiniMaxConfig) config.MiniMaxConfig {
	out := base
	if v := strings.TrimSpace(c.Chat.APIBase); v != "" {
		out.APIBase = v
	}
	if v := strings.TrimSpace(c.Chat.APIKey); v != "" {
		out.APIKey = v
	}
	if v := strings.TrimSpace(c.Chat.GroupID); v != "" {
		out.GroupID = v
	}
	if v := strings.TrimSpace(c.Chat.Model); v != "" {
		out.Model = v
	}
	if v := strings.TrimSpace(c.Assist.SystemPrompt); v != "" {
		out.SystemPrompt = v
	}
	return out
}

// ApplyVideo 把覆盖值叠加到环境变量基线上，空字段回退到 base。
func (c Config) ApplyVideo(base config.VideoConfig) config.VideoConfig {
	out := base
	if v := strings.TrimSpace(c.Video.APIBase); v != "" {
		out.APIBase = v
	}
	if v := strings.TrimSpace(c.Video.APIKey); v != "" {
		out.APIKey = v
	}
	if v := strings.TrimSpace(c.Video.Model); v != "" {
		out.Model = v
	}
	return out
}

// ApplyImage 把覆盖值叠加到环境变量基线上，空字段回退到 base。
func (c Config) ApplyImage(base config.ImageConfig) config.ImageConfig {
	out := base
	if v := strings.TrimSpace(c.Image.APIBase); v != "" {
		out.APIBase = v
	}
	if v := strings.TrimSpace(c.Image.APIKey); v != "" {
		out.APIKey = v
	}
	if v := strings.TrimSpace(c.Image.Model); v != "" {
		out.Model = v
	}
	return out
}

// ApplyAnalysis 使用语音生成同一套 MiniMax 地址/密钥/GroupID 作为基线；
// 视频分析仅允许覆盖模型名，避免误配到文本中转网关导致无法读取 video_url。
func (c Config) ApplyAnalysis(base config.MiniMaxConfig) config.MiniMaxConfig {
	out := base
	out.Model = DefaultAnalysisModel
	out.TimeoutSeconds = DefaultAnalysisTimeoutSeconds
	if v := strings.TrimSpace(c.Analysis.Model); isMiniMaxAnalysisModel(v) {
		out.Model = v
	}
	return out
}

func isMiniMaxAnalysisModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "minimax")
}

// MergeIncoming 把后台提交的新值合并到当前覆盖配置之上。
// 密钥字段为空表示"不修改"，沿用 c 中已有的值；其余字段直接覆盖（含置空）。
func (c Config) MergeIncoming(in Config) Config {
	out := in.trimmed()
	if out.Chat.APIKey == "" {
		out.Chat.APIKey = c.Chat.APIKey
	}
	if out.Video.APIKey == "" {
		out.Video.APIKey = c.Video.APIKey
	}
	if out.Image.APIKey == "" {
		out.Image.APIKey = c.Image.APIKey
	}
	if out.Analysis.APIKey == "" {
		out.Analysis.APIKey = c.Analysis.APIKey
	}
	// AI 辅助开关：提交未带 enabled 字段时沿用已存值。
	if out.Assist.Enabled == nil {
		out.Assist.Enabled = c.Assist.Enabled
	}
	return out
}

func (c Config) trimmed() Config {
	return Config{
		Chat: ChatConfig{
			APIBase: strings.TrimSpace(c.Chat.APIBase),
			APIKey:  strings.TrimSpace(c.Chat.APIKey),
			GroupID: strings.TrimSpace(c.Chat.GroupID),
			Model:   strings.TrimSpace(c.Chat.Model),
		},
		Video: VideoConfig{
			APIBase: strings.TrimSpace(c.Video.APIBase),
			APIKey:  strings.TrimSpace(c.Video.APIKey),
			Model:   strings.TrimSpace(c.Video.Model),
		},
		Image: ImageConfig{
			APIBase: strings.TrimSpace(c.Image.APIBase),
			APIKey:  strings.TrimSpace(c.Image.APIKey),
			Model:   strings.TrimSpace(c.Image.Model),
		},
		Analysis: AnalysisConfig{
			APIBase: strings.TrimSpace(c.Analysis.APIBase),
			APIKey:  strings.TrimSpace(c.Analysis.APIKey),
			GroupID: strings.TrimSpace(c.Analysis.GroupID),
			Model:   strings.TrimSpace(c.Analysis.Model),
		},
		Assist: AssistConfig{
			Enabled:      c.Assist.Enabled,
			SystemPrompt: strings.TrimSpace(c.Assist.SystemPrompt),
		},
	}
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
