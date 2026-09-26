package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

// 定时报错汇总推送。
//
// 站长的需求是"每天固定两个点告诉我谁在报错、报了什么错、各多少次"，而不是逐条告警。
// 所以这里不复用告警引擎的阈值/持续/冷却那一套，单独起一个 cron 任务：
// 到点把上一个时间段的 ops_error_logs 按 API Key 聚合成一条 Bark 通知。
//
// 调度部分完全照 OpsCleanupService 的模式写（五字段 cron + 跟随 cfg.Timezone +
// Redis 领导者锁 + 任务心跳 + 支持 Reload），因为它们面对的是同一类问题：
// 多实例部署时只该有一个节点跑，UI 改了配置要能不重启就换 schedule。
//
// 区间是"从上次成功运行到这次运行"，上次成功时间直接读任务心跳，不另存状态：
// 这样服务停过一段时间再起来，下一次汇总会自动把中间漏掉的那段一起算进去。
//
// ⚠️ 本文件的日志前缀是 [OpsDigest]，**不要**改回 [OpsErrorDigest]：
// logger.LegacyPrintf 按消息文本推断级别（inferStdLogLevel），消息里只要含 "error"
// 就整条判成 ERROR。前缀带 Error 会让这个服务的每一条日志（含正常的 scheduled /
// disabled）都以错误级别写进 ops_system_logs，把系统日志页糊满假错误。
// 真正该报错的那几条本来就带 "failed"，级别照样判得对。

const (
	opsErrorDigestJobName = "ops_error_digest"

	// settingKeyNotifyErrorDigestConfig 整段配置存 settings 表的这一个键，只走管理端接口，
	// 不进 /api/v1/settings/public（与 notify_bark_config 同样处理）。
	settingKeyNotifyErrorDigestConfig = "notify_error_digest_config"

	// opsErrorDigestDefaultSchedule 每天 11:30 与 17:30 各一条（时区跟随 cfg.Timezone）。
	opsErrorDigestDefaultSchedule = "30 11,17 * * *"

	opsErrorDigestDefaultTopKeys = 8
	// opsErrorDigestMaxTopKeys 一条 Bark 通知装不下几十个分组，给个上限防止填成 999。
	opsErrorDigestMaxTopKeys = 50
	// opsErrorDigestTopErrorTypes 每个密钥下最多列几种错误类型，剩下的不展开。
	opsErrorDigestTopErrorTypes = 3

	// opsErrorDigestFallbackWindow 首次运行、或距上次成功已超过这个时长时的兜底区间。
	// 再往前捞既慢又没意义：站长关心的是刚过去的这一段。
	opsErrorDigestFallbackWindow = 24 * time.Hour

	opsErrorDigestLeaderLockKey = "ops:error_digest:leader"
	opsErrorDigestLeaderLockTTL = 10 * time.Minute

	opsErrorDigestRunTimeout       = 2 * time.Minute
	opsErrorDigestHeartbeatTimeout = 2 * time.Second
	opsErrorDigestCronStopTimeout  = 3 * time.Second

	opsErrorDigestTimeLayout = "15:04"
	// opsErrorDigestUnassignedTitle 认证阶段就失败的请求还没解析出密钥，统一归到这一组。
	opsErrorDigestUnassignedTitle = "（未识别密钥）"
)

var opsErrorDigestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

var opsErrorDigestReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var (
	// ErrOpsErrorDigestScheduleInvalid cron 表达式写错时直接拒绝保存，而不是存进去等到点才发现不跑。
	ErrOpsErrorDigestScheduleInvalid = infraerrors.BadRequest(
		"ERROR_DIGEST_SCHEDULE_INVALID",
		"schedule must be a 5-field cron expression, e.g. \"30 11,17 * * *\"",
	)
	// ErrOpsErrorDigestConfigCorrupt 落库的 JSON 坏掉时报出来，读路径不再猜。
	ErrOpsErrorDigestConfigCorrupt = infraerrors.InternalServer(
		"ERROR_DIGEST_CONFIG_CORRUPT",
		"error digest config data is corrupted",
	)
)

