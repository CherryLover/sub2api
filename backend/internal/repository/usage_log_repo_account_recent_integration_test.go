//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// TestGetAccountRecentRequestBreakdown 校验账号最近窗口聚合：窗口过滤、账号隔离、
// 按 Key / 按模型分组、账号口径费用、排序与空窗口。
func (s *UsageLogRepoSuite) TestGetAccountRecentRequestBreakdown() {
	owner := mustCreateUser(s.T(), s.client, &service.User{Email: "recent-owner@test.com"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-recent"})
	otherAccount := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-recent-other"})

	keyA := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: owner.ID, Key: "sk-recent-a", Name: "key-a"})
	keyB := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: owner.ID, Key: "sk-recent-b", Name: "key-b"})

	now := time.Now().UTC()
	windowStart := now.Add(-15 * time.Minute)
	windowEnd := now.Add(time.Minute)

	multiplier := 2.0
	create := func(key *service.APIKey, acc *service.Account, model, requestedModel string, cost float64, accountMultiplier *float64, createdAt time.Time) {
		log := &service.UsageLog{
			UserID:                owner.ID,
			APIKeyID:              key.ID,
			AccountID:             acc.ID,
			RequestID:             uuid.New().String(),
			Model:                 model,
			RequestedModel:        requestedModel,
			InputTokens:           10,
			OutputTokens:          20,
			TotalCost:             cost,
			ActualCost:            cost,
			AccountRateMultiplier: accountMultiplier,
			CreatedAt:             createdAt,
		}
		_, err := s.repo.Create(s.ctx, log)
		s.Require().NoError(err)
	}

	// keyA：2 次，客户端请求 gpt-5-alias 映射到 gpt-5，账号倍率 2 → 账号口径费用 1.0×2 ×2 次 = 4.0
	create(keyA, account, "gpt-5", "gpt-5-alias", 1.0, &multiplier, now.Add(-time.Minute))
	create(keyA, account, "gpt-5", "gpt-5-alias", 1.0, &multiplier, now.Add(-2*time.Minute))
	// keyB：1 次，无映射、无倍率 → 0.5
	create(keyB, account, "claude-3", "", 0.5, nil, now.Add(-3*time.Minute))
	// 窗口之外（30 分钟前）不统计
	create(keyA, account, "gpt-5", "gpt-5-alias", 99, nil, now.Add(-30*time.Minute))
	// 其它账号同窗口不统计
	create(keyA, otherAccount, "gpt-5", "gpt-5-alias", 99, nil, now.Add(-time.Minute))

	s.Run("整窗聚合按 Key 与模型分组", func() {
		result, err := s.repo.GetAccountRecentRequestBreakdown(s.ctx, account.ID, windowStart, windowEnd)
		s.Require().NoError(err)
		s.Require().Equal(int64(3), result.TotalRequests)

		s.Require().Len(result.ByAPIKey, 2)
		s.Require().Equal(keyA.ID, result.ByAPIKey[0].APIKeyID)
		s.Require().Equal("key-a", result.ByAPIKey[0].Name)
		s.Require().Equal(int64(2), result.ByAPIKey[0].Count)
		s.Require().InDelta(4.0, result.ByAPIKey[0].Cost, 1e-9)
		s.Require().Equal(keyB.ID, result.ByAPIKey[1].APIKeyID)
		s.Require().Equal("key-b", result.ByAPIKey[1].Name)
		s.Require().Equal(int64(1), result.ByAPIKey[1].Count)
		s.Require().InDelta(0.5, result.ByAPIKey[1].Cost, 1e-9)

		s.Require().Len(result.ByModel, 2)
		s.Require().Equal("gpt-5-alias", result.ByModel[0].Model, "按客户端请求的模型分组")
		s.Require().Equal(int64(2), result.ByModel[0].Count)
		s.Require().InDelta(4.0, result.ByModel[0].Cost, 1e-9)
		s.Require().Equal("claude-3", result.ByModel[1].Model, "requested_model 为空回退 model")
		s.Require().Equal(int64(1), result.ByModel[1].Count)
		s.Require().InDelta(0.5, result.ByModel[1].Cost, 1e-9)
	})

	s.Run("窗口终点为开区间", func() {
		result, err := s.repo.GetAccountRecentRequestBreakdown(s.ctx, account.ID, now.Add(-3*time.Minute), now.Add(-2*time.Minute))
		s.Require().NoError(err)
		// 只有 keyB 那条（-3m）落在 [-3m, -2m)；keyA 的 -2m 那条被开区间排除。
		s.Require().Equal(int64(1), result.TotalRequests)
		s.Require().Len(result.ByAPIKey, 1)
		s.Require().Equal(keyB.ID, result.ByAPIKey[0].APIKeyID)
	})

	s.Run("空窗口返回零值与空切片", func() {
		result, err := s.repo.GetAccountRecentRequestBreakdown(s.ctx, account.ID, now.Add(time.Hour), now.Add(2*time.Hour))
		s.Require().NoError(err)
		s.Require().Equal(int64(0), result.TotalRequests)
		s.Require().NotNil(result.ByAPIKey)
		s.Require().Empty(result.ByAPIKey)
		s.Require().NotNil(result.ByModel)
		s.Require().Empty(result.ByModel)
	})

	s.Run("不存在的账号返回空", func() {
		result, err := s.repo.GetAccountRecentRequestBreakdown(s.ctx, 999999, windowStart, windowEnd)
		s.Require().NoError(err)
		s.Require().Equal(int64(0), result.TotalRequests)
		s.Require().Empty(result.ByAPIKey)
		s.Require().Empty(result.ByModel)
	})

	s.Run("明细按 created_at 倒序且补齐 Key 名称", func() {
		// 与 AccountUsageService.GetAccountRecentRequests 使用完全相同的参数，保证服务层拿到的顺序正确。
		params := pagination.PaginationParams{Page: 1, PageSize: 2, SortBy: "created_at", SortOrder: pagination.SortOrderDesc}
		filters := usagestats.UsageLogFilters{AccountID: account.ID, StartTime: &windowStart, EndTime: &windowEnd}
		logs, page, err := s.repo.ListWithFilters(s.ctx, params, filters)
		s.Require().NoError(err)
		s.Require().Equal(int64(3), page.Total, "分页总数是整窗精确计数")
		s.Require().Len(logs, 2, "只取 limit 条")
		s.Require().True(logs[0].CreatedAt.After(logs[1].CreatedAt), "最新的在前")
		s.Require().Equal(keyA.ID, logs[0].APIKeyID)
		s.Require().NotNil(logs[0].APIKey)
		s.Require().Equal("key-a", logs[0].APIKey.Name)
		s.Require().NotNil(logs[0].User)
		s.Require().Equal("recent-owner@test.com", logs[0].User.Email)
	})
}
