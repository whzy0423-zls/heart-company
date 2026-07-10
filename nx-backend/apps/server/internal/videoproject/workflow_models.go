package videoproject

import "nine-xing/nx-backend/apps/server/internal/video"

type WorkflowMessage struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Fix        string         `json:"fix"`
	TargetType string         `json:"targetType"`
	TargetID   string         `json:"targetId"`
	Details    map[string]any `json:"details"`
}

type WorkflowStepStatus struct {
	Key        string            `json:"key"`
	Status     string            `json:"status"`
	Progress   int               `json:"progress"`
	Blockers   []WorkflowMessage `json:"blockers"`
	Warnings   []WorkflowMessage `json:"warnings"`
	Evidence   map[string]any    `json:"evidence"`
	NextAction string            `json:"nextAction"`
}

type WorkflowOverview struct {
	Project      Project              `json:"project"`
	CurrentStep  string               `json:"currentStep"`
	Overall      int                  `json:"overall"`
	Steps        []WorkflowStepStatus `json:"steps"`
	Capabilities video.Capabilities   `json:"capabilities"`
	Blockers     []WorkflowMessage    `json:"blockers"`
	Warnings     []WorkflowMessage    `json:"warnings"`
}

func NewWorkflowOverview() WorkflowOverview {
	return WorkflowOverview{
		Steps:    []WorkflowStepStatus{},
		Blockers: []WorkflowMessage{},
		Warnings: []WorkflowMessage{},
	}
}

func NewWorkflowStepStatus(key string) WorkflowStepStatus {
	return WorkflowStepStatus{
		Key:      key,
		Blockers: []WorkflowMessage{},
		Warnings: []WorkflowMessage{},
		Evidence: map[string]any{},
	}
}

type BreakdownItem struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	VisualPrompt string `json:"visualPrompt"`
	UsageNote    string `json:"usageNote"`
	Required     bool   `json:"required"`
	Decision     string `json:"decision"`
}

type StoryBeat struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	SceneKeys   []string `json:"sceneKeys"`
	AssetKeys   []string `json:"assetKeys"`
}

type BreakdownVersion struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"projectId"`
	Version              int             `json:"version"`
	Revision             int             `json:"revision"`
	Status               string          `json:"status"`
	SourceScriptRevision int             `json:"sourceScriptRevision"`
	ScriptSnapshot       string          `json:"scriptSnapshot"`
	Characters           []BreakdownItem `json:"characters"`
	Scenes               []BreakdownItem `json:"scenes"`
	Props                []BreakdownItem `json:"props"`
	Outfits              []BreakdownItem `json:"outfits"`
	Styles               []BreakdownItem `json:"styles"`
	StoryBeats           []StoryBeat     `json:"storyBeats"`
	RawResult            string          `json:"rawResult"`
	ErrorMessage         string          `json:"errorMessage"`
	CreateTime           string          `json:"createTime"`
	UpdateTime           string          `json:"updateTime"`
}

func NewBreakdownVersion() BreakdownVersion {
	return BreakdownVersion{
		Characters: []BreakdownItem{},
		Scenes:     []BreakdownItem{},
		Props:      []BreakdownItem{},
		Outfits:    []BreakdownItem{},
		Styles:     []BreakdownItem{},
		StoryBeats: []StoryBeat{},
	}
}

type ProjectAsset struct {
	ID                string           `json:"id"`
	ProjectID         string           `json:"projectId"`
	Type              string           `json:"type"`
	BreakdownItemKey  string           `json:"breakdownItemKey"`
	SourceBreakdownID string           `json:"sourceBreakdownId"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	VisualPrompt      string           `json:"visualPrompt"`
	UsageNote         string           `json:"usageNote"`
	Required          bool             `json:"required"`
	GlobalAssetID     string           `json:"globalAssetId"`
	ReferenceImageURL string           `json:"referenceImageUrl"`
	Source            string           `json:"source"`
	Status            string           `json:"status"`
	Metadata          map[string]any   `json:"metadata"`
	Candidates        []AssetCandidate `json:"candidates"`
	CreateTime        string           `json:"createTime"`
	UpdateTime        string           `json:"updateTime"`
}

