//go:build unit

package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// 自定义端点列表（custom_endpoints）随批次 6 / 包 D 整键删除。
//
// 管理端保存设置是全量 PUT，请求体里再出现这个键时按「未知字段」处理：Gin 的
// JSON 绑定默认不开 DisallowUnknownFields，键被静默丢弃——既不落库，也不影响
// 同一请求里其他字段的写入。这里刻意选「忽略」而不是 400：升级窗口内旧版前端
// 或脚本做全量保存时仍会带上 custom_endpoints: []，不能因此让整次保存失败。
func TestUpdateSettingsIgnoresDroppedCustomEndpointsKey(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyDocURL: "https://docs.example.com",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"doc_url": "https://docs.example.org",
		"custom_endpoints": []map[string]string{
			{"name": "备用线路", "endpoint": "https://backup.example.com/v1", "description": "已裁剪"},
		},
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "https://docs.example.org", repo.values[service.SettingKeyDocURL],
		"同一请求里的其他字段照常写入")
	require.NotContains(t, repo.values, "custom_endpoints",
		"已删除的键不许再被写回 settings 表")
	require.NotContains(t, rec.Body.String(), "custom_endpoints",
		"管理端设置响应里不再出现该键")
}
