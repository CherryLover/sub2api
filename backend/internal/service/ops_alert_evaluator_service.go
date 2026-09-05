package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAlertEvaluatorJobName = "ops_alert_evaluator"

	opsAlertEvaluatorTimeout         = 45 * time.Second
	opsAlertEvaluatorLeaderLockKey   = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTL   = 90 * time.Second
	opsAlertEvaluatorSkipLogInterval = 1 * time.Minute
)

var opsAlertEvaluatorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsAlertEvaluatorService struct {
	opsService *OpsService
	opsRepo    OpsRepository
	proxyRepo  ProxyRepository
	// usageLogRepo 账号今日费用指标（account_today_cost）读用量日志用；为 nil 时该指标无数据。
	usageLogRepo UsageLogRepository

	// alertNotifier 告警出口（Bark）。为 nil 或未启用时评估流程与从前完全一致，只落库不外发。
	alertNotifier *BarkNotificationService

	redisClient *redis.Client
	cfg         *config.Config
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	mu         sync.Mutex
	ruleStates map[opsAlertRuleStateKey]*opsAlertRuleState

	skipLogMu sync.Mutex
	skipLogAt time.Time

	warnNoRedisOnce sync.Once
}

// opsAlertRuleStateKey 持续计数的键：健康度指标一条规则一个目标（AccountID 为 0），
// 账号用量类指标按「规则 × 账号」各自计数。
type opsAlertRuleStateKey struct {
	RuleID    int64
	AccountID int64
}

type opsAlertRuleState struct {
	LastEvaluatedAt     time.Time
	ConsecutiveBreaches int
}

// opsAlertEvalTarget 一次评估的最小单位：健康度指标一条规则一个目标（accountID=0），
// 账号用量类指标每个账号一个目标，各自查活动事件、各自冷却、各自推送。
type opsAlertEvalTarget struct {
	accountID   int64
	value       float64
	description string
	dimensions  map[string]any
	// notification 推送用的基础字段（不含触发 / 恢复时间）。
	notification OpsAlertNotification
}

// opsAlertRuleScope 规则作用域（静默匹配用）。
type opsAlertRuleScope struct {
	platform string
	groupID  *int64
	region   *string
}

type opsAlertEvalCounters struct {
	created  int
	resolved int
}

func NewOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
	alertNotifier *BarkNotificationService,
	usageLogRepo UsageLogRepository,
) *OpsAlertEvaluatorService {
	return &OpsAlertEvaluatorService{
		opsService:    opsService,
		opsRepo:       opsRepo,
		proxyRepo:     proxyRepo,
		usageLogRepo:  usageLogRepo,
		alertNotifier: alertNotifier,
		redisClient:   redisClient,
		cfg:           cfg,
		instanceID:    uuid.NewString(),
		ruleStates:    map[opsAlertRuleStateKey]*opsAlertRuleState{},
	}
}

func (s *OpsAlertEvaluatorService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpsAlertEvaluatorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.wg.Wait()
}

