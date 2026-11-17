//go:build integration

package repository

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// A failed insert must roll back the preceding destructive UPDATE/DELETE.
func TestUserGroupRateSyncRollbackPreservesExistingRows(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := mustCreateUser(t, client, &service.User{Username: "sync-rollback-" + suffix})
	group := mustCreateGroup(t, client, &service.Group{Name: "sync-rollback-" + suffix, Platform: service.PlatformOpenAI, RateMultiplier: 1})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM user_group_rate_multipliers WHERE group_id = $1", group.ID)
		_ = client.Group.DeleteOneID(group.ID).Exec(context.Background())
		_ = client.User.DeleteOneID(user.ID).Exec(context.Background())
	})

	repo := NewUserGroupRateRepository(integrationDB)
	require.NoError(t, repo.SyncGroupRateMultipliers(ctx, group.ID, []service.GroupRateMultiplierInput{{UserID: user.ID, RateMultiplier: 1.5}}))
	originalRate, err := repo.GetByUserAndGroup(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.NotNil(t, originalRate)
	require.Equal(t, 1.5, *originalRate)

	// The old row is cleared by UPDATE/DELETE before this unknown user causes an FK error.
	err = repo.SyncGroupRateMultipliers(ctx, group.ID, []service.GroupRateMultiplierInput{{UserID: math.MaxInt64, RateMultiplier: 2.0}})
	require.Error(t, err)
	rateAfterFailure, err := repo.GetByUserAndGroup(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.NotNil(t, rateAfterFailure, "transaction must restore the original row after a failed batch")
	require.Equal(t, 1.5, *rateAfterFailure)

	five := 5
	require.NoError(t, repo.SyncGroupRPMOverrides(ctx, group.ID, []service.GroupRPMOverrideInput{{UserID: user.ID, RPMOverride: &five}}))
	rpmBefore, err := repo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.Equal(t, &five, rpmBefore)

	eight := 8
	err = repo.SyncGroupRPMOverrides(ctx, group.ID, []service.GroupRPMOverrideInput{{UserID: math.MaxInt64, RPMOverride: &eight}})
	require.Error(t, err)
	rpmAfter, err := repo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.Equal(t, &five, rpmAfter)
}
