package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService service.AdminService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService: adminService,
	}
}

// adminAPIKeySearchMaxLen 与用户侧列表一致，超长搜索词截断而不是报错。
const adminAPIKeySearchMaxLen = 100

// List 跨用户列出全站 API Key，供管理端密钥总表使用。
// GET /api/v1/admin/api-keys?page&page_size&sort_by&sort_order&user_id&group_id&status&search
//
// 返回结构与 GET /api/v1/keys 相同，但每行 key 只给掩码（前 6 + 后 4）——
// 总表是跨用户读取，不回显任何人的明文 Key；要查用量直接跳 /admin/usage?api_key_id=。
func (h *AdminAPIKeyHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	var filters service.AdminAPIKeyListFilters
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > adminAPIKeySearchMaxLen {
			search = search[:adminAPIKeySearchMaxLen]
		}
		filters.Search = search
	}
	filters.Status = strings.TrimSpace(c.Query("status"))
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filters.UserID = &userID
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID < 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filters.GroupID = &groupID
	}

	keys, result, err := h.adminService.AdminListAPIKeys(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		item := dto.APIKeyFromService(&keys[i])
		item.Key = maskAdminAPIKey(item.Key)
		out = append(out, *item)
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

// maskAdminAPIKey 把 Key 串打成掩码，规则与前端 utils/maskApiKey.ts 完全一致：
// 长度 > 12 显示前 6 + "..." + 后 4；≤ 12 的短串只露前 4 位加 "***"。
func maskAdminAPIKey(key string) string {
	if key == "" {
		return ""
	}
	runes := []rune(key)
	if len(runes) <= 12 {
		head := runes
		if len(head) > 4 {
			head = head[:4]
		}
		return string(head) + "***"
	}
	return string(runes[:6]) + "..." + string(runes[len(runes)-4:])
}

// AdminUpdateAPIKeyRequest represents the request to update an API key.
type AdminUpdateAPIKeyRequest struct {
	GroupID             *int64    `json:"group_id"`               // nil=不修改, 0=解绑, >0=绑定到目标分组
	ResetRateLimitUsage *bool     `json:"reset_rate_limit_usage"` // true=重置 5h/1d/7d 限速用量
	Status              *string   `json:"status"`                 // nil=不修改，只认 active / inactive
	IPWhitelist         *[]string `json:"ip_whitelist"`           // nil 不修改，空数组清空
	IPBlacklist         *[]string `json:"ip_blacklist"`           // nil 不修改，空数组清空
}

// Update handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
//
// 顺序：先把所有输入校验完（状态枚举、IP 规则），再依次做重置限速用量 → 改分组 → 改状态/IP 名单，
// 这样非法输入不会在改了一半之后才报 400。响应结构与只改分组时相同。
func (h *AdminAPIKeyHandler) Update(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	fieldInput := service.AdminUpdateAPIKeyInput{
		Status:      req.Status,
		IPWhitelist: req.IPWhitelist,
		IPBlacklist: req.IPBlacklist,
	}
	if req.Status != nil {
		if err := service.ValidateAdminAPIKeyStatus(*req.Status); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if req.IPWhitelist != nil {
		if err := service.ValidateAPIKeyIPRules(*req.IPWhitelist); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if req.IPBlacklist != nil {
		if err := service.ValidateAPIKeyIPRules(*req.IPBlacklist); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	if !fieldInput.IsEmpty() {
		updated, err := h.adminService.AdminUpdateAPIKey(c.Request.Context(), keyID, fieldInput)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		result.APIKey = updated
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}

// Delete handles deleting any user's API key.
// DELETE /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) Delete(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	if err := h.adminService.AdminDeleteAPIKey(c.Request.Context(), keyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "API key deleted successfully"})
}
