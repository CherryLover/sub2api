package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// 登录入口 / 默认首页现在也能在管理后台改，本地配置文件是最高优先级的破窗通道。
// "显式设置"必须靠"键是否出现在配置文件/环境变量里"判断，不能靠"值等不等于默认值"：
// viper 给 login_entry_public 注册了默认值 true，"没写"和"写了 true" 解出来的结构体
// 完全一样。下面几个用例锁住的就是这条判定。

func TestLoadWebEntryPresenceFromYAML(t *testing.T) {
	tests := []struct {
		name            string
		yaml            string
		wantLoginLocked bool
		wantHomeLocked  bool
		wantLoginPublic bool
		wantDefaultHome string
		wantLoginPath   string
	}{
		{
			name:            "absent - the admin panel owns all three",
			yaml:            "server:\n  mode: debug\n",
			wantLoginLocked: false,
			wantHomeLocked:  false,
			wantLoginPublic: true,
			wantDefaultHome: DefaultHomePathFallback,
		},
		{
			// web: 出现但整块为空（示例配置文件把三个键都注释掉了就是这个样子）。
			name:            "empty web block",
			yaml:            "web:\n",
			wantLoginLocked: false,
			wantHomeLocked:  false,
			wantLoginPublic: true,
			wantDefaultHome: DefaultHomePathFallback,
		},
		{
			// 写了默认值也算显式设置：这正是"我要把入口钉死在公开"的破窗写法。
			name:            "explicit public pins the login entry",
			yaml:            "web:\n  login_entry_public: true\n",
			wantLoginLocked: true,
			wantHomeLocked:  false,
			wantLoginPublic: true,
			wantDefaultHome: DefaultHomePathFallback,
		},
		{
			name:            "hidden with a path pins both halves",
			yaml:            "web:\n  login_entry_public: false\n  login_entry_path: \"/j7q2m9x4vk3p\"\n",
			wantLoginLocked: true,
			wantHomeLocked:  false,
			wantLoginPublic: false,
			wantLoginPath:   "/j7q2m9x4vk3p",
			wantDefaultHome: DefaultHomePathFallback,
		},
		{
			name:            "default home alone pins only the landing page",
			yaml:            "web:\n  default_home_path: \"/home\"\n",
			wantLoginLocked: false,
			wantHomeLocked:  true,
			wantLoginPublic: true,
			wantDefaultHome: "/home",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			configDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(test.yaml), 0o600))
			t.Setenv("DATA_DIR", configDir)

			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, test.wantLoginLocked, cfg.WebLoginEntryLockedLocally())
			require.Equal(t, test.wantHomeLocked, cfg.WebDefaultHomeLockedLocally())
			require.Equal(t, test.wantLoginPublic, cfg.Web.LoginEntryPublic)
			require.Equal(t, test.wantLoginPath, cfg.Web.LoginEntryPath)
			require.Equal(t, test.wantDefaultHome, cfg.Web.DefaultHomePath)
		})
	}
}

// 纯环境变量部署（docker-compose）也要能钉死入口，否则那类部署没有破窗通道。
func TestLoadWebEntryPresenceFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("server:\n  mode: debug\n"), 0o600))
	t.Setenv("DATA_DIR", configDir)
	t.Setenv("WEB_LOGIN_ENTRY_PUBLIC", "true")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.WebLoginEntryLockedLocally())
	require.True(t, cfg.Web.LoginEntryPublic)
	require.False(t, cfg.WebDefaultHomeLockedLocally())
}

// 零值 Config（大量单测直接构造）必须落在"没锁、交给数据库"这一侧，
// 否则那些单测会在毫不知情的情况下拿到"配置文件锁定"的行为。
func TestWebEntryLockHelpersOnZeroConfig(t *testing.T) {
	var cfg Config
	require.False(t, cfg.WebLoginEntryLockedLocally())
	require.False(t, cfg.WebDefaultHomeLockedLocally())

	var nilCfg *Config
	require.False(t, nilCfg.WebLoginEntryLockedLocally())
	require.False(t, nilCfg.WebDefaultHomeLockedLocally())

	// 手工构造出的自定义路径本身就是最可靠的显式信号（内嵌/单测场景不过 load()）。
	withPath := Config{Web: WebConfig{LoginEntryPath: "/j7q2m9x4vk3p"}}
	require.True(t, withPath.WebLoginEntryLockedLocally())
}

func TestIsAllowedDefaultHomePath(t *testing.T) {
	for _, path := range []string{"/home", "/key-usage", "/model-plaza", "/home/"} {
		require.True(t, IsAllowedDefaultHomePath(path, true), path)
		require.True(t, IsAllowedDefaultHomePath(path, false), path)
	}
	// "/login" 只在登录入口公开时可用：隐藏时用它当落地页会无限重定向。
	require.True(t, IsAllowedDefaultHomePath("/login", true))
	require.False(t, IsAllowedDefaultHomePath("/login", false))
	for _, path := range []string{"/dashboard", "/keys", "/nope", "relative", ""} {
		require.False(t, IsAllowedDefaultHomePath(path, true), path)
	}
	require.Equal(t, []string{"/home", "/key-usage", "/model-plaza"}, AllowedDefaultHomePaths())
}
