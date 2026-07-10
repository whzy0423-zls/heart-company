package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/netguard"
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
	// UploadMaxBytes 单文件上传大小上限，单位 bytes；<=0 时默认 20 MiB。
	UploadDir       string
	UploadMaxBytes  int64
	UploadPublicURL string
	// PublicBaseURL 后端 server 对外可达的根地址（形如 https://api.example.com）。
	// 用于把本地存储产生的相对地址（/api/uploads、/api/upload-assets）补全为
	// 外部视频网关可拉取的绝对地址；外部网关链路不会从请求 Host/X-Forwarded-Host 推断。
	PublicBaseURL string
	// TrustedProxyCIDRs 显式可信反向代理网段；只有命中这些网段时才信任 X-Forwarded-For/X-Real-IP。
	TrustedProxyCIDRs []string
	MiniMax           MiniMaxConfig
	MiniappChat       MiniappChatConfig
	WeChat            WeChatConfig
	WxPay             WxPayConfig
	Embedding         EmbeddingConfig
	SMS               SMSConfig
	Video             VideoConfig
	Image             ImageConfig
	ASR               ASRConfig
	JPush             JPushConfig
}

// SMSConfig 短信发送配置。Provider 为空时非生产环境为 dev 模式；生产环境会 fail closed。
type SMSConfig struct {
	Provider           string // aliyun | spug | "" (dev)
	APIKey             string
	APISecret          string
	SignName           string
	TemplateID         string
	SpugAPIBase        string
	SpugTemplateCode   string
	SpugTemplateName   string
	SpugTimeoutSeconds int
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
	Model           string
	ModelProfile    string
	GatewayContract GatewayContractConfig
	TimeoutSeconds  int
}

type FieldEncoding struct {
	Name      string            `json:"name"`
	ValueType string            `json:"valueType"`
	ValueMap  map[string]string `json:"valueMap,omitempty"`
}

type GatewayFieldKind string

const (
	GatewayFieldDuration      GatewayFieldKind = "duration"
	GatewayFieldAspectRatio   GatewayFieldKind = "aspectRatio"
	GatewayFieldResolution    GatewayFieldKind = "resolution"
	GatewayFieldGenerateAudio GatewayFieldKind = "generateAudio"
	GatewayFieldTaskMode      GatewayFieldKind = "taskMode"
)

type ReferenceEncoding struct {
	Mode                string            `json:"mode"`
	ImageField          string            `json:"imageField"`
	VideoField          string            `json:"videoField"`
	AudioField          string            `json:"audioField"`
	RoleFields          map[string]string `json:"roleFields,omitempty"`
	SupportsRoles       []string          `json:"supportsRoles"`
	RequiresTargetFirst bool              `json:"requiresTargetFirst"`
}

type MediaLimits struct {
	MaxImages            int     `json:"maxImages"`
	MaxVideos            int     `json:"maxVideos"`
	MaxAudios            int     `json:"maxAudios"`
	MaxVideoSecondsTotal float64 `json:"maxVideoSecondsTotal"`
	MaxAudioSecondsTotal float64 `json:"maxAudioSecondsTotal"`
}

type IdempotencyContract struct {
	Header string `json:"header"`
}

type ReconciliationContract struct {
	LookupByRequestKey bool     `json:"lookupByRequestKey"`
	Method             string   `json:"method"`
	PathTemplate       string   `json:"pathTemplate"`
	TaskIDPaths        []string `json:"taskIdPaths"`
	StatusPaths        []string `json:"statusPaths"`
}

type GatewayContractConfig struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	DeclaredModes  []string               `json:"declaredModes"`
	Duration       FieldEncoding          `json:"duration"`
	AspectRatio    FieldEncoding          `json:"aspectRatio"`
	Resolution     FieldEncoding          `json:"resolution"`
	GenerateAudio  FieldEncoding          `json:"generateAudio"`
	TaskMode       FieldEncoding          `json:"taskMode"`
	References     ReferenceEncoding      `json:"references"`
	Limits         MediaLimits            `json:"limits"`
	Idempotency    IdempotencyContract    `json:"idempotency"`
	Reconciliation ReconciliationContract `json:"reconciliation"`
}

type GatewayContractValidationError struct {
	Code  string
	Field string
}

func (e *GatewayContractValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Field)
}

var gatewayFieldNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

