//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const adminWebEntryHiddenPath = "/j7q2m9x4vk3p"

func newWebEntryHandler(t *testing.T, cfg *config.Config, stored map[string]string) (*SettingHandler, *settingHandlerRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: stored}
	svc := service.NewSettingService(repo, cfg)
	return NewSettingHandler(svc, nil, nil), repo
}

func doGetSettings(t *testing.T, h *SettingHandler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	return rec
}

type webEntryResponse struct {
	Data struct {
		LoginEntryPublic              bool   `json:"login_entry_public"`
		LoginEntryPath                string `json:"login_entry_path"`
		DefaultHomePath               string `json:"default_home_path"`
		LoginEntryLockedByConfig      bool   `json:"login_entry_locked_by_config"`
		DefaultHomePathLockedByConfig bool   `json:"default_home_path_locked_by_config"`
	} `json:"data"`
}

func decodeWebEntry(t *testing.T, rec *httptest.ResponseRecorder) webEntryResponse {
	t.Helper()
	var resp webEntryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// —— 读 ——

func TestGetSettings_ReturnsStoredWebEntryToAdmins(t *testing.T) {
	h, _ := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   adminWebEntryHiddenPath,
		service.SettingKeyWebDefaultHomePath:  "/home",
	})

	rec := doGetSettings(t, h)
	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeWebEntry(t, rec).Data
	require.False(t, got.LoginEntryPublic)
	require.Equal(t, adminWebEntryHiddenPath, got.LoginEntryPath,
		"the admin payload is the one place the custom path is allowed to appear")
	require.Equal(t, "/home", got.DefaultHomePath)
	require.False(t, got.LoginEntryLockedByConfig)
	require.False(t, got.DefaultHomePathLockedByConfig)
}

// 被配置文件锁定时界面必须看得出来，并且回显的是配置文件里的那份值——
// 否则管理员会对着一个改不动的开关反复保存，还以为是 bug。
func TestGetSettings_ReportsConfigFileLock(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = true
	cfg.Web.LoginEntryConfigured = true
	cfg.Web.DefaultHomePath = "/model-plaza"
	cfg.Web.DefaultHomePathConfigured = true

	h, _ := newWebEntryHandler(t, cfg, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   adminWebEntryHiddenPath,
		service.SettingKeyWebDefaultHomePath:  "/home",
	})

	got := decodeWebEntry(t, doGetSettings(t, h)).Data
	require.True(t, got.LoginEntryLockedByConfig)
	require.True(t, got.DefaultHomePathLockedByConfig)
	require.True(t, got.LoginEntryPublic, "the config file wins over the stored hidden entry")
	require.Empty(t, got.LoginEntryPath)
	require.Equal(t, "/model-plaza", got.DefaultHomePath)
}

// —— 写 ——

func TestUpdateSettings_WritesWebEntry(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   adminWebEntryHiddenPath + "/",
		"default_home_path":  "/home",
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "false", repo.values[service.SettingKeyWebLoginEntryPublic])
	require.Equal(t, adminWebEntryHiddenPath, repo.values[service.SettingKeyWebLoginEntryPath])
	require.Equal(t, "/home", repo.values[service.SettingKeyWebDefaultHomePath])

	got := decodeWebEntry(t, rec).Data
	require.False(t, got.LoginEntryPublic)
	require.Equal(t, adminWebEntryHiddenPath, got.LoginEntryPath,
		"the response must echo the resulting login path so the UI can show the full URL")
}

// 不带这三个字段的旧客户端做一次全量保存，绝不能把藏起来的入口重置成公开。
func TestUpdateSettings_OmittedWebEntryKeepsStoredLayout(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   adminWebEntryHiddenPath,
		service.SettingKeyWebDefaultHomePath:  "/home",
	})

	rec := doUpdateSettings(t, h, map[string]any{"doc_url": "https://docs.example.com"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyWebLoginEntryPublic])
	require.Equal(t, adminWebEntryHiddenPath, repo.values[service.SettingKeyWebLoginEntryPath])
	require.Equal(t, "/home", repo.values[service.SettingKeyWebDefaultHomePath])
}

// —— 保存前校验：非法值一律拒绝，并说明原因 ——

func TestUpdateSettings_RejectsHiddenWithoutPath(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   "",
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "custom login path is required")
	require.Empty(t, repo.values[service.SettingKeyWebLoginEntryPublic],
		"a rejected save must not persist any part of the payload")
}

func TestUpdateSettings_RejectsInvalidLoginPath(t *testing.T) {
	for name, path := range map[string]string{
		"reserved backend prefix": "/api/secret-gate",
		"existing route":          "/key-usage",
		"too short":               "/ab",
		"missing leading slash":   "gate-abcdef",
		"illegal character":       "/secret gate",
	} {
		t.Run(name, func(t *testing.T) {
			h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{})
			rec := doUpdateSettings(t, h, map[string]any{
				"login_entry_public": false,
				"login_entry_path":   path,
			}, nil)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "login path")
			require.Empty(t, repo.values[service.SettingKeyWebLoginEntryPath])
		})
	}
}

