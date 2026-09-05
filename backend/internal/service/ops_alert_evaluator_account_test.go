//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// accountAlertStubOpsRepo 在 notifyStubOpsRepo 之上按账号维护活动 / 最新事件，
// 规则级的 GetActiveAlertEvent / GetLatestAlertEvent 对账号目标不该被调用，调用即报错。
type accountAlertStubOpsRepo struct {
	notifyStubOpsRepo
	activeByAccount map[int64]*OpsAlertEvent
	latestByAccount map[int64]*OpsAlertEvent
	ruleLevelCalls  int
}

func newAccountAlertStubOpsRepo(rules []*OpsAlertRule) *accountAlertStubOpsRepo {
	return &accountAlertStubOpsRepo{
		notifyStubOpsRepo: notifyStubOpsRepo{rules: rules},
		activeByAccount:   map[int64]*OpsAlertEvent{},
		latestByAccount:   map[int64]*OpsAlertEvent{},
	}
}

func (r *accountAlertStubOpsRepo) GetActiveAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ruleLevelCalls++
	return nil, errors.New("rule-level active event lookup must not be used for account targets")
}

func (r *accountAlertStubOpsRepo) GetLatestAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ruleLevelCalls++
	return nil, errors.New("rule-level latest event lookup must not be used for account targets")
}

func (r *accountAlertStubOpsRepo) GetActiveAlertEventForAccount(_ context.Context, _ int64, accountID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeByAccount[accountID], nil
}

func (r *accountAlertStubOpsRepo) GetLatestAlertEventForAccount(_ context.Context, _ int64, accountID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestByAccount[accountID], nil
}

func (r *accountAlertStubOpsRepo) CreateAlertEvent(_ context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	event.ID = r.nextID
	r.created = append(r.created, event)
	if id, ok := event.Dimensions["account_id"].(int64); ok {
		r.activeByAccount[id] = event
		r.latestByAccount[id] = event
	}
	return event, nil
}

func (r *accountAlertStubOpsRepo) UpdateAlertEventStatus(_ context.Context, eventID int64, _ string, _ *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved = append(r.resolved, eventID)
	for id, ev := range r.activeByAccount {
		if ev.ID == eventID {
			delete(r.activeByAccount, id)
		}
	}
	return nil
}

func openAIWindowAccount(id int64, name string, usedPercent float64, resetAt time.Time) *Account {
	return &Account{
		ID: id, Name: name, Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_5h_used_percent":  usedPercent,
			"codex_5h_reset_at":      resetAt.Format(time.RFC3339),
			"codex_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
}

type accountAlertFixture struct {
	svc      *OpsAlertEvaluatorService
	repo     *accountAlertStubOpsRepo
	sender   *fakeBarkSender
	accounts []*Account
}

func newAccountAlertFixture(t *testing.T, barkEnabled bool, rules []*OpsAlertRule) *accountAlertFixture {
	t.Helper()

	f := &accountAlertFixture{repo: newAccountAlertStubOpsRepo(rules), sender: &fakeBarkSender{}}
	bark := NewBarkNotificationService(newStubSettingRepo(), reversibleEncryptor{}, f.sender, true)
	_, err := bark.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled:         barkEnabled,
		ServerURL:       "https://api.day.app",
		DeviceKey:       "device-key",
		NotifyOnResolve: boolPtr(true),
	})
	require.NoError(t, err)

	opsService := &OpsService{
		opsRepo: f.repo,
		listAccountsForAlerts: func(context.Context, string, *int64, []int64) ([]*Account, error) {
			return f.accounts, nil
		},
	}
	f.svc = &OpsAlertEvaluatorService{
		opsService:    opsService,
		opsRepo:       f.repo,
		alertNotifier: bark,
		ruleStates:    map[opsAlertRuleStateKey]*opsAlertRuleState{},
	}
	return f
}

func windowRule() *OpsAlertRule {
	return &OpsAlertRule{
		ID: 1, Name: "Codex 5h 用量", Enabled: true, Severity: "P2",
		MetricType: OpsAlertMetricAccountWindowUsedPercent, Operator: ">=", Threshold: 80,
		Filters: map[string]any{"platform": "openai", "window": "5h"},
	}
}

