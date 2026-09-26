//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// digestStubOpsRepo 只实现报错汇总会碰到的三个方法：分组计数、任务心跳读、任务心跳写。
// 其余方法由嵌入的 nil 接口兜底（跑不到）。
type digestStubOpsRepo struct {
	OpsRepository

	mu           sync.Mutex
	rows         []*OpsErrorDigestRow
	heartbeats   []*OpsJobHeartbeat
	breakdownErr error

	queriedWindows []digestQueriedWindow
	upserts        []*OpsUpsertJobHeartbeatInput
}

type digestQueriedWindow struct {
	Start time.Time
	End   time.Time
}

func (r *digestStubOpsRepo) GetErrorDigestBreakdown(_ context.Context, start, end time.Time) ([]*OpsErrorDigestRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queriedWindows = append(r.queriedWindows, digestQueriedWindow{Start: start, End: end})
	if r.breakdownErr != nil {
		return nil, r.breakdownErr
	}
	return r.rows, nil
}

func (r *digestStubOpsRepo) ListJobHeartbeats(context.Context) ([]*OpsJobHeartbeat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.heartbeats, nil
}

func (r *digestStubOpsRepo) UpsertJobHeartbeat(_ context.Context, input *OpsUpsertJobHeartbeatInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts = append(r.upserts, input)
	return nil
}

// digestRow 造一行分组计数。apiKeyID <= 0 表示 api_key_id 为空（认证阶段就失败的请求）。
func digestRow(apiKeyID int64, userName, keyName, errorType string, status int, limited bool, count int64) *OpsErrorDigestRow {
	row := &OpsErrorDigestRow{
		Username:          userName,
		APIKeyName:        keyName,
		ErrorType:         errorType,
		StatusCode:        status,
		IsBusinessLimited: limited,
		Count:             count,
	}
	if apiKeyID > 0 {
		row.APIKeyID = int64Ptr(apiKeyID)
		row.UserID = int64Ptr(apiKeyID * 100)
	}
	return row
}

func newErrorDigestFixture(t *testing.T, barkEnabled bool) (*OpsErrorDigestService, *digestStubOpsRepo, *fakeBarkSender) {
	t.Helper()

	settingRepo := newStubSettingRepo()
	sender := &fakeBarkSender{}
	bark := NewBarkNotificationService(settingRepo, reversibleEncryptor{}, sender, true)
	_, err := bark.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled:   barkEnabled,
		ServerURL: "https://api.day.app",
		DeviceKey: "device-key",
		Group:     "sub2api",
		Level:     BarkLevelActive,
	})
	require.NoError(t, err)

	// 正文里的时刻按 cfg.Timezone 渲染；tzdb 缺失时要在这里就报出来，
	// 别让断言在 11:30 / 03:30 上打哑谜。
	_, tzErr := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, tzErr)

	repo := &digestStubOpsRepo{}
	svc := NewOpsErrorDigestService(repo, settingRepo, bark, nil, &config.Config{Timezone: "Asia/Shanghai"})
	return svc, repo, sender
}

// ─── 窗口计算 ───

func TestOpsErrorDigest_WindowFallsBackTo24HoursOnFirstRun(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)

	start, end := svc.resolveWindow(context.Background(), now)

	require.True(t, now.Equal(end))
	require.True(t, now.Add(-24*time.Hour).Equal(start), "读不到心跳时应退回最近 24 小时")
}

func TestOpsErrorDigest_WindowStartsAtLastSuccess(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	lastSuccess := now.Add(-18 * time.Hour)
	repo.heartbeats = []*OpsJobHeartbeat{
		{JobName: "ops_cleanup", LastSuccessAt: timePtr(now.Add(-time.Hour))},
		{JobName: opsErrorDigestJobName, LastSuccessAt: timePtr(lastSuccess)},
	}

	start, end := svc.resolveWindow(context.Background(), now)

	require.True(t, now.Equal(end))
	require.True(t, lastSuccess.Equal(start), "区间应从本任务上次成功的时刻算起，别的任务的心跳不算数")
}