func TestUpdateSettings_RejectsInvalidDefaultHomePath(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{"default_home_path": "/dashboard"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "not an allowed landing page")
	require.Empty(t, repo.values[service.SettingKeyWebDefaultHomePath])
}

// 死循环防护：隐藏入口 + 落地页 /login 会让未登录访问无限重定向。
func TestUpdateSettings_RejectsLoginLandingPageWhileHidden(t *testing.T) {
	h, _ := newWebEntryHandler(t, &config.Config{}, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   adminWebEntryHiddenPath,
		"default_home_path":  "/login",
	}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "not an allowed landing page")
}

// 已存的隐藏入口 + 新提交的 /login 落地页：也要拒绝（只改一半同样会转圈）。
func TestUpdateSettings_RejectsLoginLandingPageAgainstStoredHiddenEntry(t *testing.T) {
	h, _ := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   adminWebEntryHiddenPath,
	})

	rec := doUpdateSettings(t, h, map[string]any{"default_home_path": "/login"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// —— 配置文件锁定时后台改不动 ——

func TestUpdateSettings_RejectsChangeWhileLoginEntryPinnedByConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = true
	cfg.Web.LoginEntryConfigured = true

	h, repo := newWebEntryHandler(t, cfg, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   adminWebEntryHiddenPath,
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "pinned by the local config file")
	require.Empty(t, repo.values[service.SettingKeyWebLoginEntryPath])
}

func TestUpdateSettings_RejectsChangeWhileDefaultHomePinnedByConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.DefaultHomePath = "/home"
	cfg.Web.DefaultHomePathConfigured = true

	h, repo := newWebEntryHandler(t, cfg, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{"default_home_path": "/model-plaza"}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "pinned by the local config file")
	require.Empty(t, repo.values[service.SettingKeyWebDefaultHomePath])
}

// 界面在锁定时回传的就是生效值：这种"没改动"的保存必须照常成功，
// 否则整页设置都会因为一个只读字段保存不了。
func TestUpdateSettings_PinnedWebEntryAcceptsUnchangedValues(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = false
	cfg.Web.LoginEntryPath = adminWebEntryHiddenPath
	cfg.Web.LoginEntryConfigured = true
	cfg.Web.DefaultHomePath = "/home"
	cfg.Web.DefaultHomePathConfigured = true

	h, repo := newWebEntryHandler(t, cfg, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   adminWebEntryHiddenPath,
		"default_home_path":  "/home",
		"doc_url":            "https://docs.example.com",
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://docs.example.com", repo.values[service.SettingKeyDocURL])
	// 锁定项一律不落库：数据库里保持空，配置文件才是唯一事实来源。
	require.Empty(t, repo.values[service.SettingKeyWebLoginEntryPath])
	require.Empty(t, repo.values[service.SettingKeyWebDefaultHomePath])
}

// —— 泄漏面：管理端响应之外一概不许出现 ——

// 保存这三项不应写任何会话相关的键；会话绑定开关一变，当前管理员会话立即失效。
func TestUpdateSettings_WebEntrySaveKeepsAdminSessionSettingsUntouched(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeySessionBindingEnabled: "true",
		service.SettingKeyStepUpEnabled:         "false",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"login_entry_public": false,
		"login_entry_path":   adminWebEntryHiddenPath,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeySessionBindingEnabled],
		"hiding the login entry must not flip session binding and log the admin out")
	require.Equal(t, "false", repo.values[service.SettingKeyStepUpEnabled])
}

// 数据库里存着一份坏状态（隐藏但没有路径）时，保存任何一项无关设置都必须照常成功。
// 否则一条坏数据就能把整个设置页锁死——连"把登录入口改回公开"这个自救动作都做不了。
func TestUpdateSettings_BrokenStoredWebEntryDoesNotBlockUnrelatedSaves(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   "",
	})

	rec := doUpdateSettings(t, h, map[string]any{"doc_url": "https://docs.example.com"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://docs.example.com", repo.values[service.SettingKeyDocURL])

	// 坏数据没被顺手改写，但对外的生效布置是"登录入口公开"（fail-open）。
	require.Equal(t, "false", repo.values[service.SettingKeyWebLoginEntryPublic])
	require.True(t, decodeWebEntry(t, rec).Data.LoginEntryPublic)
}

// 自救路径：坏状态下把登录入口显式改回公开，必须能存下去。
func TestUpdateSettings_BrokenStoredWebEntryCanBeRepaired(t *testing.T) {
	h, repo := newWebEntryHandler(t, &config.Config{}, map[string]string{
		service.SettingKeyWebLoginEntryPublic: "false",
		service.SettingKeyWebLoginEntryPath:   "",
	})

	rec := doUpdateSettings(t, h, map[string]any{"login_entry_public": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyWebLoginEntryPublic])
}
