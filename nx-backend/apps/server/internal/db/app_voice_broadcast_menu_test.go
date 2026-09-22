package db

import (
	"os"
	"strings"
	"testing"
)

func TestSeedCallsVoiceBroadcastMenuMigrationOnce(t *testing.T) {
	source, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	seedStart := strings.Index(string(source), "func seed(ctx ")
	seedEnd := strings.Index(string(source), "type seedMenu struct")
	if seedStart < 0 || seedEnd <= seedStart {
		t.Fatal("could not locate seed function")
	}
	if got := strings.Count(string(source[seedStart:seedEnd]), "seedVoiceBroadcastMenu(ctx, database)"); got != 1 {
		t.Fatalf("seed must call voice broadcast menu migration exactly once, got %d", got)
	}
}

func TestVoiceBroadcastMenuMigrationHasStableIdentityAndNoSecondParent(t *testing.T) {
	source, err := os.ReadFile("app_voice_broadcast_menu.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, required := range []string{
		"voiceBroadcastMenuID        int64 = 1621",
		"voiceBroadcastMenuPath            = \"/app/voice-broadcast-config\"",
		"voiceBroadcastMenuComponent       = \"/app/voice-broadcast\"",
		"voiceBroadcastAuthCode            = \"App:VoiceBroadcast:Manage\"",
		"WHERE name='AppManage' AND path='/app' AND status=1",
		"DELETE FROM menus WHERE id<>$1 AND (path=$2 OR name=$3)",
		"seed.app_voice_broadcast_menu.v1",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("menu migration missing %q", required)
		}
	}
}

func TestVoiceBroadcastRoleBindingCastsMenuIDToBigint(t *testing.T) {
	source, err := os.ReadFile("app_voice_broadcast_menu.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.ToUpper(string(source)), "$1::BIGINT FROM"); got != 2 {
		t.Fatalf("role menu bindings must cast both menu ID parameters to bigint, got %d", got)
	}
}
