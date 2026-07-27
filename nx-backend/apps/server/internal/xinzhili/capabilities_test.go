package xinzhili

import "testing"

func TestRealtimeCapabilitiesExposeStableV1Contract(t *testing.T) {
	got := DefaultRealtimeCapabilities()
	if got.PreferredVersion != ProtocolVersion {
		t.Fatalf("preferredVersion=%q want=%q", got.PreferredVersion, ProtocolVersion)
	}
	if len(got.ProtocolVersions) != 1 || got.ProtocolVersions[0] != ProtocolVersion {
		t.Fatalf("protocolVersions=%v", got.ProtocolVersions)
	}
	wantFeatures := map[string]bool{
		"strict-envelope": false,
		"turn-key":        false,
		"playback-ack":    false,
		"generation":      false,
	}
	for _, feature := range got.Features {
		if _, ok := wantFeatures[feature]; ok {
			wantFeatures[feature] = true
		}
	}
	for feature, found := range wantFeatures {
		if !found {
			t.Errorf("missing feature %q in %v", feature, got.Features)
		}
	}
	if got.MinimumAppBuild < 0 {
		t.Fatalf("minimumAppBuild=%d", got.MinimumAppBuild)
	}
}

func TestRealtimeCapabilitiesReturnsIndependentSlices(t *testing.T) {
	first := DefaultRealtimeCapabilities()
	first.ProtocolVersions[0] = "changed"
	first.Features[0] = "changed"

	second := DefaultRealtimeCapabilities()
	if second.ProtocolVersions[0] != ProtocolVersion {
		t.Fatalf("protocol versions shared mutable state: %v", second.ProtocolVersions)
	}
	if second.Features[0] == "changed" {
		t.Fatalf("features shared mutable state: %v", second.Features)
	}
}
