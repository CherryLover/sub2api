//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// userRepoStubForGroupUpdate implements UserRepository for AdminUpdateAPIKeyGroupID tests.
type userRepoStubForGroupUpdate struct {
	addGroupErr    error
	addGroupCalled bool
	addedUserID    int64
	addedGroupID   int64
}

func (s *userRepoStubForGroupUpdate) AddGroupToAllowedGroups(_ context.Context, userID int64, groupID int64) error {
	s.addGroupCalled = true
	s.addedUserID = userID
	s.addedGroupID = groupID
	return s.addGroupErr
}

func (s *userRepoStubForGroupUpdate) Create(context.Context, *User) error { panic("unexpected") }
func (s *userRepoStubForGroupUpdate) CreateWithEmailAliasGuard(context.Context, *User) error {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) GetByID(context.Context, int64) (*User, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) GetByEmail(context.Context, string) (*User, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) GetFirstAdmin(context.Context) (*User, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) Update(context.Context, *User, UserUpdateFields) error {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) Delete(context.Context, int64) error { panic("unexpected") }
func (s *userRepoStubForGroupUpdate) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected")
}

func (s *userRepoStubForGroupUpdate) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userRepoStubForGroupUpdate) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userRepoStubForGroupUpdate) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}
func (s *userRepoStubForGroupUpdate) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) ExistsByEmailAlias(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) EnableTotp(context.Context, int64) error  { panic("unexpected") }
func (s *userRepoStubForGroupUpdate) DisableTotp(context.Context, int64) error { panic("unexpected") }
func (s *userRepoStubForGroupUpdate) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
func (s *userRepoStubForGroupUpdate) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected")
}

func (s *userRepoStubForGroupUpdate) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *userRepoStubForGroupUpdate) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
}

// apiKeyRepoStubForGroupUpdate implements APIKeyRepository for AdminUpdateAPIKeyGroupID tests.
type apiKeyRepoStubForGroupUpdate struct {
	key           *APIKey
	getErr        error
	updateErr     error
	updated       *APIKey            // captures what was passed to Update
	updatedFields APIKeyUpdateFields // captures which columns Update was asked to write
}

func (s *apiKeyRepoStubForGroupUpdate) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	clone := *s.key
	return &clone, nil
}
func (s *apiKeyRepoStubForGroupUpdate) Update(_ context.Context, key *APIKey, fields APIKeyUpdateFields) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	clone := *key
	s.updated = &clone
	s.updatedFields = fields
	return nil
}

// Unused methods – panic on unexpected call.
func (s *apiKeyRepoStubForGroupUpdate) Create(context.Context, *APIKey) error { panic("unexpected") }
func (s *apiKeyRepoStubForGroupUpdate) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) GetByKey(context.Context, string) (*APIKey, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) Delete(context.Context, int64) error { panic("unexpected") }
func (s *apiKeyRepoStubForGroupUpdate) DeleteWithAudit(context.Context, int64) error {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ExistsByKey(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) CountByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("unexpected")
}
func (s *apiKeyRepoStubForGroupUpdate) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected")
}

// groupRepoStubForGroupUpdate implements GroupRepository for AdminUpdateAPIKeyGroupID tests.
type groupRepoStubForGroupUpdate struct {
	group          *Group
	getErr         error
	lastGetByIDArg int64
}

func (s *groupRepoStubForGroupUpdate) GetByID(_ context.Context, id int64) (*Group, error) {
	s.lastGetByIDArg = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	clone := *s.group
	return &clone, nil
}

// Unused methods – panic on unexpected call.
func (s *groupRepoStubForGroupUpdate) Create(context.Context, *Group) error { panic("unexpected") }
func (s *groupRepoStubForGroupUpdate) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) Update(context.Context, *Group) error { panic("unexpected") }
func (s *groupRepoStubForGroupUpdate) Delete(context.Context, int64) error  { panic("unexpected") }
func (s *groupRepoStubForGroupUpdate) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) ListActive(context.Context) ([]Group, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected")
}
func (s *groupRepoStubForGroupUpdate) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAdminService_AdminUpdateAPIKeyGroupID_KeyNotFound(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{getErr: ErrAPIKeyNotFound}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 999, int64Ptr(1))
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_NilGroupID_NoOp(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(5)}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.APIKey.ID)
	// Update should NOT have been called (updated stays nil)
	require.Nil(t, repo.updated)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_Unbind(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(5), Group: &Group{ID: 5, Name: "Old"}}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
	require.NoError(t, err)
	require.Nil(t, got.APIKey.GroupID, "group_id should be nil after unbind")
	require.Nil(t, got.APIKey.Group, "group object should be nil after unbind")
	require.NotNil(t, repo.updated, "Update should have been called")
	require.Nil(t, repo.updated.GroupID)
	require.Equal(t, []string{"sk-test"}, cache.keys, "cache should be invalidated")
}

