package video

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/config"
)

// MapGatewayPayload encodes a validated request for the declared intermediary
// contract. Reference ordering is supplied by the caller so prompt compilation
// and gateway encoding can share one canonical value.
func MapGatewayPayload(request GenerateRequest, references CanonicalReferences, contract config.GatewayContractConfig) (map[string]any, error) {
	if reflect.DeepEqual(contract, config.GatewayContractConfig{}) {
		return nil, validationError(
			"gateway_contract_invalid",
			"gatewayContract",
			"视频中转站契约无效，已阻止创建请求。",
			"修正中转站契约配置并重新加载后再试。",
			nil,
		)
	}
	validatedContract, err := validatedGatewayContract(contract)
	if err != nil {
		return nil, err
	}
	contract = validatedContract
	canonical, err := validateCanonicalReferences(request.References, references)
	if err != nil {
		return nil, err
	}
	references = canonical
	request.References = canonicalReferenceSnapshots(references)
	if err := validateGatewayReferencePredicates(request); err != nil {
		return nil, err
	}
	if err := validateGatewayTaskMode(request.TaskMode, references, contract); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if err := addEncodedGatewayField(payload, "duration", config.GatewayFieldDuration, contract.Duration, durationGatewayValue(request.Duration)); err != nil {
		return nil, err
	}
	if err := addEncodedGatewayField(payload, "aspectRatio", config.GatewayFieldAspectRatio, contract.AspectRatio, request.AspectRatio); err != nil {
		return nil, err
	}
	if request.Resolution != "" {
		if err := addEncodedGatewayField(payload, "resolution", config.GatewayFieldResolution, contract.Resolution, request.Resolution); err != nil {
			return nil, err
		}
	}
	if request.GenerateAudio != nil {
		if err := addEncodedGatewayField(payload, "generateAudio", config.GatewayFieldGenerateAudio, contract.GenerateAudio, strconv.FormatBool(*request.GenerateAudio)); err != nil {
			return nil, err
		}
	}
	if request.TaskMode != "" {
		if err := addEncodedGatewayField(payload, "taskMode", config.GatewayFieldTaskMode, contract.TaskMode, request.TaskMode); err != nil {
			return nil, err
		}
	}

	if err := addGatewayReferences(payload, references, contract.References); err != nil {
		return nil, err
	}
	return payload, nil
}

func validateCanonicalReferences(requestReferences []Reference, references CanonicalReferences) (CanonicalReferences, error) {
	source := make([]Reference, len(references.References))
	for index := range references.References {
		source[index] = references.References[index].Reference
	}
	recomputed, err := CanonicalizeReferences(source)
	provided := references
	if len(provided.References) == 0 {
		provided.References = []CanonicalReference{}
	}
	stale := err != nil || !reflect.DeepEqual(recomputed, provided)
	if !stale && len(requestReferences) > 0 {
		requestCanonical, requestErr := CanonicalizeReferences(requestReferences)
		stale = requestErr != nil || !reflect.DeepEqual(requestCanonical, recomputed)
	}
	if stale {
		return CanonicalReferences{}, validationError(
			"canonical_references_stale",
			"references",
			"规范引用与素材快照不一致。",
			"重新校验素材并编译提示词后再提交。",
			nil,
		)
	}
	return recomputed, nil
}

func canonicalReferenceSnapshots(references CanonicalReferences) []Reference {
	snapshots := make([]Reference, len(references.References))
	for index := range references.References {
		snapshots[index] = references.References[index].Reference
	}
	return snapshots
}

func validateGatewayReferencePredicates(request GenerateRequest) error {
	for index, reference := range request.References {
		if !roleMatchesKind(reference.Role, reference.Kind) {
			return validationError(
				"reference_kind_role_mismatch",
				fmt.Sprintf("references[%d]", index),
				"引用素材类型与用途不匹配。",
				"让图片、视频或音频使用与其类型匹配的引用角色。",
				nil,
			)
		}
	}
	return validateTargetPredicates(request)
}

