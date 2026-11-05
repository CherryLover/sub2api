package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryGroupNamePagination(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "group-sort-unit@test.com")
	other := mustCreateAPIKeyRepoUser(t, ctx, client, "group-sort-other@test.com")
	z, err := client.Group.Create().SetName("z-group").SetPlatform(service.PlatformOpenAI).SetStatus(service.StatusActive).SetRateMultiplier(1).Save(ctx)
	require.NoError(t, err)
	a, err := client.Group.Create().SetName("a-group").SetPlatform(service.PlatformOpenAI).SetStatus(service.StatusActive).SetRateMultiplier(1).Save(ctx)
	require.NoError(t, err)
	create := func(uid int64, name, status string, gid *int64) int64 {
		key := &service.APIKey{UserID: uid, Key: "sk-unit-group-" + name, Name: name, Status: status, GroupID: gid}
		require.NoError(t, repo.Create(ctx, key))
		return key.ID
	}
	zi := create(user.ID, "match-z", service.StatusActive, &z.ID)
	a1 := create(user.ID, "match-a1", service.StatusActive, &a.ID)
	a2 := create(user.ID, "match-a2", service.StatusActive, &a.ID)
	n1 := create(user.ID, "match-null1", service.StatusActive, nil)
	n2 := create(user.ID, "match-null2", service.StatusActive, nil)
	create(other.ID, "match-other", service.StatusActive, &a.ID)
	create(user.ID, "excluded", service.StatusActive, &a.ID)
	create(user.ID, "match-inactive", service.StatusAPIKeyInactive, &a.ID)
	filters := service.APIKeyListFilters{Search: "match", Status: service.StatusActive}
	list := func(page int, order string) []int64 {
		keys, result, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{Page: page, PageSize: 2, SortBy: "group_name", SortOrder: order}, filters)
		require.NoError(t, err)
		require.EqualValues(t, 5, result.Total)
		ids := make([]int64, 0, len(keys))
		for _, key := range keys {
			ids = append(ids, key.ID)
		}
		return ids
	}
	for direction, expected := range map[string][]int64{"asc": {a1, a2, zi, n1, n2}, "desc": {zi, a2, a1, n2, n1}} {
		actual := append(list(1, direction), list(2, direction)...)
		actual = append(actual, list(3, direction)...)
		require.Equal(t, expected, actual)
	}
}

func TestAPIKeyListOrderGroupNameSQL(t *testing.T) {
	for _, direction := range []string{"asc", "desc"} {
		t.Run(direction, func(t *testing.T) {
			builder := entsql.Dialect(dialect.Postgres)
			table := builder.Table("api_keys")
			selector := builder.Select(table.C("id")).From(table)
			for _, order := range apiKeyListOrder(pagination.PaginationParams{SortBy: "group_name", SortOrder: direction}) {
				order(selector)
			}
			query, _ := selector.Query()
			require.Contains(t, query, `LEFT JOIN (SELECT "groups"."id", "groups"."name" FROM "groups")`)
			require.Contains(t, query, `ON "api_keys"."group_id" = "t1"."id"`)
			require.Contains(t, query, `"name"`)
			require.Contains(t, query, `NULLS LAST`)
			if direction == "asc" {
				require.NotContains(t, query, `DESC`)
			} else {
				require.Contains(t, query, `"name" DESC NULLS LAST`)
				require.Contains(t, query, `"api_keys"."id" DESC`)
			}
		})
	}
}
