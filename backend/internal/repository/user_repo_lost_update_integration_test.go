//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 这一组用例覆盖用户行上的 lost update：调用方手里的快照可能早于并发发生的
// 原子写入（扣费、状态变更、限额调整、分组授予）。Update 只写显式声明的列，
// 未声明的列一律保持库中当前值，因此陈旧快照不会回滚这些并发结果。

// 同理，风控自动封禁把 status 置为 disabled 后，
// 基于旧快照的资料更新不得把 status 刷回 active。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentBan() {
	user := s.mustCreateUser(&service.User{
		Email:    "lost-update-ban@example.com",
		Username: "before",
		Status:   service.StatusActive,
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(service.StatusActive, stale.Status)

	banned, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID for ban")
	banned.Status = service.StatusDisabled
	s.Require().NoError(
		s.repo.Update(s.ctx, banned, service.UserUpdateFields{Status: true}),
		"ban",
	)

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("after", got.Username)
	s.Require().Equal(service.StatusDisabled, got.Status, "ban must survive a stale profile save")
}

// 未声明的列不写，也意味着并发的限额调整不会被资料保存回滚。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentLimitChanges() {
	user := s.mustCreateUser(&service.User{
		Email:       "lost-update-limits@example.com",
		Username:    "before",
		Concurrency: 3,
		RPMLimit:    30,
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")

	concurrency, rpmLimit := 9, 90
	affected, err := s.repo.BatchUpdateLimits(s.ctx, []int64{user.ID}, &concurrency, &rpmLimit)
	s.Require().NoError(err, "BatchUpdateLimits")
	s.Require().Equal(1, affected)

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal(9, got.Concurrency, "concurrency must not be reverted")
	s.Require().Equal(90, got.RPMLimit, "rpm limit must not be reverted")
}

// AllowedGroups 只在显式声明时才同步，否则并发授予的分组权限会被旧快照删掉。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentAllowedGroupGrant() {
	group := s.mustCreateGroup("lost-update-group")
	user := s.mustCreateUser(&service.User{
		Email:    "lost-update-groups@example.com",
		Username: "before",
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Empty(stale.AllowedGroups)

	s.Require().NoError(s.repo.AddGroupToAllowedGroups(s.ctx, user.ID, group.ID), "AddGroupToAllowedGroups")

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal([]int64{group.ID}, got.AllowedGroups, "granted group must not be reverted")
}
