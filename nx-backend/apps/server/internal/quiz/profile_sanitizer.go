package quiz

import "encoding/json"

var retiredWingProfileKeys = map[string]struct{}{
	"wingType":   {},
	"wingLabel":  {},
	"wing_type":  {},
	"wing_label": {},
}

// SanitizeProfileJSON removes retired structured wing data at every nesting level.
// Invalid profile payloads fail closed so legacy data cannot bypass sanitization.
func SanitizeProfileJSON(raw json.RawMessage) json.RawMessage {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return json.RawMessage(`{}`)
	}
	sanitizeProfileValue(value)
	clean, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(clean)
}

func sanitizeProfileValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, retired := retiredWingProfileKeys[key]; retired {
				delete(typed, key)
				continue
			}
			sanitizeProfileValue(child)
		}
	case []any:
		for _, child := range typed {
			sanitizeProfileValue(child)
		}
	}
}
