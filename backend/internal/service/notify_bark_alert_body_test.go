//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 账号用量规则的推送文案（批次 6 / A6-2 第二步）：Details 账号行、人话指标名、单位后缀、手动试发。

func TestBuildOpsAlertBarkBody_AccountDetailsAndUnit(t *testing.T) {
	t.Parallel()

	firedAt := time.Date(2026, 9, 5, 2, 0, 5, 0, time.UTC)
	n := OpsAlertNotification{
		RuleName: "Codex 5h 用量", Severity: "P2",
		MetricType: OpsAlertMetricAccountWindowUsedPercent, MetricLabel: "账号 5 小时窗口用量",
		Operator: ">=", Threshold: 80, Value: 85.456, Unit: "%",
		Scope:   "platform=openai window=5h",
		Details: []string{"账号：codex-01（openai）"},
		FiredAt: firedAt,
	}
	lines := strings.Split(buildOpsAlertBarkBody(n, false), "\n")
	require.Len(t, lines, 5)
	require.Equal(t, "指标：账号 5 小时窗口用量", lines[0])
	require.Equal(t, "账号：codex-01（openai）", lines[1], "账号行插在指标与当前值之间")
	require.Equal(t, "当前值：85.46%（阈值 >= 80%）", lines[2])
	require.Equal(t, "作用域：platform=openai window=5h", lines[3])
	require.True(t, strings.HasPrefix(lines[4], "触发时间："))

	resolvedAt := firedAt.Add(90 * time.Minute)
	n.Value = 0
	n.ResolvedAt = &resolvedAt
	body := buildOpsAlertBarkBody(n, true)
	require.Contains(t, body, "账号：codex-01（openai）")
	require.Contains(t, body, "当前值：0%（阈值 >= 80%）")
	require.Contains(t, body, "持续 1 小时 30 分钟")

	// 旧格式：没有 MetricLabel / Unit / Details 时与第一步完全一致。
	old := OpsAlertNotification{RuleName: "CPU 过高", MetricType: "cpu_usage_percent", Operator: ">", Threshold: 90, Value: 95, FiredAt: firedAt}
	oldLines := strings.Split(buildOpsAlertBarkBody(old, false), "\n")
	require.Len(t, oldLines, 4)
	require.Equal(t, "指标：cpu_usage_percent", oldLines[0])
	require.Equal(t, "当前值：95（阈值 > 90）", oldLines[1])
	require.Equal(t, "作用域：全局", oldLines[2])
}

func TestBuildOpsAlertBarkBody_BalanceCurrencyUnit(t *testing.T) {
	t.Parallel()

	n := OpsAlertNotification{
		RuleName: "余额不足", MetricType: OpsAlertMetricAccountBalance, MetricLabel: "账号余额",
		Operator: "<", Threshold: 5, Value: 2.5, Unit: " CNY", Details: []string{"账号：kimi-01（kimi）"},
	}
	require.Contains(t, buildOpsAlertBarkBody(n, false), "当前值：2.5 CNY（阈值 < 5 CNY）")
}

func TestBarkNotificationService_NotifyOpsAlertManual(t *testing.T) {
	t.Parallel()

	svc, _, sender := newBarkSettingsFixture(t)

	// 未启用：明确报错而不是静默 nil，也不推送。
	n := OpsAlertNotification{RuleName: "Codex 5h 用量", MetricType: OpsAlertMetricAccountWindowUsedPercent, MetricLabel: "账号 5 小时窗口用量", Operator: ">=", Threshold: 80, Value: 30, Unit: "%", FiredAt: time.Now()}
	require.ErrorIs(t, svc.NotifyOpsAlertManual(context.Background(), n, true, false), ErrBarkNotEnabled)
	require.False(t, svc.IsEnabled(context.Background()))
	require.Empty(t, sender.sent())

	_, err := svc.UpdateBarkConfig(context.Background(), BarkConfigInput{Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "k", Group: "alerts"})
	require.NoError(t, err)
	require.True(t, svc.IsEnabled(context.Background()))

	n.Details = []string{"账号：a（openai）：30%", "账号：b（openai）：12%"}
	n.Scope = "platform=openai window=5h"
	require.NoError(t, svc.NotifyOpsAlertManual(context.Background(), n, true, false))
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "[Sub2API] 手动试发 Codex 5h 用量", sends[0].Msg.Title)
	lines := strings.Split(sends[0].Msg.Body, "\n")
	require.Equal(t, "这是手动试发，不代表真实告警", lines[0])
	require.Equal(t, "指标：账号 5 小时窗口用量", lines[1])
	require.Equal(t, "当前值：30%（阈值 >= 80%）", lines[2])
	require.Equal(t, "是否越阈：否", lines[3])
	require.Equal(t, "账号：a（openai）：30%", lines[4])
	require.Equal(t, "账号：b（openai）：12%", lines[5])
	require.Equal(t, "作用域：platform=openai window=5h", lines[6])
	require.True(t, strings.HasPrefix(lines[7], "评估时间："))
	require.Equal(t, "alerts", sends[0].Msg.Group)

	// 无数据：当前值与是否越阈都标「无数据」。
	require.NoError(t, svc.NotifyOpsAlertManual(context.Background(), OpsAlertNotification{RuleName: "r", MetricType: "cpu_usage_percent", Operator: ">", Threshold: 90}, false, false))
	body := sender.sent()[1].Msg.Body
	require.Contains(t, body, "当前值：无数据（阈值 > 90）")
	require.Contains(t, body, "是否越阈：无数据")
	require.Contains(t, body, "作用域：全局")
}

func TestFormatOpsAlertScope_AccountFilters(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", FormatOpsAlertScope(nil))
	require.Equal(t, "platform=openai group_id=3", FormatOpsAlertScope(map[string]any{"platform": "openai", "group_id": float64(3)}))
	require.Equal(t, "platform=openai window=5h accounts=2", FormatOpsAlertScope(map[string]any{
		"platform": "openai", "window": "5h", "account_ids": []any{float64(1), float64(2)},
	}))
	require.Equal(t, "dimension=daily", FormatOpsAlertScope(map[string]any{"dimension": "daily"}))
	require.Equal(t, "platform=kimi provider=kimi", FormatOpsAlertScope(map[string]any{"platform": "kimi", "provider": "kimi"}))
}
