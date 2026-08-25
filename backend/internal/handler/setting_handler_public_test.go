//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerPublicRepoStub struct {
	values map[string]string
}

func (s *settingHandlerPublicRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *settingHandlerPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingHandlerPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingHandlerPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingHandlerPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingHandlerPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingHandlerPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}


func TestSettingHandler_GetPublicSettings_ExposesTencentCaptchaConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyTencentCaptchaEnabled: "true",
			service.SettingKeyTencentCaptchaAppID:   "123456789",
			service.SettingKeyTencentCaptchaRegion:  service.TencentCaptchaRegionINTL,
		},
	}
	h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			TencentCaptchaEnabled bool   `json:"tencent_captcha_enabled"`
			TencentCaptchaAppID   string `json:"tencent_captcha_app_id"`
			TencentCaptchaRegion  string `json:"tencent_captcha_region"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.TencentCaptchaEnabled)
	require.Equal(t, "123456789", resp.Data.TencentCaptchaAppID)
	require.Equal(t, service.TencentCaptchaRegionINTL, resp.Data.TencentCaptchaRegion)
}


// The custom login path is the one setting that must never leave the process.
// /api/v1/settings/public is unauthenticated, so anything in this payload is
// public by definition — assert the path is absent from the raw response bytes,
// not just from the fields we happen to decode.
func TestSettingHandler_GetPublicSettings_NeverLeaksCustomLoginPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const hiddenPath = "/j7q2m9x4vk3p"
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = false
	cfg.Web.LoginEntryPath = hiddenPath
	cfg.Web.DefaultHomePath = "/key-usage"

	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{},
	}, cfg), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.NotContains(t, body, hiddenPath, "custom login path leaked through the public settings API")
	require.NotContains(t, body, "login_entry_path")

	var resp struct {
		Code int `json:"code"`
		Data struct {
			LoginEntryPublic bool   `json:"login_entry_public"`
			DefaultHomePath  string `json:"default_home_path"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.False(t, resp.Data.LoginEntryPublic, "the frontend still needs to know the entry is hidden")
	require.Equal(t, "/key-usage", resp.Data.DefaultHomePath)
}

// Default / zero config keeps the historical layout: /login is public and "/"
// lands on the login-free usage page.
func TestSettingHandler_GetPublicSettings_DefaultsToPublicLoginEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	var resp struct {
		Data struct {
			LoginEntryPublic bool   `json:"login_entry_public"`
			DefaultHomePath  string `json:"default_home_path"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Data.LoginEntryPublic)
	require.Equal(t, config.DefaultHomePathFallback, resp.Data.DefaultHomePath)
}

// 同一条红线，换成"路径存在数据库里"的新形态：后台可改之后，路径的来源从本地
// 配置文件变成了数据库，但它依然一个字节都不许出现在公开设置里。
func TestSettingHandler_GetPublicSettings_NeverLeaksStoredCustomLoginPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const hiddenPath = "/j7q2m9x4vk3p"
	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWebLoginEntryPublic: "false",
			service.SettingKeyWebLoginEntryPath:   hiddenPath,
			service.SettingKeyWebDefaultHomePath:  "/home",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.NotContains(t, body, hiddenPath, "the stored custom login path leaked through the public settings API")
	require.NotContains(t, body, "login_entry_path")

	var resp struct {
		Data struct {
			LoginEntryPublic bool   `json:"login_entry_public"`
			DefaultHomePath  string `json:"default_home_path"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.Data.LoginEntryPublic, "the frontend still needs to know the entry is hidden")
	require.Equal(t, "/home", resp.Data.DefaultHomePath)
}

// 数据库里存着"隐藏但路径不可用"时必须 fail-open：宁可入口没藏住，也不能因为一条
// 坏数据把所有人（包括站长）关在门外。
func TestSettingHandler_GetPublicSettings_StoredHiddenEntryWithoutPathFailsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWebLoginEntryPublic: "false",
			service.SettingKeyWebLoginEntryPath:   "",
		},
	}, &config.Config{}), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	var resp struct {
		Data struct {
			LoginEntryPublic bool `json:"login_entry_public"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Data.LoginEntryPublic)
}

// 本地配置文件优先于数据库：这是"把自己关在门外"之后的破窗通道。
func TestSettingHandler_GetPublicSettings_ConfigFileOverridesStoredHiddenEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = true
	cfg.Web.LoginEntryConfigured = true

	h := NewSettingHandler(service.NewSettingService(&settingHandlerPublicRepoStub{
		values: map[string]string{
			service.SettingKeyWebLoginEntryPublic: "false",
			service.SettingKeyWebLoginEntryPath:   "/j7q2m9x4vk3p",
		},
	}, cfg), "test-version")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)

	h.GetPublicSettings(c)

	var resp struct {
		Data struct {
			LoginEntryPublic bool `json:"login_entry_public"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Data.LoginEntryPublic)
	require.NotContains(t, recorder.Body.String(), "/j7q2m9x4vk3p")
}
