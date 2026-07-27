package xinzhili

type RealtimeCapabilities struct {
	ProtocolVersions []string `json:"protocolVersions"`
	PreferredVersion string   `json:"preferredVersion"`
	Features         []string `json:"features"`
	MinimumAppBuild  int      `json:"minimumAppBuild"`
}

func DefaultRealtimeCapabilities() RealtimeCapabilities {
	return RealtimeCapabilities{
		ProtocolVersions: []string{ProtocolVersion},
		PreferredVersion: ProtocolVersion,
		Features: []string{
			"strict-envelope",
			"turn-key",
			"playback-ack",
			"generation",
		},
		MinimumAppBuild: 1,
	}
}
