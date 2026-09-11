//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestGetDashboardStats_NormalAccountsMatchesSchedulerPrefilter 锁住首页「正常账号数」的口径。
//
// 这个数字曾经只看 status + schedulable，于是限流中、过载中、临时停调、已过期的账号
// 全被算成"正常"——而它恰恰是管理员每天第一眼看的数字。口径必须与调度器的 DB 预过滤
// （accountRepository.ListSchedulableByPlatforms）一致。
func (s *UsageLogRepoSuite) TestGetDashboardStats_NormalAccountsMatchesSchedulerPrefilter() {
	now := time.Now().UTC()
	future := now.Add(30 * time.Minute)
	past := now.Add(-30 * time.Minute)

	// 套件内其它用例也会建账号，绝对值不可靠，只比自己造出来的这批带来的增量。
	before, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err)

	// 唯一应当计入"正常"的账号。
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "dash-healthy"})

	// 以下每一个都不可调度，全都必须被排除。
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "dash-rate-limited", RateLimitedAt: &past, RateLimitResetAt: &future,
	})
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "dash-overloaded", OverloadUntil: &future,
	})
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "dash-error", Status: service.StatusError,
	})

	// 临时停调与过期两个字段 fixture 不支持，建完直接改行。
	tempUnsched := mustCreateAccount(s.T(), s.client, &service.Account{Name: "dash-temp-unsched"})
	_, err = s.client.Account.UpdateOneID(tempUnsched.ID).
		SetTempUnschedulableUntil(future).
		SetTempUnschedulableReason("test").
		Save(s.ctx)
	s.Require().NoError(err)

	expired := mustCreateAccount(s.T(), s.client, &service.Account{Name: "dash-expired"})
	_, err = s.client.Account.UpdateOneID(expired.ID).
		SetAutoPauseOnExpired(true).
		SetExpiresAt(past).
		Save(s.ctx)
	s.Require().NoError(err)

	// 过期但没开自动暂停：仍然可调度，必须算"正常"。
	expiredNoAutoPause := mustCreateAccount(s.T(), s.client, &service.Account{Name: "dash-expired-no-autopause"})
	_, err = s.client.Account.UpdateOneID(expiredNoAutoPause.ID).
		SetAutoPauseOnExpired(false).
		SetExpiresAt(past).
		Save(s.ctx)
	s.Require().NoError(err)

	// 限流窗口已经过去：不再算限流，应回到"正常"。
	staleLimit := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "dash-rate-limit-expired", RateLimitedAt: &past, RateLimitResetAt: &past,
	})
	s.Require().NotNil(staleLimit)

	after, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err)

	s.Require().EqualValues(8, after.TotalAccounts-before.TotalAccounts)
	// 只有 healthy + expired-no-autopause + rate-limit-expired 这三个仍可调度。
	// 限流中 / 过载中 / 异常 / 临时停调 / 已过期这五个都必须被排除。
	s.Require().EqualValues(3, after.NormalAccounts-before.NormalAccounts)
	s.Require().EqualValues(1, after.ErrorAccounts-before.ErrorAccounts)
	s.Require().EqualValues(1, after.RateLimitAccounts-before.RateLimitAccounts)
	s.Require().EqualValues(1, after.OverloadAccounts-before.OverloadAccounts)

	// 正常与限流/过载不得重复计数：这批 8 个账号里，被归入这四类的加起来不能超过 8。
	s.Require().LessOrEqual(
		(after.NormalAccounts-before.NormalAccounts)+
			(after.ErrorAccounts-before.ErrorAccounts)+
			(after.RateLimitAccounts-before.RateLimitAccounts)+
			(after.OverloadAccounts-before.OverloadAccounts),
		after.TotalAccounts-before.TotalAccounts,
	)
}
