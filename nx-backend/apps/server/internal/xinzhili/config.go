// Package xinzhili contains the configuration and runtime building blocks for
// the realtime voice experience. Its configuration is deliberately isolated
// from the legacy HTTP ASR and model_config settings used by ordinary chat.
package xinzhili

import (
	"errors"
	"fmt"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	RealtimeASRProvider = "aliyun-bailian"
	RealtimeASRModel    = "paraformer-realtime-v2"

	TTSProviderOpenAICompatible = "openai-compatible"
	TTSProviderMiniMax          = "minimax"
	TTSProviderBailian          = "bailian"
	TTSProviderAliyunCosyVoice  = "aliyun-cosyvoice"

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
		EnabledModes: []Mode{ModeNormal, ModeArgument, ModeComfort, ModeDeepListening},
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
	if err := validateProvidedStructure(c); err != nil {
		return err
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
	if c.RealtimeASR.Region == "" {
		return errors.New("实时 ASR 区域不能为空")
	}
	if c.RealtimeASR.Endpoint == "" {
		return errors.New("实时 ASR endpoint 不能为空")
	}
	if !IsOfficialDashScopeRealtimeASREndpoint(c.RealtimeASR.Endpoint) {
		return errors.New("实时 ASR endpoint 必须为官方 DashScope Paraformer 地址")
	}
	if c.TTS.Provider != TTSProviderOpenAICompatible && c.TTS.Provider != TTSProviderMiniMax && c.TTS.Provider != TTSProviderBailian && c.TTS.Provider != TTSProviderAliyunCosyVoice {
		return errors.New("TTS provider 仅支持 openai-compatible、minimax、bailian 或 aliyun-cosyvoice")
	}
	if c.TTS.Model == "" || c.TTS.Voice == "" {
		return errors.New("TTS 模型和音色不能为空")
	}
	if !TTSUsesBailianCredentials(c.TTS) && c.TTS.APIKey == "" {
		return errors.New("私有 TTS API Key 不能为空")
	}
	if c.TTS.Provider == TTSProviderMiniMax && c.TTS.GroupID == "" {
		return errors.New("MiniMax TTS 必须配置 GroupID")
	}
	if c.TTS.Provider == TTSProviderAliyunCosyVoice && c.TTS.GroupID == "" {
		return errors.New("阿里 CosyVoice TTS 必须配置业务空间")
	}
	if c.TTS.Format != "mp3" {
		return errors.New("TTS format 必须为 mp3")
	}
	if c.TTS.Endpoint == "" {
		return errors.New("TTS endpoint 不能为空")
	}
	return nil
}

func validateProvidedStructure(c Config) error {
	if c.RealtimeASR.Provider != "" && c.RealtimeASR.Provider != RealtimeASRProvider {
		return fmt.Errorf("实时 ASR provider 必须为 %s", RealtimeASRProvider)
	}
	if c.RealtimeASR.Model != "" && c.RealtimeASR.Model != RealtimeASRModel {
		return fmt.Errorf("实时 ASR model 必须为 %s", RealtimeASRModel)
	}
	if c.RealtimeASR.Endpoint != "" {
		if err := validateEndpoint(c.RealtimeASR.Endpoint, "wss", "https"); err != nil {
			return fmt.Errorf("实时 ASR endpoint: %w", err)
		}
	}
	if c.TTS.Provider != "" && c.TTS.Provider != TTSProviderOpenAICompatible && c.TTS.Provider != TTSProviderMiniMax && c.TTS.Provider != TTSProviderBailian && c.TTS.Provider != TTSProviderAliyunCosyVoice {
		return errors.New("TTS provider 仅支持 openai-compatible、minimax、bailian 或 aliyun-cosyvoice")
	}
	if c.TTS.Endpoint != "" {
		schemes := []string{"https"}
		if c.TTS.Provider == TTSProviderAliyunCosyVoice {
			schemes = []string{"wss", "https", "ws"}
		}
		if err := validateEndpoint(c.TTS.Endpoint, schemes...); err != nil {
			return fmt.Errorf("TTS endpoint: %w", err)
		}
	}
	if c.TTS.Format != "" && c.TTS.Format != "mp3" {
		return errors.New("TTS format 必须为 mp3")
	}
	return nil
}

// TTSUsesBailianCredentials reports whether this TTS configuration is served
// by Bailian and therefore receives the shared credential only at runtime.
func TTSUsesBailianCredentials(cfg TTSConfig) bool {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	return (provider == TTSProviderBailian || provider == TTSProviderOpenAICompatible) &&
		IsOfficialDashScopeTTSEndpoint(cfg.Endpoint)
}

// IsOfficialDashScopeTTSEndpoint accepts only the official DashScope HTTPS
// host and supported TTS API roots. It deliberately rejects lookalike hosts,
// userinfo, non-default ports, query strings and path traversal.
func IsOfficialDashScopeTTSEndpoint(raw string) bool {
	parsed, ok := parseOfficialDashScopeEndpoint(raw, "https")
	if !ok || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return false
	}
	endpointPath := strings.TrimSuffix(parsed.Path, "/")
	if pathpkg.Clean(endpointPath) != endpointPath {
		return false
	}
	switch endpointPath {
	case "/api/v1", "/compatible-mode/v1", "/api/v1/services/aigc/multimodal-generation/generation":
		return true
	default:
		return false
	}
}

