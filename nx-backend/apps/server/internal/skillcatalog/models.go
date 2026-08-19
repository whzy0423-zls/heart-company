package skillcatalog

import "encoding/json"

type Library struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconKey     string `json:"iconKey"`
	SkillCount  int    `json:"skillCount"`
}

type Category struct {
	ID         int64  `json:"id"`
	LibraryID  int64  `json:"libraryId"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	IconKey    string `json:"iconKey"`
	ColorToken string `json:"colorToken"`
	SkillCount int    `json:"skillCount"`
}

type SkillSummary struct {
	ID           int64  `json:"id"`
	CategoryID   int64  `json:"categoryId"`
	CategoryKey  string `json:"categoryKey"`
	CategoryName string `json:"categoryName"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Summary      string `json:"summary"`
	IconKey      string `json:"iconKey"`
	ColorToken   string `json:"colorToken"`
	VersionID    int64  `json:"versionId"`
	Version      string `json:"version"`
	SortOrder    int    `json:"-"`
}

type PublishedVersion struct {
	ID              int64           `json:"id"`
	Version         string          `json:"version"`
	RuntimeVersion  int             `json:"runtimeVersion"`
	Instructions    string          `json:"-"`
	OpeningPrompts  []string        `json:"openingPrompts"`
	TheoryReleaseID int64           `json:"-"`
	SafetyProfile   string          `json:"safetyProfile"`
	ContentHash     string          `json:"-"`
	MinAppVersion   string          `json:"minAppVersion"`
	SourceMetadata  json.RawMessage `json:"sourceMetadata"`
	PublishedAt     string          `json:"publishedAt"`
}

type SkillDetail struct {
	ID           int64            `json:"id"`
	CategoryID   int64            `json:"categoryId"`
	LibraryID    int64            `json:"libraryId"`
	Key          string           `json:"key"`
	Name         string           `json:"name"`
	Summary      string           `json:"summary"`
	Description  string           `json:"description"`
	IconKey      string           `json:"iconKey"`
	ColorToken   string           `json:"colorToken"`
	CategoryName string           `json:"categoryName"`
	Version      PublishedVersion `json:"publishedVersion"`
}

type SkillFilter struct {
	LibraryID  int64
	CategoryID int64
	Query      string
	CursorSort int
	CursorID   int64
	Limit      int
}

type SkillPage struct {
	Items      []SkillSummary `json:"items"`
	NextSort   int            `json:"-"`
	NextID     int64          `json:"-"`
	HasMore    bool           `json:"hasMore"`
	NextCursor string         `json:"nextCursor,omitempty"`
}
