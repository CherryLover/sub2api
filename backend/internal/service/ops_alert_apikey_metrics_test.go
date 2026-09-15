//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// apiKeyAlertStubRepo 只实现评估器要的那一个查询（ListAPIKeysWithDailyRateLimit），
// 其余 APIKeyRepository 方法由嵌入的 nil 接口兜底 —— 本轮评估不会碰到它们。
type apiKeyAlertStubRepo struct {
	APIKeyRepository

	keys  []*APIKey
	err   error
	calls int
}

func (r *apiKeyAlertStubRepo) ListAPIKeysWithDailyRateLimit(context.Context) ([]*APIKey, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.keys, nil
}

// apiKeyAlertStubOpsRepo 在 notifyStubOpsRepo 之上按 API Key 维护活动 / 最新事件。
// 规则级与账号级的事件查询对密钥目标都不该被调用，调用即计数并报错。
type apiKeyAlertStubOpsRepo struct {
	notifyStubOpsRepo

	activeByKey    map[int64]*OpsAlertEvent
	latestByKey    map[int64]*OpsAlertEvent
	wrongKindCalls int
}

func newAPIKeyAlertStubOpsRepo(rules []*OpsAlertRule) *apiKeyAlertStubOpsRepo {
	return &apiKeyAlertStubOpsRepo{
		notifyStubOpsRepo: notifyStubOpsRepo{rules: rules},
		activeByKey:       map[int64]*OpsAlertEvent{},
		latestByKey:       map[int64]*OpsAlertEvent{},
	}
}

func (r *apiKeyAlertStubOpsRepo) wrongKind(what string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wrongKindCalls++
	return errors.New(what + " lookup must not be used for api key targets")
}

func (r *apiKeyAlertStubOpsRepo) GetActiveAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return nil, r.wrongKind("rule-level active event")
}

func (r *apiKeyAlertStubOpsRepo) GetLatestAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return nil, r.wrongKind("rule-level latest event")
}

func (r *apiKeyAlertStubOpsRepo) GetActiveAlertEventForAccount(context.Context, int64, int64) (*OpsAlertEvent, error) {
	return nil, r.wrongKind("account-level active event")
}

func (r *apiKeyAlertStubOpsRepo) GetLatestAlertEventForAccount(context.Context, int64, int64) (*OpsAlertEvent, error) {
	return nil, r.wrongKind("account-level latest event")
}

func (r *apiKeyAlertStubOpsRepo) GetActiveAlertEventForAPIKey(_ context.Context, _ int64, apiKeyID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeByKey[apiKeyID], nil
}

func (r *apiKeyAlertStubOpsRepo) GetLatestAlertEventForAPIKey(_ context.Context, _ int64, apiKeyID int64) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestByKey[apiKeyID], nil
}

func (r *apiKeyAlertStubOpsRepo) CreateAlertEvent(_ context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	event.ID = r.nextID
	r.created = append(r.created, event)
	if id, ok := event.Dimensions["api_key_id"].(int64); ok {
		r.activeByKey[id] = event
		r.latestByKey[id] = event
	}
	return event, nil
}

func (r *apiKeyAlertStubOpsRepo) UpdateAlertEventStatus(_ context.Context, eventID int64, _ string, _ *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved = append(r.resolved, eventID)
	for id, ev := range r.activeByKey {
		if ev.ID == eventID {
			delete(r.activeByKey, id)
		}
	}
	return nil
}

type apiKeyAlertFixture struct {
	svc     *OpsAlertEvaluatorService
	repo    *apiKeyAlertStubOpsRepo
	keyRepo *apiKeyAlertStubRepo
	sender  *fakeBarkSender
}

func newAPIKeyAlertFixture(t *testing.T, barkEnabled bool, rules []*OpsAlertRule) *apiKeyAlertFixture {
	t.Helper()

	f := &apiKeyAlertFixture{
		repo:    newAPIKeyAlertStubOpsRepo(rules),
		keyRepo: &apiKeyAlertStubRepo{},
		sender:  &fakeBarkSender{},
	}
	bark := NewBarkNotificationService(newStubSettingRepo(), reversibleEncryptor{}, f.sender, true)
	_, err := bark.UpdateBarkConfig(context.Background(), BarkConfigInput{
		Enabled:         barkEnabled,
		ServerURL:       "https://api.day.app",
		DeviceKey:       "device-key",
		NotifyOnResolve: boolPtr(true),
	})
	require.NoError(t, err)

	f.svc = &OpsAlertEvaluatorService{
		opsService:    &OpsService{opsRepo: f.repo},
		opsRepo:       f.repo,
		apiKeyRepo:    f.keyRepo,
		alertNotifier: bark,
		ruleStates:    map[opsAlertRuleStateKey]*opsAlertRuleState{},
	}
	return f
}

