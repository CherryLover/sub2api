//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// notifyStubOpsRepo 只实现 evaluateOnce 一轮里会碰到的方法：规则列表、系统指标、
// 活动事件、事件落库 / 解除、心跳。其余方法由嵌入的 nil 接口兜底（不会被调用）。
type notifyStubOpsRepo struct {
	OpsRepository

	mu         sync.Mutex
	rules      []*OpsAlertRule
	metrics    *OpsSystemMetricsSnapshot
	active     *OpsAlertEvent
	created    []*OpsAlertEvent
	resolved   []int64
	heartbeats []string
	nextID     int64
}

func (r *notifyStubOpsRepo) ListAlertRules(context.Context) ([]*OpsAlertRule, error) {
	return r.rules, nil
}

func (r *notifyStubOpsRepo) GetLatestSystemMetrics(context.Context, int) (*OpsSystemMetricsSnapshot, error) {
	return r.metrics, nil
}

func (r *notifyStubOpsRepo) GetActiveAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active, nil
}

func (r *notifyStubOpsRepo) GetLatestAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return nil, nil
}

func (r *notifyStubOpsRepo) CreateAlertEvent(_ context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	event.ID = r.nextID
	r.created = append(r.created, event)
	return event, nil
}

func (r *notifyStubOpsRepo) UpdateAlertEventStatus(_ context.Context, eventID int64, _ string, _ *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved = append(r.resolved, eventID)
	return nil
}

func (r *notifyStubOpsRepo) UpsertJobHeartbeat(_ context.Context, input *OpsUpsertJobHeartbeatInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if input != nil && input.LastResult != nil {
		r.heartbeats = append(r.heartbeats, *input.LastResult)
	}
	return nil
}

func (r *notifyStubOpsRepo) IsAlertSilenced(context.Context, int64, string, *int64, *string, time.Time) (bool, error) {
	return false, nil
}

func newNotifyEvaluatorFixture(t *testing.T, barkEnabled bool, notifyOnResolve bool) (*OpsAlertEvaluatorService, *notifyStubOpsRepo, *fakeBarkSender) {
	t.Helper()

	repo := &notifyStubOpsRepo{
		rules: []*OpsAlertRule{{
			ID:         1,
			Name:       "CPU 过高",
			Enabled:    true,
			Severity:   "P1",
			MetricType: "cpu_usage_percent",
			Operator:   ">",
			Threshold:  90,
			Filters:    map[string]any{"platform": "openai", "group_id": float64(3)},
		}},
		metrics: &OpsSystemMetricsSnapshot{CPUUsagePercent: float64Ptr(95)},
	}

	sender := &fakeBarkSender{}
	bark := NewBarkNotificationService(newStubSettingRepo(), reversibleEncryptor{}, sender, true)
	_, err := bark.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled:         barkEnabled,
		ServerURL:       "https://api.day.app",
		DeviceKey:       "device-key",
		Group:           "sub2api",
		Level:           BarkLevelActive,
		ClickURL:        "https://ops.example.com/alerts",
		NotifyOnResolve: boolPtr(notifyOnResolve),
	})
	require.NoError(t, err)

	svc := &OpsAlertEvaluatorService{
		opsRepo:       repo,
		alertNotifier: bark,
		ruleStates:    map[int64]*opsAlertRuleState{},
	}
	return svc, repo, sender
}