func TestAdminService_AdminUpdateAPIKeyGroupID_BindActiveGroup(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	require.Equal(t, int64(10), *apiKeyRepo.updated.GroupID)
	require.Equal(t, []string{"sk-test"}, cache.keys)
	// M3: verify correct group ID was passed to repo
	require.Equal(t, int64(10), groupRepo.lastGetByIDArg)
	// C1 fix: verify Group object is populated
	require.NotNil(t, got.APIKey.Group)
	require.Equal(t, "Pro", got.APIKey.Group.Name)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_SameGroup_Idempotent(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(10), Group: &Group{ID: 10, Name: "Pro"}}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	// Update is still called (current impl doesn't short-circuit on same group)
	require.NotNil(t, apiKeyRepo.updated)
	require.Equal(t, []string{"sk-test"}, cache.keys)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_GroupNotFound(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{getErr: ErrGroupNotFound}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(99))
	require.ErrorIs(t, err, ErrGroupNotFound)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_GroupNotActive(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 5, Status: StatusDisabled}}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(5))
	require.Error(t, err)
	require.Equal(t, "GROUP_NOT_ACTIVE", infraerrors.Reason(err))
}

func TestAdminService_AdminUpdateAPIKeyGroupID_UpdateFails(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: int64Ptr(3)}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing, updateErr: errors.New("db write error")}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
	require.Error(t, err)
	require.Contains(t, err.Error(), "update api key")
}

func TestAdminService_AdminUpdateAPIKeyGroupID_NegativeGroupID(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(-5))
	require.Error(t, err)
	require.Equal(t, "INVALID_GROUP_ID", infraerrors.Reason(err))
}

func TestAdminService_AdminUpdateAPIKeyGroupID_PointerIsolation(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Pro", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, authCacheInvalidator: cache}

	inputGID := int64(10)
	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, &inputGID)
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	// Mutating the input pointer must NOT affect the stored value
	inputGID = 999
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	require.Equal(t, int64(10), *apiKeyRepo.updated.GroupID)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_NilCacheInvalidator(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test"}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 7, Status: StatusActive}}
	// authCacheInvalidator is nil – should not panic
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(7))
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(7), *got.APIKey.GroupID)
}

// ---------------------------------------------------------------------------
// Tests: AllowedGroup auto-sync
// ---------------------------------------------------------------------------

func TestAdminService_AdminUpdateAPIKeyGroupID_ExclusiveGroup_AddsAllowedGroup(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Exclusive", Status: StatusActive, IsExclusive: true}}
	userRepo := &userRepoStubForGroupUpdate{}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	require.Equal(t, int64(10), *got.APIKey.GroupID)
	// 验证 AddGroupToAllowedGroups 被调用，且参数正确
	require.True(t, userRepo.addGroupCalled)
	require.Equal(t, int64(42), userRepo.addedUserID)
	require.Equal(t, int64(10), userRepo.addedGroupID)
	// 验证 result 标记了自动授权
	require.True(t, got.AutoGrantedGroupAccess)
	require.NotNil(t, got.GrantedGroupID)
	require.Equal(t, int64(10), *got.GrantedGroupID)
	require.Equal(t, "Exclusive", got.GrantedGroupName)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_NonExclusiveGroup_NoAllowedGroupUpdate(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Public", Status: StatusActive, IsExclusive: false}}
	userRepo := &userRepoStubForGroupUpdate{}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
	require.NoError(t, err)
	require.NotNil(t, got.APIKey.GroupID)
	// 非专属分组不触发 AddGroupToAllowedGroups
	require.False(t, userRepo.addGroupCalled)
	require.False(t, got.AutoGrantedGroupAccess)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_ExclusiveGroup_AllowedGroupAddFails_ReturnsError(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	groupRepo := &groupRepoStubForGroupUpdate{group: &Group{ID: 10, Name: "Exclusive", Status: StatusActive, IsExclusive: true}}
	userRepo := &userRepoStubForGroupUpdate{addGroupErr: errors.New("db error")}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo, userRepo: userRepo}

	// 严格模式：AddGroupToAllowedGroups 失败时，整体操作报错
	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))
	require.Error(t, err)
	require.Contains(t, err.Error(), "add group to user allowed groups")
	require.True(t, userRepo.addGroupCalled)
	// apiKey 不应被更新
	require.Nil(t, apiKeyRepo.updated)
}