// dailyLimitedKey 一把配了日限额、窗口还没过期的启用中密钥。
func dailyLimitedKey(id int64, name, userName string, used, limit float64) *APIKey {
	start := time.Now().UTC().Add(-time.Hour)
	userID := id * 10
	return &APIKey{
		ID:            id,
		UserID:        userID,
		Name:          name,
		Status:        StatusActive,
		RateLimit1d:   limit,
		Usage1d:       used,
		Window1dStart: &start,
		User:          &User{ID: userID, Username: userName},
	}
}

func apiKeyDailyRule() *OpsAlertRule {
	return &OpsAlertRule{
		ID: 1, Name: "密钥当日用量", Enabled: true, Severity: "P2",
		MetricType: OpsAlertMetricAPIKeyDailyUsedPercent, Operator: ">=", Threshold: 80,
	}
}

func TestReadAPIKeyDailyUsedPercent(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC().Add(-time.Hour)

	// 没配日限额：当无数据跳过（百分比没有分母，按 0 算会让规则永远不触发）。
	_, _, _, ok := readAPIKeyDailyUsedPercent(&APIKey{ID: 1, Usage1d: 12})
	require.False(t, ok, "RateLimit1d <= 0 必须跳过")
	_, _, _, ok = readAPIKeyDailyUsedPercent(nil)
	require.False(t, ok)

	// 正常：425 / 500 = 85%。
	percent, used, limit, ok := readAPIKeyDailyUsedPercent(&APIKey{
		ID: 2, RateLimit1d: 500, Usage1d: 425, Window1dStart: &start,
	})
	require.True(t, ok)
	require.InDelta(t, 85, percent, 0.0001)
	require.InDelta(t, 425, used, 0.0001)
	require.InDelta(t, 500, limit, 0.0001)

	// 1 天窗口已过期：裸 Usage1d 还留着上个窗口的旧值，EffectiveUsage1d() 记 0 → 百分比 0。
	expiredStart := time.Now().UTC().Add(-25 * time.Hour)
	percent, used, _, ok = readAPIKeyDailyUsedPercent(&APIKey{
		ID: 3, RateLimit1d: 500, Usage1d: 425, Window1dStart: &expiredStart,
	})
	require.True(t, ok)
	require.Zero(t, percent, "窗口过期后用量按 0 算")
	require.Zero(t, used)

	// 压根没有窗口起点，同样视为已过期。
	percent, _, _, ok = readAPIKeyDailyUsedPercent(&APIKey{ID: 4, RateLimit1d: 500, Usage1d: 425})
	require.True(t, ok)
	require.Zero(t, percent)
}

func TestCollectAPIKeyMetricSamples_SkipsKeysWithoutDailyLimit(t *testing.T) {
	t.Parallel()

	f := newAPIKeyAlertFixture(t, false, nil)
	f.keyRepo.keys = []*APIKey{
		dailyLimitedKey(2, "dev-key", "张三", 425, 500),
		{ID: 3, Name: "no-limit", Status: StatusActive},
		dailyLimitedKey(1, "ci-key", "李四", 50, 500),
	}

	samples, err := f.svc.collectAPIKeyMetricSamples(context.Background(), apiKeyDailyRule())
	require.NoError(t, err)
	require.Equal(t, 1, f.keyRepo.calls)
	require.Len(t, samples, 2, "没配日限额的密钥被跳过")
	require.Equal(t, int64(1), samples[0].APIKeyID, "按密钥 ID 升序")
	require.InDelta(t, 10, samples[0].Value, 0.0001)
	require.Equal(t, int64(2), samples[1].APIKeyID)
	require.InDelta(t, 85, samples[1].Value, 0.0001)
	require.Equal(t, "dev-key", samples[1].APIKeyName)
	require.Equal(t, "张三", samples[1].UserName)
	require.Equal(t, int64(20), samples[1].UserID)
	require.Equal(t, "张三 / dev-key：85%（425.00 / 500.00 USD）", formatOpsAlertAPIKeyLine(samples[1]))

	// 仓储报错：整条规则当作取不到数据，不落事件。
	f.keyRepo.err = errors.New("db down")
	_, err = f.svc.collectAPIKeyMetricSamples(context.Background(), apiKeyDailyRule())
	require.Error(t, err)
	require.Nil(t, f.svc.buildAPIKeyTargets(context.Background(), apiKeyDailyRule()))
}

