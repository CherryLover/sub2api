package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAPIKeyHandler(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewAdminAPIKeyHandler(adminSvc)
	router.GET("/api/v1/admin/api-keys", h.List)
	router.PUT("/api/v1/admin/api-keys/:id", h.Update)
	router.DELETE("/api/v1/admin/api-keys/:id", h.Delete)
	return router
}

func TestAdminAPIKeyHandler_UpdateGroup_InvalidID(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/abc", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid API key ID")
}

func TestAdminAPIKeyHandler_UpdateGroup_InvalidJSON(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid request")
}

func TestAdminAPIKeyHandler_UpdateGroup_KeyNotFound(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// ErrAPIKeyNotFound maps to 404
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdminAPIKeyHandler_UpdateGroup_BindGroup(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	var data struct {
		APIKey struct {
			ID      int64  `json:"id"`
			GroupID *int64 `json:"group_id"`
		} `json:"api_key"`
		AutoGrantedGroupAccess bool `json:"auto_granted_group_access"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, int64(10), data.APIKey.ID)
	require.NotNil(t, data.APIKey.GroupID)
	require.Equal(t, int64(2), *data.APIKey.GroupID)
}

func TestAdminAPIKeyHandler_UpdateGroup_Unbind(t *testing.T) {
	svc := newStubAdminService()
	gid := int64(2)
	svc.apiKeys[0].GroupID = &gid
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 0}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			APIKey struct {
				GroupID *int64 `json:"group_id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Nil(t, resp.Data.APIKey.GroupID)
}

func TestAdminAPIKeyHandler_ResetRateLimitUsage(t *testing.T) {
	svc := newStubAdminService()
	now := time.Now()
	svc.apiKeys[0].Usage5h = 1.2
	svc.apiKeys[0].Usage1d = 3.4
	svc.apiKeys[0].Usage7d = 5.6
	svc.apiKeys[0].Window5hStart = &now
	svc.apiKeys[0].Window1dStart = &now
	svc.apiKeys[0].Window7dStart = &now
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"reset_rate_limit_usage":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			APIKey struct {
				Usage5h       float64    `json:"usage_5h"`
				Usage1d       float64    `json:"usage_1d"`
				Usage7d       float64    `json:"usage_7d"`
				Window5hStart *time.Time `json:"window_5h_start"`
				Window1dStart *time.Time `json:"window_1d_start"`
				Window7dStart *time.Time `json:"window_7d_start"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Zero(t, resp.Data.APIKey.Usage5h)
	require.Zero(t, resp.Data.APIKey.Usage1d)
	require.Zero(t, resp.Data.APIKey.Usage7d)
	require.Nil(t, resp.Data.APIKey.Window5hStart)
	require.Nil(t, resp.Data.APIKey.Window1dStart)
	require.Nil(t, resp.Data.APIKey.Window7dStart)
}

func TestAdminAPIKeyHandler_UpdateGroup_ServiceError(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              errors.New("internal failure"),
	}
	router := setupAPIKeyHandler(svc)
	body := `{"group_id": 2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// H2: empty body → group_id is nil → no-op, returns original key
func TestAdminAPIKeyHandler_UpdateGroup_EmptyBody_NoChange(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			APIKey struct {
				ID int64 `json:"id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, int64(10), resp.Data.APIKey.ID)
}

// M2: service returns GROUP_NOT_ACTIVE → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_GroupNotActive(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active"),
	}
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": 5}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "GROUP_NOT_ACTIVE")
}

// M2: service returns INVALID_GROUP_ID → handler maps to 400
func TestAdminAPIKeyHandler_UpdateGroup_NegativeGroupID(t *testing.T) {
	svc := &failingUpdateGroupService{
		stubAdminService: newStubAdminService(),
		err:              infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative"),
	}
	router := setupAPIKeyHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(`{"group_id": -5}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "INVALID_GROUP_ID")
}

// failingUpdateGroupService overrides AdminUpdateAPIKeyGroupID to return an error.
type failingUpdateGroupService struct {
	*stubAdminService
	err error
}

func (f *failingUpdateGroupService) AdminUpdateAPIKeyGroupID(_ context.Context, _ int64, _ *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	return nil, f.err
}

// ---------------------------------------------------------------------------
// 密钥总表：GET /api/v1/admin/api-keys
// ---------------------------------------------------------------------------

type adminAPIKeyListResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []struct {
			ID          int64    `json:"id"`
			UserID      int64    `json:"user_id"`
			Key         string   `json:"key"`
			Name        string   `json:"name"`
			Status      string   `json:"status"`
			GroupID     *int64   `json:"group_id"`
			IPWhitelist []string `json:"ip_whitelist"`
			User        *struct {
				ID       int64  `json:"id"`
				Email    string `json:"email"`
				Username string `json:"username"`
			} `json:"user"`
			Group *struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"group"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	} `json:"data"`
}

func doAdminAPIKeyList(t *testing.T, router *gin.Engine, query string) (*httptest.ResponseRecorder, adminAPIKeyListResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys"+query, nil)
	router.ServeHTTP(rec, req)
	var resp adminAPIKeyListResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	return rec, resp
}

func TestAdminAPIKeyHandler_List_MasksKeyAndCarriesOwner(t *testing.T) {
	svc := newStubAdminService()
	const plaintext = "sk-abcdef0123456789wxyz"
	gid := int64(2)
	svc.apiKeys[0].Key = plaintext
	svc.apiKeys[0].GroupID = &gid
	svc.apiKeys[0].User = &service.User{ID: 1, Email: "user@example.com", Username: "alice"}
	svc.apiKeys[0].Group = &service.Group{ID: 2, Name: "group"}
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyList(t, router, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Items, 1)

	row := resp.Data.Items[0]
	require.Equal(t, int64(10), row.ID)
	require.Equal(t, "sk-abc...wxyz", row.Key, "总表只回掩码：前 6 + 后 4")
	require.NotContains(t, rec.Body.String(), plaintext, "响应体任何位置都不能出现明文 Key")
	require.NotNil(t, row.User)
	require.Equal(t, int64(1), row.User.ID)
	require.Equal(t, "user@example.com", row.User.Email)
	require.Equal(t, "alice", row.User.Username)
	require.NotNil(t, row.Group)
	require.Equal(t, "group", row.Group.Name)
	require.EqualValues(t, 1, resp.Data.Total)
}

func TestAdminAPIKeyHandler_List_DefaultsToCreatedAtDesc(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyList(t, router, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, svc.lastListAPIKeys.calls)
	require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}, svc.lastListAPIKeys.params)
	require.Equal(t, service.AdminAPIKeyListFilters{}, svc.lastListAPIKeys.filters)
	require.Equal(t, 1, resp.Data.Page)
	require.Equal(t, 20, resp.Data.PageSize)
}

func TestAdminAPIKeyHandler_List_PassesPaginationSortAndFilters(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyList(t, router, "?page=2&page_size=5&sort_by=today_cost&sort_order=asc&user_id=1&group_id=0&status=active&search=%20tes%20")
	require.Equal(t, http.StatusOK, rec.Code)

	got := svc.lastListAPIKeys
	require.Equal(t, 2, got.params.Page)
	require.Equal(t, 5, got.params.PageSize)
	require.Equal(t, "today_cost", got.params.SortBy, "today_cost / last_used_at 等排序键原样透传给仓储")
	require.Equal(t, "asc", got.params.SortOrder)
	require.NotNil(t, got.filters.UserID)
	require.Equal(t, int64(1), *got.filters.UserID)
	require.NotNil(t, got.filters.GroupID)
	require.Equal(t, int64(0), *got.filters.GroupID, "group_id=0 表示只看未分组，不能被当成未传")
	require.Equal(t, "active", got.filters.Status)
	require.Equal(t, "tes", got.filters.Search, "搜索词两端空白要去掉")
	require.Equal(t, 2, resp.Data.Page)
	require.Equal(t, 5, resp.Data.PageSize)
}

func TestAdminAPIKeyHandler_List_LastUsedAtSortPassesThrough(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, _ := doAdminAPIKeyList(t, router, "?sort_by=last_used_at&sort_order=desc")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "last_used_at", svc.lastListAPIKeys.params.SortBy)
	require.Equal(t, "desc", svc.lastListAPIKeys.params.SortOrder)
}

func TestAdminAPIKeyHandler_List_FiltersApplied(t *testing.T) {
	svc := newStubAdminService()
	gid := int64(2)
	svc.apiKeys = append(svc.apiKeys,
		service.APIKey{ID: 11, UserID: 1, Key: "sk-other-user-key-000001", Name: "grouped", Status: service.StatusActive, GroupID: &gid},
		service.APIKey{ID: 12, UserID: 2, Key: "sk-second-user-key-00002", Name: "bob-key", Status: service.StatusAPIKeyInactive},
	)
	router := setupAPIKeyHandler(svc)

	_, byUser := doAdminAPIKeyList(t, router, "?user_id=2")
	require.Len(t, byUser.Data.Items, 1)
	require.Equal(t, int64(12), byUser.Data.Items[0].ID)

	_, ungrouped := doAdminAPIKeyList(t, router, "?group_id=0")
	ids := make([]int64, 0, len(ungrouped.Data.Items))
	for _, item := range ungrouped.Data.Items {
		ids = append(ids, item.ID)
	}
	require.ElementsMatch(t, []int64{10, 12}, ids)

	_, grouped := doAdminAPIKeyList(t, router, "?group_id=2")
	require.Len(t, grouped.Data.Items, 1)
	require.Equal(t, int64(11), grouped.Data.Items[0].ID)

	_, inactive := doAdminAPIKeyList(t, router, "?status=inactive")
	require.Len(t, inactive.Data.Items, 1)
	require.Equal(t, int64(12), inactive.Data.Items[0].ID)

	_, searched := doAdminAPIKeyList(t, router, "?search=bob")
	require.Len(t, searched.Data.Items, 1)
	require.Equal(t, int64(12), searched.Data.Items[0].ID)
}

func TestAdminAPIKeyHandler_List_InvalidUserID(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	for _, query := range []string{"?user_id=abc", "?user_id=0", "?user_id=-1"} {
		rec, _ := doAdminAPIKeyList(t, router, query)
		require.Equalf(t, http.StatusBadRequest, rec.Code, "query %s", query)
		require.Contains(t, rec.Body.String(), "Invalid user_id")
	}
	require.Zero(t, svc.lastListAPIKeys.calls)
}

func TestAdminAPIKeyHandler_List_InvalidGroupID(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	for _, query := range []string{"?group_id=abc", "?group_id=-1"} {
		rec, _ := doAdminAPIKeyList(t, router, query)
		require.Equalf(t, http.StatusBadRequest, rec.Code, "query %s", query)
		require.Contains(t, rec.Body.String(), "Invalid group_id")
	}
	require.Zero(t, svc.lastListAPIKeys.calls)
}

func TestAdminAPIKeyHandler_List_ServiceError(t *testing.T) {
	svc := newStubAdminService()
	svc.listAPIKeysErr = errors.New("db down")
	router := setupAPIKeyHandler(svc)

	rec, _ := doAdminAPIKeyList(t, router, "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestMaskAdminAPIKey(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"sk":                      "sk***",
		"sk-short":                "sk-s***",
		"sk-exactly12":            "sk-e***",
		"sk-abcdef0123456789wxyz": "sk-abc...wxyz",
		"sk-thirteenXX":           "sk-thi...enXX",
	}
	for in, want := range cases {
		require.Equalf(t, want, maskAdminAPIKey(in), "mask(%q)", in)
	}
}

// ---------------------------------------------------------------------------
// 状态 / IP 名单：PUT /api/v1/admin/api-keys/:id
// ---------------------------------------------------------------------------

type adminAPIKeyUpdateResponse struct {
	Code int `json:"code"`
	Data struct {
		APIKey struct {
			ID          int64    `json:"id"`
			Status      string   `json:"status"`
			GroupID     *int64   `json:"group_id"`
			IPWhitelist []string `json:"ip_whitelist"`
			IPBlacklist []string `json:"ip_blacklist"`
		} `json:"api_key"`
	} `json:"data"`
}

func doAdminAPIKeyUpdate(t *testing.T, router *gin.Engine, id, body string) (*httptest.ResponseRecorder, adminAPIKeyUpdateResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/"+id, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	var resp adminAPIKeyUpdateResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	return rec, resp
}

func TestAdminAPIKeyHandler_Update_StatusOnly(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"status":"inactive"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "inactive", resp.Data.APIKey.Status)
	require.Nil(t, resp.Data.APIKey.GroupID, "只改状态不能碰分组")

	require.Equal(t, 1, svc.lastAdminUpdateAPIKey.calls)
	require.Equal(t, int64(10), svc.lastAdminUpdateAPIKey.keyID)
	require.NotNil(t, svc.lastAdminUpdateAPIKey.input.Status)
	require.Equal(t, "inactive", *svc.lastAdminUpdateAPIKey.input.Status)
	require.Nil(t, svc.lastAdminUpdateAPIKey.input.IPWhitelist)
	require.Nil(t, svc.lastAdminUpdateAPIKey.input.IPBlacklist)
}

