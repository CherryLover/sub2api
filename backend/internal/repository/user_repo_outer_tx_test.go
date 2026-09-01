//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestCreateWithEmailAliasGuardJoinsOuterTransaction 验证用户创建会加入调用方开启的
// 外部 ent 事务（注册流程多写原子性的基础）：
//   - 外层事务回滚后，用户写入必须一并撤销（不得残留孤儿账号）；
//   - 外层事务提交后，用户写入生效。
//
// 回归背景：此前 create() 通过 r.client.Tx(ctx) 自开事务且自行 Commit —— ent 的
// Client.Tx 不感知上下文事务（只检查 driver 类型），ErrTxStarted 分支实际是死代码，
// 导致外层事务包不住用户写入；外层事务回滚时，败者用户的写入已自行提交，
// 留下可登录的孤儿账号。
func TestCreateWithEmailAliasGuardJoinsOuterTransaction(t *testing.T) {
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)

	ctx := context.Background()

	// 清理：本测试会真实提交少量数据，确保不影响同包其它集成测试。
	var committedUserEmails []string
	t.Cleanup(func() {
		if len(committedUserEmails) > 0 {
			_, _ = client.User.Delete().Where(user.EmailIn(committedUserEmails...)).Exec(ctx)
		}
	})

	t.Run("rollback removes user", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		txCtx := dbent.NewTxContext(ctx, tx)

		u := &service.User{
			Email:        "itx-rollback@example.com",
			PasswordHash: "test-password-hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			Concurrency:  1,
		}
		require.NoError(t, userRepo.CreateWithEmailAliasGuard(txCtx, u))
		require.Greater(t, u.ID, int64(0), "create 应回填用户 ID")
		require.NoError(t, tx.Rollback())

		exists, err := userRepo.ExistsByEmail(ctx, "itx-rollback@example.com")
		require.NoError(t, err)
		require.False(t, exists, "回滚后不得残留孤儿用户")
	})

	t.Run("commit persists user", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		txCtx := dbent.NewTxContext(ctx, tx)

		u := &service.User{
			Email:        "itx-commit@example.com",
			PasswordHash: "test-password-hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			Concurrency:  1,
		}
		require.NoError(t, userRepo.CreateWithEmailAliasGuard(txCtx, u))
		require.NoError(t, tx.Commit())
		committedUserEmails = append(committedUserEmails, u.Email)

		exists, err := userRepo.ExistsByEmail(ctx, "itx-commit@example.com")
		require.NoError(t, err)
		require.True(t, exists, "提交后用户应存在")
	})
}