// opsErrorDigestTypeLabels 错误类型的人话名字。取值范围见 handler 里的 isKnownOpsErrorType /
// normalizeOpsErrorType；没收录的类型原样显示，免得静默丢掉新出现的分类。
var opsErrorDigestTypeLabels = map[string]string{
	"upstream_error":               "上游错误",
	"upstream":                     "上游错误",
	"overloaded_error":             "上游过载",
	"rate_limit_error":             "触发限流",
	"authentication_error":         "认证失败",
	"invalid_request_error":        "请求无效",
	"billing_error":                "余额不足",
	"subscription_error":           "额度不足",
	"not_found_error":              "资源不存在",
	"forbidden_error":              "无权访问",
	"api_error":                    "接口错误",
	"cyber_policy":                 "内容合规拦截",
	"cyber_policy_session_blocked": "内容合规拦截（会话）",
	"image_generation_user_error":  "图片参数错误",
}

// OpsErrorDigestConfig 落库结构。
type OpsErrorDigestConfig struct {
	Enabled       bool      `json:"enabled"`
	Schedule      string    `json:"schedule"`
	SkipWhenEmpty bool      `json:"skip_when_empty"`
	TopKeys       int       `json:"top_keys"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OpsErrorDigestConfigView 管理端 GET / PUT 的返回结构。
type OpsErrorDigestConfigView struct {
	Enabled       bool       `json:"enabled"`
	Schedule      string     `json:"schedule"`
	SkipWhenEmpty bool       `json:"skip_when_empty"`
	TopKeys       int        `json:"top_keys"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// OpsErrorDigestConfigInput 管理端 PUT 的请求体。
// SkipWhenEmpty 缺省（nil）按默认值 true 处理；TopKeys <=0 表示没填，回落默认值。
type OpsErrorDigestConfigInput struct {
	Enabled       bool   `json:"enabled"`
	Schedule      string `json:"schedule"`
	SkipWhenEmpty *bool  `json:"skip_when_empty"`
	TopKeys       int    `json:"top_keys"`
}

// OpsErrorDigestTypeLine 一个密钥下的一种错误：「上游过载(529)」+ 次数。
type OpsErrorDigestTypeLine struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// OpsErrorDigestGroup 一个密钥的汇总。Types 已裁到前 opsErrorDigestTopErrorTypes 种。
type OpsErrorDigestGroup struct {
	Title string                   `json:"title"`
	Total int64                    `json:"total"`
	Types []OpsErrorDigestTypeLine `json:"types"`
}

// OpsErrorDigestSummary 一次汇总的全部结果。
//
// SLA 是真故障，Limited 是业务拦截（余额不足、额度用尽之类）。两者分开计数是这个功能的重点：
// 业务拦截每天都有一堆，混在一起真故障会被淹没。
//
// Groups 已按次数倒序裁到 TopKeys 个，剩下的折进 HiddenGroups / HiddenTotal。
type OpsErrorDigestSummary struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	Total   int64 `json:"total"`
	SLA     int64 `json:"sla"`
	Limited int64 `json:"limited"`

	Groups       []OpsErrorDigestGroup `json:"groups"`
	HiddenGroups int                   `json:"hidden_groups"`
	HiddenTotal  int64                 `json:"hidden_total"`
}

// OpsErrorDigestTestResult 「立即试推」的返回：算出来的汇总原样回给前端，
// 顺带说明这次到底推没推（Bark 没启用时不算失败，只是 Pushed=false）。
type OpsErrorDigestTestResult struct {
	Pushed bool   `json:"pushed"`
	Reason string `json:"reason,omitempty"`
	Title  string `json:"title"`
	Body   string `json:"body"`

	Summary *OpsErrorDigestSummary `json:"summary"`
}

// OpsErrorDigestService 定时把区间内的报错按 API Key 汇总成一条 Bark 通知。
type OpsErrorDigestService struct {
	opsRepo     OpsRepository
	settingRepo SettingRepository
	notifier    *BarkNotificationService
	redisClient *redis.Client
	cfg         *config.Config

	instanceID string
	now        func() time.Time

	// mu 守护 cron 实例切换 + effective 配置切换。不用 sync.Once 的原因同 OpsCleanupService：
	// Reload 要"停旧 cron 再起新 cron"，Once 触发过就没法再来一次。
	mu        sync.Mutex
	cron      *cron.Cron
	started   bool
	stopped   bool
	effective OpsErrorDigestConfig

	warnNoRedisOnce sync.Once
}