// TestOpsAlertEvaluator_APIKeyTargetsFireAndResolveIndependently
// 两把密钥先后越阈各自触发一次、活动事件在时不重复推、窗口滚过去后各自恢复。
func TestOpsAlertEvaluator_APIKeyTargetsFireAndResolveIndependently(t *testing.T) {
	t.Parallel()

	f := newAPIKeyAlertFixture(t, true, []*OpsAlertRule{apiKeyDailyRule()})
	f.keyRepo.keys = []*APIKey{
		dailyLimitedKey(1, "dev-key", "张三", 425, 500),
		dailyLimitedKey(2, "ci-key", "李四", 100, 500),
	}

	// 第一轮：只有 1 号越阈（85% >= 80%）。
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 1)
	require.Equal(t, int64(1), f.repo.created[0].Dimensions["api_key_id"])
	require.Equal(t, "dev-key", f.repo.created[0].Dimensions["api_key_name"])
	require.Equal(t, int64(10), f.repo.created[0].Dimensions["user_id"])
	require.Equal(t, "张三", f.repo.created[0].Dimensions["user_name"])
	require.Nil(t, f.repo.created[0].Dimensions["account_id"], "密钥事件不写 account_id")
	require.Equal(t, "API Key 当日用量：张三 / dev-key 当前 85%，阈值 >= 80%", f.repo.created[0].Description)
	require.Equal(t, 0, f.repo.wrongKindCalls, "密钥目标不能用规则级 / 账号级事件查询")
	require.Contains(t, f.repo.heartbeats[len(f.repo.heartbeats)-1], "evaluated=1 created=1 resolved=0")

	sends := f.sender.sent()
	require.Len(t, sends, 1)
	require.Equal(t, "[Sub2API] P2 密钥当日用量", sends[0].Msg.Title)
	require.Contains(t, sends[0].Msg.Body, "指标：API Key 当日用量")
	require.Contains(t, sends[0].Msg.Body, "张三 / dev-key：85%（425.00 / 500.00 USD）")
	require.Contains(t, sends[0].Msg.Body, "当前值：85%（阈值 >= 80%）")

	// 第二轮：1 号有活动事件不重复推；2 号涨到 90% → 它自己触发一次。
	f.keyRepo.keys[1] = dailyLimitedKey(2, "ci-key", "李四", 450, 500)
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 2)
	require.Equal(t, int64(2), f.repo.created[1].Dimensions["api_key_id"])
	require.Len(t, f.sender.sent(), 2)

	// 第三轮：1 号的日窗口滚过去 → EffectiveUsage1d() 记 0 → 只有 1 号恢复，2 号仍活跃。
	expired := time.Now().UTC().Add(-25 * time.Hour)
	f.keyRepo.keys[0].Window1dStart = &expired
	f.svc.evaluateOnce(60 * time.Second)
	require.Equal(t, []int64{1}, f.repo.resolved)
	require.Len(t, f.repo.created, 2)
	sends = f.sender.sent()
	require.Len(t, sends, 3)
	require.Equal(t, "[Sub2API] 已恢复 密钥当日用量", sends[2].Msg.Title)
	require.Contains(t, sends[2].Msg.Body, "张三 / dev-key：0%（0.00 / 500.00 USD）")
	require.Len(t, f.repo.activeByKey, 1, "2 号仍在触发中")
	require.Equal(t, 0, f.repo.wrongKindCalls)
}

func TestOpsAlertEvaluator_APIKeyRuleWithoutDataSkips(t *testing.T) {
	t.Parallel()

	f := newAPIKeyAlertFixture(t, true, []*OpsAlertRule{apiKeyDailyRule()})
	f.keyRepo.keys = []*APIKey{{ID: 1, Name: "no-limit", Status: StatusActive}}

	f.svc.evaluateOnce(60 * time.Second)
	require.Empty(t, f.repo.created)
	require.Empty(t, f.sender.sent())
	require.Contains(t, f.repo.heartbeats[len(f.repo.heartbeats)-1], "enabled=1 evaluated=0 created=0")
	require.Empty(t, f.svc.ruleStates, "没配日限额的密钥不留持续计数")
}

func TestOpsAlertEvaluator_APIKeySustainedCountsPerKey(t *testing.T) {
	t.Parallel()

	rule := apiKeyDailyRule()
	rule.SustainedMinutes = 2 // 60s 间隔 → 需要连续 2 轮
	f := newAPIKeyAlertFixture(t, false, []*OpsAlertRule{rule})
	f.keyRepo.keys = []*APIKey{
		dailyLimitedKey(1, "dev-key", "张三", 425, 500),
		dailyLimitedKey(2, "ci-key", "李四", 425, 500),
	}

	f.svc.evaluateOnce(60 * time.Second)
	require.Empty(t, f.repo.created, "第一轮只计数不触发")
	require.Len(t, f.svc.ruleStates, 2)
	stateKey := opsAlertRuleStateKey{RuleID: 1, Kind: opsAlertTargetKindAPIKey, TargetID: 1}
	require.Equal(t, 1, f.svc.ruleStates[stateKey].ConsecutiveBreaches)
	// 账号维度同 ID 的键不存在：两种拆分维度的计数互不干扰。
	_, ok := f.svc.ruleStates[opsAlertRuleStateKey{RuleID: 1, Kind: opsAlertTargetKindAccount, TargetID: 1}]
	require.False(t, ok)

	// 2 号回落后 1 号仍连续越阈：只有 1 号触发。
	f.keyRepo.keys[1] = dailyLimitedKey(2, "ci-key", "李四", 10, 500)
	f.svc.evaluateOnce(60 * time.Second)
	require.Len(t, f.repo.created, 1)
	require.Equal(t, int64(1), f.repo.created[0].Dimensions["api_key_id"])

	// 密钥不再出现在候选里（被删 / 撤掉日限额 / 被禁用）后，它的计数被清掉。
	f.keyRepo.keys = f.keyRepo.keys[:1]
	f.svc.evaluateOnce(60 * time.Second)
	_, ok = f.svc.ruleStates[opsAlertRuleStateKey{RuleID: 1, Kind: opsAlertTargetKindAPIKey, TargetID: 2}]
	require.False(t, ok)
}

