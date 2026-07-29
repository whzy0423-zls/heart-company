package server

import (
	"os"
	"strings"
	"testing"
)

func TestVoiceProfileCopyToBailianRouteRequiresProfileManagePermission(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `s.mux.HandleFunc("/api/voice/profiles/", s.requirePermission("Voice:Profile:Manage", s.voiceProfileByID))`
	if !strings.Contains(string(source), want) {
		t.Fatalf("copy-to-bailian must be routed through the voice profile manager permission: %s", want)
	}

	bodyStart := strings.Index(string(source), "func (s *Server) voiceProfileByID")
	if bodyStart < 0 {
		t.Fatal("voiceProfileByID handler not found")
	}
	body := string(source)[bodyStart:]
	if !strings.Contains(body, `"copy-to-bailian"`) || !strings.Contains(body, "CopyProfileToBailian") {
		t.Fatal("POST /api/voice/profiles/{id}/copy-to-bailian must call CopyProfileToBailian")
	}
}