func TestAdminAPIKeyHandler_Update_ReactivateStatus(t *testing.T) {
	svc := newStubAdminService()
	svc.apiKeys[0].Status = service.StatusAPIKeyInactive
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"status":"active"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "active", resp.Data.APIKey.Status)
}

func TestAdminAPIKeyHandler_Update_InvalidStatusRejectedBeforeAnyWrite(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	for _, body := range []string{`{"status":"expired"}`, `{"status":"quota_exhausted"}`, `{"status":""}`, `{"status":"ACTIVE"}`} {
		rec, _ := doAdminAPIKeyUpdate(t, router, "10", body)
		require.Equalf(t, http.StatusBadRequest, rec.Code, "body %s", body)
	}
	require.Zero(t, svc.lastAdminUpdateAPIKey.calls, "非法状态不能落到 service")
	require.Equal(t, service.StatusActive, svc.apiKeys[0].Status)
}

func TestAdminAPIKeyHandler_Update_IPRules(t *testing.T) {
	svc := newStubAdminService()
	svc.apiKeys[0].IPBlacklist = []string{"203.0.113.9"}
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"ip_whitelist":["10.0.0.0/8","192.168.1.1"],"ip_blacklist":[]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, resp.Data.APIKey.IPWhitelist)
	require.Empty(t, resp.Data.APIKey.IPBlacklist, "空数组表示清空黑名单")

	in := svc.lastAdminUpdateAPIKey.input
	require.Nil(t, in.Status)
	require.NotNil(t, in.IPWhitelist)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, *in.IPWhitelist)
	require.NotNil(t, in.IPBlacklist)
	require.Empty(t, *in.IPBlacklist)
}

