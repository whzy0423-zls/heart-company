package distribution

import (
	"errors"
	"fmt"
	"strings"
)

const (
	StatusActive = "active"
	StatusPaused = "paused"
)

var (
	ErrAgentInactive   = errors.New("agent is inactive")
	ErrAlreadyBound    = errors.New("user already has an invite relation")
	ErrSelfInvite      = errors.New("agent cannot invite self")
	ErrNotDirectInvite = errors.New("user was not directly invited by this agent")
	ErrMaxLevel        = errors.New("maximum agent level reached")
	ErrInvalidChain    = errors.New("invalid agent chain")
	ErrMissingRate     = errors.New("commission rate is missing")
)

type Agent struct {
	ID            int64
	AppUserID     int64
	AgentCode     string
	Level         int
	ParentAgentID int64
	RootAgentID   int64
	Path          string
	Status        string
}

type Relation struct {
	UserID        int64
	DirectAgentID int64
}

type RuleSet struct {
	Version int64
	// Rates are basis points: 1000 = 10%.
	Rates map[int]int64
}

type CommissionItem struct {
	AgentID     int64
	Level       int
	OrderCents  int64
	RateBPS     int64
	AmountCents int64
	RuleVersion int64
	ChainPath   string
}

func CreateChildAgent(parent Agent, invited Relation, childAgentID int64) (Agent, error) {
	if parent.Status != StatusActive {
		return Agent{}, ErrAgentInactive
	}
	if parent.Level >= 3 {
		return Agent{}, ErrMaxLevel
	}
	if invited.DirectAgentID != parent.ID {
		return Agent{}, ErrNotDirectInvite
	}
	if childAgentID <= 0 {
		return Agent{}, fmt.Errorf("child agent id must be positive")
	}
	return Agent{ID: childAgentID, AppUserID: invited.UserID, Level: parent.Level + 1, ParentAgentID: parent.ID, RootAgentID: parent.RootAgentIDOrSelf(), Path: joinPath(parent.Path, childAgentID), Status: StatusActive}, nil
}

func (a Agent) RootAgentIDOrSelf() int64 {
	if a.RootAgentID != 0 {
		return a.RootAgentID
	}
	return a.ID
}

func ValidateInviteBinding(agent Agent, appUserID int64, existing Relation) error {
	if agent.Status != StatusActive {
		return ErrAgentInactive
	}
	if agent.AppUserID == appUserID {
		return ErrSelfInvite
	}
	if existing.DirectAgentID != 0 {
		return ErrAlreadyBound
	}
	return nil
}

func BuildCommissionSnapshot(orderCents int64, chain []Agent, rules RuleSet) ([]CommissionItem, error) {
	if orderCents <= 0 || len(chain) == 0 || len(chain) > 3 {
		return nil, ErrInvalidChain
	}
	path := ""
	for i, a := range chain {
		if a.ID <= 0 || a.Level != i+1 {
			return nil, ErrInvalidChain
		}
		path = joinPath(path, a.ID)
	}
	items := make([]CommissionItem, 0, len(chain))
	for _, a := range chain {
		rate, ok := rules.Rates[a.Level]
		if !ok || rate < 0 || rate > 10000 {
			return nil, ErrMissingRate
		}
		items = append(items, CommissionItem{AgentID: a.ID, Level: a.Level, OrderCents: orderCents, RateBPS: rate, AmountCents: orderCents * rate / 10000, RuleVersion: rules.Version, ChainPath: path})
	}
	return items, nil
}

func joinPath(parent string, id int64) string {
	parent = strings.TrimSuffix(strings.TrimSpace(parent), "/")
	return fmt.Sprintf("%s/%d/", parent, id)
}
