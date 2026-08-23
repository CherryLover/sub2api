//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// webEntryRepoStub 只实现解析路径用得到的方法，别的调用一律 panic：
// 这样如果哪天有人让公开设置顺手去读一次自定义登录路径，测试会立刻炸出来。
type webEntryRepoStub struct {
	values map[string]string
	err    error
	writes map[string]string
}

func (s *webEntryRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *webEntryRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *webEntryRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *webEntryRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *webEntryRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.writes == nil {
		s.writes = map[string]string{}
	}
	for key, value := range settings {
		s.writes[key] = value
		if s.values == nil {
			s.values = map[string]string{}
		}
		s.values[key] = value
	}
	return nil
}

func (s *webEntryRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *webEntryRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

const testWebEntryHiddenPath = "/j7q2m9x4vk3p"

// —— 三层优先级 ——

func TestResolveWebEntry_DatabaseWinsWhenConfigIsSilent(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath,
		SettingKeyWebDefaultHomePath:  "/home",
	}}, &config.Config{})

	entry := svc.ResolveWebEntry(context.Background())
	require.False(t, entry.LoginEntryPublic)
	require.True(t, entry.LoginEntryHidden())
	require.Equal(t, testWebEntryHiddenPath, entry.LoginEntryPath)
	require.Equal(t, "/home", entry.DefaultHomePath)
	require.False(t, entry.LoginEntryLockedByConfig)
	require.False(t, entry.DefaultHomeLockedByConfig)
}

func TestResolveWebEntry_ConfigFileOverridesDatabase(t *testing.T) {
	// 破窗场景：数据库里是"藏起来的入口"，站长在 config.yaml 写了一行 public 并重启。
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = true
	cfg.Web.LoginEntryConfigured = true
	cfg.Web.DefaultHomePath = "/model-plaza"
	cfg.Web.DefaultHomePathConfigured = true

	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath,
		SettingKeyWebDefaultHomePath:  "/home",
	}}, cfg)

	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryPublic, "the config file must win over the stored hidden entry")
	require.Empty(t, entry.LoginEntryPath)
	require.Equal(t, "/model-plaza", entry.DefaultHomePath)
	require.True(t, entry.LoginEntryLockedByConfig)
	require.True(t, entry.DefaultHomeLockedByConfig)
}

func TestResolveWebEntry_ConfigFileCanPinAHiddenEntry(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.LoginEntryPublic = false
	cfg.Web.LoginEntryPath = testWebEntryHiddenPath
	cfg.Web.LoginEntryConfigured = true

	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "true",
	}}, cfg)

	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryHidden())
	require.Equal(t, testWebEntryHiddenPath, entry.LoginEntryPath)
	require.True(t, entry.LoginEntryLockedByConfig)
}

func TestResolveWebEntry_FallsBackToBuiltinDefaults(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{}}, &config.Config{})

	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryPublic)
	require.Empty(t, entry.LoginEntryPath)
	require.Equal(t, config.DefaultHomePathFallback, entry.DefaultHomePath)
}

// 只锁一半：登录入口交给数据库，落地页钉在配置文件里。
func TestResolveWebEntry_LocksAreIndependent(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.DefaultHomePath = "/home"
	cfg.Web.DefaultHomePathConfigured = true

	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath,
		SettingKeyWebDefaultHomePath:  "/model-plaza",
	}}, cfg)

	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryHidden())
	require.False(t, entry.LoginEntryLockedByConfig)
	require.Equal(t, "/home", entry.DefaultHomePath)
	require.True(t, entry.DefaultHomeLockedByConfig)
}

// —— 兜底：坏数据绝不能把所有人关在门外 ——

