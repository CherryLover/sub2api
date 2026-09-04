//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// fakeBarkSender 记录每次 Send / Ping，并可注入失败。
type fakeBarkSender struct {
	mu      sync.Mutex
	sends   []fakeBarkSend
	pings   []string
	sendErr error
	pingErr error
}

type fakeBarkSend struct {
	Target BarkTarget
	Msg    BarkMessage
}

func (f *fakeBarkSender) Send(_ context.Context, target BarkTarget, msg BarkMessage) (*BarkSendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, fakeBarkSend{Target: target, Msg: msg})
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &BarkSendResult{StatusCode: http.StatusOK, Message: "success", Latency: 12 * time.Millisecond}, nil
}

func (f *fakeBarkSender) Ping(_ context.Context, serverURL string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings = append(f.pings, serverURL)
	return f.pingErr
}

func (f *fakeBarkSender) sent() []fakeBarkSend {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeBarkSend, len(f.sends))
	copy(out, f.sends)
	return out
}

func newBarkSettingsFixture(t *testing.T) (*BarkNotificationService, *stubSettingRepo, *fakeBarkSender) {
	t.Helper()
	repo := newStubSettingRepo()
	sender := &fakeBarkSender{}
	svc := NewBarkNotificationService(repo, reversibleEncryptor{}, sender, true)
	return svc, repo, sender
}

func storedBarkConfig(t *testing.T, repo *stubSettingRepo) BarkConfig {
	t.Helper()
	raw, err := repo.GetValue(context.Background(), settingKeyNotifyBarkConfig)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	var cfg BarkConfig
	require.NoError(t, json.Unmarshal([]byte(raw), &cfg))
	return cfg
}