func NewOpsErrorDigestService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	notifier *BarkNotificationService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsErrorDigestService {
	return &OpsErrorDigestService{
		opsRepo:     opsRepo,
		settingRepo: settingRepo,
		notifier:    notifier,
		redisClient: redisClient,
		cfg:         cfg,
		instanceID:  uuid.NewString(),
		now:         time.Now,
		effective:   *defaultOpsErrorDigestConfig(),
	}
}

// timeNow 取当前时间。走一层包装是为了让测试能塞假时钟，同时兼容直接构造结构体
// （没走 NewOpsErrorDigestService）时 now 为 nil 的情况。
func (s *OpsErrorDigestService) timeNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func defaultOpsErrorDigestConfig() *OpsErrorDigestConfig {
	return &OpsErrorDigestConfig{
		Enabled:       false,
		Schedule:      opsErrorDigestDefaultSchedule,
		SkipWhenEmpty: true,
		TopKeys:       opsErrorDigestDefaultTopKeys,
	}
}

// ─── 调度 ───

// Start 首次启动 cron 调度。是否真的挂任务由 settings 里的 enabled 决定。重复调用幂等。
//
// 数据来源是 ops_error_logs，运维监控整体关掉（cfg.Ops.Enabled=false）时这张表不再写入，
// 汇总也就没有意义，所以这里跟 OpsCleanupService 一样直接不启动。
func (s *OpsErrorDigestService) Start() {
	if s == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}
	if s.opsRepo == nil || s.settingRepo == nil {
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] not started (missing deps)")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	if err := s.applyScheduleLocked(context.Background()); err != nil {
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] not started: %v", err)
	}
}

// Stop 关闭 cron。幂等。
func (s *OpsErrorDigestService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	s.stopCronLocked()
}

// Reload 重新读配置并按新 schedule 重建 cron（管理端保存配置后调用）。
// 未 Start 或已 Stop 时什么都不做。
func (s *OpsErrorDigestService) Reload(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stopped {
		return nil
	}
	return s.applyScheduleLocked(ctx)
}

// stopCronLocked 停掉当前 cron 实例（带超时）。调用方持锁。
func (s *OpsErrorDigestService) stopCronLocked() {
	if s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(opsErrorDigestCronStopTimeout):
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] cron stop timed out")
	}
	s.cron = nil
}

// applyScheduleLocked 重新读配置并按其 schedule 重建 cron。调用方持锁。
// enabled=false 时停掉旧 cron 就返回，不创建新的。
func (s *OpsErrorDigestService) applyScheduleLocked(ctx context.Context) error {
	s.computeEffectiveLocked(ctx)
	s.stopCronLocked()

	if !s.effective.Enabled {
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] cron disabled by settings")
		return nil
	}

	schedule := strings.TrimSpace(s.effective.Schedule)
	if schedule == "" {
		schedule = opsErrorDigestDefaultSchedule
	}

	c := cron.New(cron.WithParser(opsErrorDigestCronParser), cron.WithLocation(s.location()))
	if _, err := c.AddFunc(schedule, func() { s.runScheduled() }); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}
	c.Start()
	s.cron = c
	logger.LegacyPrintf("service.ops_error_digest",
		"[OpsDigest] scheduled (schedule=%q tz=%s top_keys=%d skip_when_empty=%v)",
		schedule, s.location().String(), s.effective.TopKeys, s.effective.SkipWhenEmpty,
	)
	return nil
}

// location cron 触发时刻与正文里的时间都按它渲染，跟随 cfg.Timezone；配错或没配就用 time.Local
// （启动时 timezone.Init 已经把 time.Local 设成了配置里的时区）。
func (s *OpsErrorDigestService) location() *time.Location {
	if s != nil && s.cfg != nil {
		if tz := strings.TrimSpace(s.cfg.Timezone); tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil && loc != nil {
				return loc
			}
		}
	}
	if loc := timezone.Location(); loc != nil {
		return loc
	}
	return time.Local
}

