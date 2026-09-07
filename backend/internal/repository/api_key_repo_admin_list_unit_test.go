package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 管理端密钥总表：跨用户列出、按 user_id / group_id(0=未分组) / status / search 筛选、
// 预载 User 与 Group、补最近使用 IP，排序白名单与用户侧一致。
func TestAPIKeyRepositoryListAllForAdmin(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	alice := mustCreateAPIKeyRepoUser(t, ctx, client, "admin-list-alice@test.com")
	bob := mustCreateAPIKeyRepoUser(t, ctx, client, "admin-list-bob@test.com")
	accountID := mustCreateAPIKeyRepoAccount(t, ctx, client, "acc-admin-list")

	group, err := client.Group.Create().
		SetName("g-admin-list").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetRateMultiplier(1).
		Save(ctx)
	require.NoError(t, err)

	newKey := func(userID int64, key, name, status string, groupID *int64) *service.APIKey {
		k := &service.APIKey{UserID: userID, Key: key, Name: name, Status: status, GroupID: groupID}
		require.NoError(t, repo.Create(ctx, k))
		return k
	}
	aliceGrouped := newKey(alice.ID, "sk-admin-list-alice-grouped", "Alpha grouped", service.StatusActive, &group.ID)
	aliceInactive := newKey(alice.ID, "sk-admin-list-alice-inactive", "Charlie inactive", service.StatusAPIKeyInactive, nil)
	bobPlain := newKey(bob.ID, "sk-admin-list-bob-plain", "Bravo plain", service.StatusActive, nil)

	lastIP := "203.0.113.20"
	mustCreateAPIKeyRepoUsageLog(t, ctx, client, alice.ID, aliceGrouped.ID, accountID, "req-admin-list-ip", time.Now().UTC().Add(-time.Hour), &lastIP)

	// 用 search 把本用例的三把 Key 与同一进程里其他用例的数据隔开。
	base := service.AdminAPIKeyListFilters{Search: "sk-admin-list-"}
	list := func(params pagination.PaginationParams, filters service.AdminAPIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult) {
		t.Helper()
		keys, result, err := repo.ListAllForAdmin(ctx, params, filters)
		require.NoError(t, err)
		return keys, result
	}
	idsOf := func(keys []service.APIKey) []int64 {
		out := make([]int64, 0, len(keys))
		for _, k := range keys {
			out = append(out, k.ID)
		}
		return out
	}
	page := pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "name", SortOrder: "asc"}

	// 不带 user_id：跨用户全量，按名字升序；每行都带归属用户，绑分组的行带分组，用过的行带最近 IP。
	all, result := list(page, base)
	require.EqualValues(t, 3, result.Total)
	require.Equal(t, []int64{aliceGrouped.ID, bobPlain.ID, aliceInactive.ID}, idsOf(all))
	byID := make(map[int64]service.APIKey, len(all))
	for _, k := range all {
		byID[k.ID] = k
		require.NotNilf(t, k.User, "key %d 应预载 User", k.ID)
	}
	require.Equal(t, "admin-list-alice@test.com", byID[aliceGrouped.ID].User.Email)
	require.Equal(t, "admin-list-bob@test.com", byID[bobPlain.ID].User.Email)
	require.NotNil(t, byID[aliceGrouped.ID].Group)
	require.Equal(t, "g-admin-list", byID[aliceGrouped.ID].Group.Name)
	require.Nil(t, byID[bobPlain.ID].Group)
	require.NotNil(t, byID[aliceGrouped.ID].LastUsedIP)
	require.Equal(t, lastIP, *byID[aliceGrouped.ID].LastUsedIP)
	require.Nil(t, byID[bobPlain.ID].LastUsedIP)

	// 名字降序：顺序反过来。
	desc, _ := list(pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "name", SortOrder: "desc"}, base)
	require.Equal(t, []int64{aliceInactive.ID, bobPlain.ID, aliceGrouped.ID}, idsOf(desc))

	// 分页：总数不变，只取第二页那一条。
	second, secondResult := list(pagination.PaginationParams{Page: 2, PageSize: 2, SortBy: "name", SortOrder: "asc"}, base)
	require.EqualValues(t, 3, secondResult.Total)
	require.Equal(t, []int64{aliceInactive.ID}, idsOf(second))

	// user_id：只看某一个用户。
	withUser := base
	withUser.UserID = &alice.ID
	onlyAlice, _ := list(page, withUser)
	require.Equal(t, []int64{aliceGrouped.ID, aliceInactive.ID}, idsOf(onlyAlice))

	// group_id=0：只看未分组。
	zero := int64(0)
	withNoGroup := base
	withNoGroup.GroupID = &zero
	ungrouped, _ := list(page, withNoGroup)
	require.Equal(t, []int64{bobPlain.ID, aliceInactive.ID}, idsOf(ungrouped))

	// group_id=<id>：只看该分组。
	withGroup := base
	withGroup.GroupID = &group.ID
	grouped, _ := list(page, withGroup)
	require.Equal(t, []int64{aliceGrouped.ID}, idsOf(grouped))

	// status：只看停用的。
	withStatus := base
	withStatus.Status = service.StatusAPIKeyInactive
	inactive, _ := list(page, withStatus)
	require.Equal(t, []int64{aliceInactive.ID}, idsOf(inactive))

	// search 同时匹配名字片段（不区分大小写）。
	byName, _ := list(page, service.AdminAPIKeyListFilters{Search: "bravo"})
	require.Equal(t, []int64{bobPlain.ID}, idsOf(byName))

	// 软删后不再出现在总表里（DeleteWithAudit 用的是 Postgres 语法，这里直接打 deleted_at 模拟软删）。
	require.NoError(t, client.APIKey.UpdateOneID(bobPlain.ID).SetDeletedAt(time.Now().UTC()).Exec(ctx))
	afterDelete, afterResult := list(page, base)
	require.EqualValues(t, 2, afterResult.Total)
	require.Equal(t, []int64{aliceGrouped.ID, aliceInactive.ID}, idsOf(afterDelete))
}