func TestOpsErrorDigest_WindowCapsAt24HoursWhenLastSuccessIsTooOld(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	repo.heartbeats = []*OpsJobHeartbeat{
		{JobName: opsErrorDigestJobName, LastSuccessAt: timePtr(now.Add(-30 * time.Hour))},
	}

	start, _ := svc.resolveWindow(context.Background(), now)

	require.True(t, now.Add(-24*time.Hour).Equal(start), "停机太久时只汇总最近 24 小时")
}

func TestOpsErrorDigest_WindowIgnoresHeartbeatInTheFuture(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	repo.heartbeats = []*OpsJobHeartbeat{
		{JobName: opsErrorDigestJobName, LastSuccessAt: timePtr(now.Add(2 * time.Hour))},
	}

	start, end := svc.resolveWindow(context.Background(), now)

	require.True(t, now.Add(-24*time.Hour).Equal(start), "心跳时间在未来时不可信，走兜底区间")
	require.True(t, end.After(start))
}

// ─── 分组、折叠与计数 ───

func TestOpsErrorDigest_SplitsBusinessLimitedFromRealFailures(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	rows := []*OpsErrorDigestRow{
		digestRow(1, "张三", "dev-key", "overloaded_error", 529, false, 30),
		digestRow(1, "张三", "dev-key", "upstream_error", 504, false, 12),
		digestRow(2, "李四", "prod-key", "authentication_error", 401, true, 18),
		digestRow(0, "", "", "authentication_error", 401, true, 12),
	}

	summary := buildOpsErrorDigestSummary(rows, start, end, 8)

	require.Equal(t, int64(72), summary.Total)
	require.Equal(t, int64(42), summary.SLA, "真故障只数 is_business_limited=false 的")
	require.Equal(t, int64(30), summary.Limited, "业务拦截单独计数，不混进真故障")
}

func TestOpsErrorDigest_GroupsByKeyAndKeepsTopThreeErrorTypes(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	rows := []*OpsErrorDigestRow{
		digestRow(1, "张三", "dev-key", "overloaded_error", 529, false, 30),
		digestRow(1, "张三", "dev-key", "upstream_error", 504, false, 12),
		digestRow(1, "张三", "dev-key", "rate_limit_error", 429, false, 7),
		digestRow(1, "张三", "dev-key", "api_error", 500, false, 3),
		digestRow(1, "张三", "dev-key", "not_found_error", 404, false, 1),
		digestRow(2, "李四", "prod-key", "authentication_error", 401, true, 18),
	}

	summary := buildOpsErrorDigestSummary(rows, start, end, 8)

	require.Len(t, summary.Groups, 2)
	require.Equal(t, "张三 / dev-key", summary.Groups[0].Title, "次数最多的密钥排在最前")
	require.Equal(t, int64(53), summary.Groups[0].Total)
	require.Equal(t, []OpsErrorDigestTypeLine{
		{Label: "上游过载(529)", Count: 30},
		{Label: "上游错误(504)", Count: 12},
		{Label: "触发限流(429)", Count: 7},
	}, summary.Groups[0].Types, "每个密钥只列前 3 种错误类型")

	require.Equal(t, "李四 / prod-key", summary.Groups[1].Title)
	require.Equal(t, 0, summary.HiddenGroups)
	require.Equal(t, int64(0), summary.HiddenTotal)
}

