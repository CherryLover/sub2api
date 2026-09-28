package handler

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	userAccountRecentDefaultMinutes = 15
	userAccountRecentMaxMinutes     = 1440
	userAccountRecentDefaultLimit   = 50
	userAccountRecentMaxLimit       = 200
)

type userAccountRecentRequestUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// userAccountRecentRequestItem intentionally exposes only enough request context
// to let a user see that an upstream account is being used by multiple users.
// Personal email addresses, API key names, costs, tokens and request IDs are omitted.
type userAccountRecentRequestItem struct {
	ID           int64                         `json:"id"`
	CreatedAt    time.Time                     `json:"created_at"`
	User         *userAccountRecentRequestUser `json:"user"`
	Model        string                        `json:"model"`
	RequestType  string                        `json:"request_type"`
	Stream       bool                          `json:"stream"`
	DurationMs   *int                          `json:"duration_ms"`
	FirstTokenMs *int                          `json:"first_token_ms"`
}

type userAccountRecentRequestsResponse struct {
	AccountID          int64                                `json:"account_id"`
	WindowMinutes      int                                  `json:"window_minutes"`
	CurrentConcurrency int                                  `json:"current_concurrency"`
	MaxConcurrency     int                                  `json:"max_concurrency"`
	WaitingCount       int                                  `json:"waiting_count"`
	TotalRequests      int64                                `json:"total_requests"`
	Items              []userAccountRecentRequestItem       `json:"items"`
	ByAPIKey           []usagestats.AccountRecentAPIKeyStat `json:"by_api_key"`
	ByModel            []usagestats.AccountRecentModelStat  `json:"by_model"`
}

// GetAccountRecentRequests returns a read-only, privacy-reduced view of recent
// activity for an upstream account the current user is actually allowed to use.
func (h *AvailableChannelHandler) GetAccountRecentRequests(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if !h.featureEnabled(c) || h.accountRepo == nil {
		response.NotFound(c, "Account not found")
		return
	}

	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	minutes, ok := parseUserAccountRecentInt(c, "minutes", userAccountRecentDefaultMinutes, 1, userAccountRecentMaxMinutes)
	if !ok {
		return
	}
	limit, ok := parseUserAccountRecentInt(c, "limit", userAccountRecentDefaultLimit, 1, userAccountRecentMaxLimit)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	account, err := h.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account == nil {
		response.NotFound(c, "Account not found")
		return
	}
	groups, err := h.apiKeyService.GetAvailableGroups(ctx, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	allowed := make(map[int64]struct{}, len(groups))
	for i := range groups {
		allowed[groups[i].ID] = struct{}{}
	}
	// Account visibility must match the actual available-channels page: the group
	// must both belong to the user and be attached to an active available channel.
	channels, err := h.channelService.ListAvailable(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	visibleGroupIDs := make(map[int64]struct{})
	for _, channel := range channels {
		if channel.Status != service.StatusActive {
			continue
		}
		for _, group := range channel.Groups {
			if _, ok := allowed[group.ID]; ok {
				visibleGroupIDs[group.ID] = struct{}{}
			}
		}
	}
	if !accountIntersectsGroups(account, visibleGroupIDs) {
		// Deliberately use 404 so account IDs cannot be enumerated across users.
		response.NotFound(c, "Account not found")
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "account usage service unavailable")
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(minutes) * time.Minute)
	recent, err := h.accountUsageService.GetAccountRecentRequests(ctx, account.ID, startTime, endTime, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	byModel := make([]usagestats.AccountRecentModelStat, 0, len(recent.ByModel))
	for _, item := range recent.ByModel {
		byModel = append(byModel, usagestats.AccountRecentModelStat{
			Model: item.Model,
			Count: item.Count,
		})
	}
	out := userAccountRecentRequestsResponse{
		AccountID:      account.ID,
		WindowMinutes:  minutes,
		MaxConcurrency: account.Concurrency,
		TotalRequests:  recent.TotalRequests,
		Items:          make([]userAccountRecentRequestItem, 0, len(recent.Items)),
		ByAPIKey:       []usagestats.AccountRecentAPIKeyStat{},
		ByModel:        byModel,
	}
	for i := range recent.Items {
		log := &recent.Items[i]
		requestType := log.EffectiveRequestType()
		stream, _ := service.ApplyLegacyRequestFields(requestType, log.Stream, log.OpenAIWSMode)
		model := log.RequestedModel
		if model == "" {
			model = log.Model
		}
		item := userAccountRecentRequestItem{}
		item.ID = log.ID
		item.CreatedAt = log.CreatedAt.UTC()
		item.User = &userAccountRecentRequestUser{
			ID:    log.UserID,
			Email: fmt.Sprintf("用户 #%d", log.UserID),
		}
		item.Model = model
		item.RequestType = requestType.String()
		item.Stream = stream
		item.DurationMs = log.DurationMs
		item.FirstTokenMs = log.FirstTokenMs
		out.Items = append(out.Items, item)
	}
	response.Success(c, out)
}

func parseUserAccountRecentInt(c *gin.Context, name string, def, minValue, maxValue int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return def, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		response.BadRequest(c, fmt.Sprintf("%s must be between %d and %d", name, minValue, maxValue))
		return 0, false
	}
	return value, true
}
