package teacher

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ErrNotFound = errors.New("teacher record not found")

type ReviewState string

const (
	ReviewDraft     ReviewState = "draft"
	ReviewPending   ReviewState = "pending_review"
	ReviewRejected  ReviewState = "rejected"
	ReviewPublished ReviewState = "published"
	ReviewOffline   ReviewState = "offline"
)

var teacherKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

type Teacher struct {
	ID              int64          `json:"id"`
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Title           string         `json:"title,omitempty"`
	Avatar          string         `json:"avatar,omitempty"`
	Cover           string         `json:"cover,omitempty"`
	ShortIntro      string         `json:"shortIntro,omitempty"`
	DetailIntro     string         `json:"detailIntro,omitempty"`
	Expertise       []string       `json:"expertise,omitempty"`
	IntroVideoURL   string         `json:"introVideoUrl,omitempty"`
	CustomerService map[string]any `json:"customerService,omitempty"`
	OfflineService  map[string]any `json:"offlineService,omitempty"`
	ShowOnHome      bool           `json:"showOnHome"`
	ShowInDrawer    bool           `json:"showInDrawer"`
	SortOrder       int            `json:"sortOrder"`
	Enabled         bool           `json:"enabled"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

func (t Teacher) Validate() error {
	t.Key = strings.TrimSpace(t.Key)
	t.Name = strings.TrimSpace(t.Name)
	if !teacherKeyPattern.MatchString(t.Key) {
		return errors.New("teacher key must use lowercase letters, numbers, _ or -")
	}
	if t.Name == "" {
		return errors.New("teacher name is required")
	}
	if len([]rune(t.Name)) > 80 {
		return errors.New("teacher name is too long")
	}
	if t.SortOrder < 0 {
		return errors.New("sort order must not be negative")
	}
	return nil
}

type RoleSet struct {
	Teacher    bool   `json:"-"`
	Agent      bool   `json:"-"`
	TeacherKey string `json:"teacherKey,omitempty"`
	AgentID    int64  `json:"agentId,omitempty"`
}

func (r RoleSet) IsTeacher() bool { return r.Teacher && strings.TrimSpace(r.TeacherKey) != "" }
func (r RoleSet) IsAgent() bool   { return r.Agent && r.AgentID > 0 }

func (r RoleSet) Roles() []string {
	roles := []string{}
	if r.IsTeacher() {
		roles = append(roles, "teacher")
	}
	if r.IsAgent() {
		roles = append(roles, "agent")
	}
	return roles
}

func CanTransitionReview(from, to ReviewState) bool {
	if from == to {
		return true
	}
	switch from {
	case ReviewDraft:
		return to == ReviewPending
	case ReviewPending:
		return to == ReviewPublished || to == ReviewRejected
	case ReviewRejected:
		return to == ReviewPending
	case ReviewPublished:
		return to == ReviewOffline
	default:
		return false
	}
}

func ValidateReviewReason(state ReviewState, reason string) error {
	if state == ReviewRejected && strings.TrimSpace(reason) == "" {
		return errors.New("rejection reason is required")
	}
	if len([]rune(reason)) > 1000 {
		return fmt.Errorf("review reason is too long")
	}
	return nil
}

type ContentDraft struct {
	ID                int64       `json:"id"`
	TeacherKey        string      `json:"teacherKey"`
	SeriesID          *int64      `json:"seriesId,omitempty"`
	Title             string      `json:"title"`
	Description       string      `json:"description,omitempty"`
	ContentType       string      `json:"contentType,omitempty"`
	FeedType          string      `json:"feedType"`
	ShowAsStandalone  bool        `json:"showAsStandalone"`
	CoverURL          string      `json:"coverUrl,omitempty"`
	MediaAssetID      *int64      `json:"mediaAssetId,omitempty"`
	MediaStatus       string      `json:"mediaStatus,omitempty"`
	Status            string      `json:"status,omitempty"`
	DurationSeconds   int         `json:"durationSeconds,omitempty"`
	ReviewStatus      ReviewState `json:"reviewStatus"`
	ReviewReason      string      `json:"reviewReason,omitempty"`
	ReplacesContentID *int64      `json:"replacesContentId,omitempty"`
	PublishedAt       *time.Time  `json:"publishedAt,omitempty"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
	LikeCount         int         `json:"likeCount"`
	FavoriteCount     int         `json:"favoriteCount"`
}
