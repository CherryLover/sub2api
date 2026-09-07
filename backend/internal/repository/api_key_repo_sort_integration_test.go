//go:build integration

package repository

import (
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *APIKeyRepoSuite) mustCreateUsageCost(userID, apiKeyID, accountID int64, createdAt time.Time, actualCost float64) {
	s.T().Helper()

	_, err := s.client.UsageLog.Create().
		SetUserID(userID).
		SetAPIKeyID(apiKeyID).
		SetAccountID(accountID).
		SetRequestID(uuid.New().String()).
		SetModel("gpt-5").
		SetActualCost(actualCost).
		SetCreatedAt(createdAt).
		Save(s.ctx)
	s.Require().NoError(err, "create usage log")
}

func (s *APIKeyRepoSuite) TestListByUserID_SortByTodayCost() {
	user := s.mustCreateUser("sort-today-cost@example.com")
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-sort-today-cost"})
	heavy := s.mustCreateApiKey(user.ID, "sk-today-heavy", "heavy", nil)
	light := s.mustCreateApiKey(user.ID, "sk-today-light", "light", nil)
	yesterdayOnly := s.mustCreateApiKey(user.ID, "sk-today-yesterday", "yesterday-only", nil)
	noLogs := s.mustCreateApiKey(user.ID, "sk-today-none", "no-logs", nil)

	// 口径与批量用量接口一致：只算 timezone.Today() 之后的 actual_cost，昨天的大额不参与。
	today := timezone.Today()
	s.mustCreateUsageCost(user.ID, heavy.ID, account.ID, today.Add(time.Hour), 0.5)
	s.mustCreateUsageCost(user.ID, heavy.ID, account.ID, today.Add(2*time.Hour), 0.25)
	s.mustCreateUsageCost(user.ID, heavy.ID, account.ID, today.Add(-time.Hour), 9)
	s.mustCreateUsageCost(user.ID, light.ID, account.ID, today.Add(30*time.Minute), 0.1)
	s.mustCreateUsageCost(user.ID, yesterdayOnly.ID, account.ID, today.Add(-2*time.Hour), 9)

	list := func(page, pageSize int, order string) []int64 {
		keys, result, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortBy:    "today_cost",
			SortOrder: order,
		}, service.APIKeyListFilters{})
		s.Require().NoError(err)
		s.Require().EqualValues(4, result.Total)
		return apiKeyIDsOf(keys)
	}

	// 无用量的 Key 记 0，同值按 id 稳定排序；分页切片跟着服务端排序结果走。
	s.Require().Equal([]int64{heavy.ID, light.ID, noLogs.ID, yesterdayOnly.ID}, list(1, 10, pagination.SortOrderDesc))
	s.Require().Equal([]int64{yesterdayOnly.ID, noLogs.ID, light.ID, heavy.ID}, list(1, 10, pagination.SortOrderAsc))
	s.Require().Equal([]int64{heavy.ID, light.ID}, list(1, 2, pagination.SortOrderDesc))
	s.Require().Equal([]int64{noLogs.ID, yesterdayOnly.ID}, list(2, 2, pagination.SortOrderDesc))
}

func (s *APIKeyRepoSuite) TestListByUserID_SortByNameAsc() {
	user := s.mustCreateUser("sort-name@example.com")
	s.mustCreateApiKey(user.ID, "sk-z", "z-key", nil)
	s.mustCreateApiKey(user.ID, "sk-a", "a-key", nil)

	keys, _, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "name",
		SortOrder: "asc",
	}, service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Len(keys, 2)
	s.Require().Equal("a-key", keys[0].Name)
	s.Require().Equal("z-key", keys[1].Name)
}

func (s *APIKeyRepoSuite) TestListByUserID_SortByID() {
	user := s.mustCreateUser("sort-id@example.com")
	first := s.mustCreateApiKey(user.ID, "sk-id-a", "a-key", nil)
	second := s.mustCreateApiKey(user.ID, "sk-id-b", "b-key", nil)

	keys, _, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "id",
		SortOrder: "desc",
	}, service.APIKeyListFilters{})
	s.Require().NoError(err)
	s.Require().Len(keys, 2)
	s.Require().Equal(second.ID, keys[0].ID)
	s.Require().Equal(first.ID, keys[1].ID)
}