func (s *OpsAlertEvaluatorService) run() {
	defer s.wg.Done()

	// Start immediately to produce early feedback in ops dashboard.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			interval := s.getInterval()
			s.evaluateOnce(interval)
			timer.Reset(interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAlertEvaluatorService) getInterval() time.Duration {
	// Default.
	interval := 60 * time.Second

	if s == nil || s.opsService == nil {
		return interval
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg, err := s.opsService.GetOpsAlertRuntimeSettings(ctx)
	if err != nil || cfg == nil {
		return interval
	}
	if cfg.EvaluationIntervalSeconds <= 0 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds < 1 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds > int((24 * time.Hour).Seconds()) {
		return interval
	}
	return time.Duration(cfg.EvaluationIntervalSeconds) * time.Second
}

func (s *OpsAlertEvaluatorService) evaluateOnce(interval time.Duration) {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAlertEvaluatorTimeout)
	defer cancel()

	if s.opsService != nil && !s.opsService.IsMonitoringEnabled(ctx) {
		return
	}

	runtimeCfg := defaultOpsAlertRuntimeSettings()
	if s.opsService != nil {
		if loaded, err := s.opsService.GetOpsAlertRuntimeSettings(ctx); err == nil && loaded != nil {
			runtimeCfg = loaded
		}
	}

	release, ok := s.tryAcquireLeaderLock(ctx, runtimeCfg.DistributedLock)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list rules failed: %v", err)
		return
	}

	rulesTotal := len(rules)
	rulesEnabled := 0
	rulesEvaluated := 0
	counters := opsAlertEvalCounters{}

	now := time.Now().UTC()
	safeEnd := now.Truncate(time.Minute)
	if safeEnd.IsZero() {
		safeEnd = now
	}

	systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)

	// Cleanup stale state for removed rules.
	s.pruneRuleStates(rules)

	for _, rule := range rules {
		if rule == nil || !rule.Enabled || rule.ID <= 0 {
			continue
		}
		rulesEnabled++

		scopePlatform, scopeGroupID, scopeRegion := parseOpsAlertRuleScope(rule.Filters)
		scope := opsAlertRuleScope{platform: scopePlatform, groupID: scopeGroupID, region: scopeRegion}

		// 账号用量类指标：一条规则拆成 N 个账号目标，各自触发 / 恢复。
		if IsOpsAlertAccountMetric(rule.MetricType) {
			targets := s.buildAccountTargets(ctx, rule, now)
			s.pruneRuleAccountStates(rule.ID, targets)
			if len(targets) == 0 {
				continue
			}
			rulesEvaluated++
			for _, target := range targets {
				s.evaluateTarget(ctx, rule, target, scope, now, interval, &counters)
			}
			continue
		}

		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		windowStart := safeEnd.Add(-time.Duration(windowMinutes) * time.Minute)
		windowEnd := safeEnd

		metricValue, ok := s.computeRuleMetric(ctx, rule, systemMetrics, windowStart, windowEnd, scopePlatform, scopeGroupID)
		if !ok {
			s.resetRuleState(opsAlertRuleStateKey{RuleID: rule.ID}, now)
			continue
		}
		rulesEvaluated++

		s.evaluateTarget(ctx, rule, opsAlertEvalTarget{
			value:        metricValue,
			description:  buildOpsAlertDescription(rule, metricValue, windowMinutes, scopePlatform, scopeGroupID),
			dimensions:   buildOpsAlertDimensions(scopePlatform, scopeGroupID),
			notification: buildOpsAlertNotification(rule, metricValue),
		}, scope, now, interval, &counters)
	}

	result := truncateString(fmt.Sprintf("rules=%d enabled=%d evaluated=%d created=%d resolved=%d", rulesTotal, rulesEnabled, rulesEvaluated, counters.created, counters.resolved), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

// evaluateTarget 对一个评估目标走完整流程：持续计数 → 查活动事件 → 越阈则（静默 / 冷却后）落事件并推送，
// 未越阈则解除活动事件并推「已恢复」。健康度指标 accountID=0，行为与拆分前完全一致。
func (s *OpsAlertEvaluatorService) evaluateTarget(
	ctx context.Context,
	rule *OpsAlertRule,
	target opsAlertEvalTarget,
	scope opsAlertRuleScope,
	now time.Time,
	interval time.Duration,
	counters *opsAlertEvalCounters,
) {
	if s == nil || rule == nil || counters == nil {
		return
	}
	stateKey := opsAlertRuleStateKey{RuleID: rule.ID, AccountID: target.accountID}

	breachedNow := compareMetric(target.value, rule.Operator, rule.Threshold)
	required := requiredSustainedBreaches(rule.SustainedMinutes, interval)
	consecutive := s.updateRuleBreaches(stateKey, now, interval, breachedNow)

	activeEvent, err := s.getActiveAlertEvent(ctx, rule.ID, target.accountID)
	if err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get active event failed (rule=%d account=%d): %v", rule.ID, target.accountID, err)
		return
	}

	if breachedNow && consecutive >= required {
		if activeEvent != nil {
			return
		}

		// Scoped silencing: if a matching silence exists, skip creating a firing event.
		if s.opsService != nil {
			platform := strings.TrimSpace(scope.platform)
			if platform != "" {
				if ok, err := s.opsService.IsAlertSilenced(ctx, rule.ID, platform, scope.groupID, scope.region, now); err == nil && ok {
					return
				}
			}
		}

		latestEvent, err := s.getLatestAlertEvent(ctx, rule.ID, target.accountID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get latest event failed (rule=%d account=%d): %v", rule.ID, target.accountID, err)
			return
		}
		if latestEvent != nil && rule.CooldownMinutes > 0 {
			cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
			if now.Sub(latestEvent.FiredAt) < cooldown {
				return
			}
		}

		firedEvent := &OpsAlertEvent{
			RuleID:         rule.ID,
			Severity:       strings.TrimSpace(rule.Severity),
			Status:         OpsAlertStatusFiring,
			Title:          fmt.Sprintf("%s: %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name)),
			Description:    target.description,
			MetricValue:    float64Ptr(target.value),
			ThresholdValue: float64Ptr(rule.Threshold),
			Dimensions:     target.dimensions,
			FiredAt:        now,
			CreatedAt:      now,
		}

		if _, err := s.opsRepo.CreateAlertEvent(ctx, firedEvent); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] create event failed (rule=%d account=%d): %v", rule.ID, target.accountID, err)
			return
		}

		counters.created++
		// 事件已落库，外发只是附加通道：失败只记日志，不影响本轮评估。
		s.notifyAlertFired(ctx, rule, target.notification, now)
		return
	}

	// Not breached: resolve active event if present.
	if activeEvent != nil {
		resolvedAt := now
		if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve event failed (event=%d): %v", activeEvent.ID, err)
		} else {
			counters.resolved++
			s.notifyAlertResolved(ctx, rule, target.notification, activeEvent.FiredAt, resolvedAt)
		}
	}
}