func validateGatewayTaskMode(mode string, references CanonicalReferences, contract config.GatewayContractConfig) error {
	if !knownTaskMode(mode) {
		return validationError(
			"task_mode_unsupported",
			"taskMode",
			fmt.Sprintf("不支持任务模式 %q。", mode),
			"任务模式只能是 reference、edit 或 extend。",
			nil,
		)
	}
	if !containsString(contract.DeclaredModes, mode) {
		return validationError(
			"gateway_task_mode_not_declared",
			"taskMode",
			fmt.Sprintf("中转站契约未声明任务模式 %q。", mode),
			"切换到契约已声明的任务模式，或更新中转站契约。",
			nil,
		)
	}
	if contract.TaskMode.Name != "" {
		return nil
	}
	if mode == "reference" && isKnownReferenceEncodingMode(contract.References.Mode) {
		return nil
	}

	targetRole := "edit_target"
	if mode == "extend" {
		targetRole = "extend_target"
	}
	if contract.References.Mode == "content_items" &&
		containsString(contract.References.SupportsRoles, targetRole) &&
		strings.TrimSpace(contract.References.RoleFields[targetRole]) != "" &&
		canonicalReferencesContainRole(references, targetRole) {
		return nil
	}
	return validationError(
		"gateway_task_mode_not_encodable",
		"taskMode",
		fmt.Sprintf("中转站契约无法无损表达任务模式 %q。", mode),
		"配置 taskMode 字段，或使用能明确编码目标角色的 content_items 契约。",
		nil,
	)
}

func canonicalReferencesContainRole(references CanonicalReferences, role string) bool {
	for _, reference := range references.References {
		if reference.Role == role {
			return true
		}
	}
	return false
}

func durationGatewayValue(duration int) string {
	if duration == -1 {
		return "smart"
	}
	return strconv.Itoa(duration)
}

func addEncodedGatewayField(payload map[string]any, logicalField string, kind config.GatewayFieldKind, encoding config.FieldEncoding, source string) error {
	if encoding.Name == "" {
		return nil
	}
	value, err := config.EncodeGatewayFieldValue(kind, encoding, source)
	if err != nil {
		return gatewayFieldPolicyError(logicalField, err)
	}
	if _, exists := payload[encoding.Name]; exists {
		return validationError(
			"gateway_field_conflict",
			logicalField,
			fmt.Sprintf("中转站字段 %q 与另一个已声明字段冲突。", encoding.Name),
			"为每个中转站参数配置不同的字段名。",
			nil,
		)
	}
	payload[encoding.Name] = value
	return nil
}

func gatewayFieldPolicyError(field string, err error) *ValidationError {
	var policyErr *config.GatewayContractValidationError
	if errors.As(err, &policyErr) && policyErr.Code == "field_mapping_missing" {
		return validationError(
			"gateway_value_not_declared",
			field,
			fmt.Sprintf("中转站契约未声明 %s 当前值的编码。", field),
			"更新中转站契约映射，或选择契约已声明的值。",
			nil,
		)
	}
	return gatewayValueError(field)
}

func gatewayValueError(field string) *ValidationError {
	return validationError(
		"gateway_value_not_encodable",
		field,
		fmt.Sprintf("%s 无法按中转站契约编码。", field),
		"检查该字段的 valueType 和 valueMap 后重试。",
		nil,
	)
}

func addGatewayReferences(payload map[string]any, references CanonicalReferences, encoding config.ReferenceEncoding) error {
	switch encoding.Mode {
	case "flat_arrays":
		return addFlatGatewayReferences(payload, references, encoding)
	case "content_items":
		return addContentGatewayReferences(payload, references, encoding)
	default:
		return validationError(
			"gateway_reference_encoding_unsupported",
			"gatewayContract.references.mode",
			fmt.Sprintf("不支持中转站引用编码模式 %q。", encoding.Mode),
			"将 references.mode 配置为 flat_arrays 或 content_items。",
			nil,
		)
	}
}

func addFlatGatewayReferences(payload map[string]any, references CanonicalReferences, encoding config.ReferenceEncoding) error {
	images := make([]string, 0)
	videos := make([]string, 0)
	audios := make([]string, 0)
	for index, reference := range references.References {
		if !containsString(encoding.SupportsRoles, reference.Role) {
			return unsupportedGatewayReferenceRole(index, reference.Role)
		}
		switch reference.Kind {
		case "image":
			if reference.Role != "reference_image" {
				return unsupportedGatewayReferenceRole(index, reference.Role)
			}
			images = append(images, reference.URL)
		case "video":
			if reference.Role != "reference_video" {
				return unsupportedGatewayReferenceRole(index, reference.Role)
			}
			videos = append(videos, reference.URL)
		case "audio":
			if reference.Role != "reference_audio" {
				return unsupportedGatewayReferenceRole(index, reference.Role)
			}
			audios = append(audios, reference.URL)
		default:
			return unsupportedGatewayReferenceKind(index, reference.Kind)
		}
	}
	if len(images) > 0 {
		if err := addReferenceArray(payload, encoding.ImageField, images, "image"); err != nil {
			return err
		}
	}
	if len(videos) > 0 {
		if err := addReferenceArray(payload, encoding.VideoField, videos, "video"); err != nil {
			return err
		}
	}
	if len(audios) > 0 {
		if err := addReferenceArray(payload, encoding.AudioField, audios, "audio"); err != nil {
			return err
		}
	}
	return nil
}