func TestOpsAlertEvaluator_FiringPushesBarkOnce(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newNotifyEvaluatorFixture(t, true, true)
	svc.evaluateOnce(60 * time.Second)

	require.Len(t, repo.created, 1, "事件应先落库")
	require.Equal(t, OpsAlertStatusFiring, repo.created[0].Status)

	sends := sender.sent()
	require.Len(t, sends, 1, "触发时只推一次")
	require.Equal(t, "device-key", sends[0].Target.DeviceKey)
	require.Equal(t, "https://api.day.app", sends[0].Target.ServerURL)
	require.Equal(t, "[Sub2API] P1 CPU 过高", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "指标：cpu_usage_percent")
	require.Contains(t, sends[0].Msg.Body, "当前值：95（阈值 > 90）")
	require.Contains(t, sends[0].Msg.Body, "作用域：platform=openai group_id=3")
	require.Contains(t, sends[0].Msg.Body, "触发时间：")
	require.Equal(t, "sub2api", sends[0].Msg.Group)
	require.Equal(t, "https://ops.example.com/alerts", sends[0].Msg.URL)

	// 同一告警仍活跃时再评估一轮：不再落库也不再推送。
	repo.active = repo.created[0]
	svc.evaluateOnce(60 * time.Second)
	require.Len(t, repo.created, 1)
	require.Len(t, sender.sent(), 1)
}

func TestOpsAlertEvaluator_ResolvePushesWhenSwitchOn(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newNotifyEvaluatorFixture(t, true, true)
	firedAt := time.Now().UTC().Add(-5 * time.Minute)
	repo.active = &OpsAlertEvent{ID: 42, RuleID: 1, Status: OpsAlertStatusFiring, FiredAt: firedAt}
	repo.metrics = &OpsSystemMetricsSnapshot{CPUUsagePercent: float64Ptr(40)}

	svc.evaluateOnce(60 * time.Second)

	require.Equal(t, []int64{42}, repo.resolved)
	require.Empty(t, repo.created)
	sends := sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "[Sub2API] 已恢复 CPU 过高", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "当前值：40（阈值 > 90）")
	require.Contains(t, sends[0].Msg.Body, "恢复时间：")
	require.Contains(t, sends[0].Msg.Body, "持续 5 分钟")
}

func TestOpsAlertEvaluator_ResolveSkipsPushWhenSwitchOff(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newNotifyEvaluatorFixture(t, true, false)
	repo.active = &OpsAlertEvent{ID: 7, RuleID: 1, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC().Add(-time.Minute)}
	repo.metrics = &OpsSystemMetricsSnapshot{CPUUsagePercent: float64Ptr(10)}

	svc.evaluateOnce(60 * time.Second)

	require.Equal(t, []int64{7}, repo.resolved, "解除仍要落库")
	require.Empty(t, sender.sent(), "notify_on_resolve=false 时不推恢复")
}

func TestOpsAlertEvaluator_PushFailureDoesNotAffectEventPersistence(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newNotifyEvaluatorFixture(t, true, true)
	sender.sendErr = errors.New("bark unreachable")

	svc.evaluateOnce(60 * time.Second)

	require.Len(t, repo.created, 1, "推送失败不影响事件落库")
	require.Len(t, sender.sent(), 1, "确实尝试过推送")
	require.NotEmpty(t, repo.heartbeats)
	last := repo.heartbeats[len(repo.heartbeats)-1]
	require.True(t, strings.Contains(last, "created=1"), "心跳结果应记录本轮成功创建事件: %s", last)

	// 解除时推送失败同样不影响状态更新。
	repo.active = repo.created[0]
	repo.metrics = &OpsSystemMetricsSnapshot{CPUUsagePercent: float64Ptr(1)}
	svc.evaluateOnce(60 * time.Second)
	require.Equal(t, []int64{1}, repo.resolved)
	require.Len(t, sender.sent(), 2)
}

func TestOpsAlertEvaluator_NoPushWhenBarkDisabledOrAbsent(t *testing.T) {
	t.Parallel()

	svc, repo, sender := newNotifyEvaluatorFixture(t, false, true)
	svc.evaluateOnce(60 * time.Second)
	require.Len(t, repo.created, 1)
	require.Empty(t, sender.sent(), "Bark 关闭时评估流程照旧但不外发")

	// 完全没有注入通知服务（例如旧的测试装配）也不能 panic。
	bare := &OpsAlertEvaluatorService{
		opsRepo:    &notifyStubOpsRepo{rules: repo.rules, metrics: repo.metrics},
		ruleStates: map[int64]*opsAlertRuleState{},
	}
	require.NotPanics(t, func() { bare.evaluateOnce(60 * time.Second) })
}
