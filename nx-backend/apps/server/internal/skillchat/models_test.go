package skillchat

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSessionJSONIncludesOpeningPrompts(t *testing.T) {
	sessionType := reflect.TypeOf(Session{})
	field, ok := sessionType.FieldByName("OpeningPrompts")
	if !ok {
		t.Fatal("Session is missing OpeningPrompts")
	}
	if field.Type != reflect.TypeOf([]string{}) {
		t.Fatalf("OpeningPrompts type=%v, want []string", field.Type)
	}
	if got := field.Tag.Get("json"); got != "openingPrompts" {
		t.Fatalf("OpeningPrompts json tag=%q, want openingPrompts", got)
	}

	session := reflect.New(sessionType).Elem()
	session.FieldByName("OpeningPrompts").Set(reflect.ValueOf([]string{"从今天的卡点聊起", "给我一个小练习"}))
	raw, err := json.Marshal(session.Interface())
	if err != nil {
		t.Fatal(err)
	}
	var dto struct {
		OpeningPrompts []string `json:"openingPrompts"`
	}
	if err := json.Unmarshal(raw, &dto); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dto.OpeningPrompts, []string{"从今天的卡点聊起", "给我一个小练习"}) {
		t.Fatalf("openingPrompts=%v, raw=%s", dto.OpeningPrompts, raw)
	}
}
