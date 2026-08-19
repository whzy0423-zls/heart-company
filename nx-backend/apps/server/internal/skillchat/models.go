package skillchat

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("skill chat: not found")
	ErrInvalidInput       = errors.New("skill chat: invalid input")
	ErrStoreUnavailable   = errors.New("skill chat: store unavailable")
	ErrVersionUnavailable = errors.New("skill chat: version unavailable")
	ErrSessionChanged     = errors.New("skill chat: session changed")
)

type Session struct {
	ID                 int64                 `json:"id"`
	AppUserID          int64                 `json:"-"`
	SkillID            int64                 `json:"skillId"`
	SkillVersionID     int64                 `json:"skillVersionId"`
	SkillKey           string                `json:"skillKey"`
	SkillName          string                `json:"skillName"`
	SkillIconKey       string                `json:"skillIconKey"`
	Title              string                `json:"title"`
	Scene              string                `json:"scene"`
	Version            string                `json:"version"`
	Instructions       string                `json:"-"`
	OpeningPrompts     []string              `json:"openingPrompts"`
	TheoryReleaseID    int64                 `json:"-"`
	SafetyProfile      string                `json:"safetyProfile"`
	MinAppVersion      string                `json:"minAppVersion"`
	SourceMetadata     SessionSourceMetadata `json:"sourceMetadata"`
	VersionStatus      string                `json:"-"`
	LibraryStatus      string                `json:"-"`
	CategoryStatus     string                `json:"-"`
	SkillStatus        string                `json:"-"`
	GenerationRevision int64                 `json:"-"`
	UpdatedAt          string                `json:"updatedAt"`
	CreateTime         string                `json:"createTime"`
}

type SessionSourceMetadata struct {
	ReviewPolicy      string   `json:"reviewPolicy,omitempty"`
	ReviewDecisionRef string   `json:"reviewDecisionRef,omitempty"`
	ReviewDecision    string   `json:"reviewDecision,omitempty"`
	RiskNotices       []string `json:"riskNotices,omitempty"`
	SourceNeeded      bool     `json:"sourceNeeded"`
	CompilerPolicy    string   `json:"compilerPolicy,omitempty"`
}

func sanitizeSessionSourceMetadata(raw []byte) SessionSourceMetadata {
	var metadata SessionSourceMetadata
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &metadata)
	}
	if metadata.RiskNotices == nil {
		metadata.RiskNotices = []string{}
	}
	return metadata
}

// GenerationTrace contains identifiers only. It deliberately excludes the
// user's question, transcript, generated answer, and retrieved chunk content.
type GenerationTrace struct {
	GenerationRevision int64   `json:"generationRevision"`
	SkillVersionID     int64   `json:"skillVersionId"`
	TheoryReleaseID    int64   `json:"theoryReleaseId"`
	ChunkIDs           []int64 `json:"chunkIds"`
}

func (s Session) Runnable() bool {
	return s.Scene == "skill_chat" && s.SkillVersionID > 0 && s.TheoryReleaseID > 0 &&
		s.LibraryStatus == "enabled" && s.CategoryStatus == "enabled" && s.SkillStatus == "enabled" &&
		(s.VersionStatus == "published" || s.VersionStatus == "retired")
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