func TestAdminAPIKeyHandler_Update_IPRulesOmittedStayUntouched(t *testing.T) {
	svc := newStubAdminService()
	svc.apiKeys[0].IPWhitelist = []string{"10.0.0.1"}
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"status":"inactive"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"10.0.0.1"}, resp.Data.APIKey.IPWhitelist, "没传 ip_whitelist 就不能动它")
	require.Nil(t, svc.lastAdminUpdateAPIKey.input.IPWhitelist)
}

func TestAdminAPIKeyHandler_Update_InvalidIPPattern(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, _ := doAdminAPIKeyUpdate(t, router, "10", `{"ip_whitelist":["999.999.1.1"]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "INVALID_IP_PATTERN")
	require.Contains(t, rec.Body.String(), "999.999.1.1", "错误信息要带上出错的那条")
	require.Zero(t, svc.lastAdminUpdateAPIKey.calls)

	rec, _ = doAdminAPIKeyUpdate(t, router, "10", `{"ip_blacklist":["not-an-ip"]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "INVALID_IP_PATTERN")
	require.Zero(t, svc.lastAdminUpdateAPIKey.calls)
}

func TestAdminAPIKeyHandler_Update_InvalidIPBlocksGroupChangeToo(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, _ := doAdminAPIKeyUpdate(t, router, "10", `{"group_id":2,"ip_whitelist":["bad"]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, svc.apiKeys[0].GroupID, "IP 非法时整个请求都不能落地，分组也不能先改掉")
}

func TestAdminAPIKeyHandler_Update_GroupAndStatusTogether(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"group_id":2,"status":"inactive"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, resp.Data.APIKey.GroupID)
	require.Equal(t, int64(2), *resp.Data.APIKey.GroupID)
	require.Equal(t, "inactive", resp.Data.APIKey.Status)
	require.Equal(t, 1, svc.lastAdminUpdateAPIKey.calls)
}

func TestAdminAPIKeyHandler_Update_GroupOnlySkipsFieldUpdate(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec, resp := doAdminAPIKeyUpdate(t, router, "10", `{"group_id":2}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, resp.Data.APIKey.GroupID)
	require.Equal(t, int64(2), *resp.Data.APIKey.GroupID)
	require.Zero(t, svc.lastAdminUpdateAPIKey.calls, "老前端只传 group_id 时不该多走一次字段更新")
}

func TestAdminAPIKeyHandler_Update_StatusKeyNotFound(t *testing.T) {
	router := setupAPIKeyHandler(newStubAdminService())

	rec, _ := doAdminAPIKeyUpdate(t, router, "999", `{"status":"inactive"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// 删除：DELETE /api/v1/admin/api-keys/:id
// ---------------------------------------------------------------------------

func doAdminAPIKeyDelete(router *gin.Engine, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/api-keys/"+id, nil)
	router.ServeHTTP(rec, req)
	return rec
}

func TestAdminAPIKeyHandler_Delete_Success(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec := doAdminAPIKeyDelete(router, "10")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.NotEmpty(t, resp.Data.Message)
	require.Equal(t, []int64{10}, svc.deletedAPIKeyIDs)
	require.Empty(t, svc.apiKeys)
}

func TestAdminAPIKeyHandler_Delete_NotFound(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec := doAdminAPIKeyDelete(router, "999")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "API_KEY_NOT_FOUND")
	require.Empty(t, svc.deletedAPIKeyIDs)
}

func TestAdminAPIKeyHandler_Delete_InvalidID(t *testing.T) {
	svc := newStubAdminService()
	router := setupAPIKeyHandler(svc)

	rec := doAdminAPIKeyDelete(router, "abc")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Invalid API key ID")
	require.Empty(t, svc.deletedAPIKeyIDs)
}

func TestAdminAPIKeyHandler_Delete_ServiceError(t *testing.T) {
	svc := newStubAdminService()
	svc.deleteAPIKeyErr = errors.New("db down")
	router := setupAPIKeyHandler(svc)

	rec := doAdminAPIKeyDelete(router, "10")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
