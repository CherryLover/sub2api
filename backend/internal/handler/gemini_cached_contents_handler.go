package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *GatewayHandler) geminiCachePreamble(c *gin.Context) (*service.APIKey, middleware.AuthSubject, *zap.Logger, bool) {
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key == nil {
		googleError(c, 401, "Invalid API key")
		return nil, middleware.AuthSubject{}, nil, false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || key.User == nil || subject.UserID != key.User.ID {
		googleError(c, 401, "Invalid user context")
		return nil, middleware.AuthSubject{}, nil, false
	}
	if key.GroupID == nil || key.Group == nil || key.Group.Platform != service.PlatformGemini || middleware.HasForcePlatform(c) {
		googleError(c, 400, "Explicit caches require a Gemini group")
		return nil, subject, nil, false
	}
	if h.geminiCachedContentService == nil {
		googleError(c, 503, "Explicit cache service is unavailable")
		return nil, subject, nil, false
	}
	log := requestLogger(c, "handler.gemini_cached_contents", zap.Int64("api_key_id", key.ID), zap.Int64("user_id", subject.UserID))
	return key, subject, log, true
}

func readGeminiCacheBody(c *gin.Context) ([]byte, bool) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleError(c, 413, buildBodyTooLargeMessage(maxErr.Limit))
		} else {
			googleError(c, 400, "Failed to read request body")
		}
		return nil, false
	}
	if len(body) == 0 {
		googleError(c, 400, "Request body is empty")
		return nil, false
	}
	return body, true
}
func (h *GatewayHandler) geminiCacheCheckBilling(c *gin.Context, key *service.APIKey) bool {
	if h.billingCacheService == nil {
		googleError(c, 503, "Billing service is unavailable")
		return false
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), key.User, key, key.Group, service.PlatformGemini); err != nil {
		writeGeminiCacheError(c, err)
		return false
	}
	return true
}
func geminiCachePersistContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), time.Minute)
}

func (h *GatewayHandler) acquireGeminiCacheAccount(c *gin.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, errors.New("no available cache account")
	}
	if selection.Acquired {
		if selection.ReleaseFunc == nil {
			return func() {}, nil
		}
		return selection.ReleaseFunc, nil
	}
	if selection.WaitPlan == nil || h.concurrencyHelper == nil {
		return nil, errors.New("no available cache concurrency slot")
	}
	helper := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)
	canWait, err := helper.IncrementAccountWaitCount(c.Request.Context(), selection.Account.ID, selection.WaitPlan.MaxWaiting)
	if err != nil {
		return nil, err
	}
	if !canWait {
		return nil, errors.New("cache account wait queue is full")
	}
	defer helper.DecrementAccountWaitCount(c.Request.Context(), selection.Account.ID)
	started := false
	return helper.AcquireAccountSlotWithWaitTimeout(c, selection.Account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, false, &started)
}