// TestOpsAlertEvaluator_AccountTargetsFireAndResolveIndependently 两个账号先后越阈都各自触发一次、各自恢复。
func TestOpsAlertEvaluator_AccountTargetsFireAndResolveIndependently(t *testing.T) {
	t.Parallel()

	f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
	future := time.Now().UTC().Add(2 * time.Hour)
	f.accounts = []*Account{
		openAIWindowAccount(1, "codex-a", 85, future),
		openAIWindowAccount(2, "codex-b", 50, future),
	}

	// 第一轮：只有 A 越阈。
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 1)
	require.Equal(t, int64(1), f.repo.created[0].Dimensions["account_id"])
	require.Equal(t, "codex-a", f.repo.created[0].Dimensions["account_name"])
	require.Equal(t, "5h", f.repo.created[0].Dimensions["window"])
	require.Contains(t, f.repo.created[0].Description, "账号 codex-a（openai）当前 85%")
	sends := f.sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "[Sub2API] P2 Codex 5h 用量", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "指标：账号 5 小时窗口用量")
	require.Contains(t, sends[0].Msg.Body, "账号：codex-a（openai）")
	require.Contains(t, sends[0].Msg.Body, "当前值：85%（阈值 >= 80%）")
	require.Contains(t, sends[0].Msg.Body, "作用域：platform=openai window=5h")
	require.Equal(t, 0, f.repo.ruleLevelCalls, "账号目标不能用规则级事件查询")
	require.Contains(t, f.repo.heartbeats[len(f.repo.heartbeats)-1], "evaluated=1 created=1 resolved=0")

	// 第二轮：A 仍在触发中（不重复推），B 随后越阈 → B 各自触发一次。
	f.accounts[1] = openAIWindowAccount(2, "codex-b", 90, future)
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 2)
	require.Equal(t, int64(2), f.repo.created[1].Dimensions["account_id"])
	sends = f.sender.sent()
	require.Len(t, sends, 2)
	require.Contains(t, sends[1].Msg.Body, "账号：codex-b（openai）")
	require.Contains(t, sends[1].Msg.Body, "当前值：90%")

	// 第三轮：A 回落 → 只解除 A，推「已恢复」带 A 的账号行；B 仍活跃。
	f.accounts[0] = openAIWindowAccount(1, "codex-a", 20, future)
	f.svc.evaluateOnce(60 * time.Second)
	require.Equal(t, []int64{1}, f.repo.resolved)
	require.Len(t, f.repo.created, 2)
	sends = f.sender.sent()
	require.Len(t, sends, 3)
	require.Equal(t, "[Sub2API] 已恢复 Codex 5h 用量", sends[2].Msg.Title)
	require.Contains(t, sends[2].Msg.Body, "账号：codex-a（openai）")
	require.Contains(t, sends[2].Msg.Body, "当前值：20%")
	require.Contains(t, sends[2].Msg.Body, "恢复时间：")

	// 第四轮：B 的窗口重置（reset_at 已过）→ 新窗口记 0 → B 也恢复。
	f.accounts[1] = openAIWindowAccount(2, "codex-b", 90, time.Now().UTC().Add(-time.Minute))
	f.svc.evaluateOnce(60 * time.Second)
	require.Equal(t, []int64{1, 2}, f.repo.resolved)
	require.Len(t, f.sender.sent(), 4)
	require.Contains(t, f.sender.sent()[3].Msg.Body, "账号：codex-b（openai）")
	require.Empty(t, f.repo.activeByAccount)
}

func TestOpsAlertEvaluator_AccountRuleWithoutDataSkips(t *testing.T) {
	t.Parallel()

	f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
	f.accounts = []*Account{{ID: 1, Name: "no-snapshot", Platform: PlatformOpenAI, Extra: map[string]any{}}}

	f.svc.evaluateOnce(60 * time.Second)
	require.Empty(t, f.repo.created)
	require.Empty(t, f.sender.sent())
	require.Contains(t, f.repo.heartbeats[len(f.repo.heartbeats)-1], "enabled=1 evaluated=0 created=0")
	require.Empty(t, f.svc.ruleStates, "无数据的账号不留持续计数")
}