func TestAdminService_AdminUpdateAPIKeyGroupID_Unbind_NoAllowedGroupUpdate(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: int64Ptr(10), Group: &Group{ID: 10, Name: "Exclusive"}}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{key: existing}
	userRepo := &userRepoStubForGroupUpdate{}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, userRepo: userRepo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(0))
	require.NoError(t, err)
	require.Nil(t, got.APIKey.GroupID)
	// 解绑时不修改 allowed_groups
	require.False(t, userRepo.addGroupCalled)
	require.False(t, got.AutoGrantedGroupAccess)
}

// ---------------------------------------------------------------------------
// 管理端密钥总表 / 状态与 IP 名单 / 删除
// ---------------------------------------------------------------------------

// apiKeyRepoStubForAdminOps 在分组桩之上补齐删除与跨用户列表所需的方法；
// 未覆盖的方法仍走内嵌桩（panic），保证这些路径不会悄悄多调仓储。
type apiKeyRepoStubForAdminOps struct {
	*apiKeyRepoStubForGroupUpdate

	ownerKey  string
	ownerErr  error
	deleted   []int64
	deleteErr error

	listKeys   []APIKey
	listErr    error
	listCalls  int
	listParams pagination.PaginationParams
	listFilter AdminAPIKeyListFilters
}

func newAdminOpsRepoStub(key *APIKey) *apiKeyRepoStubForAdminOps {
	return &apiKeyRepoStubForAdminOps{apiKeyRepoStubForGroupUpdate: &apiKeyRepoStubForGroupUpdate{key: key}}
}

func (s *apiKeyRepoStubForAdminOps) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	if s.ownerErr != nil {
		return "", 0, s.ownerErr
	}
	return s.ownerKey, 7, nil
}

func (s *apiKeyRepoStubForAdminOps) DeleteWithAudit(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *apiKeyRepoStubForAdminOps) ListAllForAdmin(_ context.Context, params pagination.PaginationParams, filters AdminAPIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	s.listCalls++
	s.listParams = params
	s.listFilter = filters
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	return s.listKeys, &pagination.PaginationResult{Total: int64(len(s.listKeys)), Page: params.Page, PageSize: params.PageSize}, nil
}

func adminKeyStrPtr(v string) *string { return &v }

func adminKeyStrsPtr(v []string) *[]string { return &v }

// --- AdminListAPIKeys ---

func TestAdminService_AdminListAPIKeys_PassesThroughToRepo(t *testing.T) {
	uid := int64(3)
	gid := int64(0)
	repo := newAdminOpsRepoStub(nil)
	repo.listKeys = []APIKey{{ID: 1, UserID: 3, Key: "sk-one"}, {ID: 2, UserID: 3, Key: "sk-two"}}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	params := pagination.PaginationParams{Page: 2, PageSize: 5, SortBy: "today_cost", SortOrder: "asc"}
	filters := AdminAPIKeyListFilters{UserID: &uid, GroupID: &gid, Status: StatusAPIKeyInactive, Search: "one"}
	keys, result, err := svc.AdminListAPIKeys(context.Background(), params, filters)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.EqualValues(t, 2, result.Total)
	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, params, repo.listParams, "分页与排序原样透传")
	require.Equal(t, filters, repo.listFilter, "user_id / group_id=0 / status / search 原样透传")
	require.Equal(t, "sk-one", keys[0].Key, "service 层不做掩码，明文留给 handler 出口处理")
}

func TestAdminService_AdminListAPIKeys_FiltersToUserSideShape(t *testing.T) {
	uid := int64(9)
	gid := int64(4)
	f := AdminAPIKeyListFilters{UserID: &uid, GroupID: &gid, Status: StatusActive, Search: "x"}
	require.Equal(t, APIKeyListFilters{Search: "x", Status: StatusActive, GroupID: &gid}, f.APIKeyListFilters())
}