// GeminiCachedContentsCreate makes the upstream resource private until one SQL
// transaction has charged the key and persisted its binding and complete log.
func (h *GatewayHandler) GeminiCachedContentsCreate(c *gin.Context) {
	key, subject, log, ok := h.geminiCachePreamble(c)
	if !ok {
		return
	}
	body, ok := readGeminiCacheBody(c)
	if !ok {
		return
	}
	svc := h.geminiCachedContentService
	req, err := service.ParseGeminiCachedContentCreateRequest(body, svc.Now(), svc.MaxTTL())
	if err != nil {
		googleError(c, 400, err.Error())
		return
	}
	if h.gatewayService == nil || h.concurrencyHelper == nil {
		googleError(c, 503, "Gateway is unavailable")
		return
	}
	if decision := h.checkSecurityAudit(c, log, key, subject, service.SecurityAuditProtocolGemini, req.Model, body); decision != nil && !decision.AllowNextStage {
		googleSecurityAuditError(c, decision)
		return
	}
	mapping, restricted := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), key.GroupID, req.Model)
	if restricted {
		googleError(c, 400, "Model is not available for this group")
		return
	}
	model := req.Model
	if mapping.Mapped {
		model = mapping.MappedModel
	}
	helper := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)
	started := false
	releaseUser, err := helper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &started)
	if err != nil {
		googleError(c, 429, "Too many concurrent requests")
		return
	}
	if releaseUser != nil {
		defer releaseUser()
	}
	if !h.geminiCacheCheckBilling(c, key) {
		return
	}
	excluded, err := h.gatewayService.GeminiExplicitCacheExclusions(c.Request.Context(), key.GroupID, 0)
	if err != nil {
		googleError(c, 503, "No available Gemini cache accounts")
		return
	}
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		selection, selectErr := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), key.GroupID, "", model, excluded, "", int64(0))
		if selectErr != nil {
			break
		}
		account := selection.Account
		release, slotErr := h.acquireGeminiCacheAccount(c, selection)
		if slotErr != nil {
			googleError(c, 429, "No cache concurrency slot available")
			return
		}
		latest, vetoed, _ := h.gatewayService.GatewayProfitControlVetoLatest(service.ContextWithSelectionProfitGate(c.Request.Context(), selection), account)
		if vetoed {
			release()
			excluded[account.ID] = struct{}{}
			continue
		}
		account = latest
		if !service.IsGeminiExplicitCacheAccount(account) {
			release()
			excluded[account.ID] = struct{}{}
			continue
		}
		ttl, allowed := req.TTLForAccount(account)
		if !allowed {
			release()
			excluded[account.ID] = struct{}{}
			continue
		}
		mapped := account.GetMappedModel(model)
		upstreamModel, modelErr := service.GeminiCachedContentUpstreamModelResource(account, mapped)
		if modelErr != nil {
			release()
			excluded[account.ID] = struct{}{}
			continue
		}
		publicID, idErr := service.NewGeminiCachedContentPublicID()
		if idErr != nil {
			release()
			googleError(c, 500, "Failed to allocate cache identifier")
			return
		}
		fields := clientRequestedUsageFields(c, mapping, req.Model, mapped)
		input := &service.RecordUsageInput{Result: &service.ForwardResult{RequestID: service.GeminiCachedContentCreateUsageRequestID(publicID), Model: model, UpstreamModel: mapped}, APIKey: key, User: key.User, Account: account, APIKeyService: h.apiKeyService, PricingAt: pricingAt, ChannelUsageFields: fields, InboundEndpoint: "/v1beta/cachedContents", UpstreamEndpoint: "/v1beta/cachedContents", RequestPayloadHash: service.HashUsageRequestPayload(body)}
		// Preflight both input and storage tariffs before creating anything
		// upstream. This one-token quote is never persisted or charged; the
		// actual upstream token count replaces it below.
		input.Result.Usage.InputTokens = 1
		if _, quoteErr := h.gatewayService.QuoteGeminiCachedContent(c.Request.Context(), input, 1); quoteErr != nil {
			release()
			writeGeminiCacheError(c, quoteErr)
			return
		}
		setOpsSelectedAccount(c, account.ID, account.Platform)
		upstream, createErr := svc.CreateUpstream(c.Request.Context(), account, req, mapped, publicID, ttl)
		if createErr != nil {
			release()
			lastErr = createErr
			var upErr *service.GeminiCachedContentUpstreamError
			// Only a definite rejected request may move to another account.
			// A transport failure can have created a resource: do not duplicate it.
			if errors.As(createErr, &upErr) && upErr.RetryableOnOtherAccount() {
				excluded[account.ID] = struct{}{}
				continue
			}
			var malformed *service.GeminiCachedContentMalformedResponseError
			if errors.As(createErr, &malformed) && malformed.Name != "" {
				h.cleanupGeminiCache(c, log, account, malformed.Name)
			}
			writeGeminiCacheError(c, createErr)
			return
		}
		if upstream.TotalTokenCount <= 0 || upstream.TotalTokenCount > int64(math.MaxInt) {
			release()
			h.cleanupGeminiCache(c, log, account, upstream.Name)
			googleError(c, 502, "Invalid cache token count")
			return
		}
		now := svc.Now()
		expiry := service.ClampGeminiCachedContentExpire(upstream.ExpireTime, now.Add(ttl)).UTC().Truncate(time.Microsecond)
		if !expiry.After(now) {
			release()
			h.cleanupGeminiCache(c, log, account, upstream.Name)
			googleError(c, 502, "Upstream returned an expired cache")
			return
		}
		record := &service.GeminiCachedContent{PublicID: publicID, UserID: subject.UserID, APIKeyID: key.ID, GroupID: *key.GroupID, AccountID: account.ID, UpstreamName: upstream.Name, Model: model, UpstreamModel: upstreamModel, DisplayName: req.DisplayName, TotalTokenCount: upstream.TotalTokenCount, ExpireTime: expiry, ChannelUsage: fields}
		input.Result.Usage.InputTokens = int(upstream.TotalTokenCount)
		quote, quoteErr := h.gatewayService.QuoteGeminiCachedContent(c.Request.Context(), input, service.GeminiCachedContentStorageTokenHours(upstream.TotalTokenCount, min(ttl, expiry.Sub(now))))
		if quoteErr != nil {
			release()
			h.cleanupGeminiCache(c, log, account, upstream.Name)
			writeGeminiCacheError(c, quoteErr)
			return
		}
		ctx, cancel := geminiCachePersistContext(c)
		result, saveErr := svc.SaveWithSettlement(ctx, record, quote)
		if saveErr == nil {
			h.gatewayService.FinalizeGeminiCachedContentSettlement(ctx, quote, result)
		}
		cancel()
		release()
		if saveErr != nil {
			// A commit response can be lost after PostgreSQL commits. Reconcile
			// on the primary before deleting a possibly paid upstream resource.
			checkCtx, checkCancel := geminiCachePersistContext(c)
			existing, checkErr := svc.Resolve(checkCtx, key.ID, key.GroupID, record.PublicName())
			checkCancel()
			if checkErr == nil && existing.UpstreamName == record.UpstreamName {
				c.JSON(200, service.GeminiCachedContentView(existing))
				return
			}
			if errors.Is(checkErr, service.ErrGeminiCachedContentNotFound) {
				h.cleanupGeminiCache(c, log, account, upstream.Name)
			}
			log.Error("gemini.cache.settlement_failed", zap.Error(saveErr))
			writeGeminiCacheError(c, saveErr)
			return
		}
		c.JSON(200, service.GeminiCachedContentView(record))
		return
	}
	if lastErr != nil {
		writeGeminiCacheError(c, lastErr)
		return
	}
	googleError(c, 503, "No available Gemini accounts support this cache and TTL")
}