func (s *OpsErrorDigestService) computeEffectiveLocked(ctx context.Context) {
	// load 出错时返回的就是 nil，下面统一回落默认值。
	cfg, err := s.load(ctx)
	if err != nil {
		logger.LegacyPrintf("service.ops_error_digest",
			"[OpsDigest] read digest settings failed, using defaults: %v", err)
	}
	if cfg == nil {
		cfg = defaultOpsErrorDigestConfig()
	}
	s.effective = *cfg
}

func (s *OpsErrorDigestService) snapshotEffective() OpsErrorDigestConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effective
}

// refreshEffectiveBeforeRun cron 触发时刷新一次，让 skip_when_empty / top_keys 的改动当次生效。
// schedule 与 enabled 的改动由 Reload 负责（cron 调度归库管）。
func (s *OpsErrorDigestService) refreshEffectiveBeforeRun(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computeEffectiveLocked(ctx)
}

// ─── 执行 ───

func (s *OpsErrorDigestService) runScheduled() {
	if s == nil || s.opsRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsErrorDigestRunTimeout)
	defer cancel()

	s.refreshEffectiveBeforeRun(ctx)

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := s.timeNow()
	runAt := startedAt.UTC()

	result, err := s.runDigestOnce(ctx, startedAt, s.snapshotEffective())
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] digest failed: %v", err)
		return
	}
	// 即使这次因为"区间内没有报错"跳过了推送，也要记成功：心跳是下一次算区间的起点，
	// 不记的话窗口会一直往前长，直到撞上 24 小时兜底。
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

// runDigestOnce 算一次汇总并按配置决定推不推。返回给心跳用的一句话结果。
func (s *OpsErrorDigestService) runDigestOnce(
	ctx context.Context,
	now time.Time,
	cfg OpsErrorDigestConfig,
) (string, error) {
	start, end := s.resolveWindow(ctx, now)
	summary, err := s.buildSummary(ctx, start, end, cfg.TopKeys)
	if err != nil {
		return "", err
	}

	if summary.Total == 0 && cfg.SkipWhenEmpty {
		return "no errors in window, push skipped", nil
	}

	title, body := s.renderDigest(ctx, summary, end)
	pushed, err := s.push(ctx, title, body)
	if err != nil {
		return "", err
	}
	if !pushed {
		return fmt.Sprintf("errors=%d (sla=%d limited=%d), bark disabled", summary.Total, summary.SLA, summary.Limited), nil
	}
	return fmt.Sprintf("errors=%d (sla=%d limited=%d) pushed", summary.Total, summary.SLA, summary.Limited), nil
}

// RunManualDigest 「立即试推」：不看 schedule、不看 skip_when_empty，按最近 24 小时算一份并推送，
// 让站长在真到点之前就能确认通知长什么样、Bark 通不通。
func (s *OpsErrorDigestService) RunManualDigest(ctx context.Context) (*OpsErrorDigestTestResult, error) {
	if s == nil || s.opsRepo == nil {
		return nil, errors.New("ops error digest service not initialized")
	}
	now := s.timeNow()
	end := now.UTC()
	start := end.Add(-opsErrorDigestFallbackWindow)

	cfg, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = defaultOpsErrorDigestConfig()
	}

	summary, err := s.buildSummary(ctx, start, end, cfg.TopKeys)
	if err != nil {
		return nil, err
	}
	title, body := s.renderDigest(ctx, summary, end)

	out := &OpsErrorDigestTestResult{Title: title, Body: body, Summary: summary}
	pushed, err := s.push(ctx, title, body)
	if err != nil {
		return nil, err
	}
	out.Pushed = pushed
	if !pushed {
		out.Reason = "bark_disabled"
	}
	return out, nil
}

// push 把汇总交给 Bark。Bark 没启用时返回 (false, nil)：汇总是周期性的"顺带说一声"，
// 没配推送通道不该让整个任务记成失败。
func (s *OpsErrorDigestService) push(ctx context.Context, title, body string) (bool, error) {
	if s.notifier == nil {
		return false, nil
	}
	return s.notifier.NotifyOpsErrorDigest(ctx, title, body)
}