// buildAccountTargets 把账号用量类规则拆成按账号的评估目标；取不到账号或全部无数据时返回空。
func (s *OpsAlertEvaluatorService) buildAccountTargets(ctx context.Context, rule *OpsAlertRule, now time.Time) []opsAlertEvalTarget {
	samples, err := s.collectAccountMetricSamples(ctx, rule, now)
	if err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list accounts failed (rule=%d): %v", rule.ID, err)
		return nil
	}
	if len(samples) == 0 {
		return nil
	}
	filters := parseOpsAlertAccountFilters(rule.Filters)
	targets := make([]opsAlertEvalTarget, 0, len(samples))
	for _, sample := range samples {
		notification := buildOpsAlertNotification(rule, sample.Value)
		notification.MetricLabel = opsAlertAccountMetricLabel(rule.MetricType, filters)
		notification.Unit = opsAlertAccountMetricUnit(rule.MetricType, sample.Currency)
		notification.Details = []string{formatOpsAlertAccountLine(sample)}
		targets = append(targets, opsAlertEvalTarget{
			accountID:    sample.AccountID,
			value:        sample.Value,
			description:  buildOpsAlertAccountDescription(rule, sample, filters),
			dimensions:   buildOpsAlertAccountDimensions(sample, filters),
			notification: notification,
		})
	}
	return targets
}

func (s *OpsAlertEvaluatorService) getActiveAlertEvent(ctx context.Context, ruleID, accountID int64) (*OpsAlertEvent, error) {
	if accountID > 0 {
		return s.opsRepo.GetActiveAlertEventForAccount(ctx, ruleID, accountID)
	}
	return s.opsRepo.GetActiveAlertEvent(ctx, ruleID)
}

func (s *OpsAlertEvaluatorService) getLatestAlertEvent(ctx context.Context, ruleID, accountID int64) (*OpsAlertEvent, error) {
	if accountID > 0 {
		return s.opsRepo.GetLatestAlertEventForAccount(ctx, ruleID, accountID)
	}
	return s.opsRepo.GetLatestAlertEvent(ctx, ruleID)
}

