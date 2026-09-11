//go:build unit

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const accountRuntimeBlocksClearPath = "/api/v1/admin/accounts/runtime-blocks/clear"

func setupRuntimeBlocksClearRouter(gateway *service.OpenAIGatewayService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := &AccountHandler{}
	if gateway != nil {
		handler.SetOpenAIGatewayService(gateway)
	}
	router := gin.New()
	router.POST(accountRuntimeBlocksClearPath, handler.ClearRuntimeBlocks)
	return router
}

func postRuntimeBlocksClear(router *gin.Engine, body string, withBody bool) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var reader io.Reader
	if withBody {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, accountRuntimeBlocksClearPath, reader)
	if withBody {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(rec, req)
	return rec
}

// decodeRuntimeBlocksClear 解成通用 map，用来钉死 JSON 字段名本身。
// 前端是照契约并行对接的，字段名改了必须当场挂测试，而不是等到联调才发现。
func decodeRuntimeBlocksClear(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	require.Equal(t, 0, env.Code)
	require.NotNil(t, env.Data)
	return env.Data
}

func requireRuntimeBlocksClearCountKeys(t *testing.T, data map[string]any) {
	t.Helper()
	cleared, ok := data["cleared"].(map[string]any)
	require.True(t, ok, "cleared 必须是对象: %#v", data["cleared"])
	for _, key := range []string{
		"account_runtime_blocks",
		"model_transient_cooldowns",
		"proxy_quarantines",
		"grok_model_quota_blocks",
		"grok_team_rate_limits",
		"grok_free_quota_gates",
	} {
		require.Contains(t, cleared, key)
	}
	require.Contains(t, data, "total_cleared")
}

// 运维在终端里 `curl -X POST` 救场时不会带 body，这必须等价于「全部账号」。
// ShouldBindJSON 对空 body 会返回 EOF，所以这条路径是单独处理的，值得单独钉住。
func TestClearRuntimeBlocks_NoBodyMeansAllAccounts(t *testing.T) {
	router := setupRuntimeBlocksClearRouter(&service.OpenAIGatewayService{})

	data := decodeRuntimeBlocksClear(t, postRuntimeBlocksClear(router, "", false))

	require.Equal(t, "all", data["scope"])
	require.Equal(t, []any{}, data["account_ids"], "空账号列表要序列化成 []，不能是 null")
	requireRuntimeBlocksClearCountKeys(t, data)
}

func TestClearRuntimeBlocks_EmptyBodyVariantsMeanAllAccounts(t *testing.T) {
	router := setupRuntimeBlocksClearRouter(&service.OpenAIGatewayService{})

	for _, body := range []string{"", "   ", "{}", `{"account_ids": []}`, `{"account_ids": null}`} {
		data := decodeRuntimeBlocksClear(t, postRuntimeBlocksClear(router, body, true))
		require.Equal(t, "all", data["scope"], "body=%q", body)
		require.Equal(t, []any{}, data["account_ids"], "body=%q", body)
	}
}

func TestClearRuntimeBlocks_ExplicitAccountIDsAreNormalized(t *testing.T) {
	router := setupRuntimeBlocksClearRouter(&service.OpenAIGatewayService{})

	data := decodeRuntimeBlocksClear(t, postRuntimeBlocksClear(router, `{"account_ids": [2, 1, 1, 0, -5]}`, true))

	require.Equal(t, "accounts", data["scope"])
	require.Equal(t, []any{float64(1), float64(2)}, data["account_ids"], "去重、丢掉非正数、升序")
	requireRuntimeBlocksClearCountKeys(t, data)
}

func TestClearRuntimeBlocks_InvalidJSONReturns400(t *testing.T) {
	router := setupRuntimeBlocksClearRouter(&service.OpenAIGatewayService{})

	rec := postRuntimeBlocksClear(router, `{"account_ids": [1,`, true)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// 逃生口不该因为网关实例没注入就 500：Grok 那几张表是进程级全局变量，照样清得掉。
func TestClearRuntimeBlocks_NilGatewayStillSucceeds(t *testing.T) {
	router := setupRuntimeBlocksClearRouter(nil)

	data := decodeRuntimeBlocksClear(t, postRuntimeBlocksClear(router, "", false))

	require.Equal(t, "all", data["scope"])
	requireRuntimeBlocksClearCountKeys(t, data)
}
