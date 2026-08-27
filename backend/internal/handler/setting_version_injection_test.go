package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// settingVersionRepoStub 是一个只读的空设置仓库，公开设置全部走默认值。
type settingVersionRepoStub struct{}

func (r *settingVersionRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *settingVersionRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (r *settingVersionRepoStub) Set(ctx context.Context, key, value string) error { return nil }

func (r *settingVersionRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *settingVersionRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
}

func (r *settingVersionRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *settingVersionRepoStub) Delete(ctx context.Context, key string) error { return nil }

// TestProvideSettingHandlerInjectsBuildVersion 守住"后台侧边栏版本号"的传递链路。
//
// 版本号有两条出口：/api/v1/settings/public 走 SettingHandler 自己持有的 version，
// 而首屏 HTML 注入的 window.__APP_CONFIG__ 走 SettingService.version。装配阶段一旦
// 漏掉 SettingService.SetVersion，注入出去的 version 就是空串，前端拿不到版本号，
// 侧边栏 Logo 旁只剩空占位。
func TestProvideSettingHandlerInjectsBuildVersion(t *testing.T) {
	svc := service.NewSettingService(&settingVersionRepoStub{}, &config.Config{})

	const wantVersion = "internal-rc-test"
	h := ProvideSettingHandler(svc, BuildInfo{Version: wantVersion, BuildType: "source"}, nil)
	require.NotNil(t, h)

	payload, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)

	injected, ok := payload.(*service.PublicSettingsInjectionPayload)
	require.True(t, ok, "injection payload type changed")
	require.Equal(t, wantVersion, injected.Version,
		"HTML 注入的 version 为空会导致后台侧边栏版本号不显示")
}
