package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/sync/singleflight"
)

var (
	ErrUserNotFound             = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect        = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms        = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
	ErrAvatarInvalid            = infraerrors.BadRequest("AVATAR_INVALID", "avatar must be a valid image data URL or http(s) URL")
	ErrAvatarTooLarge           = infraerrors.BadRequest("AVATAR_TOO_LARGE", "avatar image must be 100KB or smaller")
	ErrAvatarNotImage           = infraerrors.BadRequest("AVATAR_NOT_IMAGE", "avatar content must be an image")
	ErrIdentityProviderInvalid  = infraerrors.BadRequest("IDENTITY_PROVIDER_INVALID", "identity provider is invalid")
	ErrIdentityUnbindLastMethod = infraerrors.Conflict(
		"IDENTITY_UNBIND_LAST_METHOD",
		"bind another sign-in method before unbinding this provider",
	)
)

const (
	maxInlineAvatarBytes = 100 * 1024
	targetAvatarBytes    = 20 * 1024

	userLastActiveMinTouch    = 10 * time.Minute
	userLastActiveFailBackoff = 30 * time.Second
)

var (
	avatarScaleSteps   = []float64{1, 0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36}
	avatarQualitySteps = []int{88, 80, 72, 64, 56, 48, 40, 32}
)

// UserListFilters contains all filter options for listing users
type UserListFilters struct {
	Status    string // User status filter
	Role      string // User role filter
	Search    string // Search in email, username
	GroupName string // Filter by allowed group name (fuzzy match)
	// APIKeyGroupID filters users who own at least one non-soft-deleted API key
	// bound to this group (api_keys.group_id). 0 = no filter. Covers all three
	// group types since it matches the key's group directly, not allowed_groups.
	APIKeyGroupID int64
	Attributes    map[int64]string // Custom attribute filters: attributeID -> value
	// IncludeDeleted 为 true 时绕过软删除过滤，返回含已删除（deleted_at 非空）的用户。
	// 仅供 /admin/usage 的 SearchUsers 端点使用，其他列表调用方不要设置。
	IncludeDeleted bool
}

// UserUpdateFields 声明 UserRepository.Update 允许写回的列。
//
// 未声明的列保持数据库当前值，不会被调用方手里的快照覆盖。用户行上有多条
// 不经过 Update 的原子写入路径（UpdateConcurrency、BatchUpdateLimits、
// UpdateUserLastActiveAt 等），status/role 也可能被其他流程并发改写。
// 若 Update 无条件整行回写，一次"读-改-写"就会静默回滚这些并发结果（lost update），
// 因此每个调用方必须显式声明它真正要改的列。
type UserUpdateFields struct {
	Email        bool
	Username     bool
	Notes        bool
	PasswordHash bool
	Role         bool
	Status       bool
	Concurrency  bool
	RPMLimit     bool
	SignupSource bool
	LastLoginAt  bool
	LastActiveAt bool
	// AllowedGroups 为 true 时才同步 user_allowed_groups 关联表。
	AllowedGroups bool
}