func TestOpsAlertEvaluator_AccountSustainedCountsPerAccount(t *testing.T) {
	t.Parallel()

	rule := windowRule()
	rule.SustainedMinutes = 2 // 60s 间隔 → 需要连续 2 轮
	f := newAccountAlertFixture(t, false, []*OpsAlertRule{rule})
	future := time.Now().UTC().Add(2 * time.Hour)
	f.accounts = []*Account{
		openAIWindowAccount(1, "a", 85, future),
		openAIWindowAccount(2, "b", 85, future),
	}

	f.svc.evaluateOnce(60 * time.Second)
	require.Empty(t, f.repo.created, "第一轮只计数不触发")
	require.Len(t, f.svc.ruleStates, 2)
	require.Equal(t, 1, f.svc.ruleStates[opsAlertRuleStateKey{RuleID: 1, AccountID: 1}].ConsecutiveBreaches)

	// B 回落后 A 仍连续越阈：只有 A 触发。
	f.accounts[1] = openAIWindowAccount(2, "b", 10, future)
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 1)
	require.Equal(t, int64(1), f.repo.created[0].Dimensions["account_id"])
	require.Equal(t, 0, f.svc.ruleStates[opsAlertRuleStateKey{RuleID: 1, AccountID: 2}].ConsecutiveBreaches)

	// 账号被移出作用域后它的计数被清掉。
	f.accounts = f.accounts[:1]
	f.svc.evaluateOnce(60 * time.Second)
	_, ok := f.svc.ruleStates[opsAlertRuleStateKey{RuleID: 1, AccountID: 2}]
	require.False(t, ok)
}