func addReferenceArray(payload map[string]any, field string, values []string, kind string) error {
	if field == "" {
		return validationError(
			"gateway_reference_field_missing",
			"gatewayContract.references."+kind+"Field",
			fmt.Sprintf("中转站契约未声明 %s 引用字段。", kind),
			"补充对应引用字段配置后重试。",
			nil,
		)
	}
	if _, exists := payload[field]; exists {
		return validationError(
			"gateway_field_conflict",
			"gatewayContract.references."+kind+"Field",
			fmt.Sprintf("中转站引用字段 %q 与另一个字段冲突。", field),
			"为引用数组配置不同且未占用的字段名。",
			nil,
		)
	}
	payload[field] = values
	return nil
}

func addContentGatewayReferences(payload map[string]any, references CanonicalReferences, encoding config.ReferenceEncoding) error {
	items := make([]map[string]any, 0, len(references.References))
	for index, reference := range references.References {
		mediaField, ok := gatewayReferenceMediaField(encoding, reference.Kind)
		if !ok {
			return unsupportedGatewayReferenceKind(index, reference.Kind)
		}
		if mediaField == "" {
			return validationError(
				"gateway_reference_field_missing",
				fmt.Sprintf("references[%d].kind", index),
				fmt.Sprintf("中转站契约未声明 %s 的素材地址字段。", reference.Label),
				"补充该素材类型的字段配置后重试。",
				nil,
			)
		}
		if !containsString(encoding.SupportsRoles, reference.Role) {
			return unsupportedGatewayReferenceRole(index, reference.Role)
		}
		roleField := encoding.RoleFields[reference.Role]
		if strings.TrimSpace(roleField) == "" {
			return unsupportedGatewayReferenceRole(index, reference.Role)
		}
		if !roleMatchesKind(reference.Role, reference.Kind) {
			return validationError(
				"gateway_reference_kind_role_mismatch",
				fmt.Sprintf("references[%d]", index),
				fmt.Sprintf("%s 的素材类型与用途 %q 不匹配。", reference.Label, reference.Role),
				"让图片、视频或音频使用与其类型匹配的引用用途。",
				nil,
			)
		}
		if encoding.RequiresTargetFirst && (reference.Role == "edit_target" || reference.Role == "extend_target") && reference.Ordinal != 1 {
			return validationError(
				"gateway_target_not_first",
				fmt.Sprintf("references[%d]", index),
				fmt.Sprintf("当前中转站要求编辑或延长目标排在同类素材首位，但目标实际是%s。", reference.Label),
				"把目标视频调整为视频1后重新编译提示词。",
				nil,
			)
		}
		if mediaField == roleField {
			return validationError(
				"gateway_reference_field_conflict",
				fmt.Sprintf("references[%d]", index),
				fmt.Sprintf("%s 的 URL 字段与用途字段都映射为 %q。", reference.Label, mediaField),
				"为素材地址和用途标记配置不同字段名。",
				nil,
			)
		}
		items = append(items, map[string]any{
			mediaField: reference.URL,
			roleField:  true,
		})
	}
	if len(items) == 0 {
		return nil
	}
	if _, exists := payload["content_items"]; exists {
		return validationError(
			"gateway_field_conflict",
			"gatewayContract.references.mode",
			"固定引用字段 content_items 与另一个已声明字段冲突。",
			"不要把其他参数映射到 content_items。",
			nil,
		)
	}
	payload["content_items"] = items
	return nil
}

func gatewayReferenceMediaField(encoding config.ReferenceEncoding, kind string) (string, bool) {
	switch kind {
	case "image":
		return encoding.ImageField, true
	case "video":
		return encoding.VideoField, true
	case "audio":
		return encoding.AudioField, true
	default:
		return "", false
	}
}

func unsupportedGatewayReferenceKind(index int, kind string) *ValidationError {
	return validationError(
		"gateway_reference_kind_unsupported",
		fmt.Sprintf("references[%d].kind", index),
		fmt.Sprintf("中转站契约不能编码素材类型 %q。", kind),
		"引用素材类型只能是 image、video 或 audio。",
		nil,
	)
}

func unsupportedGatewayReferenceRole(index int, role string) *ValidationError {
	return validationError(
		"gateway_reference_role_unsupported",
		fmt.Sprintf("references[%d].role", index),
		fmt.Sprintf("中转站契约未声明引用用途 %q 的无损编码。", role),
		"删除该引用，或更新中转站契约后重试。",
		nil,
	)
}