func (h *GatewayHandler) resolveGeminiCache(c *gin.Context, key *service.APIKey, name string) (*service.GeminiCachedContent, bool) {
	record, err := h.geminiCachedContentService.Resolve(c.Request.Context(), key.ID, key.GroupID, name)
	if err != nil {
		writeGeminiCacheError(c, err)
		return nil, false
	}
	if record.UserID != key.User.ID {
		writeGeminiCacheError(c, service.ErrGeminiCachedContentNotFound)
		return nil, false
	}
	return record, true
}
func (h *GatewayHandler) GeminiCachedContentsList(c *gin.Context) {
	key, _, _, ok := h.geminiCachePreamble(c)
	if !ok {
		return
	}
	size := service.GeminiCachedContentListDefaultPageSize
	if raw := c.Query("pageSize"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			googleError(c, 400, "Invalid pageSize")
			return
		}
		size = min(n, service.GeminiCachedContentListMaxPageSize)
	}
	before := int64(0)
	if raw := c.Query("pageToken"); raw != "" {
		if len(raw) > 32 {
			googleError(c, 400, "Invalid pageToken")
			return
		}
		b, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(b) > 24 {
			googleError(c, 400, "Invalid pageToken")
			return
		}
		before, err = strconv.ParseInt(string(b), 10, 64)
		if err != nil || before <= 0 {
			googleError(c, 400, "Invalid pageToken")
			return
		}
	}
	records, err := h.geminiCachedContentService.List(c.Request.Context(), key.ID, *key.GroupID, before, size+1)
	if err != nil {
		writeGeminiCacheError(c, err)
		return
	}
	views := make([]map[string]any, 0, min(size, len(records)))
	for _, record := range records[:min(size, len(records))] {
		if record.UserID != key.User.ID {
			writeGeminiCacheError(c, service.ErrGeminiCachedContentNotFound)
			return
		}
		views = append(views, service.GeminiCachedContentView(record))
	}
	response := gin.H{"cachedContents": views}
	if len(records) > size {
		response["nextPageToken"] = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(records[size-1].ID, 10)))
	}
	c.JSON(200, response)
}
func (h *GatewayHandler) GeminiCachedContentsGet(c *gin.Context) {
	key, _, _, ok := h.geminiCachePreamble(c)
	if !ok {
		return
	}
	record, ok := h.resolveGeminiCache(c, key, c.Param("cacheID"))
	if !ok {
		return
	}
	c.JSON(200, service.GeminiCachedContentView(record))
}
func (h *GatewayHandler) GeminiCachedContentsPatch(c *gin.Context) {
	key, subject, log, ok := h.geminiCachePreamble(c)
	if !ok {
		return
	}
	body, ok := readGeminiCacheBody(c)
	if !ok {
		return
	}
	record, ok := h.resolveGeminiCache(c, key, c.Param("cacheID"))
	if !ok {
		return
	}
	svc := h.geminiCachedContentService
	account, err := svc.BoundAccount(c.Request.Context(), record)
	if err != nil {
		writeGeminiCacheError(c, err)
		return
	}
	if account == nil {
		writeGeminiCacheError(c, service.ErrGeminiCachedContentNotFound)
		return
	}
	ttl, err := service.ParseGeminiCachedContentPatchRequest(body, c.Query("updateMask"), svc.Now(), svc.MaxTTLForAccount(account))
	if err != nil {
		googleError(c, 400, err.Error())
		return
	}
	if h.gatewayService == nil || h.concurrencyHelper == nil {
		googleError(c, 503, "Gateway is unavailable")
		return
	}
	helper := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)
	started := false
	releaseUser, err := helper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &started)
	if err != nil {
		googleError(c, 429, "Too many concurrent requests")
		return
	}
	if releaseUser != nil {
		defer releaseUser()
	}
	if !h.geminiCacheCheckBilling(c, key) {
		return
	}
	excluded, err := h.gatewayService.GeminiExplicitCacheExclusions(c.Request.Context(), key.GroupID, record.AccountID)
	if err != nil {
		googleError(c, 503, "Cached content account is unavailable")
		return
	}
	selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), key.GroupID, "", record.Model, excluded, "", int64(0))
	if err != nil {
		googleError(c, 503, "Cached content account is unavailable")
		return
	}
	releaseAccount, err := h.acquireGeminiCacheAccount(c, selection)
	if err != nil {
		googleError(c, 429, "Cached content concurrency slot is unavailable")
		return
	}
	defer releaseAccount()
	latest, vetoed, _ := h.gatewayService.GatewayProfitControlVetoLatest(service.ContextWithSelectionProfitGate(c.Request.Context(), selection), selection.Account)
	if vetoed || latest.ID != record.AccountID || !service.IsGeminiExplicitCacheAccount(latest) || latest.GetMappedModel(record.Model) != record.UpstreamModelID() {
		googleError(c, 503, "Cached content account or model configuration has changed")
		return
	}
	account = latest
	ctx, cancel := geminiCachePersistContext(c)
	defer cancel()
	mutation, err := svc.BeginMutation(ctx, record)
	if err != nil {
		writeGeminiCacheError(c, err)
		return
	}
	newExpiry := svc.Now().Add(ttl).UTC().Truncate(time.Microsecond)
	additional := max(time.Duration(0), newExpiry.Sub(mutation.Baseline()))
	pricingAt := svc.Now()
	input := &service.RecordUsageInput{Result: &service.ForwardResult{RequestID: service.GeminiCachedContentPatchUsageRequestID(record.PublicID), Model: record.Model, UpstreamModel: record.UpstreamModelID()}, APIKey: key, User: key.User, Account: account, APIKeyService: h.apiKeyService, PricingAt: pricingAt, ChannelUsageFields: record.ChannelUsage, InboundEndpoint: "/v1beta/cachedContents", UpstreamEndpoint: "/v1beta/cachedContents", RequestPayloadHash: service.HashUsageRequestPayload(body)}
	_, err = h.gatewayService.QuoteGeminiCachedContent(ctx, input, service.GeminiCachedContentStorageTokenHours(record.TotalTokenCount, additional))
	if err != nil {
		_ = svc.ReleaseMutation(ctx, mutation)
		writeGeminiCacheError(c, err)
		return
	}
	upstream, err := svc.PatchUpstream(ctx, account, record, ttl)
	if err != nil {
		var upErr *service.GeminiCachedContentUpstreamError
		if errors.Is(err, service.ErrGeminiCachedContentRequestNotSent) {
			_ = svc.ReleaseMutation(ctx, mutation)
		} else if errors.As(err, &upErr) && upErr.NotFound() {
			_ = svc.Forget(ctx, record)
		}
		// Unknown outcomes retain the lease for recovery, never silently make
		// an uncharged extension available to the client.
		writeGeminiCacheError(c, err)
		return
	}
	newExpiry = service.ClampGeminiCachedContentExpire(upstream.ExpireTime, newExpiry).UTC().Truncate(time.Microsecond)
	if !newExpiry.After(svc.Now()) {
		_ = svc.RollbackMutation(ctx, mutation)
		googleError(c, 502, "Upstream returned an expired cache")
		return
	}
	additional = max(time.Duration(0), newExpiry.Sub(mutation.Baseline()))
	quote, err := h.gatewayService.QuoteGeminiCachedContent(ctx, input, service.GeminiCachedContentStorageTokenHours(record.TotalTokenCount, additional))
	if err != nil {
		_ = svc.RollbackMutation(ctx, mutation)
		writeGeminiCacheError(c, err)
		return
	}
	result, err := svc.CommitMutationWithSettlement(ctx, mutation, newExpiry, quote)
	if err != nil {
		check, checkErr := svc.Resolve(ctx, key.ID, key.GroupID, record.PublicName())
		if checkErr == nil && check.ExpireTime.Equal(newExpiry) {
			c.JSON(200, service.GeminiCachedContentView(check))
			return
		}
		// Only known rejected transactions are rolled back immediately;
		// unresolved DB outcomes stay fenced until recovery can reconcile.
		if isGeminiCacheQuotaError(err) {
			if rollbackErr := svc.RollbackMutation(ctx, mutation); rollbackErr != nil {
				log.Error("gemini.cache.rollback_pending", zap.Error(rollbackErr))
			}
		}
		writeGeminiCacheError(c, err)
		return
	}
	h.gatewayService.FinalizeGeminiCachedContentSettlement(ctx, quote, result)
	c.JSON(200, service.GeminiCachedContentView(record))
}
func (h *GatewayHandler) GeminiCachedContentsDelete(c *gin.Context) {
	key, subject, _, ok := h.geminiCachePreamble(c)
	if !ok {
		return
	}
	record, ok := h.resolveGeminiCache(c, key, c.Param("cacheID"))
	if !ok {
		return
	}
	svc := h.geminiCachedContentService
	if h.concurrencyHelper == nil {
		googleError(c, 503, "Gateway is unavailable")
		return
	}
	helper := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)
	started := false
	releaseUser, err := helper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &started)
	if err != nil {
		googleError(c, 429, "Too many concurrent requests")
		return
	}
	if releaseUser != nil {
		defer releaseUser()
	}
	ctx, cancel := geminiCachePersistContext(c)
	defer cancel()
	mutation, err := svc.BeginMutation(ctx, record)
	if err != nil {
		writeGeminiCacheError(c, err)
		return
	}
	account, err := svc.CleanupOwner(ctx, record)
	if err != nil {
		_ = svc.ReleaseMutation(ctx, mutation)
		writeGeminiCacheError(c, err)
		return
	}
	if account != nil {
		// Cleanup must remain possible for a quota-limited or unschedulable
		// account, while still respecting its concurrent connection limit.
		releaseAccount, slotErr := helper.AcquireAccountSlotWithWaitTimeout(c, account.ID, account.Concurrency, 30*time.Second, false, &started)
		if slotErr != nil {
			_ = svc.ReleaseMutation(ctx, mutation)
			googleError(c, 429, "No cache cleanup concurrency slot available")
			return
		}
		if releaseAccount != nil {
			defer releaseAccount()
		}
		err = svc.DeleteUpstream(ctx, account, record)
		var upErr *service.GeminiCachedContentUpstreamError
		if err != nil && (!errors.As(err, &upErr) || !upErr.NotFound()) {
			writeGeminiCacheError(c, err)
			return
		}
	}
	if err = svc.DeleteWithMutation(ctx, mutation); err != nil {
		writeGeminiCacheError(c, err)
		return
	}
	c.JSON(200, gin.H{})
}