// EvaluateRuleNow 立即试算一条规则：用当前数据算一遍并返回明细，不落事件、不动持续计数、
// 不取领导锁。send 为真时把结果以「手动试发」文案推到 Bark：Bark 未启用直接返回
// ErrBarkNotEnabled；推送失败不算接口失败，结果里 sent=false 并带 send_error。
func (s *OpsAlertEvaluatorService) EvaluateRuleNow(ctx context.Context, ruleID int64, send bool) (*OpsAlertRuleEvaluation, error) {
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if send && (s.alertNotifier == nil || !s.alertNotifier.IsEnabled(ctx)) {
		return nil, ErrBarkNotEnabled
	}

	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		return nil, err
	}
	var rule *OpsAlertRule
	for _, candidate := range rules {
		if candidate != nil && candidate.ID == ruleID {
			rule = candidate
			break
		}
	}
	if rule == nil {
		return nil, infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
	}

	now := time.Now().UTC()
	result := &OpsAlertRuleEvaluation{
		RuleID:      rule.ID,
		RuleName:    strings.TrimSpace(rule.Name),
		MetricType:  strings.TrimSpace(rule.MetricType),
		Operator:    strings.TrimSpace(rule.Operator),
		Threshold:   rule.Threshold,
		EvaluatedAt: now,
		Accounts:    []OpsAlertAccountSample{},
	}
	notification := buildOpsAlertNotification(rule, 0)

	if IsOpsAlertAccountMetric(rule.MetricType) {
		samples, err := s.collectAccountMetricSamples(ctx, rule, now)
		if err != nil {
			return nil, err
		}
		filters := parseOpsAlertAccountFilters(rule.Filters)
		notification.MetricLabel = opsAlertAccountMetricLabel(rule.MetricType, filters)
		var aggregate float64
		for i, sample := range samples {
			breached := compareMetric(sample.Value, rule.Operator, rule.Threshold)
			result.Accounts = append(result.Accounts, OpsAlertAccountSample{
				AccountID:   sample.AccountID,
				AccountName: sample.AccountName,
				Platform:    sample.Platform,
				Value:       sample.Value,
				Breached:    breached,
				Currency:    sample.Currency,
			})
			if breached {
				result.Breached = true
			}
			// 聚合：percent / cost 取最大，balance 取最小。
			if i == 0 || (rule.MetricType == OpsAlertMetricAccountBalance && sample.Value < aggregate) ||
				(rule.MetricType != OpsAlertMetricAccountBalance && sample.Value > aggregate) {
				aggregate = sample.Value
			}
			if notification.Unit == "" {
				notification.Unit = opsAlertAccountMetricUnit(rule.MetricType, sample.Currency)
			}
		}
		if len(samples) > 0 {
			result.HasData = true
			result.Value = float64Ptr(aggregate)
		}
		notification.Details = buildOpsAlertManualDetails(samples, rule)
	} else {
		safeEnd := now.Truncate(time.Minute)
		if safeEnd.IsZero() {
			safeEnd = now
		}
		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		scopePlatform, scopeGroupID, _ := parseOpsAlertRuleScope(rule.Filters)
		systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)
		value, ok := s.computeRuleMetric(ctx, rule, systemMetrics, safeEnd.Add(-time.Duration(windowMinutes)*time.Minute), safeEnd, scopePlatform, scopeGroupID)
		if ok {
			result.HasData = true
			result.Value = float64Ptr(value)
			result.Breached = compareMetric(value, rule.Operator, rule.Threshold)
		}
	}

	if !send {
		return result, nil
	}
	if result.Value != nil {
		notification.Value = *result.Value
	} else {
		notification.Value = math.NaN()
	}
	notification.FiredAt = now
	if err := s.alertNotifier.NotifyOpsAlertManual(ctx, notification, result.HasData, result.Breached); err != nil {
		if errors.Is(err, ErrBarkNotEnabled) {
			return nil, err
		}
		result.SendError = err.Error()
		return result, nil
	}
	result.Sent = true
	return result, nil
}