// buildSummary 查区间内的分组计数并折叠成汇总。
func (s *OpsErrorDigestService) buildSummary(
	ctx context.Context,
	start, end time.Time,
	topKeys int,
) (*OpsErrorDigestSummary, error) {
	rows, err := s.opsRepo.GetErrorDigestBreakdown(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("query error digest breakdown: %w", err)
	}
	return buildOpsErrorDigestSummary(rows, start, end, topKeys), nil
}

// resolveWindow 算这次汇总覆盖的区间：从上次成功运行到现在。
//
// 上次成功时间取任务心跳里的 last_success_at。首次运行读不到心跳，或者距上次成功已经超过
// 24 小时（服务停过、任务一直失败），都退回"最近 24 小时"。
func (s *OpsErrorDigestService) resolveWindow(ctx context.Context, now time.Time) (time.Time, time.Time) {
	end := now.UTC()
	start := end.Add(-opsErrorDigestFallbackWindow)

	last, ok := s.lastSuccessAt(ctx)
	if !ok {
		return start, end
	}
	last = last.UTC()
	// 心跳时间不早于现在（改过系统时钟、多实例时钟不齐）时当成不可信，走兜底区间，
	// 否则会算出一个空的甚至倒过来的区间。
	if !last.Before(end) {
		return start, end
	}
	if last.After(start) {
		start = last
	}
	return start, end
}

// lastSuccessAt 从任务心跳里取本任务上次成功的时间。
// 仓储层只提供 ListJobHeartbeats（任务数是个位数），这里按 job_name 挑出自己那条。
func (s *OpsErrorDigestService) lastSuccessAt(ctx context.Context) (time.Time, bool) {
	if s == nil || s.opsRepo == nil {
		return time.Time{}, false
	}
	beats, err := s.opsRepo.ListJobHeartbeats(ctx)
	if err != nil {
		logger.LegacyPrintf("service.ops_error_digest",
			"[OpsDigest] read job heartbeats failed, falling back to 24h window: %v", err)
		return time.Time{}, false
	}
	for _, beat := range beats {
		if beat == nil || beat.JobName != opsErrorDigestJobName {
			continue
		}
		if beat.LastSuccessAt == nil || beat.LastSuccessAt.IsZero() {
			return time.Time{}, false
		}
		return *beat.LastSuccessAt, true
	}
	return time.Time{}, false
}

func (s *OpsErrorDigestService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil {
		return nil, false
	}
	// 单机模式没有第二个实例，省掉一次 Redis 往返。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] redis not configured; running without distributed lock")
		})
		return nil, true
	}

	ok, err := s.redisClient.SetNX(ctx, opsErrorDigestLeaderLockKey, s.instanceID, opsErrorDigestLeaderLockTTL).Result()
	if err != nil {
		// Redis 抽风时宁可这次不跑：漏掉的这段不会丢，心跳没更新，下一次的区间会自动把它包进来；
		// 反过来每个实例都推一遍，站长手机上就是几条一模一样的通知。
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] leader lock SetNX failed; skipping this run: %v", err)
		})
		return nil, false
	}
	if !ok {
		return nil, false
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = opsErrorDigestReleaseScript.Run(releaseCtx, s.redisClient, []string{opsErrorDigestLeaderLockKey}, s.instanceID).Result()
	}, true
}

func (s *OpsErrorDigestService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := s.timeNow().UTC()
	durMs := duration.Milliseconds()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	ctx, cancel := context.WithTimeout(context.Background(), opsErrorDigestHeartbeatTimeout)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsErrorDigestJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsErrorDigestService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := s.timeNow().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), opsErrorDigestHeartbeatTimeout)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsErrorDigestJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

// ─── 管理端接口 ───

// GetErrorDigestConfig 返回当前配置；从未保存过时返回默认值。
func (s *OpsErrorDigestService) GetErrorDigestConfig(ctx context.Context) (*OpsErrorDigestConfigView, error) {
	stored, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return toOpsErrorDigestConfigView(defaultOpsErrorDigestConfig()), nil
	}
	return toOpsErrorDigestConfigView(stored), nil
}

