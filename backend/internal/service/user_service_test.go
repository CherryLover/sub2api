//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// --- mock: UserRepository ---

type mockUserRepo struct {
	getByIDUser             *User
	getByIDErr              error
	identities              []UserAuthIdentityRecord
	unbindIdentityErr       error
	unboundProviders        []string
	updateLastActiveErr     error
	updateLastActiveUserIDs []int64
	updateLastActiveAt      []time.Time
	updateFn                func(ctx context.Context, user *User) error
	updateCalls             int
	updateFields            []UserUpdateFields
	upsertAvatarFn          func(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error)
	upsertAvatarArgs        []UpsertUserAvatarInput
	deleteAvatarFn          func(ctx context.Context, userID int64) error
	deleteAvatarIDs         []int64
	getAvatarFn             func(ctx context.Context, userID int64) (*UserAvatar, error)
	txCalls                 int
}

type mockUserRepoTxKey struct{}

type mockUserRepoTxState struct {
	getByIDUser      *User
	upsertAvatarArgs []UpsertUserAvatarInput
	deleteAvatarIDs  []int64
}

type mockUserSettingRepo struct {
	values map[string]string
}

func (m *mockUserSettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (m *mockUserSettingRepo) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (m *mockUserSettingRepo) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (m *mockUserSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := m.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (m *mockUserSettingRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (m *mockUserSettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (m *mockUserSettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (m *mockUserRepo) Create(context.Context, *User) error                    { return nil }
func (m *mockUserRepo) CreateWithEmailAliasGuard(context.Context, *User) error { return nil }
func (m *mockUserRepo) GetByID(ctx context.Context, _ int64) (*User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil && txState.getByIDUser != nil {
		cloned := *txState.getByIDUser
		return &cloned, nil
	}
	if m.getByIDUser != nil {
		cloned := *m.getByIDUser
		return &cloned, nil
	}
	return &User{}, nil
}
func (m *mockUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{}, nil }
func (m *mockUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{}, nil }
func (m *mockUserRepo) Update(ctx context.Context, user *User, fields UserUpdateFields) error {
	m.updateCalls++
	m.updateFields = append(m.updateFields, fields)
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}
func (m *mockUserRepo) Delete(context.Context, int64) error { return nil }
func (m *mockUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error) {
	if m.getAvatarFn != nil {
		return m.getAvatarFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error) {
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil {
		txState.upsertAvatarArgs = append(txState.upsertAvatarArgs, input)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = input.URL
			txState.getByIDUser.AvatarSource = input.StorageProvider
			txState.getByIDUser.AvatarMIME = input.ContentType
			txState.getByIDUser.AvatarByteSize = input.ByteSize
			txState.getByIDUser.AvatarSHA256 = input.SHA256
		}
		if m.upsertAvatarFn != nil {
			return m.upsertAvatarFn(ctx, userID, input)
		}
		return &UserAvatar{
			StorageProvider: input.StorageProvider,
			StorageKey:      input.StorageKey,
			URL:             input.URL,
			ContentType:     input.ContentType,
			ByteSize:        input.ByteSize,
			SHA256:          input.SHA256,
		}, nil
	}
	m.upsertAvatarArgs = append(m.upsertAvatarArgs, input)
	if m.upsertAvatarFn != nil {
		return m.upsertAvatarFn(ctx, userID, input)
	}
	return &UserAvatar{
		StorageProvider: input.StorageProvider,
		StorageKey:      input.StorageKey,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
	}, nil
}
func (m *mockUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	if txState, _ := ctx.Value(mockUserRepoTxKey{}).(*mockUserRepoTxState); txState != nil {
		txState.deleteAvatarIDs = append(txState.deleteAvatarIDs, userID)
		if txState.getByIDUser != nil {
			txState.getByIDUser.AvatarURL = ""
			txState.getByIDUser.AvatarSource = ""
			txState.getByIDUser.AvatarMIME = ""
			txState.getByIDUser.AvatarByteSize = 0
			txState.getByIDUser.AvatarSHA256 = ""
		}
		if m.deleteAvatarFn != nil {
			return m.deleteAvatarFn(ctx, userID)
		}
		return nil
	}
	m.deleteAvatarIDs = append(m.deleteAvatarIDs, userID)
	if m.deleteAvatarFn != nil {
		return m.deleteAvatarFn(ctx, userID)
	}
	return nil
}
func (m *mockUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) UpdateUserLastActiveAt(_ context.Context, userID int64, activeAt time.Time) error {
	m.updateLastActiveUserIDs = append(m.updateLastActiveUserIDs, userID)
	m.updateLastActiveAt = append(m.updateLastActiveAt, activeAt)
	return m.updateLastActiveErr
}
func (m *mockUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (m *mockUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (m *mockUserRepo) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return false, nil
}
func (m *mockUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (m *mockUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (m *mockUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (m *mockUserRepo) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}
func (m *mockUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error { return nil }
func (m *mockUserRepo) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	out := make([]UserAuthIdentityRecord, len(m.identities))
	copy(out, m.identities)
	return out, nil
}
func (m *mockUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (m *mockUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (m *mockUserRepo) EnableTotp(context.Context, int64) error                { return nil }
func (m *mockUserRepo) DisableTotp(context.Context, int64) error               { return nil }
func (m *mockUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (m *mockUserRepo) UnbindUserAuthProvider(_ context.Context, _ int64, provider string) error {
	if m.unbindIdentityErr != nil {
		return m.unbindIdentityErr
	}
	m.unboundProviders = append(m.unboundProviders, provider)
	filtered := m.identities[:0]
	for _, identity := range m.identities {
		if identity.ProviderType == provider {
			continue
		}
		filtered = append(filtered, identity)
	}
	m.identities = append([]UserAuthIdentityRecord(nil), filtered...)
	return nil
}

func (m *mockUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return m.GetByID(ctx, id)
}

func (m *mockUserRepo) WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.txCalls++
	txState := &mockUserRepoTxState{
		upsertAvatarArgs: append([]UpsertUserAvatarInput(nil), m.upsertAvatarArgs...),
		deleteAvatarIDs:  append([]int64(nil), m.deleteAvatarIDs...),
	}
	if m.getByIDUser != nil {
		userCopy := *m.getByIDUser
		txState.getByIDUser = &userCopy
	}
	err := fn(context.WithValue(ctx, mockUserRepoTxKey{}, txState))
	if err != nil {
		return err
	}
	m.getByIDUser = txState.getByIDUser
	m.upsertAvatarArgs = txState.upsertAvatarArgs
	m.deleteAvatarIDs = txState.deleteAvatarIDs
	return nil
}

// --- mock: APIKeyAuthCacheInvalidator ---

type mockAuthCacheInvalidator struct {
	invalidatedUserIDs []int64
	mu                 sync.Mutex
}

func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByKey(context.Context, string)    {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
}

// --- 测试 ---

func TestUnbindUserAuthProviderRejectsLastRemainingLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    9,
			Email: "only-user@linuxdo-connect.invalid",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil)

	_, err := svc.UnbindUserAuthProvider(context.Background(), 9, "linuxdo")

	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestGetProfileIdentitySummaries_DoesNotTreatOAuthOnlyCompatEmailAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "oauth-only@example.com",
			SignupSource: "oidc",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "oidc",
				ProviderKey:     "https://issuer.example.com",
				ProviderSubject: "oidc-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil)

	_, err := svc.UnbindUserAuthProvider(context.Background(), 10, "oidc")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestGetProfileIdentitySummaries_DoesNotTreatCompatBackfilledEmailIdentityAsAlternativeLoginMethod(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "oauth-only@example.com",
			SignupSource: "wechat",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "oauth-only@example.com",
				Metadata: map[string]any{
					"backfill_source": "users.email",
					"migration":       "109_auth_identity_compat_backfill",
				},
			},
			{
				ProviderType:    "wechat",
				ProviderKey:     "wechat",
				ProviderSubject: "wechat-only-subject",
			},
		},
	}
	svc := NewUserService(repo, nil, nil)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 11, repo.getByIDUser)

	require.NoError(t, err)
	require.True(t, summaries.Email.Bound)

	_, err = svc.UnbindUserAuthProvider(context.Background(), 11, "wechat")
	require.ErrorIs(t, err, ErrIdentityUnbindLastMethod)
	require.Empty(t, repo.unboundProviders)
}

func TestUnbindUserAuthProviderRemovesProviderAndReturnsUpdatedProfile(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:    12,
			Email: "alice@example.com",
		},
		identities: []UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "alice@example.com",
			},
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-12",
			},
		},
	}
	invalidator := &mockAuthCacheInvalidator{}
	svc := NewUserService(repo, nil, invalidator)

	user, err := svc.UnbindUserAuthProvider(context.Background(), 12, "linuxdo")

	require.NoError(t, err)
	require.Equal(t, []string{"linuxdo"}, repo.unboundProviders)
	require.Equal(t, int64(12), user.ID)
	require.Equal(t, []int64{12}, invalidator.invalidatedUserIDs)

	summaries, err := svc.GetProfileIdentitySummaries(context.Background(), 12, user)
	require.NoError(t, err)
	require.True(t, summaries.Email.Bound)
}