func TestBarkNotificationService_GetDefaultWhenUnset(t *testing.T) {
	t.Parallel()

	svc, _, _ := newBarkSettingsFixture(t)
	view, err := svc.GetBarkConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, &BarkConfigView{
		Enabled:         false,
		ServerURL:       "",
		DeviceKey:       "",
		HasDeviceKey:    false,
		Group:           "sub2api",
		Level:           BarkLevelActive,
		NotifyOnResolve: true,
		UpdatedAt:       nil,
	}, view)

	// 契约：device_key 永远返回空串；未配置过时 updated_at 省略。
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"device_key":""`)
	require.NotContains(t, string(raw), "updated_at")
}

func TestBarkNotificationService_UpdateEncryptsKeyAndMasksResponse(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newBarkSettingsFixture(t)
	fixed := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	view, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled:         true,
		ServerURL:       "https://api.day.app/",
		DeviceKey:       "  secret-device-key ",
		Group:           "",
		Level:           BarkLevelCritical,
		Sound:           "alarm",
		ClickURL:        "https://ops.example.com/alerts",
		NotifyOnResolve: boolPtr(false),
	})
	require.NoError(t, err)
	require.True(t, view.Enabled)
	require.Equal(t, "https://api.day.app", view.ServerURL, "末尾 / 应被去掉")
	require.Equal(t, "", view.DeviceKey)
	require.True(t, view.HasDeviceKey)
	require.Equal(t, "sub2api", view.Group, "group 留空回落默认值")
	require.Equal(t, BarkLevelCritical, view.Level)
	require.Equal(t, "alarm", view.Sound)
	require.Equal(t, "https://ops.example.com/alerts", view.ClickURL)
	require.False(t, view.NotifyOnResolve)
	require.NotNil(t, view.UpdatedAt)
	require.Equal(t, fixed, *view.UpdatedAt)

	stored := storedBarkConfig(t, repo)
	require.Equal(t, "enc:secret-device-key", stored.DeviceKey, "落库必须是密文且已 trim")
	require.NotContains(t, repo.values[settingKeyNotifyBarkConfig], `"device_key":"secret-device-key"`)

	got, err := svc.GetBarkConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", got.DeviceKey)
	require.True(t, got.HasDeviceKey)
	require.Equal(t, fixed, *got.UpdatedAt)
}

func TestBarkNotificationService_UpdateEmptyKeyKeepsStoredKey(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "first-key",
	})
	require.NoError(t, err)

	view, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://bark.example.com", DeviceKey: "", Level: BarkLevelPassive,
	})
	require.NoError(t, err)
	require.True(t, view.HasDeviceKey)
	require.Equal(t, "https://bark.example.com", view.ServerURL)
	require.Equal(t, BarkLevelPassive, view.Level)
	require.Equal(t, "enc:first-key", storedBarkConfig(t, repo).DeviceKey)

	// 再次给非空 key 则覆盖。
	_, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://bark.example.com", DeviceKey: "second-key",
	})
	require.NoError(t, err)
	require.Equal(t, "enc:second-key", storedBarkConfig(t, repo).DeviceKey)
}

func TestBarkNotificationService_UpdateInvalidLevel(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: false, ServerURL: "https://api.day.app", DeviceKey: "k", Level: "urgent",
	})
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "BARK_LEVEL_INVALID", infraerrors.Reason(err))
	require.Empty(t, repo.values[settingKeyNotifyBarkConfig], "校验失败不应落库")
}

func TestBarkNotificationService_UpdateEnabledWithoutKey(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "",
	})
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "BARK_DEVICE_KEY_REQUIRED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "device_key")
	require.Empty(t, repo.values[settingKeyNotifyBarkConfig])

	// 关闭状态下允许先存服务器地址，不强制 key。
	view, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: false, ServerURL: "https://api.day.app",
	})
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.False(t, view.HasDeviceKey)
}

func TestBarkNotificationService_UpdateInvalidServerURL(t *testing.T) {
	t.Parallel()

	svc, _, _ := newBarkSettingsFixture(t)
	for _, bad := range []string{"api.day.app", "ftp://api.day.app", "https://"} {
		_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
			Enabled: true, ServerURL: bad, DeviceKey: "k",
		})
		require.Errorf(t, err, "server_url %q", bad)
		require.True(t, infraerrors.IsBadRequest(err))
		require.Equal(t, "BARK_SERVER_URL_INVALID", infraerrors.Reason(err))
	}
	// 启用时 server_url 必填。
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "", DeviceKey: "k"})
	require.True(t, infraerrors.IsBadRequest(err))

	// click_url 若填必须是 http(s)。
	_, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: false, ServerURL: "https://api.day.app", ClickURL: "javascript:alert(1)",
	})
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "BARK_CLICK_URL_INVALID", infraerrors.Reason(err))
}

func TestBarkNotificationService_UpdateRequiresFixedEncryptionKey(t *testing.T) {
	t.Parallel()

	repo := newStubSettingRepo()
	svc := NewBarkNotificationService(repo, reversibleEncryptor{}, &fakeBarkSender{}, false)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k",
	})
	require.ErrorIs(t, err, ErrBarkEncryptionKeyNotConfigured)
	require.Empty(t, repo.values[settingKeyNotifyBarkConfig])

	// 不带新 key 的保存（例如只改开关）不受影响。
	_, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: false, ServerURL: "https://api.day.app"})
	require.NoError(t, err)
}

func TestBarkNotificationService_NotifyOnResolveDefaultsTrue(t *testing.T) {
	t.Parallel()

	svc, _, _ := newBarkSettingsFixture(t)
	view, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k",
	})
	require.NoError(t, err)
	require.True(t, view.NotifyOnResolve, "请求缺省时按默认值 true")

	view, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", NotifyOnResolve: boolPtr(false),
	})
	require.NoError(t, err)
	require.False(t, view.NotifyOnResolve)
}

func TestBarkNotificationService_TestBarkUsesStoredKeyAndDefaultsText(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "stored-key", Group: "ops", Level: BarkLevelTimeSensitive,
	})
	require.NoError(t, err)

	res, err := svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app/", DeviceKey: "", Group: "ops", Level: BarkLevelTimeSensitive, ClickURL: "https://ops.example.com",
	}})
	require.NoError(t, err)
	require.True(t, res.OK)
	require.True(t, res.PingOK)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "success", res.Message)
	require.Equal(t, int64(12), res.LatencyMs)

	require.Equal(t, []string{"https://api.day.app"}, sender.pings)
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "https://api.day.app", sends[0].Target.ServerURL)
	require.Equal(t, "stored-key", sends[0].Target.DeviceKey, "请求里 key 为空时用库里已存的")
	require.Equal(t, "Sub2API 测试通知", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "时间：")
	require.Contains(t, sends[0].Msg.Body, "服务器：https://api.day.app")
	require.Equal(t, "ops", sends[0].Msg.Group)
	require.Equal(t, BarkLevelTimeSensitive, sends[0].Msg.Level)
	require.Equal(t, "https://ops.example.com", sends[0].Msg.URL)
}

func TestBarkNotificationService_TestBarkPrefersRequestKeyAndCustomText(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "stored-key",
	})
	require.NoError(t, err)

	_, err = svc.TestBark(context.Background(), BarkTestInput{
		BarkConfigInput: BarkConfigInput{ServerURL: "https://other.example.com", DeviceKey: "request-key"},
		Title:           "自定义标题",
		Body:            "自定义正文",
	})
	require.NoError(t, err)
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "request-key", sends[0].Target.DeviceKey)
	require.Equal(t, "https://other.example.com", sends[0].Target.ServerURL)
	require.Equal(t, "自定义标题", sends[0].Msg.Title)
	require.Equal(t, "自定义正文", sends[0].Msg.Body)
}

func TestBarkNotificationService_TestBarkWithoutAnyKeyOnlyPings(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	res, err := svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app/"}})
	require.NoError(t, err, "没有任何 key 不再是 400，而是只探活")
	require.False(t, res.OK)
	require.True(t, res.PingOK)
	require.Equal(t, 0, res.StatusCode)
	require.Equal(t, "未配置设备 Key，仅测试了服务器连通性", res.Message)
	require.GreaterOrEqual(t, res.LatencyMs, int64(0))
	require.Equal(t, []string{"https://api.day.app"}, sender.pings, "只探活一次，地址已规范化")
	require.Empty(t, sender.sent(), "没有 key 绝不能发 push")

	// 探活失败：仍是 200 结果而不是错误，只是 ping_ok=false。
	sender.pingErr = errors.New("connection refused")
	res, err = svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app"}})
	require.NoError(t, err)
	require.False(t, res.OK)
	require.False(t, res.PingOK)
	require.Equal(t, 0, res.StatusCode)
	require.Equal(t, "未配置设备 Key，仅测试了服务器连通性", res.Message)
	require.Empty(t, sender.sent())
}

func TestBarkNotificationService_GroupFallsBackToDefault(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newBarkSettingsFixture(t)

	// PUT：group 去空白后为空 → 落默认值，落库、返回与 GET 回显都是 sub2api。
	view, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k", Group: "   ",
	})
	require.NoError(t, err)
	require.Equal(t, "sub2api", view.Group)
	require.Equal(t, "sub2api", storedBarkConfig(t, repo).Group)
	got, err := svc.GetBarkConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "sub2api", got.Group)

	// 测试推送：请求里 group 为空白 → 发出去的消息用默认分组。
	_, err = svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{
		ServerURL: "https://api.day.app", DeviceKey: "k", Group: " \t",
	}})
	require.NoError(t, err)
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "sub2api", sends[0].Msg.Group)

	// 库里直接是空串（旧数据 / 手改）：GET 回显与告警推送同样回落默认值。
	raw := strings.Replace(repo.values[settingKeyNotifyBarkConfig], `"group":"sub2api"`, `"group":""`, 1)
	require.Contains(t, raw, `"group":""`)
	require.NoError(t, repo.Set(context.Background(), settingKeyNotifyBarkConfig, raw))
	svc.invalidateCache()
	got, err = svc.GetBarkConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "sub2api", got.Group)
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), OpsAlertNotification{RuleName: "r", Severity: "P1"}))
	sends = sender.sent()
	require.Len(t, sends, 2)
	require.Equal(t, "sub2api", sends[1].Msg.Group)
}

func TestBarkNotificationService_TestBarkRequiresServerURLAndValidLevel(t *testing.T) {
	t.Parallel()

	svc, _, _ := newBarkSettingsFixture(t)
	_, err := svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{DeviceKey: "k"}})
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "BARK_SERVER_URL_INVALID", infraerrors.Reason(err))

	_, err = svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app", DeviceKey: "k", Level: "loud"}})
	require.ErrorIs(t, err, ErrBarkLevelInvalid)
}

func TestBarkNotificationService_TestBarkPingFailureDoesNotBlock(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	sender.pingErr = errors.New("connection refused")

	res, err := svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app", DeviceKey: "k"}})
	require.NoError(t, err)
	require.True(t, res.OK)
	require.False(t, res.PingOK)
	require.Len(t, sender.sent(), 1)
}

func TestBarkNotificationService_TestBarkPushFailureIs502(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	sender.sendErr = &BarkSendError{StatusCode: http.StatusForbidden, Snippet: `{"code":400,"message":"device key is invalid"}`}

	_, err := svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app", DeviceKey: "k"}})
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
	require.Equal(t, "BARK_PUSH_FAILED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "403")
	require.Contains(t, infraerrors.Message(err), "device key is invalid")

	// 网络层错误同样 502，且错误信息里不能带 device_key。
	sender.sendErr = errors.New("dial tcp: i/o timeout for key super-secret-key")
	_, err = svc.TestBark(context.Background(), BarkTestInput{BarkConfigInput: BarkConfigInput{ServerURL: "https://api.day.app", DeviceKey: "super-secret-key"}})
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
	require.NotContains(t, infraerrors.Message(err), "super-secret-key")
}

func TestBarkNotificationService_NotifyOpsAlertFiredAndResolved(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k", Group: "alerts", Level: BarkLevelCritical, ClickURL: "https://ops.example.com/alerts",
	})
	require.NoError(t, err)

	firedAt := time.Date(2026, 9, 4, 2, 0, 5, 0, time.UTC)
	resolvedAt := firedAt.Add(12 * time.Minute)
	n := OpsAlertNotification{
		RuleName: "CPU 过高", Severity: "P1", MetricType: "cpu_usage_percent", Operator: ">", Threshold: 90, Value: 93.456,
		Scope: "platform=openai group_id=3", FiredAt: firedAt,
	}
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), n))

	n.Value = 61
	n.ResolvedAt = &resolvedAt
	require.NoError(t, svc.NotifyOpsAlertResolved(context.Background(), n))

	sends := sender.sent()
	require.Len(t, sends, 2)

	fired := sends[0]
	require.Equal(t, "k", fired.Target.DeviceKey)
	require.Equal(t, "https://api.day.app", fired.Target.ServerURL)
	require.Equal(t, "[Sub2API] P1 CPU 过高", fired.Msg.Title)
	require.Contains(t, fired.Msg.Body, "指标：cpu_usage_percent")
	require.Contains(t, fired.Msg.Body, "当前值：93.46（阈值 > 90）")
	require.Contains(t, fired.Msg.Body, "作用域：platform=openai group_id=3")
	require.Contains(t, fired.Msg.Body, "触发时间：")
	require.NotContains(t, fired.Msg.Body, "恢复时间")
	require.Equal(t, "alerts", fired.Msg.Group)
	require.Equal(t, BarkLevelCritical, fired.Msg.Level)
	require.Equal(t, "https://ops.example.com/alerts", fired.Msg.URL)

	resolved := sends[1]
	require.Equal(t, "[Sub2API] 已恢复 CPU 过高", resolved.Msg.Title)
	require.Contains(t, resolved.Msg.Body, "当前值：61（阈值 > 90）")
	require.Contains(t, resolved.Msg.Body, "恢复时间：")
	require.Contains(t, resolved.Msg.Body, "持续 12 分钟")
}

func TestBarkNotificationService_NotifyRespectsEnabledAndResolveSwitch(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	n := OpsAlertNotification{RuleName: "r", Severity: "P2", MetricType: "error_rate", Operator: ">=", Threshold: 5, Value: 9}

	// 从未配置：静默跳过。
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), n))
	require.NoError(t, svc.NotifyOpsAlertResolved(context.Background(), n))
	require.Empty(t, sender.sent())

	// 已配置但关闭：仍跳过。
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: false, ServerURL: "https://api.day.app", DeviceKey: "k"})
	require.NoError(t, err)
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), n))
	require.Empty(t, sender.sent())

	// 启用但 notify_on_resolve=false：只推触发，不推恢复。
	_, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "https://api.day.app", NotifyOnResolve: boolPtr(false)})
	require.NoError(t, err)
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), n))
	require.NoError(t, svc.NotifyOpsAlertResolved(context.Background(), n))
	require.Len(t, sender.sent(), 1)

	// 空作用域显示「全局」。
	require.Contains(t, sender.sent()[0].Msg.Body, "作用域：全局")
}

func TestBarkNotificationService_NotifyReturnsSendErrorWithoutKey(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)
	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "top-secret"})
	require.NoError(t, err)
	sender.sendErr = errors.New("upstream said top-secret is bad")

	err = svc.NotifyOpsAlertFired(context.Background(), OpsAlertNotification{RuleName: "r", Severity: "P0"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "top-secret")
}

func TestBarkNotificationService_RuntimeConfigCache(t *testing.T) {
	t.Parallel()

	repo := newStubSettingRepo()
	sender := &fakeBarkSender{}
	svc := NewBarkNotificationService(repo, reversibleEncryptor{}, sender, true)
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k"})
	require.NoError(t, err)
	cfg, ok := svc.runtimeConfig(context.Background())
	require.True(t, ok)
	require.Equal(t, "k", cfg.DeviceKey, "运行时配置里 device_key 已解密")

	// 绕过服务直接改库（模拟另一实例改配置）：缓存期内仍是旧值，过期后读到新值。
	raw := strings.Replace(repo.values[settingKeyNotifyBarkConfig], `"enabled":true`, `"enabled":false`, 1)
	require.NoError(t, repo.Set(context.Background(), settingKeyNotifyBarkConfig, raw))
	_, ok = svc.runtimeConfig(context.Background())
	require.True(t, ok, "30 秒内命中缓存")

	now = now.Add(barkConfigCacheTTL + time.Second)
	_, ok = svc.runtimeConfig(context.Background())
	require.False(t, ok, "缓存过期后应读到已关闭")

	// 本实例保存配置会立即失效缓存。
	_, err = svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "https://api.day.app"})
	require.NoError(t, err)
	_, ok = svc.runtimeConfig(context.Background())
	require.True(t, ok)
}

func TestBarkNotificationService_CorruptConfig(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newBarkSettingsFixture(t)
	require.NoError(t, repo.Set(context.Background(), settingKeyNotifyBarkConfig, "{not json"))
	_, err := svc.GetBarkConfig(context.Background())
	require.ErrorIs(t, err, ErrBarkConfigCorrupt)

	// 告警出口遇到坏配置只跳过，不报错。
	require.NoError(t, svc.NotifyOpsAlertFired(context.Background(), OpsAlertNotification{RuleName: "r"}))
}

func TestFormatOpsAlertScope(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", FormatOpsAlertScope(nil))
	require.Equal(t, "platform=openai", FormatOpsAlertScope(map[string]any{"platform": " openai "}))
	require.Equal(t, "platform=openai group_id=3 region=us", FormatOpsAlertScope(map[string]any{
		"platform": "openai", "group_id": float64(3), "region": "us",
	}))
	require.Equal(t, "group_id=7", FormatOpsAlertScope(map[string]any{"group_id": "7"}))
}

func TestFormatBarkNumberAndDuration(t *testing.T) {
	t.Parallel()

	require.Equal(t, "3", formatBarkNumber(3))
	require.Equal(t, "93.46", formatBarkNumber(93.456))
	require.Equal(t, "0.5", formatBarkNumber(0.5))
	require.Equal(t, "45 秒", formatBarkDuration(45*time.Second))
	require.Equal(t, "12 分钟", formatBarkDuration(12*time.Minute))
	require.Equal(t, "2 小时", formatBarkDuration(2*time.Hour))
	require.Equal(t, "1 小时 5 分钟", formatBarkDuration(65*time.Minute))
}
