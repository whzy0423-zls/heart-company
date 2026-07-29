package xinzhili

import "testing"

func TestDefaultRealtimeCapabilitiesMatchesAppContract(t *testing.T) {
	got := DefaultRealtimeCapabilities()
	if len(got.ProtocolVersions) != 1 || got.ProtocolVersions[0] != ProtocolVersion {
		t.Fatalf("protocol versions = %#v", got.ProtocolVersions)
	}
	if got.PreferredVersion != ProtocolVersion {
		t.Fatalf("preferred version = %q", got.PreferredVersion)
	}
	wantFeatures := []string{"strict-envelope", "turn-key", "playback-ack", "generation"}
	if len(got.Features) != len(wantFeatures) {
		t.Fatalf("features = %#v", got.Features)
	}
	for i, want := range wantFeatures {
		if got.Features[i] != want {
			t.Fatalf("features[%d] = %q, want %q", i, got.Features[i], want)
		}
	}
	if got.MinimumAppBuild != 1 {
		t.Fatalf("minimum app build = %d", got.MinimumAppBuild)
	}
}