func TestResolveWebEntry_HiddenWithUnusablePathFailsOpen(t *testing.T) {
	for name, path := range map[string]string{
		"empty":     "",
		"reserved":  "/api/oops",
		"too short": "/ab",
	} {
		t.Run(name, func(t *testing.T) {
			svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
				SettingKeyWebLoginEntryPublic: "false",
				SettingKeyWebLoginEntryPath:   path,
			}}, &config.Config{})

			entry := svc.ResolveWebEntry(context.Background())
			require.True(t, entry.LoginEntryPublic, "a broken stored entry must fall back to the public login page")
			require.Empty(t, entry.LoginEntryPath)
		})
	}
}

// 死循环防护：默认首页 /login + 隐藏入口 = 「首页 -> 登录跳转 -> 首页」。
func TestResolveWebEntry_LoginLandingPageRejectedWhileHidden(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath,
		SettingKeyWebDefaultHomePath:  "/login",
	}}, &config.Config{})
	require.Equal(t, config.DefaultHomePathFallback, svc.ResolveWebEntry(context.Background()).DefaultHomePath)

	// 公开模式下 /login 仍然是合法落地页。
	svc = NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebDefaultHomePath: "/login",
	}}, &config.Config{})
	require.Equal(t, "/login", svc.ResolveWebEntry(context.Background()).DefaultHomePath)
}

func TestResolveWebEntry_UnknownLandingPageFallsBack(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebDefaultHomePath: "/dashboard",
	}}, &config.Config{})
	require.Equal(t, config.DefaultHomePathFallback, svc.ResolveWebEntry(context.Background()).DefaultHomePath)
}

func TestResolveWebEntry_DatabaseErrorKeepsLoginReachable(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{err: errors.New("db down")}, &config.Config{})
	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryPublic, "a database outage must not hide the login page")
	require.Equal(t, config.DefaultHomePathFallback, entry.DefaultHomePath)
}

// —— 保存前校验 ——

func TestNormalizeAndValidateWebEntryInput(t *testing.T) {
	path, home, err := NormalizeAndValidateWebEntryInput(false, testWebEntryHiddenPath+"/", "/home/")
	require.NoError(t, err)
	require.Equal(t, testWebEntryHiddenPath, path)
	require.Equal(t, "/home", home)

	// 空的默认首页补成内置默认值，而不是报错。
	_, home, err = NormalizeAndValidateWebEntryInput(true, "", "")
	require.NoError(t, err)
	require.Equal(t, config.DefaultHomePathFallback, home)
}

func TestNormalizeAndValidateWebEntryInput_RejectsHiddenWithoutPath(t *testing.T) {
	_, _, err := NormalizeAndValidateWebEntryInput(false, "   ", "/key-usage")
	require.Error(t, err)
	var verr *WebEntryValidationError
	require.True(t, errors.As(err, &verr))
	require.Equal(t, "login_entry_path", verr.Field)
	require.Contains(t, err.Error(), "custom login path is required")
}

func TestNormalizeAndValidateWebEntryInput_RejectsBadPaths(t *testing.T) {
	for name, path := range map[string]string{
		"missing leading slash": "j7q2m9x4vk",
		"too short":             "/ab",
		"is /login":             "/login",
		"existing route":        "/key-usage",
		"backend prefix":        "/api/secret-gate",
		"admin prefix":          "/admin/secret-gate",
		"space":                 "/secret gate",
		"empty segment":         "/secret//gate",
		"root":                  "/",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := NormalizeAndValidateWebEntryInput(false, path, "/key-usage")
			require.Error(t, err)
			var verr *WebEntryValidationError
			require.True(t, errors.As(err, &verr))
			require.Equal(t, "login_entry_path", verr.Field)
		})
	}
}

// 公开模式下的非法路径也要拒绝：先存一条坏路径、之后翻成隐藏模式才发现进不去，
// 是最难查的一种自锁。
func TestNormalizeAndValidateWebEntryInput_RejectsBadPathEvenWhilePublic(t *testing.T) {
	_, _, err := NormalizeAndValidateWebEntryInput(true, "/api/oops", "/key-usage")
	require.Error(t, err)
}