var gatewayReferenceRoles = map[string]struct{}{
	"reference_image": {},
	"first_frame":     {},
	"last_frame":      {},
	"reference_video": {},
	"reference_audio": {},
	"edit_target":     {},
	"extend_target":   {},
}

var reservedIdempotencyHeaders = map[string]struct{}{
	"authorization":          {},
	"proxy-authorization":    {},
	"proxy-authenticate":     {},
	"cookie":                 {},
	"set-cookie":             {},
	"host":                   {},
	"content-type":           {},
	"content-length":         {},
	"content-encoding":       {},
	"transfer-encoding":      {},
	"expect":                 {},
	"connection":             {},
	"keep-alive":             {},
	"proxy-connection":       {},
	"te":                     {},
	"trailer":                {},
	"upgrade":                {},
	"x-http-method-override": {},
}

func ValidateGatewayContract(contract GatewayContractConfig) error {
	if reflect.DeepEqual(contract, GatewayContractConfig{}) {
		return nil
	}
	var err error
	contract, err = normalizeGatewayContract(contract)
	if err != nil {
		return err
	}
	fields := []struct {
		name  string
		value FieldEncoding
	}{
		{"duration", contract.Duration},
		{"aspectRatio", contract.AspectRatio},
		{"resolution", contract.Resolution},
		{"generateAudio", contract.GenerateAudio},
		{"taskMode", contract.TaskMode},
	}
	for _, field := range fields {
		if err := validateGatewayFieldEncoding(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validateGatewayDeclaredModes(contract.DeclaredModes); err != nil {
		return err
	}
	if err := validateGenerateAudioEncoding(contract.GenerateAudio); err != nil {
		return err
	}
	if err := validateTaskModeEncoding(contract.TaskMode, contract.DeclaredModes); err != nil {
		return err
	}
	if contract.References.Mode != "flat_arrays" && contract.References.Mode != "content_items" {
		return gatewayContractError("invalid_reference_mode", "references.mode")
	}

	referenceFields := []struct {
		name  string
		value string
	}{
		{"references.imageField", contract.References.ImageField},
		{"references.videoField", contract.References.VideoField},
		{"references.audioField", contract.References.AudioField},
	}
	for _, field := range referenceFields {
		if field.value != "" && !gatewayFieldNamePattern.MatchString(field.value) {
			return gatewayContractError("invalid_field_name", field.name)
		}
	}
	roleFieldKeys := make([]string, 0, len(contract.References.RoleFields))
	for role := range contract.References.RoleFields {
		roleFieldKeys = append(roleFieldKeys, role)
	}
	sort.Strings(roleFieldKeys)
	for _, role := range roleFieldKeys {
		field := contract.References.RoleFields[role]
		if _, ok := gatewayReferenceRoles[role]; !ok {
			return gatewayContractError("invalid_reference_role", "references.roleFields")
		}
		if !gatewayFieldNamePattern.MatchString(field) {
			return gatewayContractError("invalid_field_name", "references.roleFields")
		}
	}
	seenRoles := make(map[string]struct{}, len(contract.References.SupportsRoles))
	for index, role := range contract.References.SupportsRoles {
		if _, ok := gatewayReferenceRoles[role]; !ok {
			return gatewayContractError("invalid_reference_role", "references.supportsRoles")
		}
		if _, exists := seenRoles[role]; exists {
			return gatewayContractError("duplicate_reference_role", fmt.Sprintf("references.supportsRoles[%d]", index))
		}
		seenRoles[role] = struct{}{}
	}
	if err := validateGatewayReferenceSchema(contract.References); err != nil {
		return err
	}
	if err := validateGatewayTopLevelNamespace(contract); err != nil {
		return err
	}

	header := contract.Idempotency.Header
	if header != "" {
		if !isRFCHeaderToken(header) {
			return gatewayContractError("invalid_header_name", "idempotency.header")
		}
		if _, reserved := reservedIdempotencyHeaders[strings.ToLower(header)]; reserved {
			return gatewayContractError("reserved_header", "idempotency.header")
		}
	}

	method := contract.Reconciliation.Method
	if method != "" && !strings.EqualFold(method, "GET") {
		return gatewayContractError("invalid_reconciliation_method", "reconciliation.method")
	}
	if contract.Reconciliation.LookupByRequestKey && !strings.EqualFold(method, "GET") {
		return gatewayContractError("invalid_reconciliation_method", "reconciliation.method")
	}
	pathTemplate := contract.Reconciliation.PathTemplate
	if pathTemplate != "" && !isSafeRelativeReconciliationPath(pathTemplate) {
		return gatewayContractError("invalid_reconciliation_path", "reconciliation.pathTemplate")
	}
	if contract.Reconciliation.LookupByRequestKey && pathTemplate == "" {
		return gatewayContractError("invalid_reconciliation_path", "reconciliation.pathTemplate")
	}
	return nil
}

func validateGatewayDeclaredModes(modes []string) error {
	seen := make(map[string]struct{}, len(modes))
	for index, mode := range modes {
		switch mode {
		case "reference", "edit", "extend":
		default:
			return gatewayContractError("invalid_declared_mode", fmt.Sprintf("declaredModes[%d]", index))
		}
		if _, exists := seen[mode]; exists {
			return gatewayContractError("duplicate_declared_mode", fmt.Sprintf("declaredModes[%d]", index))
		}
		seen[mode] = struct{}{}
	}
	return nil
}

func validateGatewayReferenceSchema(references ReferenceEncoding) error {
	switch references.Mode {
	case "content_items":
		return validateContentItemsReferenceSchema(references)
	case "flat_arrays":
		return validateFlatArraysReferenceSchema(references)
	default:
		return gatewayContractError("invalid_reference_mode", "references.mode")
	}
}

func validateContentItemsReferenceSchema(references ReferenceEncoding) error {
	if len(references.RoleFields) != len(references.SupportsRoles) {
		return gatewayContractError("reference_role_fields_mismatch", "references.roleFields")
	}
	mediaFields := []string{references.ImageField, references.VideoField, references.AudioField}
	seenRoleFields := make(map[string]struct{}, len(references.RoleFields))
	for _, role := range references.SupportsRoles {
		roleField, exists := references.RoleFields[role]
		if !exists {
			return gatewayContractError("reference_role_fields_mismatch", "references.roleFields")
		}
		roleFieldPath := "references.roleFields." + role
		if _, duplicate := seenRoleFields[roleField]; duplicate {
			return gatewayContractError("duplicate_reference_role_field", roleFieldPath)
		}
		seenRoleFields[roleField] = struct{}{}

		mediaField, mediaFieldPath := gatewayReferenceMediaFieldForRole(references, role)
		if mediaField == "" {
			return gatewayContractError("missing_reference_media_field", mediaFieldPath)
		}
		for _, candidate := range mediaFields {
			if candidate != "" && roleField == candidate {
				return gatewayContractError("reference_field_conflict", roleFieldPath)
			}
		}
	}
	return nil
}

func validateFlatArraysReferenceSchema(references ReferenceEncoding) error {
	if len(references.RoleFields) != 0 {
		return gatewayContractError("flat_role_fields_not_allowed", "references.roleFields")
	}
	for index, role := range references.SupportsRoles {
		switch role {
		case "reference_image", "reference_video", "reference_audio":
		default:
			return gatewayContractError("flat_reference_role_not_supported", fmt.Sprintf("references.supportsRoles[%d]", index))
		}
		mediaField, mediaFieldPath := gatewayReferenceMediaFieldForRole(references, role)
		if mediaField == "" {
			return gatewayContractError("missing_reference_media_field", mediaFieldPath)
		}
	}

	seenMediaFields := make(map[string]struct{}, 3)
	for _, field := range []struct {
		path  string
		value string
	}{
		{"references.imageField", references.ImageField},
		{"references.videoField", references.VideoField},
		{"references.audioField", references.AudioField},
	} {
		if field.value == "" {
			continue
		}
		if _, duplicate := seenMediaFields[field.value]; duplicate {
			return gatewayContractError("duplicate_reference_media_field", field.path)
		}
		seenMediaFields[field.value] = struct{}{}
	}
	return nil
}

func gatewayReferenceMediaFieldForRole(references ReferenceEncoding, role string) (string, string) {
	switch role {
	case "reference_image", "first_frame", "last_frame":
		return references.ImageField, "references.imageField"
	case "reference_video", "edit_target", "extend_target":
		return references.VideoField, "references.videoField"
	case "reference_audio":
		return references.AudioField, "references.audioField"
	default:
		return "", "references.supportsRoles"
	}
}

func validateGatewayTopLevelNamespace(contract GatewayContractConfig) error {
	used := map[string]struct{}{
		"model":         {},
		"prompt":        {},
		"content_items": {},
	}
	add := func(path, field string) error {
		if field == "" {
			return nil
		}
		if _, exists := used[field]; exists {
			return gatewayContractError("duplicate_gateway_field", path)
		}
		used[field] = struct{}{}
		return nil
	}
	for _, field := range []struct {
		path  string
		value string
	}{
		{"duration.name", contract.Duration.Name},
		{"aspectRatio.name", contract.AspectRatio.Name},
		{"resolution.name", contract.Resolution.Name},
		{"generateAudio.name", contract.GenerateAudio.Name},
		{"taskMode.name", contract.TaskMode.Name},
	} {
		if err := add(field.path, field.value); err != nil {
			return err
		}
	}
	if contract.References.Mode == "flat_arrays" {
		for _, field := range []struct {
			path  string
			value string
		}{
			{"references.imageField", contract.References.ImageField},
			{"references.videoField", contract.References.VideoField},
			{"references.audioField", contract.References.AudioField},
		} {
			if err := add(field.path, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func gatewayContractError(code, field string) error {
	return &GatewayContractValidationError{Code: code, Field: field}
}

// EncodeGatewayFieldValue applies the field-specific scalar policy shared by
// gateway payload mapping, capability discovery, and degradation reporting.
func EncodeGatewayFieldValue(kind GatewayFieldKind, field FieldEncoding, source string) (any, error) {
	if field.Name == "" {
		return nil, gatewayContractError("field_name_missing", "name")
	}
	encoded := source
	mapped, exists := field.ValueMap[source]
	switch kind {
	case GatewayFieldDuration:
		if source == "smart" && !exists {
			return nil, gatewayContractError("field_mapping_missing", "valueMap")
		}
	case GatewayFieldResolution:
		if len(field.ValueMap) == 0 || !exists {
			return nil, gatewayContractError("field_mapping_missing", "valueMap")
		}
	case GatewayFieldAspectRatio, GatewayFieldTaskMode:
		if len(field.ValueMap) > 0 && !exists {
			return nil, gatewayContractError("field_mapping_missing", "valueMap")
		}
	case GatewayFieldGenerateAudio:
		if (field.ValueType != "bool" || len(field.ValueMap) > 0) && !exists {
			return nil, gatewayContractError("field_mapping_missing", "valueMap")
		}
	default:
		return nil, gatewayContractError("invalid_field_kind", "kind")
	}
	if exists {
		encoded = mapped
	}
	value, err := encodeGatewayRawValue(encoded, field.ValueType)
	if err != nil {
		return nil, gatewayContractError("field_value_not_encodable", "value")
	}
	return value, nil
}

func CanEncodeGatewayFieldValue(kind GatewayFieldKind, field FieldEncoding, source string) bool {
	_, err := EncodeGatewayFieldValue(kind, field, source)
	return err == nil
}

func GatewayFieldEncodingsAreDistinct(kind GatewayFieldKind, field FieldEncoding, sources []string) bool {
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		key, err := gatewayFieldEncodingKey(kind, field, source)
		if err != nil {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func GatewayFieldEncodingKey(kind GatewayFieldKind, field FieldEncoding, source string) (string, error) {
	return gatewayFieldEncodingKey(kind, field, source)
}

func gatewayFieldEncodingKey(kind GatewayFieldKind, field FieldEncoding, source string) (string, error) {
	value, err := EncodeGatewayFieldValue(kind, field, source)
	if err != nil {
		return "", err
	}
	return canonicalGatewayFieldValueKey(value)
}

func canonicalGatewayFieldValueKey(value any) (string, error) {
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", gatewayContractError("field_value_not_encodable", "value")
	}
	return fmt.Sprintf("%T:%s", value, canonical), nil
}

func encodeGatewayRawValue(encoded, valueType string) (any, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, gatewayContractError("field_value_not_encodable", "value")
	}
	switch valueType {
	case "string":
		return encoded, nil
	case "int":
		value, err := strconv.Atoi(encoded)
		if err != nil {
			return nil, gatewayContractError("field_value_not_encodable", "value")
		}
		return value, nil
	case "bool":
		value, err := strconv.ParseBool(encoded)
		if err != nil {
			return nil, gatewayContractError("field_value_not_encodable", "value")
		}
		return value, nil
	default:
		return nil, gatewayContractError("invalid_value_type", "valueType")
	}
}

func validateGatewayFieldEncoding(name string, field FieldEncoding) error {
	if field.Name == "" && field.ValueType == "" && len(field.ValueMap) == 0 {
		return nil
	}
	if !gatewayFieldNamePattern.MatchString(field.Name) {
		return gatewayContractError("invalid_field_name", name+".name")
	}
	switch field.ValueType {
	case "string", "int", "bool":
	default:
		return gatewayContractError("invalid_value_type", name+".valueType")
	}
	keys := make([]string, 0, len(field.ValueMap))
	for key := range field.ValueMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seenEncodings := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			return gatewayContractError("empty_value_map_key", name+".valueMap")
		}
		mapped := field.ValueMap[key]
		if mapped == "" {
			return gatewayContractError("empty_value_map_value", name+".valueMap")
		}
		value, err := encodeGatewayRawValue(mapped, field.ValueType)
		if err != nil {
			return gatewayContractError("invalid_mapped_value", name+".valueMap")
		}
		encodingKey, err := canonicalGatewayFieldValueKey(value)
		if err != nil {
			return gatewayContractError("invalid_mapped_value", name+".valueMap")
		}
		if _, duplicate := seenEncodings[encodingKey]; duplicate && name != "taskMode" {
			return gatewayContractError("duplicate_field_encoding", name+".valueMap")
		}
		seenEncodings[encodingKey] = struct{}{}
	}
	return nil
}

func validateGenerateAudioEncoding(field FieldEncoding) error {
	if field.Name == "" {
		return nil
	}
	if field.ValueType != "bool" || len(field.ValueMap) > 0 {
		if _, trueMapped := field.ValueMap["true"]; !trueMapped {
			return gatewayContractError("missing_boolean_mapping", "generateAudio.valueMap")
		}
		if _, falseMapped := field.ValueMap["false"]; !falseMapped {
			return gatewayContractError("missing_boolean_mapping", "generateAudio.valueMap")
		}
	}
	return validateDistinctBooleanValues(field, "generateAudio.valueMap")
}

func validateDistinctBooleanValues(field FieldEncoding, errorField string) error {
	trueValue, trueErr := EncodeGatewayFieldValue(GatewayFieldGenerateAudio, field, "true")
	falseValue, falseErr := EncodeGatewayFieldValue(GatewayFieldGenerateAudio, field, "false")
	if trueErr != nil || falseErr != nil {
		return gatewayContractError("invalid_boolean_mapping", errorField)
	}
	if reflect.DeepEqual(trueValue, falseValue) {
		return gatewayContractError("indistinguishable_boolean_mapping", errorField)
	}
	return nil
}

func validateTaskModeEncoding(field FieldEncoding, modes []string) error {
	if field.Name == "" {
		return nil
	}
	seen := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		key, err := gatewayFieldEncodingKey(GatewayFieldTaskMode, field, mode)
		if err != nil {
			return gatewayContractError("declared_mode_not_encodable", "taskMode")
		}
		if _, duplicate := seen[key]; duplicate {
			return gatewayContractError("duplicate_task_mode_encoding", "taskMode")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func isRFCHeaderToken(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		char := value[i]
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func isSafeRelativeReconciliationPath(raw string) bool {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || containsControlCharacter(raw) {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return false
	}
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		decoded := segment
		for i := 0; i < 3; i++ {
			next, err := url.PathUnescape(decoded)
			if err != nil {
				return false
			}
			if next == decoded {
				break
			}
			decoded = next
		}
		if decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\") || containsControlCharacter(decoded) {
			return false
		}
	}
	return true
}

func containsControlCharacter(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}

var legacyVideoGatewayContractTemplate = GatewayContractConfig{
	Name:          "legacy_flat_v1",
	Version:       "1",
	DeclaredModes: []string{"reference"},
	Duration: FieldEncoding{
		Name:      "seconds",
		ValueType: "string",
	},
	AspectRatio: FieldEncoding{
		Name:      "aspect_ratio",
		ValueType: "string",
	},
	References: ReferenceEncoding{
		Mode:          "flat_arrays",
		ImageField:    "images",
		VideoField:    "videos",
		AudioField:    "audios",
		SupportsRoles: []string{"reference_image", "reference_video", "reference_audio"},
	},
	Limits: MediaLimits{
		MaxImages:            4,
		MaxVideos:            3,
		MaxAudios:            1,
		MaxVideoSecondsTotal: 15,
		MaxAudioSecondsTotal: 15,
	},
}

func LegacyVideoGatewayContract() GatewayContractConfig {
	return cloneGatewayContract(legacyVideoGatewayContractTemplate)
}

func TrimGatewayContract(contract GatewayContractConfig) GatewayContractConfig {
	normalized, err := normalizeGatewayContract(contract)
	if err != nil {
		return cloneGatewayContract(contract)
	}
	return normalized
}

func normalizeGatewayContract(contract GatewayContractConfig) (GatewayContractConfig, error) {
	if reflect.DeepEqual(contract, GatewayContractConfig{}) {
		return GatewayContractConfig{}, nil
	}
	contract.Name = strings.TrimSpace(contract.Name)
	contract.Version = strings.TrimSpace(contract.Version)
	if contract.Name == "" {
		return GatewayContractConfig{}, gatewayContractError("missing_contract_name", "name")
	}
	if contract.Version == "" {
		return GatewayContractConfig{}, gatewayContractError("missing_contract_version", "version")
	}
	contract.DeclaredModes = trimContractStrings(contract.DeclaredModes)
	var err error
	if contract.Duration, err = trimFieldEncoding(contract.Duration, "duration"); err != nil {
		return GatewayContractConfig{}, err
	}
	if contract.AspectRatio, err = trimFieldEncoding(contract.AspectRatio, "aspectRatio"); err != nil {
		return GatewayContractConfig{}, err
	}
	if contract.Resolution, err = trimFieldEncoding(contract.Resolution, "resolution"); err != nil {
		return GatewayContractConfig{}, err
	}
	if contract.GenerateAudio, err = trimFieldEncoding(contract.GenerateAudio, "generateAudio"); err != nil {
		return GatewayContractConfig{}, err
	}
	if contract.TaskMode, err = trimFieldEncoding(contract.TaskMode, "taskMode"); err != nil {
		return GatewayContractConfig{}, err
	}
	contract.References.Mode = strings.TrimSpace(contract.References.Mode)
	contract.References.ImageField = strings.TrimSpace(contract.References.ImageField)
	contract.References.VideoField = strings.TrimSpace(contract.References.VideoField)
	contract.References.AudioField = strings.TrimSpace(contract.References.AudioField)
	if contract.References.RoleFields, err = trimContractStringMap(contract.References.RoleFields, "references.roleFields"); err != nil {
		return GatewayContractConfig{}, err
	}
	contract.References.SupportsRoles = trimContractStrings(contract.References.SupportsRoles)
	contract.Idempotency.Header = strings.TrimSpace(contract.Idempotency.Header)
	contract.Reconciliation.Method = strings.TrimSpace(contract.Reconciliation.Method)
	contract.Reconciliation.PathTemplate = strings.TrimSpace(contract.Reconciliation.PathTemplate)
	contract.Reconciliation.TaskIDPaths = trimContractStrings(contract.Reconciliation.TaskIDPaths)
	contract.Reconciliation.StatusPaths = trimContractStrings(contract.Reconciliation.StatusPaths)
	return contract, nil
}

func trimFieldEncoding(field FieldEncoding, name string) (FieldEncoding, error) {
	field.Name = strings.TrimSpace(field.Name)
	field.ValueType = strings.TrimSpace(field.ValueType)
	var err error
	field.ValueMap, err = trimContractStringMap(field.ValueMap, name+".valueMap")
	return field, err
}

func trimContractStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.TrimSpace(value)
	}
	return out
}

func trimContractStringMap(values map[string]string, field string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(values))
	seen := make(map[string]string, len(values))
	for _, key := range keys {
		normalizedKey := strings.TrimSpace(key)
		if previous, ok := seen[normalizedKey]; ok && previous != key {
			return nil, gatewayContractError("duplicate_normalized_key", field)
		}
		seen[normalizedKey] = key
		out[normalizedKey] = strings.TrimSpace(values[key])
	}
	return out, nil
}

func cloneGatewayContract(contract GatewayContractConfig) GatewayContractConfig {
	clone := contract
	clone.DeclaredModes = append([]string(nil), contract.DeclaredModes...)
	clone.Duration.ValueMap = cloneContractStringMap(contract.Duration.ValueMap)
	clone.AspectRatio.ValueMap = cloneContractStringMap(contract.AspectRatio.ValueMap)
	clone.Resolution.ValueMap = cloneContractStringMap(contract.Resolution.ValueMap)
	clone.GenerateAudio.ValueMap = cloneContractStringMap(contract.GenerateAudio.ValueMap)
	clone.TaskMode.ValueMap = cloneContractStringMap(contract.TaskMode.ValueMap)
	clone.References.RoleFields = cloneContractStringMap(contract.References.RoleFields)
	clone.References.SupportsRoles = append([]string(nil), contract.References.SupportsRoles...)
	clone.Reconciliation.TaskIDPaths = append([]string(nil), contract.Reconciliation.TaskIDPaths...)
	clone.Reconciliation.StatusPaths = append([]string(nil), contract.Reconciliation.StatusPaths...)
	return clone
}

func cloneContractStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func loadVideoGatewayContract() GatewayContractConfig {
	name := strings.TrimSpace(os.Getenv("VIDEO_GATEWAY_CONTRACT"))
	version := strings.TrimSpace(os.Getenv("VIDEO_GATEWAY_CONTRACT_VERSION"))
	raw := strings.TrimSpace(os.Getenv("VIDEO_GATEWAY_CONTRACT_JSON"))
	if name == "" && version == "" && raw == "" {
		return TrimGatewayContract(LegacyVideoGatewayContract())
	}

	contract := GatewayContractConfig{}
	if raw != "" {
		if json.Unmarshal([]byte(raw), &contract) != nil {
			return GatewayContractConfig{}
		}
	} else if name == "legacy_flat_v1" && version == "1" {
		contract = LegacyVideoGatewayContract()
	}
	if name != "" {
		contract.Name = name
	}
	if version != "" {
		contract.Version = version
	}

	normalized, err := normalizeGatewayContract(contract)
	if err != nil {
		return GatewayContractConfig{}
	}
	if ValidateGatewayContract(normalized) == nil {
		return normalized
	}
	fallback := GatewayContractConfig{Name: normalized.Name, Version: normalized.Version}
	if raw == "" && normalized.Name == "legacy_flat_v1" && normalized.Version == "1" {
		fallback = LegacyVideoGatewayContract()
	}
	return TrimGatewayContract(fallback)
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

	appEnv := NormalizeAppEnv(getenv("APP_ENV", ""))

	return Env{
		AdminPassword:      getenv("ADMIN_PASSWORD", "123456"),
		AdminUsername:      getenv("ADMIN_USERNAME", "admin"),
		AppEnv:             appEnv,
		AppVersion:         getenv("APP_VERSION", "0.0.1"),
		CORSAllowedOrigins: parseCSV(getenv("CORS_ALLOWED_ORIGINS", "")),
		JWTSecret:          getenv("JWT_SECRET", "nine-xing-dev-secret"),
		Port:               port,
		SiteConfig:         siteConfig,
		AdminConfig:        adminConfig,
		BuildScript:        buildScript,
		BuildTimeout:       buildTimeout,
		DatabaseURL:        getenv("DATABASE_URL", "postgres://nx:nx@localhost:5432/nx_admin?sslmode=disable"),
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
			Provider:           getenv("SMS_PROVIDER", ""),
			APIKey:             getenv("SMS_API_KEY", ""),
			APISecret:          getenv("SMS_API_SECRET", ""),
			SignName:           getenv("SMS_SIGN_NAME", ""),
			TemplateID:         getenv("SMS_TEMPLATE_ID", ""),
			SpugAPIBase:        getenv("SPUG_PUSH_API_BASE", "https://push.spug.cc"),
			SpugTemplateCode:   getenv("SPUG_PUSH_TEMPLATE_CODE", ""),
			SpugTemplateName:   getenv("SPUG_PUSH_TEMPLATE_NAME", "芯之力"),
			SpugTimeoutSeconds: spugTimeout,
		},
		Video: VideoConfig{
			APIBase:         getenv("VIDEO_API_BASE", ""),
			APIKey:          getenv("VIDEO_API_KEY", ""),
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
