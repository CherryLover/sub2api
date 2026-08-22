//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestGetAPIKeyUsageAggregates 校验 Key 排行榜聚合：窗口过滤、账户过滤、三种排序指标。
func (s *UsageLogRepoSuite) TestGetAPIKeyUsageAggregates() {
	owner := mustCreateUser(s.T(), s.client, &service.User{Email: "ranking-owner@test.com"})
	other := mustCreateUser(s.T(), s.client, &service.User{Email: "ranking-other@test.com"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-ranking"})

	keyA := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: owner.ID, Key: "sk-rank-a", Name: "key-a"})
	keyB := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: owner.ID, Key: "sk-rank-b", Name: "key-b"})
	keyC := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: other.ID, Key: "sk-rank-c", Name: "key-c"})

	now := time.Now().UTC()
	windowStart := now.Add(-24 * time.Hour)
	windowEnd := now.Add(time.Hour)

	// keyA: 1 次请求 / 300 tokens / 1.0 费用
	s.createUsageLog(owner, keyA, account, 100, 200, 1.0, now.Add(-time.Hour))
	// keyB: 2 次请求 / 40 tokens / 0.5 费用（请求数最多，费用与 token 最少）
	s.createUsageLog(owner, keyB, account, 10, 10, 0.25, now.Add(-2*time.Hour))
	s.createUsageLog(owner, keyB, account, 10, 10, 0.25, now.Add(-3*time.Hour))
	// keyC: 另一个账户，1 次请求 / 1000 tokens / 0.75 费用
	s.createUsageLog(other, keyC, account, 500, 500, 0.75, now.Add(-time.Hour))
	// 窗口之外的数据不应被统计
	s.createUsageLog(owner, keyA, account, 9999, 9999, 99, now.Add(-72*time.Hour))

	s.Run("全站按费用排序", func() {
		rows, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, windowStart, windowEnd, 0, usagestats.KeyRankingMetricCost)
		s.Require().NoError(err)
		s.Require().Len(rows, 3)
		s.Require().Equal(keyA.ID, rows[0].APIKeyID)
		s.Require().Equal(keyC.ID, rows[1].APIKeyID)
		s.Require().Equal(keyB.ID, rows[2].APIKeyID)
		s.Require().Equal(int64(1), rows[0].Requests)
		s.Require().Equal(int64(300), rows[0].Tokens)
		s.Require().InDelta(1.0, rows[0].Cost, 1e-9)
	})

	s.Run("全站按 token 排序", func() {
		rows, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, windowStart, windowEnd, 0, usagestats.KeyRankingMetricTokens)
		s.Require().NoError(err)
		s.Require().Len(rows, 3)
		s.Require().Equal(keyC.ID, rows[0].APIKeyID)
		s.Require().Equal(int64(1000), rows[0].Tokens)
	})

	s.Run("全站按请求数排序", func() {
		rows, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, windowStart, windowEnd, 0, usagestats.KeyRankingMetricRequests)
		s.Require().NoError(err)
		s.Require().Len(rows, 3)
		s.Require().Equal(keyB.ID, rows[0].APIKeyID)
		s.Require().Equal(int64(2), rows[0].Requests)
		s.Require().Equal(int64(40), rows[0].Tokens)
	})

	s.Run("账户维度只统计该用户的 Key", func() {
		rows, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, windowStart, windowEnd, owner.ID, usagestats.KeyRankingMetricCost)
		s.Require().NoError(err)
		s.Require().Len(rows, 2)
		for _, row := range rows {
			s.Require().NotEqual(keyC.ID, row.APIKeyID)
		}
	})

	s.Run("空窗口返回空结果", func() {
		rows, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, now.Add(time.Hour), now.Add(2*time.Hour), 0, usagestats.KeyRankingMetricCost)
		s.Require().NoError(err)
		s.Require().Empty(rows)
	})

	s.Run("非法 metric 被拒绝而不是拼进 SQL", func() {
		_, err := s.repo.GetAPIKeyUsageAggregates(s.ctx, windowStart, windowEnd, 0, "cost; DROP TABLE usage_logs")
		s.Require().Error(err)
	})

	s.Run("批量回查 Key 名称", func() {
		names, err := s.repo.GetAPIKeyNamesByIDs(s.ctx, []int64{keyA.ID, keyB.ID, 999999})
		s.Require().NoError(err)
		s.Require().Equal("key-a", names[keyA.ID])
		s.Require().Equal("key-b", names[keyB.ID])
		s.Require().NotContains(names, int64(999999))

		empty, err := s.repo.GetAPIKeyNamesByIDs(s.ctx, nil)
		s.Require().NoError(err)
		s.Require().Empty(empty)
	})
}