func TestOpsErrorDigest_FoldsKeysBeyondTopIntoOneLine(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)

	// 11 个密钥，次数 110、100、…、10，默认只展开前 8 个。
	rows := make([]*OpsErrorDigestRow, 0, 11)
	for i := 1; i <= 11; i++ {
		rows = append(rows, digestRow(int64(i), "用户"+string(rune('A'+i-1)), "key", "api_error", 500, false, int64((12-i)*10)))
	}

	summary := buildOpsErrorDigestSummary(rows, start, end, 8)

	require.Len(t, summary.Groups, 8)
	require.Equal(t, int64(110), summary.Groups[0].Total)
	require.Equal(t, int64(40), summary.Groups[7].Total)
	require.Equal(t, 3, summary.HiddenGroups)
	require.Equal(t, int64(60), summary.HiddenTotal, "被折叠的 30+20+10")
	require.Equal(t, int64(660), summary.Total)
}

func TestOpsErrorDigest_GroupsUnassignedKeysTogether(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	rows := []*OpsErrorDigestRow{
		digestRow(0, "", "", "authentication_error", 401, true, 8),
		digestRow(0, "", "", "invalid_request_error", 0, false, 4),
		digestRow(1, "张三", "dev-key", "api_error", 500, false, 2),
	}

	summary := buildOpsErrorDigestSummary(rows, start, end, 8)

	require.Len(t, summary.Groups, 2)
	require.Equal(t, opsErrorDigestUnassignedTitle, summary.Groups[0].Title, "认证阶段就失败的请求归到同一组")
	require.Equal(t, int64(12), summary.Groups[0].Total)
	require.Equal(t, []OpsErrorDigestTypeLine{
		{Label: "认证失败(401)", Count: 8},
		{Label: "请求无效", Count: 4},
	}, summary.Groups[0].Types, "没记到状态码时只写类型名")
}

func TestOpsErrorDigest_FallsBackToIDsWhenNamesAreMissing(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)

	byEmail := digestRow(7, "", "old-key", "api_error", 500, false, 3)
	byEmail.UserEmail = "a@example.com"
	noNames := digestRow(9, "", "", "api_error", 500, false, 2)

	summary := buildOpsErrorDigestSummary([]*OpsErrorDigestRow{byEmail, noNames}, start, end, 8)

	require.Equal(t, "a@example.com / old-key", summary.Groups[0].Title, "没有用户名就退回邮箱")
	require.Equal(t, "用户#900 / 密钥#9", summary.Groups[1].Title, "名字都取不到时退回 ID")
}

// ─── 正文 ───

func TestOpsErrorDigest_RendersBody(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)
	// 09:30Z = 17:30 +08，03:30Z（次日）= 11:30 +08，正好 18 小时。
	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	rows := []*OpsErrorDigestRow{
		digestRow(1, "张三", "dev-key", "overloaded_error", 529, false, 30),
		digestRow(1, "张三", "dev-key", "upstream_error", 504, false, 12),
		digestRow(2, "李四", "prod-key", "authentication_error", 401, true, 18),
		digestRow(0, "", "", "authentication_error", 401, true, 12),
	}

	title, body := svc.renderDigest(context.Background(), buildOpsErrorDigestSummary(rows, start, end, 8), end)

	require.Equal(t, "[Sub2API] 报错汇总 · 11:30", title)
	require.Equal(t, strings.Join([]string{
		"区间 17:30 – 11:30（18 小时）",
		"失败 72 次：真故障 42，业务拦截 30",
		"",
		"张三 / dev-key：42 次",
		"  上游过载(529) 30、上游错误(504) 12",
		"李四 / prod-key：18 次",
		"  认证失败(401) 18",
		"（未识别密钥）：12 次",
		"  认证失败(401) 12",
	}, "\n"), body)
}

func TestOpsErrorDigest_TitleUsesBarkNotificationName(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)
	_, err := svc.notifier.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled: true, ServerURL: "https://api.day.app", DeviceKey: "device-key", TitlePrefix: barkStringPtr("生产网关"),
	})
	require.NoError(t, err)

	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	title, _ := svc.renderDigest(context.Background(), &OpsErrorDigestSummary{}, end)
	require.Equal(t, "[生产网关] 报错汇总 · 11:30", title)
}

