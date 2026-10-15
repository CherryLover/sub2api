package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrGeminiCacheBillingUnavailable = infraerrors.ServiceUnavailable("GEMINI_CACHE_BILLING_UNAVAILABLE", "Explicit cache billing is unavailable")

// GeminiCachedContentSettlement is a trusted server-side quote. The repository
// commits this command, its usage log and the cache binding in one transaction.
// Unlike ordinary postpaid requests, cache creation/renewal must fit the current
// key lifetime quota and spending windows before the binding can become usable.
type GeminiCachedContentSettlement struct {
	Command       *UsageBillingCommand
	Log           *UsageLog
	EnforceLimits bool
	params        *postUsageBillingParams
}

type GeminiCachedContentBillingRepository interface {
	CreateWithSettlement(context.Context, *GeminiCachedContent, *GeminiCachedContentSettlement) (*UsageBillingApplyResult, error)
	CommitMutationWithSettlement(context.Context, *GeminiCachedContent, string, time.Time, *GeminiCachedContentSettlement) (*UsageBillingApplyResult, error)
	DeleteWithMutation(context.Context, *GeminiCachedContent, string) error
}

// CacheStoragePricePerTokenHour preserves explicit model-file prices. An invalid
// configured price fails closed instead of silently falling back to free storage.
// Built-in Standard prices are sourced from https://ai.google.dev/gemini-api/docs/pricing.
func (s *BillingService) CacheStoragePricePerTokenHour(model string) float64 {
	if s == nil {
		return 0
	}
	if s.pricingService != nil {
		if p := s.pricingService.GetModelPricing(model); p != nil && p.CacheStorageCostPerTokenPerHour != nil {
			v := *p.CacheStorageCostPerTokenPerHour
			if v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
				return v
			}
			return 0
		}
	}
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if !strings.HasPrefix(name, "gemini-") {
		return 0
	}
	parts := strings.Split(name, "-")
	has := func(w string) bool {
		for _, p := range parts {
			if p == w {
				return true
			}
		}
		return false
	}
	if has("pro") {
		return 4.5e-6
	}
	if has("flash") {
		// The upstream #8006 promotional Flash rate expires on 2027-01-01.
		if !has("lite") && !has("cyber") && time.Now().Before(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
			for _, prefix := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
				if name == prefix || strings.HasPrefix(name, prefix+"-") {
					return 0.5e-6
				}
			}
		}
		return 1e-6
	}
	return 0
}

