//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 应用内更新检查/在线升级/回滚已整套移除，SystemHandler 只剩版本号展示与服务重启。
// 重启走 sysutil.RestartServiceAsync（Linux 上以 os.Exit(0) 交给 systemd 拉起），
// 在单测里触发会直接结束测试进程，因此这里只覆盖 GetVersion。
func TestSystemHandlerGetVersionReturnsBuildVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSystemHandler("0.1.180", nil)

	router := gin.New()
	router.GET("/api/v1/admin/system/version", handler.GetVersion)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/version", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "0.1.180", body.Data.Version)
}