// buildOpsAlertManualDetails 手动试发正文里的账号行：最多 5 行，其余折成「另有 N 个账号」。
func buildOpsAlertManualDetails(samples []accountMetricSample, rule *OpsAlertRule) []string {
	const maxLines = 5
	if len(samples) == 0 || rule == nil {
		return nil
	}
	lines := make([]string, 0, maxLines+1)
	for i, sample := range samples {
		if i >= maxLines {
			lines = append(lines, fmt.Sprintf("另有 %d 个账号", len(samples)-maxLines))
			break
		}
		unit := opsAlertAccountMetricUnit(rule.MetricType, sample.Currency)
		line := fmt.Sprintf("%s：%s%s", formatOpsAlertAccountLine(sample), formatBarkNumber(sample.Value), unit)
		if compareMetric(sample.Value, rule.Operator, rule.Threshold) {
			line += "（越阈）"
		}
		lines = append(lines, line)
	}
	return lines
}

// notifyAlertFired 把刚落库的告警事件推到 Bark；通道未启用时是空操作。
func (s *OpsAlertEvaluatorService) notifyAlertFired(ctx context.Context, rule *OpsAlertRule, n OpsAlertNotification, firedAt time.Time) {
	if s == nil || s.alertNotifier == nil || rule == nil {
		return
	}
	n.FiredAt = firedAt
	n.ResolvedAt = nil
	if err := s.alertNotifier.NotifyOpsAlertFired(ctx, n); err != nil {
		slog.Warn("ops_alert_bark_notify_failed", "kind", "fired", "rule_id", rule.ID, "rule", rule.Name, "error", err)
	}
}

// notifyAlertResolved 告警解除后推「已恢复」；是否推由 Bark 配置里的 notify_on_resolve 决定。
func (s *OpsAlertEvaluatorService) notifyAlertResolved(ctx context.Context, rule *OpsAlertRule, n OpsAlertNotification, firedAt, resolvedAt time.Time) {
	if s == nil || s.alertNotifier == nil || rule == nil {
		return
	}
	n.FiredAt = firedAt
	n.ResolvedAt = &resolvedAt
	if err := s.alertNotifier.NotifyOpsAlertResolved(ctx, n); err != nil {
		slog.Warn("ops_alert_bark_notify_failed", "kind", "resolved", "rule_id", rule.ID, "rule", rule.Name, "error", err)
	}
}

// buildOpsAlertNotification 由规则与当前值组出推送摘要的基础字段；触发 / 恢复时间由调用方补。
func buildOpsAlertNotification(rule *OpsAlertRule, value float64) OpsAlertNotification {
	return OpsAlertNotification{
		RuleName:   strings.TrimSpace(rule.Name),
		Severity:   strings.TrimSpace(rule.Severity),
		MetricType: strings.TrimSpace(rule.MetricType),
		Operator:   strings.TrimSpace(rule.Operator),
		Threshold:  rule.Threshold,
		Value:      value,
		Scope:      FormatOpsAlertScope(rule.Filters),
	}
}

func (s *OpsAlertEvaluatorService) pruneRuleStates(rules []*OpsAlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	live := map[int64]struct{}{}
	for _, r := range rules {
		if r != nil && r.ID > 0 {
			live[r.ID] = struct{}{}
		}
	}
	for key := range s.ruleStates {
		if _, ok := live[key.RuleID]; !ok {
			delete(s.ruleStates, key)
		}
	}
}

// pruneRuleAccountStates 清掉本轮不再出现的「规则 × 账号」计数（账号被删 / 移出作用域 / 无数据）。
func (s *OpsAlertEvaluatorService) pruneRuleAccountStates(ruleID int64, targets []opsAlertEvalTarget) {
	if ruleID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	live := make(map[int64]struct{}, len(targets))
	for _, target := range targets {
		live[target.accountID] = struct{}{}
	}
	for key := range s.ruleStates {
		if key.RuleID != ruleID {
			continue
		}
		if _, ok := live[key.AccountID]; !ok {
			delete(s.ruleStates, key)
		}
	}
}