func TestTouchLastActive_UpdatesWhenStale(t *testing.T) {
	stale := time.Now().Add(-11 * time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &stale,
		},
	}
	svc := NewUserService(repo, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Equal(t, []int64{42}, repo.updateLastActiveUserIDs)
	require.Len(t, repo.updateLastActiveAt, 1)
	require.WithinDuration(t, time.Now(), repo.updateLastActiveAt[0], 2*time.Second)
}

func TestTouchLastActive_SkipsWhenRecent(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           42,
			LastActiveAt: &recent,
		},
	}
	svc := NewUserService(repo, nil, nil)

	svc.TouchLastActive(context.Background(), 42)

	require.Empty(t, repo.updateLastActiveUserIDs)
	require.Empty(t, repo.updateLastActiveAt)
}

func TestNewUserService_FieldsAssignment(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}

	svc := NewUserService(repo, nil, auth)
	require.NotNil(t, svc)
	require.Equal(t, repo, svc.userRepo)
	require.Equal(t, auth, svc.authCacheInvalidator)
}

func TestUpdateProfile_StoresInlineAvatarWithinLimit(t *testing.T) {
	raw := []byte("small-avatar")
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	expectedSum := sha256.Sum256(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       7,
			Email:    "avatar@example.com",
			Username: "avatar-user",
		},
	}
	svc := NewUserService(repo, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 7, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.NoError(t, err)
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "inline", repo.upsertAvatarArgs[0].StorageProvider)
	require.Equal(t, "image/png", repo.upsertAvatarArgs[0].ContentType)
	require.Equal(t, len(raw), repo.upsertAvatarArgs[0].ByteSize)
	require.Equal(t, hex.EncodeToString(expectedSum[:]), repo.upsertAvatarArgs[0].SHA256)
	require.Equal(t, dataURL, updated.AvatarURL)
	require.Equal(t, "inline", updated.AvatarSource)
	require.Equal(t, "image/png", updated.AvatarMIME)
	require.Equal(t, len(raw), updated.AvatarByteSize)
	require.Equal(t, hex.EncodeToString(expectedSum[:]), updated.AvatarSHA256)
}