func TestAdminService_AdminListAPIKeys_RepoWithoutSupport(t *testing.T) {
	svc := &adminServiceImpl{apiKeyRepo: &apiKeyRepoStubForGroupUpdate{}}

	_, _, err := svc.AdminListAPIKeys(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, AdminAPIKeyListFilters{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support")
}

func TestAdminService_AdminListAPIKeys_RejectsNegativeIDs(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	svc := &adminServiceImpl{apiKeyRepo: repo}
	neg := int64(-1)

	_, _, err := svc.AdminListAPIKeys(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, AdminAPIKeyListFilters{UserID: &neg})
	require.Equal(t, "INVALID_USER_ID", infraerrors.Reason(err))

	_, _, err = svc.AdminListAPIKeys(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, AdminAPIKeyListFilters{GroupID: &neg})
	require.Equal(t, "INVALID_GROUP_ID", infraerrors.Reason(err))
	require.Zero(t, repo.listCalls)
}

func TestAdminService_AdminListAPIKeys_RepoError(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	repo.listErr = errors.New("boom")
	svc := &adminServiceImpl{apiKeyRepo: repo}

	_, _, err := svc.AdminListAPIKeys(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, AdminAPIKeyListFilters{})
	require.ErrorContains(t, err, "boom")
}

// --- AdminUpdateAPIKey ---

func TestAdminService_AdminUpdateAPIKey_StatusOnlyWritesStatus(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", Status: StatusActive, Quota: 5, QuotaUsed: 5, IPWhitelist: []string{"10.0.0.1"}}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{Status: adminKeyStrPtr(StatusAPIKeyInactive)})
	require.NoError(t, err)
	require.Equal(t, StatusAPIKeyInactive, got.Status)
	require.NotNil(t, repo.updated)
	require.Equal(t, StatusAPIKeyInactive, repo.updated.Status)
	require.Equal(t, APIKeyUpdateFields{Status: true}, repo.updatedFields, "只写 status 一列，不碰 IP / 额度 / 分组")
	require.Equal(t, []string{"10.0.0.1"}, repo.updated.IPWhitelist, "没传的 IP 名单原样保留")
	require.Equal(t, []string{"sk-test"}, cache.keys, "写完要失效认证缓存")
}

func TestAdminService_AdminUpdateAPIKey_ReactivateDoesNotTouchQuota(t *testing.T) {
	// 与用户侧不同：管理员把 quota_exhausted 的 Key 置回 active 只写 status，不会顺手改额度。
	existing := &APIKey{ID: 1, Key: "sk-test", Status: StatusAPIKeyQuotaExhausted, Quota: 1, QuotaUsed: 1}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	got, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{Status: adminKeyStrPtr(StatusAPIKeyActive)})
	require.NoError(t, err)
	require.Equal(t, StatusAPIKeyActive, got.Status)
	require.Equal(t, APIKeyUpdateFields{Status: true}, repo.updatedFields)
	require.Equal(t, float64(1), repo.updated.Quota)
	require.Equal(t, float64(1), repo.updated.QuotaUsed)
}

func TestAdminService_AdminUpdateAPIKey_InvalidStatus(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{key: &APIKey{ID: 1, Key: "sk-test", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	for _, status := range []string{"", "expired", "quota_exhausted", "disabled", "Active"} {
		_, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{Status: adminKeyStrPtr(status)})
		require.ErrorIsf(t, err, ErrInvalidAPIKeyStatus, "status %q", status)
	}
	require.Nil(t, repo.updated, "非法状态不能写库")
	require.Empty(t, cache.keys)
}

func TestAdminService_AdminUpdateAPIKey_IPRulesValid(t *testing.T) {
	existing := &APIKey{ID: 1, Key: "sk-test", Status: StatusActive, IPBlacklist: []string{"203.0.113.9"}}
	repo := &apiKeyRepoStubForGroupUpdate{key: existing}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{
		IPWhitelist: adminKeyStrsPtr([]string{"10.0.0.0/8", "192.168.1.1", "2001:db8::/32"}),
		IPBlacklist: adminKeyStrsPtr([]string{}),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1", "2001:db8::/32"}, got.IPWhitelist)
	require.Empty(t, got.IPBlacklist, "空数组表示清空")
	require.Equal(t, APIKeyUpdateFields{IPRules: true}, repo.updatedFields, "只写 IP 两列，status 不动")
	require.Equal(t, StatusActive, repo.updated.Status)
	require.Equal(t, []string{"sk-test"}, cache.keys)
}

func TestAdminService_AdminUpdateAPIKey_IPRulesInvalid(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{key: &APIKey{ID: 1, Key: "sk-test", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	_, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{IPWhitelist: adminKeyStrsPtr([]string{"10.0.0.1", "999.999.1.1"})})
	require.ErrorIs(t, err, ErrInvalidIPPattern)
	require.Equal(t, "INVALID_IP_PATTERN", infraerrors.Reason(err))
	require.Contains(t, err.Error(), "999.999.1.1", "错误里要点名出错的那条")
	require.NotContains(t, err.Error(), "message=\"invalid IP or CIDR pattern: 10.0.0.1", "合法的条目不该被列进去")

	_, err = svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{IPBlacklist: adminKeyStrsPtr([]string{"not-an-ip"})})
	require.ErrorIs(t, err, ErrInvalidIPPattern)

	require.Nil(t, repo.updated, "非法 IP 不能写库")
	require.Empty(t, cache.keys)
}

func TestAdminService_AdminUpdateAPIKey_EmptyInputNoWrite(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{key: &APIKey{ID: 1, Key: "sk-test", Status: StatusActive}}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	got, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ID)
	require.Nil(t, repo.updated)
	require.Empty(t, cache.keys)
}

func TestAdminService_AdminUpdateAPIKey_KeyNotFound(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{getErr: ErrAPIKeyNotFound}
	svc := &adminServiceImpl{apiKeyRepo: repo}

	_, err := svc.AdminUpdateAPIKey(context.Background(), 404, AdminUpdateAPIKeyInput{Status: adminKeyStrPtr(StatusAPIKeyInactive)})
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestAdminService_AdminUpdateAPIKey_RepoUpdateError(t *testing.T) {
	repo := &apiKeyRepoStubForGroupUpdate{key: &APIKey{ID: 1, Key: "sk-test", Status: StatusActive}, updateErr: errors.New("write failed")}
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	_, err := svc.AdminUpdateAPIKey(context.Background(), 1, AdminUpdateAPIKeyInput{Status: adminKeyStrPtr(StatusAPIKeyInactive)})
	require.ErrorContains(t, err, "write failed")
	require.Empty(t, cache.keys, "写库失败不能先把缓存清掉")
}

// --- AdminDeleteAPIKey ---

func TestAdminService_AdminDeleteAPIKey_DeletesThenInvalidatesCache(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	repo.ownerKey = "sk-victim"
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	require.NoError(t, svc.AdminDeleteAPIKey(context.Background(), 42))
	require.Equal(t, []int64{42}, repo.deleted, "走 DeleteWithAudit（软删 + 墓碑）")
	require.Equal(t, []string{"sk-victim"}, cache.keys, "删完用先取到的明文 Key 失效认证缓存")
}

func TestAdminService_AdminDeleteAPIKey_NotFound(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	repo.ownerErr = ErrAPIKeyNotFound
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	err := svc.AdminDeleteAPIKey(context.Background(), 404)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Empty(t, repo.deleted)
	require.Empty(t, cache.keys)
}

func TestAdminService_AdminDeleteAPIKey_DeleteFailureKeepsCache(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	repo.ownerKey = "sk-victim"
	repo.deleteErr = errors.New("db down")
	cache := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{apiKeyRepo: repo, authCacheInvalidator: cache}

	err := svc.AdminDeleteAPIKey(context.Background(), 42)
	require.ErrorContains(t, err, "db down")
	require.Empty(t, cache.keys, "删失败时缓存不能被清，避免「缓存已清但记录还在」")
}

func TestAdminService_AdminDeleteAPIKey_NilInvalidatorIsSafe(t *testing.T) {
	repo := newAdminOpsRepoStub(nil)
	repo.ownerKey = "sk-victim"
	svc := &adminServiceImpl{apiKeyRepo: repo}

	require.NoError(t, svc.AdminDeleteAPIKey(context.Background(), 42))
	require.Equal(t, []int64{42}, repo.deleted)
}