func TestNormalizeAndValidateWebEntryInput_RejectsBadLandingPage(t *testing.T) {
	_, _, err := NormalizeAndValidateWebEntryInput(true, "", "/dashboard")
	require.Error(t, err)
	var verr *WebEntryValidationError
	require.True(t, errors.As(err, &verr))
	require.Equal(t, "default_home_path", verr.Field)
	require.Contains(t, err.Error(), "/home")

	// 隐藏模式下 /login 不是合法落地页。
	_, _, err = NormalizeAndValidateWebEntryInput(false, testWebEntryHiddenPath, "/login")
	require.Error(t, err)
}

// —— 落库与生效 ——

func TestUpdateSettings_PersistsWebEntryAndRefreshesCache(t *testing.T) {
	repo := &webEntryRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	require.True(t, svc.ResolveWebEntry(context.Background()).LoginEntryPublic)

	settings := &SystemSettings{
		LoginEntryPublic: false,
		LoginEntryPath:   testWebEntryHiddenPath + "/",
		DefaultHomePath:  "/home",
	}
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))

	require.Equal(t, "false", repo.writes[SettingKeyWebLoginEntryPublic])
	require.Equal(t, testWebEntryHiddenPath, repo.writes[SettingKeyWebLoginEntryPath],
		"the stored path must be normalized, not written back with its trailing slash")
	require.Equal(t, "/home", repo.writes[SettingKeyWebDefaultHomePath])

	// 缓存必须在保存后立即重建，否则本节点还会按旧布置服务最多 30 秒。
	entry := svc.ResolveWebEntry(context.Background())
	require.True(t, entry.LoginEntryHidden())
	require.Equal(t, testWebEntryHiddenPath, entry.LoginEntryPath)
}

// 保存这三项绝不能顺手动到会话相关的键——会话绑定开关一变，当前管理员会话立刻失效，
// 那就成了"改个登录入口把自己踢下线"。
func TestUpdateSettings_WebEntryChangeDoesNotTouchSessionKeys(t *testing.T) {
	repo := &webEntryRepoStub{values: map[string]string{
		SettingKeySessionBindingEnabled: "true",
		SettingKeyStepUpEnabled:         "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings := &SystemSettings{
		SessionBindingEnabled: true,
		StepUpEnabled:         true,
		LoginEntryPublic:      false,
		LoginEntryPath:        testWebEntryHiddenPath,
		DefaultHomePath:       "/key-usage",
	}
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))

	require.Equal(t, "true", repo.values[SettingKeySessionBindingEnabled])
	require.Equal(t, "true", repo.values[SettingKeyStepUpEnabled])
}

// —— 泄漏面：公开设置里绝不能出现自定义路径 ——

func TestGetPublicSettings_NeverCarriesTheStoredCustomLoginPath(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath,
		SettingKeyWebDefaultHomePath:  "/key-usage",
	}}, &config.Config{})

	public, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, public.LoginEntryPublic)
	require.Equal(t, "/key-usage", public.DefaultHomePath)

	encoded, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), testWebEntryHiddenPath)
	require.NotContains(t, string(encoded), "login_entry_path")

	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	encoded, err = json.Marshal(injected)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), testWebEntryHiddenPath)
	require.NotContains(t, string(encoded), "login_entry_path")
}

// SystemSettings 是管理端结构（管理员鉴权后才拿得到），路径在这里是允许出现的。
func TestGetAllSettings_ExposesStoredWebEntryToAdmins(t *testing.T) {
	svc := NewSettingService(&webEntryRepoStub{values: map[string]string{
		SettingKeyWebLoginEntryPublic: "false",
		SettingKeyWebLoginEntryPath:   testWebEntryHiddenPath + "/",
		SettingKeyWebDefaultHomePath:  "/home",
	}}, &config.Config{})

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.LoginEntryPublic)
	require.Equal(t, testWebEntryHiddenPath, settings.LoginEntryPath)
	require.Equal(t, "/home", settings.DefaultHomePath)
}