func TestOpsAlertEvaluator_EvaluateRuleNow(t *testing.T) {
	t.Parallel()

	future := time.Now().UTC().Add(2 * time.Hour)
	ctx := context.Background()

	t.Run("越阈：返回聚合值与逐账号明细，不落事件、不动计数", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{
			openAIWindowAccount(2, "b", 85, future),
			openAIWindowAccount(1, "a", 30, future),
		}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, false)
		require.NoError(t, err)
		require.Equal(t, int64(1), got.RuleID)
		require.Equal(t, "Codex 5h 用量", got.RuleName)
		require.Equal(t, OpsAlertMetricAccountWindowUsedPercent, got.MetricType)
		require.Equal(t, ">=", got.Operator)
		require.InDelta(t, 80, got.Threshold, 0.0001)
		require.False(t, got.EvaluatedAt.IsZero())
		require.True(t, got.HasData)
		require.NotNil(t, got.Value)
		require.InDelta(t, 85, *got.Value, 0.0001, "percent 取最大")
		require.True(t, got.Breached)
		require.Len(t, got.Accounts, 2)
		require.Equal(t, int64(1), got.Accounts[0].AccountID)
		require.False(t, got.Accounts[0].Breached)
		require.Equal(t, int64(2), got.Accounts[1].AccountID)
		require.True(t, got.Accounts[1].Breached)
		require.Equal(t, "b", got.Accounts[1].AccountName)
		require.Equal(t, "openai", got.Accounts[1].Platform)
		require.False(t, got.Sent)
		require.Empty(t, got.SendError)

		require.Empty(t, f.repo.created, "试算不落事件")
		require.Empty(t, f.svc.ruleStates, "试算不动持续计数")
		require.Empty(t, f.sender.sent(), "send=false 不推送")
	})

	t.Run("未越阈", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{openAIWindowAccount(1, "a", 30, future)}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, false)
		require.NoError(t, err)
		require.True(t, got.HasData)
		require.False(t, got.Breached)
		require.Len(t, got.Accounts, 1)
	})

	t.Run("无数据：value 为 null、accounts 为空数组", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{{ID: 1, Platform: PlatformOpenAI, Extra: map[string]any{}}}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, false)
		require.NoError(t, err)
		require.False(t, got.HasData)
		require.Nil(t, got.Value)
		require.False(t, got.Breached)
		require.NotNil(t, got.Accounts)
		require.Empty(t, got.Accounts)
	})

	t.Run("余额指标聚合取最小", func(t *testing.T) {
		t.Parallel()
		rule := &OpsAlertRule{ID: 1, Name: "余额", Enabled: true, MetricType: OpsAlertMetricAccountBalance, Operator: "<", Threshold: 5}
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{rule})
		f.accounts = []*Account{
			{ID: 1, Name: "k1", Platform: PlatformKimi, Extra: map[string]any{"kimi_balance": 9.0, "kimi_balance_currency": "CNY"}},
			{ID: 2, Name: "k2", Platform: PlatformKimi, Extra: map[string]any{"kimi_balance": 2.5, "kimi_balance_currency": "CNY"}},
		}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, false)
		require.NoError(t, err)
		require.InDelta(t, 2.5, *got.Value, 0.0001)
		require.True(t, got.Breached)
		require.Equal(t, "CNY", got.Accounts[1].Currency)
	})

	t.Run("规则不存在 → 404", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		_, err := f.svc.EvaluateRuleNow(ctx, 99, false)
		require.Error(t, err)
		require.Equal(t, 404, infraerrors.Code(err))
	})

	t.Run("send=true 推「手动试发」", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{
			openAIWindowAccount(1, "a", 85, future),
			openAIWindowAccount(2, "b", 30, future),
		}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, true)
		require.NoError(t, err)
		require.True(t, got.Sent)
		require.Empty(t, got.SendError)
		sends := f.sender.sent()
		require.Len(t, sends, 1)
		require.Equal(t, "[Sub2API] 手动试发 Codex 5h 用量", sends[0].Msg.Title)
		body := sends[0].Msg.Body
		require.True(t, strings.HasPrefix(body, "这是手动试发，不代表真实告警\n"), body)
		require.Contains(t, body, "指标：账号 5 小时窗口用量")
		require.Contains(t, body, "当前值：85%（阈值 >= 80%）")
		require.Contains(t, body, "是否越阈：是")
		require.Contains(t, body, "账号：a（openai）：85%（越阈）")
		require.Contains(t, body, "账号：b（openai）：30%\n")
		require.Contains(t, body, "作用域：platform=openai window=5h")
		require.Contains(t, body, "评估时间：")
		require.Empty(t, f.repo.created, "手动试发同样不落事件")
	})

	t.Run("send=true 推送失败 → 不报错，sent=false 带 send_error", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{openAIWindowAccount(1, "a", 85, future)}
		f.sender.sendErr = errors.New("bark unreachable")

		got, err := f.svc.EvaluateRuleNow(ctx, 1, true)
		require.NoError(t, err)
		require.False(t, got.Sent)
		require.Contains(t, got.SendError, "bark unreachable")
		require.True(t, got.HasData, "推送失败不影响试算结果")
	})

	t.Run("send=true 但 Bark 未启用 → BARK_NOT_ENABLED", func(t *testing.T) {
		t.Parallel()
		f := newAccountAlertFixture(t, false, []*OpsAlertRule{windowRule()})
		f.accounts = []*Account{openAIWindowAccount(1, "a", 85, future)}

		_, err := f.svc.EvaluateRuleNow(ctx, 1, true)
		require.ErrorIs(t, err, ErrBarkNotEnabled)
		require.Equal(t, 400, infraerrors.Code(err))
		require.Empty(t, f.sender.sent())
	})

	t.Run("健康度指标同样可试算：accounts 为空", func(t *testing.T) {
		t.Parallel()
		rule := &OpsAlertRule{ID: 1, Name: "CPU 过高", Enabled: true, MetricType: "cpu_usage_percent", Operator: ">", Threshold: 90}
		f := newAccountAlertFixture(t, true, []*OpsAlertRule{rule})
		f.repo.metrics = &OpsSystemMetricsSnapshot{CPUUsagePercent: float64Ptr(95)}

		got, err := f.svc.EvaluateRuleNow(ctx, 1, true)
		require.NoError(t, err)
		require.True(t, got.HasData)
		require.InDelta(t, 95, *got.Value, 0.0001)
		require.True(t, got.Breached)
		require.Empty(t, got.Accounts)
		require.True(t, got.Sent)
		require.Contains(t, f.sender.sent()[0].Msg.Body, "指标：cpu_usage_percent")
		require.Contains(t, f.sender.sent()[0].Msg.Body, "当前值：95（阈值 > 90）")
	})
}

func TestBuildOpsAlertManualDetails_CapsAtFiveLines(t *testing.T) {
	t.Parallel()

	rule := &OpsAlertRule{MetricType: OpsAlertMetricAccountWindowUsedPercent, Operator: ">=", Threshold: 80}
	samples := make([]accountMetricSample, 0, 7)
	for i := int64(1); i <= 7; i++ {
		samples = append(samples, accountMetricSample{AccountID: i, AccountName: "acc", Platform: "openai", Value: float64(70 + i*3)})
	}
	lines := buildOpsAlertManualDetails(samples, rule)
	require.Len(t, lines, 6)
	require.Equal(t, "账号：acc（openai）：73%", lines[0])
	require.Equal(t, "账号：acc（openai）：85%（越阈）", lines[4])
	require.Equal(t, "另有 2 个账号", lines[5])
	require.Nil(t, buildOpsAlertManualDetails(nil, rule))
}
