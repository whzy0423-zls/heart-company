package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

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
	if err := validateDurationCandidateEncodings(contract.Duration); err != nil {
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

func validateDurationCandidateEncodings(field FieldEncoding) error {
	if field.Name == "" {
		return nil
	}
	candidates := make([]string, 0, 13)
	for value := 4; value <= 15; value++ {
		candidates = append(candidates, strconv.Itoa(value))
	}
	if _, hasSmart := field.ValueMap["smart"]; hasSmart {
		candidates = append(candidates, "smart")
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		key, err := gatewayFieldEncodingKey(GatewayFieldDuration, field, candidate)
		if err != nil {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			return gatewayContractError("duplicate_field_encoding", "duration.valueMap")
		}
		seen[key] = struct{}{}
	}
	return nil
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
