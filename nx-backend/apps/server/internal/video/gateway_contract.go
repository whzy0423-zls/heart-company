package video

import (
	"fmt"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/config"
)

// MapGatewayPayload encodes a validated request for the declared intermediary
// contract. Reference ordering is supplied by the caller so prompt compilation
// and gateway encoding can share one canonical value.
func MapGatewayPayload(request GenerateRequest, references CanonicalReferences, contract config.GatewayContractConfig) (map[string]any, error) {
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if err := addEncodedGatewayField(payload, "duration", contract.Duration, durationGatewayValue(request.Duration), true); err != nil {
		return nil, err
	}
	if err := addEncodedGatewayField(payload, "aspectRatio", contract.AspectRatio, request.AspectRatio, false); err != nil {
		return nil, err
	}
	if request.Resolution != "" {
		if contract.Resolution.Name != "" && len(contract.Resolution.ValueMap) == 0 {
			return nil, validationError(
				"gateway_value_not_declared",
				"resolution",
				"中转站契约未声明分辨率枚举映射。",
				"为 resolution.valueMap 声明当前分辨率的明确映射后重试。",
				nil,
			)
		}
		if err := addEncodedGatewayField(payload, "resolution", contract.Resolution, request.Resolution, false); err != nil {
			return nil, err
		}
	}
	if request.GenerateAudio != nil {
		if err := addEncodedGatewayField(payload, "generateAudio", contract.GenerateAudio, strconv.FormatBool(*request.GenerateAudio), false); err != nil {
			return nil, err
		}
	}
	if request.TaskMode != "" {
		if err := addEncodedGatewayField(payload, "taskMode", contract.TaskMode, request.TaskMode, false); err != nil {
			return nil, err
		}
	}

	if err := addGatewayReferences(payload, references, contract.References); err != nil {
		return nil, err
	}
	return payload, nil
}

func durationGatewayValue(duration int) string {
	if duration == -1 {
		return "smart"
	}
	return strconv.Itoa(duration)
}

func addEncodedGatewayField(payload map[string]any, logicalField string, encoding config.FieldEncoding, source string, allowPartialMap bool) error {
	if encoding.Name == "" {
		return nil
	}

	encoded := source
	if allowPartialMap && source == "smart" {
		mapped, declared := encoding.ValueMap[source]
		if !declared {
			return validationError(
				"gateway_value_not_declared",
				logicalField,
				"中转站契约未声明智能时长的编码。",
				"为 duration.valueMap.smart 声明明确映射后重试。",
				nil,
			)
		}
		if strings.TrimSpace(mapped) == "" {
			return gatewayValueError(logicalField, "中转站契约为该值声明了空映射。")
		}
		encoded = mapped
	} else if len(encoding.ValueMap) > 0 {
		mapped, declared := encoding.ValueMap[source]
		if declared {
			if strings.TrimSpace(mapped) == "" {
				return gatewayValueError(logicalField, "中转站契约为该值声明了空映射。")
			}
			encoded = mapped
		} else if !allowPartialMap {
			return validationError(
				"gateway_value_not_declared",
				logicalField,
				fmt.Sprintf("中转站契约未声明 %s 值 %q 的编码。", logicalField, source),
				"更新中转站契约映射，或选择契约已声明的值。",
				nil,
			)
		}
	}

	value, err := gatewayJSONValue(encoded, encoding.ValueType)
	if err != nil {
		return gatewayValueError(logicalField, err.Error())
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

func gatewayJSONValue(encoded, valueType string) (any, error) {
	switch valueType {
	case "string":
		return encoded, nil
	case "int":
		value, err := strconv.Atoi(encoded)
		if err != nil {
			return nil, fmt.Errorf("声明的 int 编码 %q 不是整数", encoded)
		}
		return value, nil
	case "bool":
		if encoded == "true" {
			return true, nil
		}
		if encoded == "false" {
			return false, nil
		}
		return nil, fmt.Errorf("声明的 bool 编码 %q 必须是 true 或 false", encoded)
	default:
		return nil, fmt.Errorf("不支持声明的值类型 %q", valueType)
	}
}

func gatewayValueError(field, detail string) *ValidationError {
	return validationError(
		"gateway_value_not_encodable",
		field,
		fmt.Sprintf("%s 无法按中转站契约编码：%s。", field, strings.TrimSuffix(detail, "。")),
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
