//go:build unit

package service

import "context"

// authCacheInvalidatorStub 记录被失效的用户 / 分组 / Key，供 admin service 的单元测试断言。
// 原先随 admin_service_update_balance_test.go 一起定义，余额语义拆除后该文件整体删除，
// 这里把仍被多处引用的 stub 单独保留下来。
type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}
