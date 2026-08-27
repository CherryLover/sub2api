//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	values map[string]string
	err    error
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
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

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","*.EDU.CN","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestSettingService_GetPublicSettings_ExposesTablePreferences(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyTableDefaultPageSize: "50",
			SettingKeyTablePageSizeOptions: "[20,50,100]",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50, settings.TableDefaultPageSize)
	require.Equal(t, []int{20, 50, 100}, settings.TablePageSizeOptions)
}

func TestSettingService_GetPublicSettings_ExposesCompactHomeEnabled(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCompactHomeEnabled: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.CompactHomeEnabled)

	missingSettings, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).
		GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missingSettings.CompactHomeEnabled)
}

func TestSettingService_ChannelMonitorHideThroughputDefaultsToPrivate(t *testing.T) {
	missing := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.True(t, missing.HideThroughput)
	public, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, public.ChannelMonitorHideThroughput)

	for _, value := range []string{"false", "0", "off", "disabled"} {
		runtime := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorHideThroughput: value,
		}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
		require.False(t, runtime.HideThroughput, "value=%q", value)
	}
}

func TestSettingService_ChannelMonitorShowQuotaFailsClosed(t *testing.T) {
	// 缺省（迁移插入 'false' / 老库无行）一律不展示。
	missingRuntime := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.False(t, missingRuntime.ShowQuota)
	missingPublic, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).
		GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missingPublic.ChannelMonitorShowQuota)

	// 仅字面 "true" 视为开启；其余值（含异常值）fail-closed。
	runtime := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorShowQuota: "true",
	}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.True(t, runtime.ShowQuota)

	for _, value := range []string{"false", "TRUE", "1", "yes", "on", "garbage"} {
		rt := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorShowQuota: value,
		}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
		require.False(t, rt.ShowQuota, "value=%q", value)
	}
}

func TestSettingService_GetPublicSettings_ExposesAllowUserViewErrorRequests(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyAllowUserViewErrorRequests: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.AllowUserViewErrorRequests)
}
