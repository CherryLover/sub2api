package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// adminAPIKeyLister 是密钥总表用到的窄仓储接口。
// 不直接扩 APIKeyRepository：那样所有实现它的测试桩都得补方法；用类型断言，
// 只有真实仓储实现，桩不支持时返回明确错误（与 apiKeyAllByUserIDLister 同一思路）。
type adminAPIKeyLister interface {
	ListAllForAdmin(ctx context.Context, params pagination.PaginationParams, filters AdminAPIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
}

// AdminUpdateAPIKeyInput 管理员可改的 Key 字段。nil 一律表示不改；IP 名单传空切片表示清空。
type AdminUpdateAPIKeyInput struct {
	Status      *string   // active / inactive
	IPWhitelist *[]string // nil 不改，空数组清空
	IPBlacklist *[]string // nil 不改，空数组清空
}

// IsEmpty 报告本次是否没有任何字段要改。
func (in AdminUpdateAPIKeyInput) IsEmpty() bool {
	return in.Status == nil && in.IPWhitelist == nil && in.IPBlacklist == nil
}

// ErrInvalidAPIKeyStatus 管理员只能把 Key 置为 active 或 inactive；
// expired / quota_exhausted 由系统按到期与额度自动打上，不允许手工写入。
var ErrInvalidAPIKeyStatus = infraerrors.BadRequest("INVALID_API_KEY_STATUS", "status must be active or inactive")

// ValidateAdminAPIKeyStatus 校验管理员写入的状态值是否合法。
func ValidateAdminAPIKeyStatus(status string) error {
	switch status {
	case StatusAPIKeyActive, StatusAPIKeyInactive:
		return nil
	default:
		return ErrInvalidAPIKeyStatus
	}
}

// ValidateAPIKeyIPRules 校验 IP 名单里的每一条是否是合法的 IP / CIDR / 通配模式，
// 非法时返回 ErrInvalidIPPattern 并把出错的条目带在错误信息里。
func ValidateAPIKeyIPRules(patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	if invalid := ip.ValidateIPPatterns(patterns); len(invalid) > 0 {
		joined := strings.Join(invalid, ", ")
		// 同 code + reason，errors.Is(err, ErrInvalidIPPattern) 仍成立；message 和 metadata 把出错条目带给前端。
		return infraerrors.BadRequest(ErrInvalidIPPattern.Reason, "invalid IP or CIDR pattern: "+joined).
			WithMetadata(map[string]string{"invalid": joined})
	}
	return nil
}

// AdminListAPIKeys 跨用户分页列出全站 API Key（带 User / Group），供管理端密钥总表使用。
// 分页、排序、筛选都透传给仓储；返回的是明文 Key，掩码由 handler 在出口处做。
func (s *adminServiceImpl) AdminListAPIKeys(ctx context.Context, params pagination.PaginationParams, filters AdminAPIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	lister, ok := s.apiKeyRepo.(adminAPIKeyLister)
	if !ok {
		return nil, nil, fmt.Errorf("list api keys for admin: repository does not support cross-user listing")
	}
	if filters.UserID != nil && *filters.UserID < 0 {
		return nil, nil, infraerrors.BadRequest("INVALID_USER_ID", "user_id must be non-negative")
	}
	if filters.GroupID != nil && *filters.GroupID < 0 {
		return nil, nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative")
	}

	keys, result, err := lister.ListAllForAdmin(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list api keys: %w", err)
	}
	return keys, result, nil
}

// AdminUpdateAPIKey 管理员改某把 Key 的状态与 IP 名单。
// 只写显式给出的列：改 status 就只写 status，不复制用户侧「扩容自动复活」那套逻辑；
// IP 规则在认证路径按需编译，写完失效认证缓存即可让新名单立刻生效。
func (s *adminServiceImpl) AdminUpdateAPIKey(ctx context.Context, keyID int64, input AdminUpdateAPIKeyInput) (*APIKey, error) {
	if input.Status != nil {
		if err := ValidateAdminAPIKeyStatus(*input.Status); err != nil {
			return nil, err
		}
	}
	if input.IPWhitelist != nil {
		if err := ValidateAPIKeyIPRules(*input.IPWhitelist); err != nil {
			return nil, err
		}
	}
	if input.IPBlacklist != nil {
		if err := ValidateAPIKeyIPRules(*input.IPBlacklist); err != nil {
			return nil, err
		}
	}

	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if input.IsEmpty() {
		return apiKey, nil
	}

	var fields APIKeyUpdateFields
	if input.Status != nil {
		apiKey.Status = *input.Status
		fields.Status = true
	}
	if input.IPWhitelist != nil {
		apiKey.IPWhitelist = *input.IPWhitelist
		fields.IPRules = true
	}
	if input.IPBlacklist != nil {
		apiKey.IPBlacklist = *input.IPBlacklist
		fields.IPRules = true
	}

	if err := s.apiKeyRepo.Update(ctx, apiKey, fields); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}

	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	}
	return apiKey, nil
}

// AdminDeleteAPIKey 管理员删除任意用户的 Key，不做归属校验。
// 顺序固定为：先取明文 Key → 软删 + 墓碑 → 再失效认证缓存；
// 删失败时缓存还在，不会出现「缓存已清但记录还在」的空窗。
func (s *adminServiceImpl) AdminDeleteAPIKey(ctx context.Context, keyID int64) error {
	key, _, err := s.apiKeyRepo.GetKeyAndOwnerID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("get api key: %w", err)
	}

	if err := s.apiKeyRepo.DeleteWithAudit(ctx, keyID); err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}

	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key)
	}
	return nil
}