// TestOpsAlertEvaluator_APIKeyEvaluateRuleNow 「立即试算」复用 accounts[] 承载密钥明细。
func TestOpsAlertEvaluator_APIKeyEvaluateRuleNow(t *testing.T) {
	t.Parallel()

	f := newAPIKeyAlertFixture(t, true, []*OpsAlertRule{apiKeyDailyRule()})
	f.keyRepo.keys = []*APIKey{
		dailyLimitedKey(1, "dev-key", "张三", 425, 500),
		dailyLimitedKey(2, "ci-key", "李四", 100, 500),
	}

	got, err := f.svc.EvaluateRuleNow(context.Background(), 1, true)
	require.NoError(t, err)
	require.Equal(t, OpsAlertMetricAPIKeyDailyUsedPercent, got.MetricType)
	require.True(t, got.HasData)
	require.NotNil(t, got.Value)
	require.InDelta(t, 85, *got.Value, 0.0001, "百分比聚合取最大")
	require.True(t, got.Breached)
	require.Len(t, got.Accounts, 2)
	// accounts[] 是复用的：account_id 放 key 的 id，account_name 放「用户名 / 密钥名」，platform 留空。
	require.Equal(t, int64(1), got.Accounts[0].AccountID)
	require.Equal(t, "张三 / dev-key", got.Accounts[0].AccountName)
	require.Empty(t, got.Accounts[0].Platform)
	require.True(t, got.Accounts[0].Breached)
	require.Equal(t, int64(2), got.Accounts[1].AccountID)
	require.Equal(t, "李四 / ci-key", got.Accounts[1].AccountName)
	require.False(t, got.Accounts[1].Breached)

	require.True(t, got.Sent)
	body := f.sender.sent()[0].Msg.Body
	require.Equal(t, "[Sub2API] 手动试发 密钥当日用量", f.sender.sent()[0].Msg.Title)
	require.Contains(t, body, "指标：API Key 当日用量")
	require.Contains(t, body, "当前值：85%（阈值 >= 80%）")
	require.Contains(t, body, "张三 / dev-key：85%（425.00 / 500.00 USD）（越阈）")
	require.Contains(t, body, "李四 / ci-key：20%（100.00 / 500.00 USD）")
	require.Empty(t, f.repo.created, "试算不落事件")
	require.Empty(t, f.svc.ruleStates, "试算不动持续计数")
}

func TestBuildOpsAlertAPIKeyManualDetails_CapsAtFiveLines(t *testing.T) {
	t.Parallel()

	rule := apiKeyDailyRule()
	samples := make([]apiKeyMetricSample, 0, 7)
	for i := int64(1); i <= 7; i++ {
		percent := float64(70 + i*3)
		used := percent * 5
		samples = append(samples, apiKeyMetricSample{
			APIKeyID:   i,
			APIKeyName: "k",
			UserName:   "u",
			Value:      percent,
			Used:       used,
			Limit:      500,
		})
	}
	lines := buildOpsAlertAPIKeyManualDetails(samples, rule)
	require.Len(t, lines, 6)
	require.Equal(t, "u / k：73%（365.00 / 500.00 USD）", lines[0])
	require.Equal(t, "u / k：85%（425.00 / 500.00 USD）（越阈）", lines[4])
	require.Equal(t, "另有 2 把密钥", lines[5])
	require.Nil(t, buildOpsAlertAPIKeyManualDetails(nil, rule))

	// 没有用户名时退化成只有密钥名，密钥名也缺时用 #id。
	require.Equal(t, "k", formatOpsAlertAPIKeyName(apiKeyMetricSample{APIKeyID: 9, APIKeyName: "k"}))
	require.Equal(t, "#9", formatOpsAlertAPIKeyName(apiKeyMetricSample{APIKeyID: 9}))
}