func TestUpdateProfile_CompressesInlineAvatarToTwentyKilobytes(t *testing.T) {
	var encoded bytes.Buffer
	for _, size := range []int{192, 224, 256, 288} {
		encoded.Reset()
		var img image.RGBA
		img.Rect = image.Rect(0, 0, size, size)
		img.Stride = size * 4
		img.Pix = make([]byte, size*size*4)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				offset := y*img.Stride + x*4
				img.Pix[offset] = uint8((x*x + y*17) % 255)
				img.Pix[offset+1] = uint8((y*y + x*29) % 255)
				img.Pix[offset+2] = uint8(((x * y) + x*13 + y*7) % 255)
				img.Pix[offset+3] = 0xff
			}
		}
		require.NoError(t, png.Encode(&encoded, &img))
		if encoded.Len() > 20*1024 && encoded.Len() <= maxInlineAvatarBytes {
			break
		}
	}
	require.Greater(t, encoded.Len(), 20*1024)
	require.LessOrEqual(t, encoded.Len(), maxInlineAvatarBytes)

	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       17,
			Email:    "avatar-compress@example.com",
			Username: "avatar-compress",
		},
	}
	svc := NewUserService(repo, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 17, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.NoError(t, err)
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "inline", repo.upsertAvatarArgs[0].StorageProvider)
	require.LessOrEqual(t, repo.upsertAvatarArgs[0].ByteSize, 20*1024)
	require.Equal(t, "image/jpeg", repo.upsertAvatarArgs[0].ContentType)
	require.Contains(t, repo.upsertAvatarArgs[0].URL, "data:image/jpeg;base64,")
	require.Equal(t, "inline", updated.AvatarSource)
	require.Equal(t, "image/jpeg", updated.AvatarMIME)
	require.LessOrEqual(t, updated.AvatarByteSize, 20*1024)
	require.Contains(t, updated.AvatarURL, "data:image/jpeg;base64,")
	require.NotEmpty(t, updated.AvatarSHA256)
}