func (h *GatewayHandler) cleanupGeminiCache(c *gin.Context, log *zap.Logger, account *service.Account, name string) {
	ctx, cancel := geminiCachePersistContext(c)
	defer cancel()
	if err := h.geminiCachedContentService.DeleteUpstream(ctx, account, &service.GeminiCachedContent{UpstreamName: name}); err != nil {
		log.Error("gemini.cache.cleanup_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
}
func isGeminiCacheQuotaError(err error) bool {
	return errors.Is(err, service.ErrAPIKeyQuotaExhausted) || errors.Is(err, service.ErrAPIKeyRateLimit5hExceeded) || errors.Is(err, service.ErrAPIKeyRateLimit1dExceeded) || errors.Is(err, service.ErrAPIKeyRateLimit7dExceeded) || errors.Is(err, service.ErrAPIKeyExpired) || errors.Is(err, service.ErrAPIKeyNotFound)
}
func writeGeminiCacheError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrGeminiCachedContentNotFound) {
		c.Data(403, "application/json", []byte(service.GeminiCachedContentNotFoundResponse))
		return
	}
	if errors.Is(err, service.ErrGeminiCachedContentMutationConflict) {
		googleError(c, 409, "Another update to this cached content is in progress")
		return
	}
	if errors.Is(err, service.ErrModelPricingUnavailable) {
		googleError(c, 400, "Explicit cache storage pricing is not configured for this model")
		return
	}
	if isGeminiCacheQuotaError(err) {
		status, _, message, retry := billingErrorDetails(err)
		if retry > 0 {
			c.Header("Retry-After", strconv.Itoa(retry))
		}
		googleError(c, status, message)
		return
	}
	var upErr *service.GeminiCachedContentUpstreamError
	if errors.As(err, &upErr) {
		if upErr.NotFound() {
			c.Data(403, "application/json", []byte(service.GeminiCachedContentNotFoundResponse))
			return
		}
		status := upErr.StatusCode
		if status < 400 || status > 599 {
			status = 502
		}
		googleError(c, status, fmt.Sprintf("Upstream cached content request failed (HTTP %d)", status))
		return
	}
	googleError(c, 503, "Cached content operation could not be completed; retry later")
}