// QuoteGeminiCachedContent uses lite's actual key-quota billing model. It does
// not reintroduce removed subscriptions or user cash balances. Renewal passes
// zero input tokens: storage-only extensions never incur another request fee.
func (s *GatewayService) QuoteGeminiCachedContent(ctx context.Context, input *RecordUsageInput, tokenHours float64) (*GeminiCachedContentSettlement, error) {
	if s == nil || s.billingService == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil || input.Account == nil || input.APIKey.Group == nil || input.APIKey.GroupID == nil {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	r, key, user, account := input.Result, input.APIKey, input.User, input.Account
	if (s.cfg == nil || s.cfg.RunMode != config.RunModeSimple) && input.APIKeyService == nil {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	if key.UserID != user.ID || key.Group.ID != *key.GroupID || account.Platform != PlatformGemini {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	if tokenHours < 0 || math.IsNaN(tokenHours) || math.IsInf(tokenHours, 0) || r.Usage.InputTokens < 0 || !strings.HasPrefix(r.RequestID, GeminiCachedContentUsageRequestIDPrefix) || len(r.RequestID) > 64 {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	model := forwardResultBillingModel(r.Model, r.UpstreamModel)
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		model = input.OriginalModel
	}
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" {
		model = input.ChannelMappedModel
	}
	model = s.billableModelWithFallback(ctx, key, model, r.UpstreamModel, r.Model)
	storagePrice := s.billingService.CacheStoragePricePerTokenHour(model)
	if storagePrice <= 0 {
		storagePrice = s.billingService.CacheStoragePricePerTokenHour(r.UpstreamModel)
	}
	if storagePrice <= 0 {
		return nil, ErrModelPricingUnavailable
	}
	if r.Usage.InputTokens > 0 && !s.hasResolvableTokenPricing(ctx, model, key) {
		return nil, ErrModelPricingUnavailable
	}
	rate := s.ResolveUserGroupRateMultiplier(ctx, user.ID, *key.GroupID, key.Group.RateMultiplier)
	at := input.PricingAt
	if at.IsZero() {
		at = time.Now()
	}
	rate, imageRate := computePeakAwareMultipliers(key, rate, at)
	if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	cost := &CostBreakdown{}
	if r.Usage.InputTokens > 0 {
		cost = s.calculateRecordUsageCost(ctx, r, key, model, rate, imageRate, at, &recordUsageOpts{})
	}
	if cost == nil {
		return nil, ErrModelPricingUnavailable
	}
	inputTotal := cost.TotalCost
	storageCost := tokenHours * storagePrice
	cost.TotalCost += storageCost
	cost.ActualCost += storageCost * rate
	if cost.TotalCost < 0 || math.IsNaN(cost.TotalCost) || math.IsInf(cost.TotalCost, 0) || cost.ActualCost < 0 || math.IsNaN(cost.ActualCost) || math.IsInf(cost.ActualCost, 0) {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	requestedModel := input.OriginalModel
	if requestedModel == "" {
		requestedModel = r.Model
	}
	core := &recordUsageCoreInput{InboundEndpoint: input.InboundEndpoint, UpstreamEndpoint: input.UpstreamEndpoint, UserAgent: input.UserAgent, IPAddress: input.IPAddress, ChannelUsageFields: input.ChannelUsageFields}
	accountRate := account.BillingRateMultiplier()
	log := s.buildRecordUsageLog(ctx, core, r, key, user, account, requestedModel, rate, imageRate, accountRate, BillingTypeBalance, false, cost, &recordUsageOpts{})
	// The cache money-event id is independent of any client request-id header.
	log.RequestID = r.RequestID
	if r.Usage.InputTokens > 0 {
		applyAccountStatsCost(ctx, log, s.channelService, s.billingService, account.ID, *key.GroupID, r.UpstreamModel, r.Model, UsageTokens{InputTokens: r.Usage.InputTokens}, inputTotal, at)
		if log.AccountStatsCost != nil {
			*log.AccountStatsCost += storageCost
		}
	}
	params := &postUsageBillingParams{Cost: cost, User: user, APIKey: key, Account: account, RequestPayloadHash: input.RequestPayloadHash, AccountRateMultiplier: accountRate, APIKeyService: input.APIKeyService, Platform: PlatformGemini, SimpleModeKeyRateLimitOnly: simpleModeKeyRateLimitBillingEnabled(s.cfg, key)}
	cmd := buildUsageBillingCommand(r.RequestID, log, params)
	if cmd == nil {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	simple := s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
	if simple && !params.SimpleModeKeyRateLimitOnly {
		cmd.APIKeyQuotaCost = 0
		cmd.APIKeyRateLimitCost = 0
		cmd.AccountQuotaCost = 0
	}
	return &GeminiCachedContentSettlement{Command: cmd, Log: log, EnforceLimits: !simple || params.SimpleModeKeyRateLimitOnly, params: params}, nil
}

// FinalizeGeminiCachedContentSettlement updates derived caches only after the
// SQL resource/billing/log transaction commits. It never performs a second debit.
func (s *GatewayService) FinalizeGeminiCachedContentSettlement(ctx context.Context, quote *GeminiCachedContentSettlement, result *UsageBillingApplyResult) {
	if quote == nil || quote.params == nil || result == nil || !result.Applied {
		return
	}
	p := quote.params
	if p.APIKeyService != nil && quote.Command.APIKeyQuotaCost > 0 {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok {
			invalidator.InvalidateAuthCacheByKey(ctx, p.APIKey.Key)
		}
	}
	if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(p.Account.ID)
	}
	if s.billingCacheService != nil && quote.Command.APIKeyRateLimitCost > 0 {
		_ = s.billingCacheService.InvalidateAPIKeyRateLimit(ctx, p.APIKey.ID)
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return
	}
	// Preserve lite's user-platform quota accounting path and flusher behavior.
	if s.billingCacheService != nil && s.userPlatformQuotaRepo != nil && p.Cost.ActualCost > 0 && s.billingCacheService.HasUserPlatformQuotaLimit(ctx, p.User.ID, PlatformGemini) {
		s.billingCacheService.IncrementUserPlatformQuotaUsage(p.User.ID, PlatformGemini, p.Cost.ActualCost)
		if s.cfg == nil || !s.cfg.Database.UserPlatformQuotaFlusherEnabled {
			_ = s.userPlatformQuotaRepo.IncrementUsageWithReset(ctx, p.User.ID, PlatformGemini, p.Cost.ActualCost, time.Now().UTC())
		}
	}
}

func (s *GeminiCachedContentService) SaveWithSettlement(ctx context.Context, record *GeminiCachedContent, quote *GeminiCachedContentSettlement) (*UsageBillingApplyResult, error) {
	r, ok := s.repo.(GeminiCachedContentBillingRepository)
	if !ok {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	return r.CreateWithSettlement(ctx, record, quote)
}
func (s *GeminiCachedContentService) CommitMutationWithSettlement(ctx context.Context, m *GeminiCachedContentMutation, expiry time.Time, quote *GeminiCachedContentSettlement) (*UsageBillingApplyResult, error) {
	r, ok := s.repo.(GeminiCachedContentBillingRepository)
	if !ok {
		return nil, ErrGeminiCacheBillingUnavailable
	}
	result, err := r.CommitMutationWithSettlement(ctx, m.record, m.token, expiry, quote)
	if err == nil {
		m.record.ExpireTime = expiry
		m.record.UpdatedAt = s.now()
	}
	return result, err
}
func (s *GeminiCachedContentService) DeleteWithMutation(ctx context.Context, m *GeminiCachedContentMutation) error {
	r, ok := s.repo.(GeminiCachedContentBillingRepository)
	if !ok {
		return ErrGeminiCacheBillingUnavailable
	}
	return r.DeleteWithMutation(ctx, m.record, m.token)
}

// RollbackMutation retains its durable intent if upstream rollback fails, so
// the existing recovery worker retries it instead of exposing unpaid storage.
func (s *GeminiCachedContentService) RollbackMutation(ctx context.Context, m *GeminiCachedContentMutation) error {
	forget, err := s.rollbackUpstream(ctx, m.record)
	if err != nil {
		return err
	}
	if forget {
		return s.Forget(ctx, m.record)
	}
	return s.ReleaseMutation(ctx, m)
}

// CleanupOwner distinguishes a deleted owner (nil) from a temporarily disabled
// cache capability (error). Deletion must not silently orphan a live upstream.
func (s *GeminiCachedContentService) CleanupOwner(ctx context.Context, record *GeminiCachedContent) (*Account, error) {
	return s.boundAccount(ctx, record)
}

func ValidateGeminiCachedContentSettlement(record *GeminiCachedContent, q *GeminiCachedContentSettlement) error {
	if record == nil || q == nil || q.Command == nil || q.Log == nil {
		return ErrGeminiCacheBillingUnavailable
	}
	c, l := q.Command, q.Log
	if c.APIKeyID != record.APIKeyID || c.UserID != record.UserID || c.AccountID != record.AccountID || l.APIKeyID != record.APIKeyID || l.UserID != record.UserID || l.AccountID != record.AccountID || l.GroupID == nil || *l.GroupID != record.GroupID || l.RequestID != c.RequestID || !strings.HasPrefix(c.RequestID, GeminiCachedContentUsageRequestIDPrefix) {
		return errors.New("cache settlement ownership mismatch")
	}
	for _, v := range []float64{c.APIKeyQuotaCost, c.APIKeyRateLimitCost, c.AccountQuotaCost, l.TotalCost, l.ActualCost} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrGeminiCacheBillingUnavailable
		}
	}
	return nil
}
