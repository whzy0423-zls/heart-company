// Package xinzhili contains the configuration and runtime building blocks for
// the realtime voice experience. Its configuration is deliberately isolated
// from the legacy HTTP ASR and model_config settings used by ordinary chat.
package xinzhili

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	RealtimeASRProvider = "aliyun-bailian"
	RealtimeASRModel    = "paraformer-realtime-v2"

	TTSProviderOpenAICompatible = "openai-compatible"
	TTSProviderMiniMax          = "minimax"
	TTSProviderBailian          = "bailian"

	maxEndpointRunes = 2048
	maxAPIKeyRunes   = 4096
	maxShortRunes    = 256
	maxPromptRunes   = 8000
)

var (
	ErrNormalModeRequired = errors.New("芯之力启用时必须包含正常模式")
	ErrConfigConflict     = errors.New("芯之力配置版本冲突")
)

type Mode string

const (
	ModeNormal        Mode = "normal"
	ModeArgument      Mode = "argument"
	ModeComfort       Mode = "comfort"
	ModeDeepListening Mode = "deep_listening"
)

type RealtimeASRConfig struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"apiKey"`
	Region   string `json:"region"`
	Model    string `json:"model"`
}

type TTSConfig struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"apiKey"`
	GroupID  string `json:"groupId,omitempty"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
	Format   string `json:"format"`
}

type TimingConfig struct {
	PartialStableMs            int `json:"partialStableMs"`
	ArgumentCandidateSilenceMs int `json:"argumentCandidateSilenceMs"`
	NormalEndSilenceMs         int `json:"normalEndSilenceMs"`
	ComfortEndSilenceMs        int `json:"comfortEndSilenceMs"`
	DeepListeningEndSilenceMs  int `json:"deepListeningEndSilenceMs"`
	ComfortFirstPromptMs       int `json:"comfortFirstPromptMs"`
	ComfortSecondPromptMs      int `json:"comfortSecondPromptMs"`
	DeepListeningPromptMs      int `json:"deepListeningPromptMs"`
	MaxProactivePrompts        int `json:"maxProactivePrompts"`
}

type Config struct {
	Enabled      bool              `json:"enabled"`
	Version      int64             `json:"version"`
	RealtimeASR  RealtimeASRConfig `json:"realtimeAsr"`
	TTS          TTSConfig         `json:"tts"`
	EnabledModes []Mode            `json:"enabledModes"`
	Timing       TimingConfig      `json:"timing"`
	CommonPrompt string            `json:"commonPrompt"`
	ModePrompts  map[Mode]string   `json:"modePrompts"`
	ClearASRKey  bool              `json:"clearAsrKey,omitempty"`
	ClearTTSKey  bool              `json:"clearTtsKey,omitempty"`
}

// DefaultConfig returns the editable, disabled configuration shown before the
// first configuration row is saved.
func DefaultConfig() Config {
	cfg := Config{
		RealtimeASR: RealtimeASRConfig{
			Provider: RealtimeASRProvider,
			Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
			Region:   "cn-beijing",
			Model:    RealtimeASRModel,
		},
		TTS: TTSConfig{
			Provider: TTSProviderOpenAICompatible,
			Format:   "mp3",
		},
		EnabledModes: []Mode{ModeNormal},
		ModePrompts:  map[Mode]string{},
	}
	applyTimingDefaults(&cfg.Timing)
	return cfg
}

func (c Config) Validate() error {
	_, err := c.WithDefaults()
	return err
}

// WithDefaults returns the canonical form persisted by the configuration
// store. Zero timing values receive defaults and repeated modes are removed
// while preserving their first-seen order.
func (c Config) WithDefaults() (Config, error) {
	c.RealtimeASR.Provider = strings.TrimSpace(c.RealtimeASR.Provider)
	c.RealtimeASR.Endpoint = strings.TrimSpace(c.RealtimeASR.Endpoint)
	c.RealtimeASR.APIKey = strings.TrimSpace(c.RealtimeASR.APIKey)
	c.RealtimeASR.Region = strings.TrimSpace(c.RealtimeASR.Region)
	c.RealtimeASR.Model = strings.TrimSpace(c.RealtimeASR.Model)
	c.TTS.Provider = strings.TrimSpace(c.TTS.Provider)
	c.TTS.Endpoint = strings.TrimSpace(c.TTS.Endpoint)
	c.TTS.APIKey = strings.TrimSpace(c.TTS.APIKey)
	c.TTS.GroupID = strings.TrimSpace(c.TTS.GroupID)
	c.TTS.Model = strings.TrimSpace(c.TTS.Model)
	c.TTS.Voice = strings.TrimSpace(c.TTS.Voice)
	c.TTS.Format = strings.TrimSpace(c.TTS.Format)
	c.CommonPrompt = strings.TrimSpace(c.CommonPrompt)

	if c.ClearASRKey {
		c.RealtimeASR.APIKey = ""
	}
	if c.ClearTTSKey {
		c.TTS.APIKey = ""
	}
	c.ClearASRKey = false
	c.ClearTTSKey = false

	if c.TTS.Provider == TTSProviderOpenAICompatible || c.TTS.Provider == TTSProviderBailian {
		c.TTS.GroupID = ""
	}

	seen := make(map[Mode]struct{}, len(c.EnabledModes))
	modes := make([]Mode, 0, len(c.EnabledModes))
	for _, raw := range c.EnabledModes {
		mode := Mode(strings.TrimSpace(string(raw)))
		if !knownMode(mode) {
			return Config{}, fmt.Errorf("未知芯之力模式 %q", raw)
		}
		if _, exists := seen[mode]; exists {
			continue
		}
		seen[mode] = struct{}{}
		modes = append(modes, mode)
	}
	c.EnabledModes = modes

	if c.ModePrompts == nil {
		c.ModePrompts = map[Mode]string{}
	} else {
		prompts := make(map[Mode]string, len(c.ModePrompts))
		for rawMode, prompt := range c.ModePrompts {
			mode := Mode(strings.TrimSpace(string(rawMode)))
			if !knownMode(mode) {
				return Config{}, fmt.Errorf("未知芯之力提示词模式 %q", rawMode)
			}
			if _, exists := prompts[mode]; exists {
				return Config{}, fmt.Errorf("芯之力模式提示词 %q 重复", mode)
			}
			prompts[mode] = strings.TrimSpace(prompt)
		}
		c.ModePrompts = prompts
	}

	applyTimingDefaults(&c.Timing)
	if err := validateNormalized(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func validateNormalized(c Config) error {
	if err := validateLengths(c); err != nil {
		return err
	}
	if err := validateTiming(c.Timing); err != nil {
		return err
	}
	if utf8.RuneCountInString(c.CommonPrompt) > maxPromptRunes {
		return fmt.Errorf("公共提示词不能超过 %d 个字符", maxPromptRunes)
	}
	for mode, prompt := range c.ModePrompts {
		if utf8.RuneCountInString(prompt) > maxPromptRunes {
			return fmt.Errorf("模式 %s 的提示词不能超过 %d 个字符", mode, maxPromptRunes)
		}
	}
	if !c.Enabled {
		return nil
	}
	if !containsMode(c.EnabledModes, ModeNormal) {
		return ErrNormalModeRequired
	}
	if c.RealtimeASR.Provider != RealtimeASRProvider {
		return fmt.Errorf("实时 ASR provider 必须为 %s", RealtimeASRProvider)
	}
	if c.RealtimeASR.Model != RealtimeASRModel {
		return fmt.Errorf("实时 ASR model 必须为 %s", RealtimeASRModel)
	}
	if c.RealtimeASR.APIKey == "" || c.RealtimeASR.Region == "" {
		return errors.New("实时 ASR API Key 和区域不能为空")
	}
	if err := validateEndpoint(c.RealtimeASR.Endpoint, "wss", "https"); err != nil {
		return fmt.Errorf("实时 ASR endpoint: %w", err)
	}
	if c.TTS.Provider != TTSProviderOpenAICompatible && c.TTS.Provider != TTSProviderMiniMax && c.TTS.Provider != TTSProviderBailian {
		return errors.New("TTS provider 仅支持 openai-compatible、minimax 或 bailian")
	}
	if c.TTS.APIKey == "" || c.TTS.Model == "" || c.TTS.Voice == "" {
		return errors.New("TTS API Key、模型和音色不能为空")
	}
	if c.TTS.Provider == TTSProviderMiniMax && c.TTS.GroupID == "" {
		return errors.New("MiniMax TTS 必须配置 GroupID")
	}
	if c.TTS.Format != "mp3" {
		return errors.New("TTS format 必须为 mp3")
	}
	if err := validateEndpoint(c.TTS.Endpoint, "https"); err != nil {
		return fmt.Errorf("TTS endpoint: %w", err)
	}
	return nil
}

func validateLengths(c Config) error {
	values := []struct {
		name  string
		value string
		max   int
	}{
		{"实时 ASR endpoint", c.RealtimeASR.Endpoint, maxEndpointRunes},
		{"TTS endpoint", c.TTS.Endpoint, maxEndpointRunes},
		{"实时 ASR API Key", c.RealtimeASR.APIKey, maxAPIKeyRunes},
		{"TTS API Key", c.TTS.APIKey, maxAPIKeyRunes},
		{"实时 ASR provider", c.RealtimeASR.Provider, maxShortRunes},
		{"实时 ASR region", c.RealtimeASR.Region, maxShortRunes},
		{"实时 ASR model", c.RealtimeASR.Model, maxShortRunes},
		{"TTS provider", c.TTS.Provider, maxShortRunes},
		{"TTS GroupID", c.TTS.GroupID, maxShortRunes},
		{"TTS model", c.TTS.Model, maxShortRunes},
		{"TTS voice", c.TTS.Voice, maxShortRunes},
		{"TTS format", c.TTS.Format, maxShortRunes},
	}
	for _, value := range values {
		if utf8.RuneCountInString(value.value) > value.max {
			return fmt.Errorf("%s 不能超过 %d 个字符", value.name, value.max)
		}
	}
	return nil
}

func validateEndpoint(raw string, allowedSchemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("必须是完整 URL")
	}
	for _, scheme := range allowedSchemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return nil
		}
	}
	return fmt.Errorf("协议必须为 %s", strings.Join(allowedSchemes, " 或 "))
}

func applyTimingDefaults(v *TimingConfig) {
	allZero := *v == (TimingConfig{})
	if v.PartialStableMs == 0 {
		v.PartialStableMs = 150
	}
	if v.ArgumentCandidateSilenceMs == 0 {
		v.ArgumentCandidateSilenceMs = 350
	}
	if v.NormalEndSilenceMs == 0 {
		v.NormalEndSilenceMs = 700
	}
	if v.ComfortEndSilenceMs == 0 {
		v.ComfortEndSilenceMs = 1200
	}
	if v.DeepListeningEndSilenceMs == 0 {
		v.DeepListeningEndSilenceMs = 1500
	}
	if v.ComfortFirstPromptMs == 0 {
		v.ComfortFirstPromptMs = 5000
	}
	if v.ComfortSecondPromptMs == 0 {
		v.ComfortSecondPromptMs = 12000
	}
	if v.DeepListeningPromptMs == 0 {
		v.DeepListeningPromptMs = 12000
	}
	if allZero {
		v.MaxProactivePrompts = 2
	}
}

func validateTiming(v TimingConfig) error {
	ranges := []struct {
		name     string
		value    int
		min, max int
	}{
		{"partialStableMs", v.PartialStableMs, 100, 1000},
		{"argumentCandidateSilenceMs", v.ArgumentCandidateSilenceMs, 250, 600},
		{"normalEndSilenceMs", v.NormalEndSilenceMs, 350, 2000},
		{"comfortEndSilenceMs", v.ComfortEndSilenceMs, 700, 3000},
		{"deepListeningEndSilenceMs", v.DeepListeningEndSilenceMs, 1000, 5000},
		{"comfortFirstPromptMs", v.ComfortFirstPromptMs, 3000, 30000},
		{"comfortSecondPromptMs", v.ComfortSecondPromptMs, 3001, 60000},
		{"deepListeningPromptMs", v.DeepListeningPromptMs, 5000, 60000},
		{"maxProactivePrompts", v.MaxProactivePrompts, 0, 5},
	}
	for _, item := range ranges {
		if item.value < item.min || item.value > item.max {
			return fmt.Errorf("%s 必须在 %d 到 %d 之间", item.name, item.min, item.max)
		}
	}
	if v.ArgumentCandidateSilenceMs >= v.NormalEndSilenceMs {
		return errors.New("argumentCandidateSilenceMs 必须小于 normalEndSilenceMs")
	}
	if v.ComfortFirstPromptMs >= v.ComfortSecondPromptMs {
		return errors.New("comfortFirstPromptMs 必须小于 comfortSecondPromptMs")
	}
	return nil
}

func knownMode(mode Mode) bool {
	switch mode {
	case ModeNormal, ModeArgument, ModeComfort, ModeDeepListening:
		return true
	default:
		return false
	}
}

func containsMode(modes []Mode, wanted Mode) bool {
	for _, mode := range modes {
		if mode == wanted {
			return true
		}
	}
	return false
}
