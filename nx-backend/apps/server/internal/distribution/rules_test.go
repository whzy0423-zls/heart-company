package distribution

import "testing"

func TestCreateChildAgentRequiresDirectInviteAndNextLevel(t *testing.T) {
	parent := Agent{ID: 10, AppUserID: 100, Level: 1, Status: StatusActive, RootAgentID: 10, Path: "/10/"}
	invited := Relation{UserID: 200, DirectAgentID: 10}
	child, err := CreateChildAgent(parent, invited, 99)
	if err != nil {
		t.Fatalf("CreateChildAgent() error = %v", err)
	}
	if child.Level != 2 || child.ParentAgentID != 10 || child.RootAgentID != 10 || child.Path != "/10/99/" {
		t.Fatalf("unexpected child: %+v", child)
	}
}

func TestCreateChildAgentRejectsIndirectInviteAndLevelThree(t *testing.T) {
	parent := Agent{ID: 10, AppUserID: 100, Level: 1, Status: StatusActive, RootAgentID: 10, Path: "/10/"}
	if _, err := CreateChildAgent(parent, Relation{UserID: 200, DirectAgentID: 11}, 99); err != ErrNotDirectInvite {
		t.Fatalf("indirect error = %v", err)
	}
	level3 := Agent{ID: 30, AppUserID: 300, Level: 3, Status: StatusActive, RootAgentID: 10, Path: "/10/20/30/"}
	if _, err := CreateChildAgent(level3, Relation{UserID: 400, DirectAgentID: 30}, 99); err != ErrMaxLevel {
		t.Fatalf("level 3 error = %v", err)
	}
}

func TestBindInviteIsFirstBindingAndRejectsCycles(t *testing.T) {
	if err := ValidateInviteBinding(Agent{ID: 10, AppUserID: 100, Status: StatusActive}, 100, Relation{}); err != ErrSelfInvite {
		t.Fatalf("self invite = %v", err)
	}
	if err := ValidateInviteBinding(Agent{ID: 10, AppUserID: 100, Status: StatusPaused}, 200, Relation{}); err != ErrAgentInactive {
		t.Fatalf("inactive = %v", err)
	}
	if err := ValidateInviteBinding(Agent{ID: 10, AppUserID: 100, Status: StatusActive}, 200, Relation{DirectAgentID: 20}); err != ErrAlreadyBound {
		t.Fatalf("existing binding = %v", err)
	}
}

func TestCommissionSnapshotSplitsDirectAndOverride(t *testing.T) {
	chain := []Agent{{ID: 10, Level: 1}, {ID: 20, Level: 2}, {ID: 30, Level: 3}}
	rules := RuleSet{Version: 7, Rates: map[int]int64{1: 1000, 2: 500, 3: 250}}
	items, err := BuildCommissionSnapshot(10000, chain, rules)
	if err != nil {
		t.Fatalf("snapshot error = %v", err)
	}
	if len(items) != 3 || items[0].AmountCents != 1000 || items[1].AmountCents != 500 || items[2].AmountCents != 250 {
		t.Fatalf("unexpected items: %+v", items)
	}
	if items[0].RuleVersion != 7 || items[0].ChainPath != "/10/20/30/" {
		t.Fatalf("missing snapshot: %+v", items[0])
	}
}