func TestOpsErrorDigest_BodyMentionsHiddenKeys(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)
	start := time.Date(2026, 9, 15, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	rows := make([]*OpsErrorDigestRow, 0, 11)
	for i := 1; i <= 11; i++ {
		rows = append(rows, digestRow(int64(i), "用户"+string(rune('A'+i-1)), "key", "api_error", 500, false, int64((12-i)*10)))
	}

	_, body := svc.renderDigest(context.Background(), buildOpsErrorDigestSummary(rows, start, end, 8), end)

	require.Contains(t, body, "另有 3 个密钥共 60 次")
}

// ─── 推送与跳过 ───

func TestOpsErrorDigest_SkipsPushWhenWindowIsEmpty(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	result, err := svc.runDigestOnce(context.Background(), now, OpsErrorDigestConfig{
		SkipWhenEmpty: true,
		TopKeys:       8,
	})

	require.NoError(t, err)
	require.Contains(t, result, "skipped")
	require.Empty(t, sender.sent(), "区间内没有报错且开了 skip_when_empty 时不该推送")
	require.Len(t, repo.queriedWindows, 1)
}

func TestOpsErrorDigest_PushesEmptyWindowWhenSkipDisabled(t *testing.T) {
	t.Parallel()

	svc, _, sender := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	result, err := svc.runDigestOnce(context.Background(), now, OpsErrorDigestConfig{
		SkipWhenEmpty: false,
		TopKeys:       8,
	})

	require.NoError(t, err)
	require.Contains(t, result, "pushed")
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Contains(t, sends[0].Msg.Body, "区间内没有报错")
}

func TestOpsErrorDigest_PushesSummaryWhenThereAreErrors(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	repo.rows = []*OpsErrorDigestRow{
		digestRow(1, "张三", "dev-key", "overloaded_error", 529, false, 30),
		digestRow(2, "李四", "prod-key", "billing_error", 402, true, 5),
	}

	result, err := svc.runDigestOnce(context.Background(), now, OpsErrorDigestConfig{
		SkipWhenEmpty: true,
		TopKeys:       8,
	})

	require.NoError(t, err)
	require.Contains(t, result, "errors=35")
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "[Sub2API] 报错汇总 · 11:30", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "失败 35 次：真故障 30，业务拦截 5")
	require.Contains(t, sends[0].Msg.Body, "张三 / dev-key：30 次")
}

func TestOpsErrorDigest_SkipsSilentlyWhenBarkDisabled(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, false)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	repo.rows = []*OpsErrorDigestRow{
		digestRow(1, "张三", "dev-key", "api_error", 500, false, 3),
	}

	result, err := svc.runDigestOnce(context.Background(), now, OpsErrorDigestConfig{TopKeys: 8})

	require.NoError(t, err, "Bark 没启用不算任务失败")
	require.Contains(t, result, "bark disabled")
	require.Empty(t, sender.sent())
}

func TestOpsErrorDigest_ScheduledRunRecordsErrorHeartbeatWhenQueryFails(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, true)
	svc.now = func() time.Time { return time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC) }
	repo.breakdownErr = errors.New("db is down")

	svc.runScheduled()

	require.Empty(t, sender.sent())
	require.Len(t, repo.upserts, 1)
	require.NotNil(t, repo.upserts[0].LastErrorAt)
	require.Nil(t, repo.upserts[0].LastSuccessAt, "查库失败不该记成成功，否则下一次的区间会漏掉这一段")
	require.NotNil(t, repo.upserts[0].LastError)
	require.Contains(t, *repo.upserts[0].LastError, "db is down")
}

func TestOpsErrorDigest_ScheduledRunRecordsHeartbeatEvenWhenSkipped(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	// runScheduled 会自己从 settings 读一遍生效配置：没保存过就是默认值
	// （skip_when_empty=true、top_keys=8），正好是这条用例要的场景。
	svc.runScheduled()

	require.Empty(t, sender.sent())
	require.Len(t, repo.upserts, 1, "跳过推送也要记成功心跳，否则窗口会一直往前长")
	require.Equal(t, opsErrorDigestJobName, repo.upserts[0].JobName)
	require.NotNil(t, repo.upserts[0].LastSuccessAt)
	require.Nil(t, repo.upserts[0].LastErrorAt)
}