func TestUpdateProfile_RejectsInlineAvatarOverLimit(t *testing.T) {
	raw := make([]byte, maxInlineAvatarBytes+1)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       8,
			Email:    "large-avatar@example.com",
			Username: "too-large",
		},
	}
	svc := NewUserService(repo, nil, nil)

	_, err := svc.UpdateProfile(context.Background(), 8, UpdateProfileRequest{
		AvatarURL: &dataURL,
	})
	require.ErrorIs(t, err, ErrAvatarTooLarge)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Zero(t, repo.updateCalls)
}

func TestUpdateProfile_StoresRemoteAvatarURL(t *testing.T) {
	remoteURL := "https://cdn.example.com/avatar.png"
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       9,
			Email:    "remote-avatar@example.com",
			Username: "remote-avatar",
		},
	}
	svc := NewUserService(repo, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 9, UpdateProfileRequest{
		AvatarURL: &remoteURL,
	})
	require.NoError(t, err)
	require.Len(t, repo.upsertAvatarArgs, 1)
	require.Equal(t, "remote_url", repo.upsertAvatarArgs[0].StorageProvider)
	require.Equal(t, remoteURL, repo.upsertAvatarArgs[0].URL)
	require.Equal(t, remoteURL, updated.AvatarURL)
	require.Equal(t, "remote_url", updated.AvatarSource)
	require.Zero(t, updated.AvatarByteSize)
}

func TestUpdateProfile_DeletesAvatarOnEmptyString(t *testing.T) {
	empty := ""
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           10,
			Email:        "delete-avatar@example.com",
			Username:     "delete-avatar",
			AvatarURL:    "https://cdn.example.com/old.png",
			AvatarSource: "remote_url",
		},
	}
	svc := NewUserService(repo, nil, nil)

	updated, err := svc.UpdateProfile(context.Background(), 10, UpdateProfileRequest{
		AvatarURL: &empty,
	})
	require.NoError(t, err)
	require.Equal(t, []int64{10}, repo.deleteAvatarIDs)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, updated.AvatarURL)
	require.Empty(t, updated.AvatarSource)
}

func TestUpdateProfile_RollsBackAvatarMutationWhenUserUpdateFails(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:           11,
			Email:        "rollback@example.com",
			AvatarURL:    "https://cdn.example.com/original.png",
			AvatarSource: "remote_url",
		},
		updateFn: func(context.Context, *User) error {
			return errors.New("write user failed")
		},
	}
	svc := NewUserService(repo, nil, nil)

	remoteURL := "https://cdn.example.com/new.png"
	_, err := svc.UpdateProfile(context.Background(), 11, UpdateProfileRequest{
		AvatarURL: &remoteURL,
	})

	require.EqualError(t, err, "update user: write user failed")
	require.Equal(t, 1, repo.txCalls)
	require.Empty(t, repo.upsertAvatarArgs)
	require.Empty(t, repo.deleteAvatarIDs)
	require.Equal(t, "https://cdn.example.com/original.png", repo.getByIDUser.AvatarURL)
	require.Equal(t, "remote_url", repo.getByIDUser.AvatarSource)
}

func TestGetProfile_HydratesAvatarFromRepository(t *testing.T) {
	repo := &mockUserRepo{
		getByIDUser: &User{
			ID:       12,
			Email:    "profile-avatar@example.com",
			Username: "profile-avatar",
		},
		getAvatarFn: func(context.Context, int64) (*UserAvatar, error) {
			return &UserAvatar{
				StorageProvider: "remote_url",
				URL:             "https://cdn.example.com/profile.png",
			}, nil
		},
	}
	svc := NewUserService(repo, nil, nil)

	user, err := svc.GetProfile(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example.com/profile.png", user.AvatarURL)
	require.Equal(t, "remote_url", user.AvatarSource)
}