func (s *OpsAlertEvaluatorService) resetRuleState(key opsAlertRuleStateKey, now time.Time) {
	if key.RuleID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ruleStates == nil {
		s.ruleStates = map[opsAlertRuleStateKey]*opsAlertRuleState{}
	}
	state, ok := s.ruleStates[key]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[key] = state
	}
	state.LastEvaluatedAt = now
	state.ConsecutiveBreaches = 0
}

func (s *OpsAlertEvaluatorService) updateRuleBreaches(key opsAlertRuleStateKey, now time.Time, interval time.Duration, breached bool) int {
	if key.RuleID <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ruleStates == nil {
		s.ruleStates = map[opsAlertRuleStateKey]*opsAlertRuleState{}
	}

	state, ok := s.ruleStates[key]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[key] = state
	}

	if !state.LastEvaluatedAt.IsZero() && interval > 0 {
		if now.Sub(state.LastEvaluatedAt) > interval*2 {
			state.ConsecutiveBreaches = 0
		}
	}

	state.LastEvaluatedAt = now
	if breached {
		state.ConsecutiveBreaches++
	} else {
		state.ConsecutiveBreaches = 0
	}
	return state.ConsecutiveBreaches
}

func requiredSustainedBreaches(sustainedMinutes int, interval time.Duration) int {
	if sustainedMinutes <= 0 {
		return 1
	}
	if interval <= 0 {
		return sustainedMinutes
	}
	required := int(math.Ceil(float64(sustainedMinutes*60) / interval.Seconds()))
	if required < 1 {
		return 1
	}
	return required
}

func parseOpsAlertRuleScope(filters map[string]any) (platform string, groupID *int64, region *string) {
	if filters == nil {
		return "", nil, nil
	}
	if v, ok := filters["platform"]; ok {
		if s, ok := v.(string); ok {
			platform = strings.TrimSpace(s)
		}
	}
	if v, ok := filters["group_id"]; ok {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case int64:
			if t > 0 {
				id := t
				groupID = &id
			}
		case int:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err == nil && n > 0 {
				groupID = &n
			}
		}
	}
	if v, ok := filters["region"]; ok {
		if s, ok := v.(string); ok {
			vv := strings.TrimSpace(s)
			if vv != "" {
				region = &vv
			}
		}
	}
	return platform, groupID, region
}

func (s *OpsAlertEvaluatorService) computeRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	systemMetrics *OpsSystemMetricsSnapshot,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) (float64, bool) {
	if rule == nil {
		return 0, false
	}
	switch strings.TrimSpace(rule.MetricType) {
	case "cpu_usage_percent":
		if systemMetrics != nil && systemMetrics.CPUUsagePercent != nil {
			return *systemMetrics.CPUUsagePercent, true
		}
		return 0, false
	case "memory_usage_percent":
		if systemMetrics != nil && systemMetrics.MemoryUsagePercent != nil {
			return *systemMetrics.MemoryUsagePercent, true
		}
		return 0, false
	case "concurrency_queue_depth":
		if systemMetrics != nil && systemMetrics.ConcurrencyQueueDepth != nil {
			return float64(*systemMetrics.ConcurrencyQueueDepth), true
		}
		return 0, false
	case "group_available_accounts":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil {
			return 0, true
		}
		return float64(availability.Group.AvailableCount), true
	case "group_available_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return computeGroupAvailableRatio(availability.Group), true
	case "account_rate_limited_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})), true
	case "account_error_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})), true
	case "account_temp_unscheduled_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		now := time.Now().UTC()
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil)
		})), true
	case "group_rate_limit_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil || availability.Group.TotalAccounts <= 0 {
			return 0, true
		}
		return (float64(availability.Group.RateLimitCount) / float64(availability.Group.TotalAccounts)) * 100, true
	case "account_error_ratio":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		total := int64(len(availability.Accounts))
		if total <= 0 {
			return 0, true
		}
		errorCount := countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		return (float64(errorCount) / float64(total)) * 100, true
	case "overload_account_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsOverloaded
		})), true
	case "proxy_expired_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpired(ctx)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case "proxy_expiring_soon_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpiringSoon(ctx, time.Now())
		if err != nil {
			return 0, false
		}
		return float64(n), true
	}

	overview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: OpsQueryModeRaw,
	})
	if err != nil {
		return 0, false
	}
	if overview == nil {
		return 0, false
	}

	switch strings.TrimSpace(rule.MetricType) {
	case "success_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.SLA * 100, true
	case "error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.ErrorRate * 100, true
	case "upstream_error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.UpstreamErrorRate * 100, true
	default:
		return 0, false
	}
}

