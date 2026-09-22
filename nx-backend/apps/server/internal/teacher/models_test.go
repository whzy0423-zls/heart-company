package teacher

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTeacherProfileValidate(t *testing.T) {
	valid := Teacher{Key: "han", Name: "老韩", Enabled: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid teacher rejected: %v", err)
	}
	for name, item := range map[string]Teacher{
		"missing key":  {Name: "老韩"},
		"bad key":      {Key: "老 韩", Name: "老韩"},
		"missing name": {Key: "han"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := item.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestReviewStateTransitions(t *testing.T) {
	allowed := [][2]ReviewState{
		{ReviewDraft, ReviewPending},
		{ReviewPending, ReviewPublished},
		{ReviewPending, ReviewRejected},
		{ReviewRejected, ReviewPending},
		{ReviewPublished, ReviewOffline},
	}
	for _, pair := range allowed {
		if !CanTransitionReview(pair[0], pair[1]) {
			t.Errorf("expected transition %s -> %s", pair[0], pair[1])
		}
	}
	if CanTransitionReview(ReviewDraft, ReviewPublished) || CanTransitionReview(ReviewOffline, ReviewPublished) {
		t.Fatal("invalid review transition accepted")
	}
}

func TestRoleSetKeepsTeacherAndAgentIndependent(t *testing.T) {
	roles := RoleSet{Teacher: true, Agent: true, TeacherKey: "han", AgentID: 9}
	if !roles.IsTeacher() || !roles.IsAgent() {
		t.Fatal("teacher and agent roles must coexist")
	}
	roles.Teacher = false
	if roles.IsAgent() == false || roles.TeacherKey != "han" || roles.AgentID != 9 {
		t.Fatal("disabling one role must preserve the other role and bindings")
	}
}

func TestContentDraftExposesMediaMetadata(t *testing.T) {
	mediaID := int64(17)
	encoded, err := json.Marshal(ContentDraft{
		ID:              9,
		ContentType:     "video",
		MediaAssetID:    &mediaID,
		MediaStatus:     "processing",
		Status:          "processing",
		DurationSeconds: 42,
		CoverURL:        "/api/classroom/covers/9",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"contentType":"video"`, `"mediaAssetId":17`, `"mediaStatus":"processing"`, `"status":"processing"`, `"durationSeconds":42`, `"coverUrl":"/api/classroom/covers/9"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s in %s", want, encoded)
		}
	}
}