type AssetCandidate struct {
	ID                  string `json:"id"`
	ProjectID           string `json:"projectId"`
	TargetType          string `json:"targetType"`
	TargetID            string `json:"targetId"`
	Prompt              string `json:"prompt"`
	ImageAssetID        string `json:"imageAssetId"`
	ImageURL            string `json:"imageUrl"`
	Source              string `json:"source"`
	GenerationRequestID string `json:"generationRequestId"`
	Status              string `json:"status"`
	ErrorMessage        string `json:"errorMessage"`
	Selected            bool   `json:"selected"`
	CreateTime          string `json:"createTime"`
	UpdateTime          string `json:"updateTime"`
}

type StoryboardReferenceIntent struct {
	AssetKey  string `json:"assetKey"`
	Role      string `json:"role"`
	SortOrder int    `json:"sortOrder"`
	UsageNote string `json:"usageNote"`
}

type StoryboardShot struct {
	SourceKey     string                      `json:"sourceKey"`
	Name          string                      `json:"name"`
	Enabled       bool                        `json:"enabled"`
	Duration      int                         `json:"duration"`
	SceneKey      string                      `json:"sceneKey"`
	CharacterKeys []string                    `json:"characterKeys"`
	AssetKeys     []string                    `json:"assetKeys"`
	Action        string                      `json:"action"`
	Camera        string                      `json:"camera"`
	Composition   string                      `json:"composition"`
	Lighting      string                      `json:"lighting"`
	Audio         string                      `json:"audio"`
	Dialogue      string                      `json:"dialogue"`
	TaskMode      string                      `json:"taskMode"`
	References    []StoryboardReferenceIntent `json:"references"`
}

type StoryboardVersion struct {
	ID                      string           `json:"id"`
	ProjectID               string           `json:"projectId"`
	Version                 int              `json:"version"`
	Revision                int              `json:"revision"`
	Status                  string           `json:"status"`
	SourceScriptRevision    int              `json:"sourceScriptRevision"`
	SourceBreakdownID       string           `json:"sourceBreakdownId"`
	SourceAssetRevision     int              `json:"sourceAssetRevision"`
	SourceCapabilityVersion string           `json:"sourceCapabilityVersion"`
	BaselineStoryboardID    string           `json:"baselineStoryboardId"`
	Shots                   []StoryboardShot `json:"shots"`
	RawResult               string           `json:"rawResult"`
	ErrorMessage            string           `json:"errorMessage"`
	CreateTime              string           `json:"createTime"`
	UpdateTime              string           `json:"updateTime"`
}

func NewStoryboardVersion() StoryboardVersion {
	return StoryboardVersion{Shots: []StoryboardShot{}}
}

type StoryboardDiffItem struct {
	Operation string         `json:"operation"`
	SourceKey string         `json:"sourceKey"`
	ShotID    string         `json:"shotId"`
	Before    map[string]any `json:"before"`
	After     map[string]any `json:"after"`
}

type StoryboardDiff struct {
	StoryboardID string               `json:"storyboardId"`
	Revision     int                  `json:"revision"`
	DiffToken    string               `json:"diffToken"`
	Items        []StoryboardDiffItem `json:"items"`
	Warnings     []WorkflowMessage    `json:"warnings"`
}

func NewStoryboardDiff() StoryboardDiff {
	return StoryboardDiff{Items: []StoryboardDiffItem{}, Warnings: []WorkflowMessage{}}
}

type WorkflowConflictError struct {
	Code            string         `json:"code"`
	Message         string         `json:"message"`
	CurrentRevision int            `json:"currentRevision"`
	Details         map[string]any `json:"details"`
}

func (e *WorkflowConflictError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type WorkflowValidationError struct {
	Code    string         `json:"code"`
	Field   string         `json:"field"`
	Message string         `json:"message"`
	Fix     string         `json:"fix"`
	Details map[string]any `json:"details"`
}

func (e *WorkflowValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}
