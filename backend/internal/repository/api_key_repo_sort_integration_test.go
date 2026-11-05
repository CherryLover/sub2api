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

func (s *APIKeyRepoSuite) TestListByUserID_SortByGroupName() {
	user := s.mustCreateUser("sort-group@example.com")
	otherUser := s.mustCreateUser("sort-group-other@example.com")
	zGroup := s.mustCreateGroup("z-group")
	aGroup := s.mustCreateGroup("a-group")
	z := s.mustCreateApiKey(user.ID, "sk-group-z", "match-z", &zGroup.ID)
	a1 := s.mustCreateApiKey(user.ID, "sk-group-a1", "match-a1", &aGroup.ID)
	a2 := s.mustCreateApiKey(user.ID, "sk-group-a2", "match-a2", &aGroup.ID)
	n1 := s.mustCreateApiKey(user.ID, "sk-group-null1", "match-null1", nil)
	n2 := s.mustCreateApiKey(user.ID, "sk-group-null2", "match-null2", nil)
	s.mustCreateApiKey(otherUser.ID, "sk-group-other", "match-other", &aGroup.ID)
	excluded := s.mustCreateApiKey(user.ID, "sk-group-excluded", "excluded", &aGroup.ID)
	_, err := s.client.APIKey.UpdateOneID(excluded.ID).SetStatus(service.StatusAPIKeyInactive).Save(s.ctx)
	s.Require().NoError(err)

	list := func(page, size int, order string, filters service.APIKeyListFilters, total int) []int64 {
		keys, result, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{
			Page: page, PageSize: size, SortBy: "group_name", SortOrder: order,
		}, filters)
		s.Require().NoError(err)
		s.Require().EqualValues(total, result.Total)
		return apiKeyIDsOf(keys)
	}
	filters := service.APIKeyListFilters{Search: "match", Status: service.StatusActive}
	s.Require().Equal([]int64{a1.ID, a2.ID, z.ID, n1.ID, n2.ID}, list(1, 10, "asc", filters, 5))
	s.Require().Equal([]int64{z.ID, a2.ID, a1.ID, n2.ID, n1.ID}, list(1, 10, "desc", filters, 5))
	for _, order := range []string{"asc", "desc"} {
		all := list(1, 10, order, filters, 5)
		paged := append(list(1, 2, order, filters, 5), list(2, 2, order, filters, 5)...)
		paged = append(paged, list(3, 2, order, filters, 5)...)
		s.Require().Equal(all, paged)
	}
	filters.GroupID = &aGroup.ID
	s.Require().Equal([]int64{a2.ID, a1.ID}, list(1, 10, "desc", filters, 2))
	// Resolve the current bound group and current name, not a cached display label.
	_, err = s.client.APIKey.UpdateOneID(z.ID).SetGroupID(aGroup.ID).Save(s.ctx)
	s.Require().NoError(err)
	_, err = s.client.Group.UpdateOneID(aGroup.ID).SetName("renamed-group").Save(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal([]int64{z.ID, a1.ID, a2.ID}, list(1, 10, "asc", filters, 3))
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
