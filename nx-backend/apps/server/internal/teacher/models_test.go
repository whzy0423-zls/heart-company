package teacher

import "testing"

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