// ─── 配置 ───

func TestOpsErrorDigest_ConfigDefaultsBeforeFirstSave(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)

	view, err := svc.GetErrorDigestConfig(context.Background())

	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.Equal(t, "30 11,17 * * *", view.Schedule)
	require.True(t, view.SkipWhenEmpty)
	require.Equal(t, 8, view.TopKeys)
	require.Nil(t, view.UpdatedAt)
}

func TestOpsErrorDigest_ConfigRoundTripAndNormalization(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	saved, err := svc.UpdateErrorDigestConfig(context.Background(), OpsErrorDigestConfigInput{
		Enabled:       true,
		Schedule:      "  0 9 * * *  ",
		SkipWhenEmpty: boolPtr(false),
		TopKeys:       0, // 没填 → 回落默认 8
	})

	require.NoError(t, err)
	require.True(t, saved.Enabled)
	require.Equal(t, "0 9 * * *", saved.Schedule)
	require.False(t, saved.SkipWhenEmpty)
	require.Equal(t, 8, saved.TopKeys)
	require.NotNil(t, saved.UpdatedAt)

	reloaded, err := svc.GetErrorDigestConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, saved.Schedule, reloaded.Schedule)
	require.False(t, reloaded.SkipWhenEmpty)
	require.True(t, reloaded.Enabled)
}

func TestOpsErrorDigest_ConfigRejectsBadSchedule(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)

	_, err := svc.UpdateErrorDigestConfig(context.Background(), OpsErrorDigestConfigInput{
		Enabled:  true,
		Schedule: "every day at noon",
	})

	require.ErrorIs(t, err, ErrOpsErrorDigestScheduleInvalid)
}

func TestOpsErrorDigest_ConfigClampsTopKeys(t *testing.T) {
	t.Parallel()

	svc, _, _ := newErrorDigestFixture(t, true)

	saved, err := svc.UpdateErrorDigestConfig(context.Background(), OpsErrorDigestConfigInput{
		Schedule: "30 11,17 * * *",
		TopKeys:  999,
	})

	require.NoError(t, err)
	require.Equal(t, opsErrorDigestMaxTopKeys, saved.TopKeys)
}

func TestOpsErrorDigest_ManualRunAlwaysPushesLast24Hours(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newErrorDigestFixture(t, true)
	now := time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	// 心跳显示 10 分钟前刚跑过，试推仍然要按最近 24 小时算。
	repo.heartbeats = []*OpsJobHeartbeat{
		{JobName: opsErrorDigestJobName, LastSuccessAt: timePtr(now.Add(-10 * time.Minute))},
	}

	result, err := svc.RunManualDigest(context.Background())

	require.NoError(t, err)
	require.True(t, result.Pushed, "即使区间内没有报错，试推也要发出去")
	require.Len(t, repo.queriedWindows, 1)
	require.True(t, now.Add(-24*time.Hour).Equal(repo.queriedWindows[0].Start))
	require.True(t, now.Equal(repo.queriedWindows[0].End))
	require.Len(t, sender.sent(), 1)
}

func TestOpsErrorDigest_ManualRunReportsBarkDisabled(t *testing.T) {
	t.Parallel()

	svc, _, sender := newErrorDigestFixture(t, false)
	svc.now = func() time.Time { return time.Date(2026, 9, 16, 3, 30, 0, 0, time.UTC) }

	result, err := svc.RunManualDigest(context.Background())

	require.NoError(t, err)
	require.False(t, result.Pushed)
	require.Equal(t, "bark_disabled", result.Reason)
	require.NotEmpty(t, result.Body, "没推出去也要把算好的正文回给前端，方便站长先看内容")
	require.Empty(t, sender.sent())
}