// UpdateErrorDigestConfig 保存配置并立刻按新 schedule 重建 cron（不用重启服务）。
func (s *OpsErrorDigestService) UpdateErrorDigestConfig(
	ctx context.Context,
	in OpsErrorDigestConfigInput,
) (*OpsErrorDigestConfigView, error) {
	next, err := normalizeOpsErrorDigestConfigInput(in)
	if err != nil {
		return nil, err
	}
	next.UpdatedAt = s.timeNow().UTC()

	data, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("marshal error digest config: %w", err)
	}
	if s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if err := s.settingRepo.Set(ctx, settingKeyNotifyErrorDigestConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save error digest config: %w", err)
	}
	// 重建 cron 失败（例如表达式在这一步才被 cron 库拒绝）只记日志：配置已经存下了，
	// 报错回前端会让人以为没保存成功。
	if err := s.Reload(ctx); err != nil {
		logger.LegacyPrintf("service.ops_error_digest", "[OpsDigest] reload after save failed: %v", err)
	}
	return toOpsErrorDigestConfigView(next), nil
}

// load 读取落库配置；从未保存过时返回 nil。
func (s *OpsErrorDigestService) load(ctx context.Context) (*OpsErrorDigestConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil //nolint:nilnil // no config is a valid state
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyNotifyErrorDigestConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil //nolint:nilnil // no config is a valid state
		}
		return nil, fmt.Errorf("load error digest config: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil //nolint:nilnil // no config is a valid state
	}
	cfg := defaultOpsErrorDigestConfig()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, ErrOpsErrorDigestConfigCorrupt
	}
	// 老数据或手改过的行可能缺字段 / 写了非法值：读出来统一回落，读路径不因为一行坏数据罢工。
	if strings.TrimSpace(cfg.Schedule) == "" || !isValidOpsErrorDigestSchedule(cfg.Schedule) {
		cfg.Schedule = opsErrorDigestDefaultSchedule
	}
	cfg.TopKeys = clampOpsErrorDigestTopKeys(cfg.TopKeys)
	return cfg, nil
}

func normalizeOpsErrorDigestConfigInput(in OpsErrorDigestConfigInput) (*OpsErrorDigestConfig, error) {
	cfg := defaultOpsErrorDigestConfig()
	cfg.Enabled = in.Enabled

	schedule := strings.TrimSpace(in.Schedule)
	if schedule == "" {
		schedule = opsErrorDigestDefaultSchedule
	}
	if !isValidOpsErrorDigestSchedule(schedule) {
		return nil, ErrOpsErrorDigestScheduleInvalid
	}
	cfg.Schedule = schedule

	if in.SkipWhenEmpty != nil {
		cfg.SkipWhenEmpty = *in.SkipWhenEmpty
	}
	cfg.TopKeys = clampOpsErrorDigestTopKeys(in.TopKeys)
	return cfg, nil
}

func isValidOpsErrorDigestSchedule(schedule string) bool {
	_, err := opsErrorDigestCronParser.Parse(strings.TrimSpace(schedule))
	return err == nil
}

// clampOpsErrorDigestTopKeys 0 或负数当成"没填"回落默认值；超上限直接夹住而不是报错，
// 免得站长填了个 999 保存不了还不知道为什么。
func clampOpsErrorDigestTopKeys(v int) int {
	if v <= 0 {
		return opsErrorDigestDefaultTopKeys
	}
	if v > opsErrorDigestMaxTopKeys {
		return opsErrorDigestMaxTopKeys
	}
	return v
}

func toOpsErrorDigestConfigView(cfg *OpsErrorDigestConfig) *OpsErrorDigestConfigView {
	if cfg == nil {
		cfg = defaultOpsErrorDigestConfig()
	}
	view := &OpsErrorDigestConfigView{
		Enabled:       cfg.Enabled,
		Schedule:      cfg.Schedule,
		SkipWhenEmpty: cfg.SkipWhenEmpty,
		TopKeys:       cfg.TopKeys,
	}
	if !cfg.UpdatedAt.IsZero() {
		updatedAt := cfg.UpdatedAt.UTC()
		view.UpdatedAt = &updatedAt
	}
	return view
}

// ─── 分组与折叠 ───

// opsErrorDigestGroupKey 分组键。unassigned 为真表示 api_key_id 为空的那一组：
// 这些请求在认证阶段就失败了，不同用户的失败也只能合在一起。
type opsErrorDigestGroupKey struct {
	apiKeyID   int64
	unassigned bool
}