// IsOfficialDashScopeRealtimeASREndpoint accepts only the official
// Paraformer realtime inference endpoint. Shared Bailian credentials must
// never be sent to custom proxies or lookalike hosts.
func IsOfficialDashScopeRealtimeASREndpoint(raw string) bool {
	parsed, ok := parseOfficialDashScopeEndpoint(raw, "wss", "https")
	if !ok || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return false
	}
	if pathpkg.Clean(parsed.Path) != parsed.Path {
		return false
	}
	return parsed.Path == "/api-ws/v1/inference"
}

func parseOfficialDashScopeEndpoint(raw string, schemes ...string) (*url.URL, bool) {
	parsed, err := parseEndpoint(raw)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "dashscope.aliyuncs.com") {
		return nil, false
	}
	schemeAllowed := false
	for _, scheme := range schemes {
		if strings.EqualFold(parsed.Scheme, scheme) {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return nil, false
	}
	port, err := endpointPort(parsed)
	if err != nil || (port != 0 && port != 443) {
		return nil, false
	}
	return parsed, true
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
	u, err := parseEndpoint(raw)
	if err != nil {
		return errors.New("必须是完整 URL")
	}
	for _, scheme := range allowedSchemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return nil
		}
	}
	return fmt.Errorf("协议必须为 %s", strings.Join(allowedSchemes, " 或 "))
}

func parseEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("invalid endpoint URL")
	}
	if _, err := endpointPort(u); err != nil {
		return nil, err
	}
	return u, nil
}

// endpointPort returns zero when the URL omits an explicit port. Numeric forms
// such as 0443 normalize to 443, while malformed and out-of-range ports fail.
func endpointPort(u *url.URL) (int, error) {
	raw := u.Port()
	if raw == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("invalid endpoint port")
	}
	return port, nil
}

func applyTimingDefaults(v *TimingConfig) {
	allZero := *v == (TimingConfig{})
	if v.PartialStableMs == 0 {
		v.PartialStableMs = 120
	}
	if v.ArgumentCandidateSilenceMs == 0 {
		v.ArgumentCandidateSilenceMs = 250
	}
	if v.NormalEndSilenceMs == 0 {
		v.NormalEndSilenceMs = 350
	}
	if v.ComfortEndSilenceMs == 0 {
		v.ComfortEndSilenceMs = 700
	}
	if v.DeepListeningEndSilenceMs == 0 {
		v.DeepListeningEndSilenceMs = 1000
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
