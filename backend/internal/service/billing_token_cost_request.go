package service

import (
	"context"
	"time"
)

// TokenCostRequest is the shared token-billing input used by gateway paths.
type TokenCostRequest struct {
	Ctx             context.Context
	Model           string
	Group           *Group
	Tokens          UsageTokens
	RateMultiplier  float64
	PricingAt       time.Time
	ServiceTier     string
	ReasoningEffort string
	Resolver        *ModelPricingResolver
	Resolved        *ResolvedPricing
}

// CalculateTokenCostForRequest keeps channel/group interval pricing, service
// tiers, long-context policy and reasoning-effort multipliers on one path.
func (s *BillingService) CalculateTokenCostForRequest(req TokenCostRequest) (*CostBreakdown, error) {
	resolved := req.Resolved
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		return s.CalculateCostUnified(s.tokenCostInput(req, resolved))
	}
	if req.Resolver != nil && req.Group != nil {
		return s.CalculateCostUnified(s.tokenCostInput(req, resolved))
	}
	if req.ReasoningEffort != "" {
		return s.CalculateCostUnified(s.tokenCostInput(req, resolved))
	}
	return s.CalculateCost(req.Model, req.Tokens, req.RateMultiplier)
}

func (s *BillingService) tokenCostInput(req TokenCostRequest, resolved *ResolvedPricing) CostInput {
	input := CostInput{
		Ctx:             req.Ctx,
		Model:           req.Model,
		Group:           req.Group,
		Tokens:          req.Tokens,
		RequestCount:    1,
		RateMultiplier:  req.RateMultiplier,
		PricingAt:       req.PricingAt,
		ServiceTier:     req.ServiceTier,
		ReasoningEffort: req.ReasoningEffort,
		Resolver:        req.Resolver,
		Resolved:        resolved,
	}
	if req.Group != nil {
		groupID := req.Group.ID
		input.GroupID = &groupID
	}
	return input
}
