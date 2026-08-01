package video

type Reference struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Role            string   `json:"role"`
	URL             string   `json:"url"`
	SortOrder       int      `json:"sortOrder"`
	SourceType      string   `json:"sourceType"`
	SourceID        string   `json:"sourceId"`
	UsageNote       string   `json:"usageNote"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
}

type GenerateRequest struct {
	Model             string      `json:"model"`
	Prompt            string      `json:"prompt"`
	Duration          int         `json:"duration"`
	AspectRatio       string      `json:"aspectRatio"`
	Resolution        string      `json:"resolution"`
	GenerateAudio     *bool       `json:"generateAudio"`
	TaskMode          string      `json:"taskMode"`
	References        []Reference `json:"references"`
	RequestKey        string      `json:"requestKey"`
	CapabilityVersion string      `json:"capabilityVersion"`
	Seed              *int        `json:"seed,omitempty"`
	CameraFixed       *bool       `json:"cameraFixed,omitempty"`
}