// IsEmpty 报告该次 Update 是否不写任何列（此时仓储直接返回，不产生写操作）。
func (f UserUpdateFields) IsEmpty() bool {
	return f == UserUpdateFields{}
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	// CreateWithEmailAliasGuard 创建用户，并在邮箱唯一性锁内复查"收件箱身份"是否已被占用
	// （+别名 / Gmail 点号 / FQDN 根点变体，见 NormalizeEmailForAliasDedup），
	// 冲突时返回 ErrEmailExists。仅注册路径使用：同一收件箱的多个别名变体并发注册时，
	// 服务层的前置查重会同时通过，必须由这里串行化兜底。管理员建号仍走 Create，不受限制。
	CreateWithEmailAliasGuard(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	// GetByIDIncludeDeleted 绕过软删除过滤按 ID 取用户（含已删）。仅供管理员审计/usage 点击使用。
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetFirstAdmin(ctx context.Context) (*User, error)
	// Update 只写 fields 中显式声明的列，其余列保持库中当前值。
	Update(ctx context.Context, user *User, fields UserUpdateFields) error
	Delete(ctx context.Context, id int64) error
	GetUserAvatar(ctx context.Context, userID int64) (*UserAvatar, error)
	UpsertUserAvatar(ctx context.Context, userID int64, input UpsertUserAvatarInput) (*UserAvatar, error)
	DeleteUserAvatar(ctx context.Context, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)
	GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error)
	GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error)
	UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error

	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	BatchSetConcurrency(ctx context.Context, userIDs []int64, value int) (int, error)
	BatchAddConcurrency(ctx context.Context, userIDs []int64, delta int) (int, error)
	BatchUpdateLimits(ctx context.Context, userIDs []int64, concurrency, rpmLimit *int) (int, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	// ExistsByEmailAlias 判断是否已有账号与该邮箱指向同一收件箱（+别名 / Gmail 点号 /
	// FQDN 根点变体，见 NormalizeEmailForAliasDedup）。用于注册与发送验证码前的查重。
	ExistsByEmailAlias(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups 将指定分组增量添加到用户的 allowed_groups（幂等，冲突忽略）
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	// RemoveGroupFromUserAllowedGroups 移除单个用户的指定分组权限
	RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	ListUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error)
	UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
}

// RegistrationEmailDomainRepository 是生产用户仓储为非白名单域名单账户兜底策略提供的可选能力。
// 它独立于 UserRepository，避免无关测试桩和服务消费者实现注册专用方法。
type RegistrationEmailDomainRepository interface {
	CountUsersByEmailDomain(ctx context.Context, domain string) (int, error)
	CreateWithEmailAliasGuardAndDomainLimit(ctx context.Context, user *User, domain string) error
}