// resolveGeminiCachedContentBinding maps only this key's public cache identifier
// to the upstream resource and returns the owner account to hard-pin scheduling.
func (h *GatewayHandler) resolveGeminiCachedContentBinding(c *gin.Context, key *service.APIKey, model string, body []byte) (*service.GeminiCachedContent, []byte, bool) {
	ref, err := service.FindGeminiCachedContentReference(body)
	if err != nil {
		googleError(c, 400, err.Error())
		return nil, nil, false
	}
	if ref == nil {
		return nil, body, true
	}
	if h.geminiCachedContentService == nil {
		googleError(c, 503, "Explicit cache service is unavailable")
		return nil, nil, false
	}
	if middleware.HasForcePlatform(c) || key.Group == nil || key.Group.Platform != service.PlatformGemini {
		googleError(c, 400, "Explicit caches require a Gemini group")
		return nil, nil, false
	}
	record, ok := h.resolveGeminiCache(c, key, ref.Name)
	if !ok {
		return nil, nil, false
	}
	if strings.TrimPrefix(model, "models/") != record.Model {
		googleError(c, 400, "Model must match the cached content model")
		return nil, nil, false
	}
	body, err = service.RewriteGeminiCachedContentReference(body, ref, record.UpstreamName)
	if err != nil {
		googleError(c, 400, "Invalid cachedContent reference")
		return nil, nil, false
	}
	c.Request = c.Request.WithContext(service.WithGeminiCachedContentBound(c.Request.Context()))
	return record, body, true
}