type opsErrorDigestTypeBucket struct {
	errorType string
	total     int64
	// statuses 同一种错误类型下各状态码各多少次；展示时取最多的那个，
	// 好让正文写成「上游过载(529)」而不是干巴巴一个「上游过载」。
	statuses map[int]int64
}

type opsErrorDigestGroupBucket struct {
	title string
	total int64
	types map[string]*opsErrorDigestTypeBucket
}

// buildOpsErrorDigestSummary 把原始分组计数折叠成一份汇总。
//
// 纯函数（不碰 ctx / 仓储），排序全部带 tie-break，次数相同的组按标题排，
// 这样同一份数据每次算出来的正文都一样。
func buildOpsErrorDigestSummary(
	rows []*OpsErrorDigestRow,
	start, end time.Time,
	topKeys int,
) *OpsErrorDigestSummary {
	topKeys = clampOpsErrorDigestTopKeys(topKeys)

	out := &OpsErrorDigestSummary{
		Start:  start,
		End:    end,
		Groups: []OpsErrorDigestGroup{},
	}

	buckets := map[opsErrorDigestGroupKey]*opsErrorDigestGroupBucket{}
	for _, row := range rows {
		if row == nil || row.Count <= 0 {
			continue
		}
		out.Total += row.Count
		if row.IsBusinessLimited {
			out.Limited += row.Count
		} else {
			out.SLA += row.Count
		}

		key := opsErrorDigestGroupKey{unassigned: row.APIKeyID == nil}
		if row.APIKeyID != nil {
			key.apiKeyID = *row.APIKeyID
		}
		bucket, ok := buckets[key]
		if !ok {
			bucket = &opsErrorDigestGroupBucket{
				title: opsErrorDigestGroupTitle(row),
				types: map[string]*opsErrorDigestTypeBucket{},
			}
			buckets[key] = bucket
		}
		bucket.total += row.Count

		typeBucket, ok := bucket.types[row.ErrorType]
		if !ok {
			typeBucket = &opsErrorDigestTypeBucket{errorType: row.ErrorType, statuses: map[int]int64{}}
			bucket.types[row.ErrorType] = typeBucket
		}
		typeBucket.total += row.Count
		typeBucket.statuses[row.StatusCode] += row.Count
	}

	groups := make([]OpsErrorDigestGroup, 0, len(buckets))
	for _, bucket := range buckets {
		groups = append(groups, OpsErrorDigestGroup{
			Title: bucket.title,
			Total: bucket.total,
			Types: topOpsErrorDigestTypes(bucket.types),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Total != groups[j].Total {
			return groups[i].Total > groups[j].Total
		}
		return groups[i].Title < groups[j].Title
	})

	if len(groups) > topKeys {
		for _, hidden := range groups[topKeys:] {
			out.HiddenTotal += hidden.Total
		}
		out.HiddenGroups = len(groups) - topKeys
		groups = groups[:topKeys]
	}
	out.Groups = groups
	return out
}

// topOpsErrorDigestTypes 取一个密钥下出现最多的前几种错误类型。
func topOpsErrorDigestTypes(types map[string]*opsErrorDigestTypeBucket) []OpsErrorDigestTypeLine {
	lines := make([]OpsErrorDigestTypeLine, 0, len(types))
	for _, bucket := range types {
		lines = append(lines, OpsErrorDigestTypeLine{
			Label: opsErrorDigestTypeLabel(bucket.errorType, dominantOpsErrorDigestStatus(bucket.statuses)),
			Count: bucket.total,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Count != lines[j].Count {
			return lines[i].Count > lines[j].Count
		}
		return lines[i].Label < lines[j].Label
	})
	if len(lines) > opsErrorDigestTopErrorTypes {
		lines = lines[:opsErrorDigestTopErrorTypes]
	}
	return lines
}

// dominantOpsErrorDigestStatus 取同一错误类型下出现最多的状态码；次数相同时取小的那个，保证结果稳定。
func dominantOpsErrorDigestStatus(statuses map[int]int64) int {
	best, bestCount := 0, int64(-1)
	for status, count := range statuses {
		if count > bestCount || (count == bestCount && status < best) {
			best, bestCount = status, count
		}
	}
	return best
}

// opsErrorDigestTypeLabel 「上游过载(529)」。状态码为 0（没记到，比如网络层就断了）时只写类型名。
func opsErrorDigestTypeLabel(errorType string, statusCode int) string {
	label := strings.TrimSpace(opsErrorDigestTypeLabels[errorType])
	if label == "" {
		label = strings.TrimSpace(errorType)
	}
	if label == "" {
		label = "未分类"
	}
	if statusCode > 0 {
		return fmt.Sprintf("%s(%d)", label, statusCode)
	}
	return label
}

// opsErrorDigestGroupTitle 分组标题：「用户名 / 密钥名」。
// 用户名优先取 username，没有就用邮箱，都没有（用户被硬删）回落成 用户#ID；密钥名同理。
func opsErrorDigestGroupTitle(row *OpsErrorDigestRow) string {
	if row == nil {
		return opsErrorDigestUnassignedTitle
	}
	if row.APIKeyID == nil {
		return opsErrorDigestUnassignedTitle
	}

	keyName := strings.TrimSpace(row.APIKeyName)
	if keyName == "" {
		keyName = fmt.Sprintf("密钥#%d", *row.APIKeyID)
	}

	userName := strings.TrimSpace(row.Username)
	if userName == "" {
		userName = strings.TrimSpace(row.UserEmail)
	}
	if userName == "" && row.UserID != nil {
		userName = fmt.Sprintf("用户#%d", *row.UserID)
	}
	if userName == "" {
		return keyName
	}
	return userName + " / " + keyName
}

// ─── 正文 ───

// renderDigest 拼 Bark 的标题与正文。
//
// 正文长这样：
//
//	区间 17:30 – 11:30（18 小时）
//	失败 128 次：真故障 28，业务拦截 100
//
//	张三 / dev-key：42 次
//	  上游过载(529) 30、请求超时 12
//	（未识别密钥）：12 次
//	  认证失败 12
//	另有 3 个密钥共 12 次
func (s *OpsErrorDigestService) renderDigest(ctx context.Context, summary *OpsErrorDigestSummary, runAt time.Time) (string, string) {
	loc := s.location()
	prefix := barkDefaultTitlePrefix
	if s != nil && s.notifier != nil {
		prefix = s.notifier.NotificationTitlePrefix(ctx)
	}
	title := fmt.Sprintf("[%s] 报错汇总 · %s", prefix, runAt.In(loc).Format(opsErrorDigestTimeLayout))
	return title, buildOpsErrorDigestBody(summary, loc)
}

func buildOpsErrorDigestBody(summary *OpsErrorDigestSummary, loc *time.Location) string {
	if summary == nil {
		return "没有可用的汇总数据"
	}
	if loc == nil {
		loc = time.Local
	}

	lines := []string{
		fmt.Sprintf("区间 %s – %s（%s）",
			summary.Start.In(loc).Format(opsErrorDigestTimeLayout),
			summary.End.In(loc).Format(opsErrorDigestTimeLayout),
			formatBarkDuration(summary.End.Sub(summary.Start)),
		),
	}
	if summary.Total == 0 {
		return strings.Join(append(lines, "区间内没有报错"), "\n")
	}

	lines = append(lines,
		fmt.Sprintf("失败 %d 次：真故障 %d，业务拦截 %d", summary.Total, summary.SLA, summary.Limited),
		"",
	)
	for _, group := range summary.Groups {
		lines = append(lines, fmt.Sprintf("%s：%d 次", group.Title, group.Total))
		if detail := formatOpsErrorDigestTypes(group.Types); detail != "" {
			lines = append(lines, "  "+detail)
		}
	}
	if summary.HiddenGroups > 0 {
		lines = append(lines, fmt.Sprintf("另有 %d 个密钥共 %d 次", summary.HiddenGroups, summary.HiddenTotal))
	}
	return strings.Join(lines, "\n")
}

func formatOpsErrorDigestTypes(types []OpsErrorDigestTypeLine) string {
	if len(types) == 0 {
		return ""
	}
	parts := make([]string, 0, len(types))
	for _, line := range types {
		parts = append(parts, fmt.Sprintf("%s %d", line.Label, line.Count))
	}
	return strings.Join(parts, "、")
}