type UserAuthIdentityRecord struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
	VerifiedAt      *time.Time
	Issuer          *string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserIdentitySummary struct {
	Provider    string     `json:"provider"`
	Bound       bool       `json:"bound"`
	BoundCount  int        `json:"bound_count"`
	DisplayName string     `json:"display_name,omitempty"`
	AvatarURL   string     `json:"-"`
	SubjectHint string     `json:"subject_hint,omitempty"`
	ProviderKey string     `json:"provider_key,omitempty"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	CanBind     bool       `json:"can_bind"`
	CanUnbind   bool       `json:"can_unbind"`
	NoteKey     string     `json:"note_key,omitempty"`
	Note        string     `json:"note,omitempty"`
}

// UserIdentitySummarySet 只保留 email 绑定摘要：第三方 OAuth 登录已删除，
// 历史第三方身份行仍可通过 DELETE /user/account-bindings/:provider 解绑。
type UserIdentitySummarySet struct {
	Email UserIdentitySummary `json:"email"`
}

const userIdentityNoteEmailManagedFromProfile = "profile.authBindings.notes.emailManagedFromProfile"

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email       *string `json:"email"`
	Username    *string `json:"username"`
	AvatarURL   *string `json:"avatar_url"`
	Concurrency *int    `json:"concurrency"`
}

type UserAvatar struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

type UpsertUserAvatarInput struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

type userProfileIdentityTxRunner interface {
	WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UserService 用户服务
type UserService struct {
	userRepo             UserRepository
	settingRepo          SettingRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	lastActiveTouchL1    sync.Map
	lastActiveTouchSF    singleflight.Group
}

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository, settingRepo SettingRepository, authCacheInvalidator APIKeyAuthCacheInvalidator) *UserService {
	return &UserService{
		userRepo:             userRepo,
		settingRepo:          settingRepo,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
	}
	return admin, nil
}

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	normalizeLoadedUserTokenVersion(user)
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
	}
	return user, nil
}

func (s *UserService) GetProfileIdentitySummaries(ctx context.Context, userID int64, user *User) (UserIdentitySummarySet, error) {
	if user == nil {
		var err error
		user, err = s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return UserIdentitySummarySet{}, fmt.Errorf("get user: %w", err)
		}
	}

	records, err := s.listUserAuthIdentities(ctx, userID)
	if err != nil {
		return UserIdentitySummarySet{}, err
	}

	return UserIdentitySummarySet{
		Email: s.buildEmailIdentitySummary(user, records),
	}, nil
}

func (s *UserService) UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) (*User, error) {
	user, _, err := s.UnbindUserAuthProviderWithResult(ctx, userID, provider)
	return user, err
}

func (s *UserService) UnbindUserAuthProviderWithResult(ctx context.Context, userID int64, provider string) (*User, bool, error) {
	provider = normalizeUserIdentityProvider(provider)
	if provider == "" || provider == "email" {
		return nil, false, ErrIdentityProviderInvalid
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("get user: %w", err)
	}

	records, err := s.listUserAuthIdentities(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if len(filterUserAuthIdentities(records, provider)) == 0 {
		return user, false, nil
	}
	if !s.canUnbindProvider(provider, user, records) {
		return nil, false, ErrIdentityUnbindLastMethod
	}

	if err := s.userRepo.UnbindUserAuthProvider(ctx, userID, provider); err != nil {
		return nil, false, err
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	updatedUser, err := s.GetProfile(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return updatedUser, true, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	if txRunner, ok := s.userRepo.(userProfileIdentityTxRunner); ok {
		var (
			updated        *User
			oldConcurrency int
		)
		if err := txRunner.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
			var err error
			updated, oldConcurrency, err = s.updateProfile(txCtx, userID, req)
			return err
		}); err != nil {
			return nil, err
		}
		if s.authCacheInvalidator != nil && updated != nil && updated.Concurrency != oldConcurrency {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		return updated, nil
	}

	updated, oldConcurrency, err := s.updateProfile(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if s.authCacheInvalidator != nil && updated.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return updated, nil
}

func (s *UserService) updateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, int, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("get user: %w", err)
	}
	oldConcurrency := user.Concurrency

	// fields 只登记本次请求真正带上的字段。余额、状态等列不由这里回写，
	// 否则并发的扣费与状态变更会被这份快照回滚。
	var fields UserUpdateFields

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, oldConcurrency, fmt.Errorf("check email exists: %w", err)
		}
		if exists && *req.Email != user.Email {
			return nil, oldConcurrency, ErrEmailExists
		}
		user.Email = *req.Email
		fields.Email = true
	}

	if req.Username != nil {
		user.Username = *req.Username
		fields.Username = true
	}

	if req.AvatarURL != nil {
		avatar, err := s.SetAvatar(ctx, userID, *req.AvatarURL)
		if err != nil {
			return nil, oldConcurrency, err
		}
		applyUserAvatar(user, avatar)
	}

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
		fields.Concurrency = true
	}

	if err := s.userRepo.Update(ctx, user, fields); err != nil {
		return nil, oldConcurrency, fmt.Errorf("update user: %w", err)
	}

	return user, oldConcurrency, nil
}

func (s *UserService) SetAvatar(ctx context.Context, userID int64, raw string) (*UserAvatar, error) {
	avatarValue := strings.TrimSpace(raw)
	if avatarValue == "" {
		if err := s.userRepo.DeleteUserAvatar(ctx, userID); err != nil {
			return nil, fmt.Errorf("delete avatar: %w", err)
		}
		return nil, nil
	}

	avatarInput, err := normalizeUserAvatarInput(avatarValue)
	if err != nil {
		return nil, err
	}

	avatar, err := s.userRepo.UpsertUserAvatar(ctx, userID, avatarInput)
	if err != nil {
		return nil, fmt.Errorf("upsert avatar: %w", err)
	}
	return avatar, nil
}

func applyUserAvatar(user *User, avatar *UserAvatar) {
	if user == nil {
		return
	}
	if avatar == nil {
		user.AvatarURL = ""
		user.AvatarSource = ""
		user.AvatarMIME = ""
		user.AvatarByteSize = 0
		user.AvatarSHA256 = ""
		return
	}

	user.AvatarURL = avatar.URL
	user.AvatarSource = avatar.StorageProvider
	user.AvatarMIME = avatar.ContentType
	user.AvatarByteSize = avatar.ByteSize
	user.AvatarSHA256 = avatar.SHA256
}

func normalizeUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if strings.HasPrefix(raw, "data:") {
		return normalizeInlineUserAvatarInput(raw)
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}

	return UpsertUserAvatarInput{
		StorageProvider: "remote_url",
		URL:             raw,
	}, nil
}

func ValidateUserAvatar(raw string) error {
	_, err := normalizeUserAvatarInput(raw)
	return err
}

func normalizeInlineUserAvatarInput(raw string) (UpsertUserAvatarInput, error) {
	body := strings.TrimPrefix(raw, "data:")
	meta, encoded, ok := strings.Cut(body, ",")
	if !ok {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	meta = strings.TrimSpace(meta)
	encoded = strings.TrimSpace(encoded)
	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}

	contentType := strings.TrimSpace(meta[:len(meta)-len(";base64")])
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return UpsertUserAvatarInput{}, ErrAvatarNotImage
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return UpsertUserAvatarInput{}, ErrAvatarInvalid
	}
	if len(decoded) > maxInlineAvatarBytes {
		return UpsertUserAvatarInput{}, ErrAvatarTooLarge
	}

	if len(decoded) > targetAvatarBytes {
		decoded, contentType, err = compressInlineAvatar(decoded)
		if err != nil {
			return UpsertUserAvatarInput{}, err
		}
		raw = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(decoded)
	}

	sum := sha256.Sum256(decoded)
	return UpsertUserAvatarInput{
		StorageProvider: "inline",
		URL:             raw,
		ContentType:     contentType,
		ByteSize:        len(decoded),
		SHA256:          hex.EncodeToString(sum[:]),
	}, nil
}

func compressInlineAvatar(decoded []byte) ([]byte, string, error) {
	src, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil, "", ErrAvatarInvalid
	}

	srcBounds := src.Bounds()
	if srcBounds.Empty() {
		return nil, "", ErrAvatarInvalid
	}

	for _, scale := range avatarScaleSteps {
		width := max(1, int(float64(srcBounds.Dx())*scale))
		height := max(1, int(float64(srcBounds.Dy())*scale))
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, stddraw.Over, nil)

		for _, quality := range avatarQualitySteps {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
				return nil, "", ErrAvatarInvalid
			}
			if buf.Len() <= targetAvatarBytes {
				return buf.Bytes(), "image/jpeg", nil
			}
		}
	}

	return nil, "", ErrAvatarTooLarge
}

func (s *UserService) buildEmailIdentitySummary(user *User, records []UserAuthIdentityRecord) UserIdentitySummary {
	summary := UserIdentitySummary{
		Provider:  "email",
		CanBind:   false,
		CanUnbind: false,
		NoteKey:   userIdentityNoteEmailManagedFromProfile,
		Note:      "Primary account email is managed from the profile form.",
	}
	if user == nil {
		return summary
	}

	filtered := filterUserAuthIdentities(records, "email")
	if len(filtered) > 0 {
		primary := selectPrimaryUserAuthIdentity(filtered)
		email := strings.TrimSpace(firstStringIdentityValue(primary.Metadata, "email"))
		if email == "" {
			email = strings.TrimSpace(primary.ProviderSubject)
		}
		if email == "" || isReservedEmail(email) {
			email = strings.TrimSpace(user.Email)
		}
		if email == "" || isReservedEmail(email) {
			email = strings.TrimSpace(primary.ProviderKey)
		}

		summary.Bound = true
		summary.BoundCount = len(filtered)
		summary.DisplayName = email
		summary.SubjectHint = maskEmailIdentity(email)
		summary.ProviderKey = strings.TrimSpace(primary.ProviderKey)
		summary.VerifiedAt = primary.VerifiedAt
		return summary
	}

	// Compatibility fallback for legacy normal-email users that predate auth_identities backfill.
	email := strings.TrimSpace(user.Email)
	if email == "" || isReservedEmail(email) {
		return summary
	}
	summary.Bound = true
	summary.BoundCount = 1
	summary.DisplayName = email
	summary.SubjectHint = maskEmailIdentity(email)
	summary.ProviderKey = "email"
	return summary
}

func (s *UserService) canUnbindProvider(provider string, user *User, records []UserAuthIdentityRecord) bool {
	if provider == "" || provider == "email" || len(filterUserAuthIdentities(records, provider)) == 0 {
		return false
	}

	if s.canUseEmailAsSignInMethod(user, records) {
		return true
	}

	for _, candidate := range []string{"linuxdo", "oidc", "wechat", "dingtalk"} {
		if candidate == provider {
			continue
		}
		if len(filterUserAuthIdentities(records, candidate)) > 0 {
			return true
		}
	}

	return false
}

func (s *UserService) canUseEmailAsSignInMethod(user *User, records []UserAuthIdentityRecord) bool {
	if user == nil {
		return false
	}

	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" || isReservedEmail(email) {
		return false
	}

	if emailSignupSourceAllowsLogin(user.SignupSource) {
		return true
	}

	for _, record := range filterUserAuthIdentities(records, "email") {
		if emailIdentitySupportsSignIn(record) {
			return true
		}
	}

	return false
}

func emailSignupSourceAllowsLogin(signupSource string) bool {
	signupSource = strings.ToLower(strings.TrimSpace(signupSource))
	return signupSource == "" || signupSource == "email"
}

func emailIdentitySupportsSignIn(record UserAuthIdentityRecord) bool {
	source := strings.TrimSpace(firstStringIdentityValue(record.Metadata, "source"))
	switch source {
	case "auth_service_email_bind", "auth_service_login_backfill", "auth_service_dual_write":
		return true
	default:
		return false
	}
}

func (s *UserService) listUserAuthIdentities(ctx context.Context, userID int64) ([]UserAuthIdentityRecord, error) {
	if userID <= 0 || s == nil || s.userRepo == nil {
		return nil, nil
	}
	return s.userRepo.ListUserAuthIdentities(ctx, userID)
}

func normalizeUserIdentityProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "linuxdo":
		return "linuxdo"
	case "oidc":
		return "oidc"
	case "wechat":
		return "wechat"
	case "dingtalk":
		return "dingtalk"
	case "email":
		return "email"
	default:
		return ""
	}
}

func filterUserAuthIdentities(records []UserAuthIdentityRecord, provider string) []UserAuthIdentityRecord {
	if len(records) == 0 {
		return nil
	}
	filtered := make([]UserAuthIdentityRecord, 0, len(records))
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.ProviderType), provider) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func selectPrimaryUserAuthIdentity(records []UserAuthIdentityRecord) UserAuthIdentityRecord {
	if len(records) == 0 {
		return UserAuthIdentityRecord{}
	}
	sort.SliceStable(records, func(i, j int) bool {
		left := userAuthIdentitySortTime(records[i])
		right := userAuthIdentitySortTime(records[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return records[i].ProviderKey < records[j].ProviderKey
	})
	return records[0]
}

func userAuthIdentitySortTime(record UserAuthIdentityRecord) time.Time {
	if record.VerifiedAt != nil && !record.VerifiedAt.IsZero() {
		return record.VerifiedAt.UTC()
	}
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt.UTC()
	}
	if !record.CreatedAt.IsZero() {
		return record.CreatedAt.UTC()
	}
	return time.Time{}
}

func firstStringIdentityValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func maskEmailIdentity(email string) string {
	local, domain, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" || domain == "" {
		return maskOpaqueIdentity(email)
	}
	runes := []rune(local)
	if len(runes) == 1 {
		return string(runes[0]) + "***@" + domain
	}
	return string(runes[0]) + "***" + string(runes[len(runes)-1]) + "@" + domain
}

func maskOpaqueIdentity(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	switch {
	case len(runes) == 0:
		return ""
	case len(runes) <= 4:
		return string(runes[0]) + "***"
	case len(runes) <= 8:
		return string(runes[:2]) + "***" + string(runes[len(runes)-1:])
	default:
		return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
	}
}

// ChangePassword 修改密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// 验证当前密码
	if !user.CheckPassword(req.CurrentPassword) {
		return ErrPasswordIncorrect
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	// Increment TokenVersion to invalidate all existing tokens
	// This ensures that any tokens issued before the password change become invalid
	user.TokenVersion++

	// TokenVersion 没有对应的数据库列（见 resolvedTokenVersion：它由 email+password_hash
	// 指纹推导），改密写回 password_hash 即可让旧 token 失效。
	if err := s.userRepo.Update(ctx, user, UserUpdateFields{PasswordHash: true}); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	normalizeLoadedUserTokenVersion(user)
	if err := s.hydrateUserAvatar(ctx, user); err != nil {
		return nil, fmt.Errorf("get user avatar: %w", err)
	}
	return user, nil
}

func normalizeLoadedUserTokenVersion(user *User) {
	if user == nil || user.TokenVersionResolved {
		return
	}
	user.TokenVersion = resolvedTokenVersion(user)
	user.TokenVersionResolved = true
}

// TouchLastActive 通过防抖更新 users.last_active_at，减少鉴权热路径写放大。
// 该操作为尽力而为，不应中断正常请求。
func (s *UserService) TouchLastActive(ctx context.Context, userID int64) {
	if s == nil || s.userRepo == nil || userID <= 0 {
		return
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		slog.Debug("skip touch user last active after load failure", "user_id", userID, "error", err)
		return
	}
	s.TouchLastActiveForUser(ctx, user)
}

// TouchLastActiveForUser 使用已加载的用户信息更新 last_active_at，避免重复读取数据库。
func (s *UserService) TouchLastActiveForUser(ctx context.Context, user *User) {
	if s == nil || s.userRepo == nil || user == nil || user.ID <= 0 {
		return
	}

	now := time.Now()
	if userLastActiveFresh(user.LastActiveAt, now) {
		return
	}
	if v, ok := s.lastActiveTouchL1.Load(user.ID); ok {
		if nextAllowedAt, ok := v.(time.Time); ok && now.Before(nextAllowedAt) {
			return
		}
	}

	_, err, _ := s.lastActiveTouchSF.Do(strconv.FormatInt(user.ID, 10), func() (any, error) {
		latest := time.Now()
		if v, ok := s.lastActiveTouchL1.Load(user.ID); ok {
			if nextAllowedAt, ok := v.(time.Time); ok && latest.Before(nextAllowedAt) {
				return nil, nil
			}
		}
		if userLastActiveFresh(user.LastActiveAt, latest) {
			return nil, nil
		}
		if err := s.userRepo.UpdateUserLastActiveAt(ctx, user.ID, latest); err != nil {
			s.lastActiveTouchL1.Store(user.ID, latest.Add(userLastActiveFailBackoff))
			return nil, fmt.Errorf("touch user last active: %w", err)
		}
		s.lastActiveTouchL1.Store(user.ID, latest.Add(userLastActiveMinTouch))
		return nil, nil
	})
	if err != nil {
		slog.Warn("touch user last active failed", "user_id", user.ID, "error", err)
	}
}

func userLastActiveFresh(lastActiveAt *time.Time, now time.Time) bool {
	if lastActiveAt == nil {
		return false
	}
	return now.Before(lastActiveAt.Add(userLastActiveMinTouch))
}

func (s *UserService) hydrateUserAvatar(ctx context.Context, user *User) error {
	if s == nil || s.userRepo == nil || user == nil || user.ID == 0 {
		return nil
	}

	avatar, err := s.userRepo.GetUserAvatar(ctx, user.ID)
	if err != nil {
		return err
	}
	applyUserAvatar(user, avatar)
	return nil
}

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	users, pagination, err := s.userRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	return users, pagination, nil
}

// UpdateConcurrency 更新用户并发数（管理员功能）
func (s *UserService) UpdateConcurrency(ctx context.Context, userID int64, concurrency int) error {
	if err := s.userRepo.UpdateConcurrency(ctx, userID, concurrency); err != nil {
		return fmt.Errorf("update concurrency: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return nil
}

// UpdateStatus 更新用户状态（管理员功能）
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	user.Status = status

	if err := s.userRepo.Update(ctx, user, UserUpdateFields{Status: true}); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	return nil
}

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
