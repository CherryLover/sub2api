package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func mustCreateAPIKeyRepoUsageCost(t *testing.T, ctx context.Context, client *dbent.Client, userID, apiKeyID, accountID int64, requestID string, createdAt time.Time, actualCost float64) {
	t.Helper()
	_, err := client.UsageLog.Create().
		SetUserID(userID).
		SetAPIKeyID(apiKeyID).
		SetAccountID(accountID).
		SetRequestID(requestID).
		SetModel("gpt-5").
		SetActualCost(actualCost).
		SetCreatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)
}

func apiKeyIDsOf(keys []service.APIKey) []int64 {
	ids := make([]int64, 0, len(keys))
	for i := range keys {
		ids = append(ids, keys[i].ID)
	}
	return ids
}

func TestAPIKeyRepositoryListByUserIDSortsByTodayCost(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "sort-today-cost@test.com")
	accountID := mustCreateAPIKeyRepoAccount(t, ctx, client, "acc-sort-today-cost")

	newKey := func(key, name string) *service.APIKey {
		k := &service.APIKey{UserID: user.ID, Key: key, Name: name, Status: service.StatusActive}
		require.NoError(t, repo.Create(ctx, k))
		return k
	}
	heavy := newKey("sk-today-cost-heavy", "Heavy Today")
	light := newKey("sk-today-cost-light", "Light Today")
	yesterdayOnly := newKey("sk-today-cost-yesterday", "Yesterday Only")
	noLogs := newKey("sk-today-cost-none", "No Logs")

	// 日志时间从 timezone.Today() 派生：与排序里的今日零点同一时区、同一序列化格式，
	// SQLite 里按字符串比较也能得到正确的先后。
	today := timezone.Today()
	mustCreateAPIKeyRepoUsageCost(t, ctx, client, user.ID, heavy.ID, accountID, "req-today-heavy-1", today.Add(time.Hour), 0.5)
	mustCreateAPIKeyRepoUsageCost(t, ctx, client, user.ID, heavy.ID, accountID, "req-today-heavy-2", today.Add(2*time.Hour), 0.25)
	mustCreateAPIKeyRepoUsageCost(t, ctx, client, user.ID, heavy.ID, accountID, "req-today-heavy-old", today.Add(-time.Hour), 9)
	mustCreateAPIKeyRepoUsageCost(t, ctx, client, user.ID, light.ID, accountID, "req-today-light", today.Add(30*time.Minute), 0.1)
	mustCreateAPIKeyRepoUsageCost(t, ctx, client, user.ID, yesterdayOnly.ID, accountID, "req-today-yesterday", today.Add(-2*time.Hour), 9)

	list := func(page, pageSize int, order string) []service.APIKey {
		keys, result, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortBy:    "today_cost",
			SortOrder: order,
		}, service.APIKeyListFilters{})
		require.NoError(t, err)
		require.EqualValues(t, 4, result.Total)
		return keys
	}

	// 倒序：昨天的 9 不算，只比今天；无用量的两把 Key 记 0，同值按 id 倒序。
	require.Equal(t, []int64{heavy.ID, light.ID, noLogs.ID, yesterdayOnly.ID}, apiKeyIDsOf(list(1, 10, pagination.SortOrderDesc)))
	// 正序：同值按 id 正序。
	require.Equal(t, []int64{yesterdayOnly.ID, noLogs.ID, light.ID, heavy.ID}, apiKeyIDsOf(list(1, 10, pagination.SortOrderAsc)))
	// 排序发生在服务端，分页切片要跟着排序结果走。
	require.Equal(t, []int64{heavy.ID, light.ID}, apiKeyIDsOf(list(1, 2, pagination.SortOrderDesc)))
	require.Equal(t, []int64{noLogs.ID, yesterdayOnly.ID}, apiKeyIDsOf(list(2, 2, pagination.SortOrderDesc)))
}

func TestAPIKeyTodayCostOrderPostgresUsesCorrelatedSubquery(t *testing.T) {
	today := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		sortOrder string
		wantOrder string
	}{
		{
			name:      "desc",
			sortOrder: pagination.SortOrderDesc,
			wantOrder: `ORDER BY COALESCE((SELECT SUM(ul.actual_cost) FROM usage_logs AS ul WHERE ul.api_key_id = "api_keys"."id" AND ul.created_at >= $2), 0) DESC, "api_keys"."id" DESC`,
		},
		{
			name:      "asc",
			sortOrder: pagination.SortOrderAsc,
			wantOrder: `ORDER BY COALESCE((SELECT SUM(ul.actual_cost) FROM usage_logs AS ul WHERE ul.api_key_id = "api_keys"."id" AND ul.created_at >= $2), 0) ASC, "api_keys"."id" ASC`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := entsql.Dialect(dialect.Postgres).
				Select("id").
				From(entsql.Table("api_keys")).
				Where(entsql.EQ("user_id", int64(7)))
			for _, order := range apiKeyTodayCostOrder(today, tt.sortOrder) {
				order(selector)
			}
			query, args := selector.Query()

			// 占位符要接在 WHERE 参数之后编号（$2），否则 Postgres 会把今日零点绑到错误的位置。
			require.True(t, strings.HasSuffix(query, tt.wantOrder), query)
			require.Equal(t, []any{int64(7), today}, args)
		})
	}
}

func TestAPIKeyListOrderTodayCostUsesConfiguredToday(t *testing.T) {
	selector := entsql.Dialect(dialect.Postgres).Select("id").From(entsql.Table("api_keys"))
	for _, order := range apiKeyListOrder(pagination.PaginationParams{SortBy: " Today_Cost ", SortOrder: "asc"}) {
		order(selector)
	}
	query, args := selector.Query()

	require.Contains(t, query, "ul.created_at >= $1), 0) ASC")
	require.Len(t, args, 1)
	todayArg, ok := args[0].(time.Time)
	require.True(t, ok, "today boundary should be bound as time.Time")
	require.True(t, todayArg.Equal(timezone.Today()), "today boundary must follow timezone.Today()")
}
