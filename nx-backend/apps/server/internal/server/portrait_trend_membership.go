package server

import (
	"context"
	"fmt"
)

// Portrait and trend details have a VIP floor independently of basic card
// ownership. Keep this local to these detail endpoints so free primary-card
// history and ordinary chat retain their existing access rules.
func (s *Server) portraitTrendMembershipResourceMetadata(ctx context.Context, userID, cardID int64, reason string) (membershipResourceMetadata, error) {
	if s == nil || s.db == nil || userID <= 0 || cardID <= 0 {
		return membershipResourceMetadata{}, errMembershipPlanUnavailable
	}
	plan, err := s.currentAppMembershipPlanWithError(ctx, userID)
	if err != nil {
		// A missing membership column is not evidence of a paid entitlement.
		// Surface the lookup failure instead of inheriting the basic-card
		// compatibility fallback that permits a free primary card.
		return membershipResourceMetadata{}, fmt.Errorf("portrait/trend membership lookup: %w", err)
	}
	state, found, err := s.compatibilityCardResourceAccess(ctx, userID, cardID, plan.PlanLevel)
	if err != nil {
		return membershipResourceMetadata{}, fmt.Errorf("portrait/trend card access lookup: %w", err)
	}
	access := membershipMetadataForCard(plan.PlanLevel, state, found, reason)
	if membershipLevelRank(access.RequiredPlanLevel) < membershipLevelRank("vip") {
		return membershipResourceMetadataForPlan(plan.PlanLevel, "vip", reason), nil
	}
	// A retained excess secondary card can require SVIP. Do not weaken that
	// resource-specific requirement when applying the general VIP floor.
	return access, nil
}