func compareMetric(value float64, operator string, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func buildOpsAlertDimensions(platform string, groupID *int64) map[string]any {
	dims := map[string]any{}
	if strings.TrimSpace(platform) != "" {
		dims["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		dims["group_id"] = *groupID
	}
	if len(dims) == 0 {
		return nil
	}
	return dims
}

func buildOpsAlertDescription(rule *OpsAlertRule, value float64, windowMinutes int, platform string, groupID *int64) string {
	if rule == nil {
		return ""
	}
	scope := "overall"
	if strings.TrimSpace(platform) != "" {
		scope = fmt.Sprintf("platform=%s", strings.TrimSpace(platform))
	}
	if groupID != nil && *groupID > 0 {
		scope = fmt.Sprintf("%s group_id=%d", scope, *groupID)
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	return fmt.Sprintf("%s %s %.2f (current %.2f) over last %dm (%s)",
		strings.TrimSpace(rule.MetricType),
		strings.TrimSpace(rule.Operator),
		rule.Threshold,
		value,
		windowMinutes,
		strings.TrimSpace(scope),
	)
}

func (s *OpsAlertEvaluatorService) tryAcquireLeaderLock(ctx context.Context, lock OpsDistributedLockSettings) (func(), bool) {
	if !lock.Enabled {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] redis not configured; running without distributed lock")
		})
		return nil, true
	}
	key := strings.TrimSpace(lock.Key)
	if key == "" {
		key = opsAlertEvaluatorLeaderLockKey
	}
	ttl := time.Duration(lock.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = opsAlertEvaluatorLeaderLockTTL
	}

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate evaluators stampeding the DB when Redis is flaky.
		// Single-node deployments can disable the distributed lock via runtime settings.
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock SetNX failed; skipping this cycle: %v", err)
		})
		return nil, false
	}
	if !ok {
		s.maybeLogSkip(key)
		return nil, false
	}
	return func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_, _ = opsAlertEvaluatorReleaseScript.Run(releaseCtx, s.redisClient, []string{key}, s.instanceID).Result()
	}, true
}

func (s *OpsAlertEvaluatorService) maybeLogSkip(key string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < opsAlertEvaluatorSkipLogInterval {
		return
	}
	s.skipLogAt = now
	logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock held by another instance; skipping (key=%q)", key)
}

func (s *OpsAlertEvaluatorService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsAlertEvaluatorService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

// computeGroupAvailableRatio returns the available percentage for a group.
// Formula: (AvailableCount / TotalAccounts) * 100.
// Returns 0 when TotalAccounts is 0.
func computeGroupAvailableRatio(group *GroupAvailability) float64 {
	if group == nil || group.TotalAccounts <= 0 {
		return 0
	}
	return (float64(group.AvailableCount) / float64(group.TotalAccounts)) * 100
}

// countAccountsByCondition counts accounts that satisfy the given condition.
func countAccountsByCondition(accounts map[int64]*AccountAvailability, condition func(*AccountAvailability) bool) int64 {
	if len(accounts) == 0 || condition == nil {
		return 0
	}
	var count int64
	for _, account := range accounts {
		if account != nil && condition(account) {
			count++
		}
	}
	return count
}
