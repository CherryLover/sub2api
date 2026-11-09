package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 单请求调度轨迹：调度器在过滤候选账号时顺手记录每个账号过了哪些关卡、
// 卡在哪一关；handler 在 failover 循环里记录每一次上游尝试。请求结束时
// 失败请求生成 RoutingDiagnosis（写 ops_error_logs），成功请求生成
// RoutingInfo（写 usage_logs），同时向 RoutingObserver 推送实时事件。
//
// 轨迹挂在请求 context 上；没有轨迹（非网关入口、内部调用）时所有方法都是
// nil 安全的空操作，调度决策本身不受影响。

// Gate names used in RoutingCandidate.Checks.
const (
	RoutingGatePlatform    = "platform"
	RoutingGateSchedulable = "schedulable"
	RoutingGateQuota       = "quota"
	RoutingGateModel       = "model"
	RoutingGateCapacity    = "capacity"
)

// Reasons for candidates that passed every filter gate but were not chosen.
const (
	RoutingReasonCapacityFull = "capacity_full" // 并发已满
	RoutingReasonNotSelected  = "not_selected"  // 合格但按负载/优先级没轮到
)

// Size caps. Diagnosis/info JSON is persisted per row, so it stays small.
const (
	RoutingDiagnosisMaxCandidates = 20
	RoutingDiagnosisMaxAttempts   = 12
	RoutingInfoMaxCandidates      = 10
	RoutingInfoMaxAttempts        = 12

	routingTraceMaxRecordedCandidates = 64
	routingTraceMaxRecordedAttempts   = 32
	routingTraceRawMaxLen             = 2048
)

// routingGateOrder is the canonical display order of gates. A candidate
// rejected at gate i is shown as having passed gates [0, i) and failed gate i.
var routingGateOrder = [...]string{
	RoutingGatePlatform,
	RoutingGateSchedulable,
	RoutingGateQuota,
	RoutingGateModel,
	RoutingGateCapacity,
}

const (
	routingGatePassedAll   int8 = -1 // passed every filter gate (eligible)
	routingGateUnevaluated int8 = -2 // skipped before any gate (already tried in this request)
)

// routingGateIndexForReason maps a scheduler filter reason code to the gate
// that rejected the account.
func routingGateIndexForReason(reason string) int8 {
	switch {
	case reason == OpsNoAccountReasonExcluded:
		return routingGateUnevaluated
	case reason == "platform_mismatch" || reason == "transport_incompatible":
		return 0
	case reason == OpsNoAccountReasonModelRateLimited,
		strings.HasPrefix(reason, "quota_"),
		strings.HasPrefix(reason, "profit_"),
		strings.Contains(reason, "quota"):
		return 2
	case reason == OpsNoAccountReasonModelNotSupported,
		reason == "capability_mismatch",
		reason == "compact_unsupported",
		reason == "channel_restricted",
		reason == "channel_upstream_restricted":
		return 3
	case reason == RoutingReasonCapacityFull:
		return 4
	default:
		// not_schedulable, runtime_blocked, privacy_not_set,
		// shadow_parent_unhealthy, proxy_stream_quarantined, ineligible, ...
		return 1
	}
}

// RoutingTraceMeta identifies the request a trace belongs to.
type RoutingTraceMeta struct {
	RequestID string
	APIKeyID  int64
	GroupID   int64
	GroupName string
	Model     string
}

type routingCandidateRecord struct {
	accountID   int64
	name        string
	platform    string
	reason      string
	failGate    int8
	recoverAt   *time.Time
	loadCurrent int
	loadMax     int
	loadKnown   bool
	chosen      bool
}

func (r routingCandidateRecord) eligible() bool { return r.failGate == routingGatePassedAll }

// RoutingTrace collects one request's routing decisions. All methods are safe
// for concurrent use and no-ops on a nil receiver.
type RoutingTrace struct {
	mu   sync.Mutex
	meta RoutingTraceMeta
	now  func() time.Time

	// Latest selection pass.
	pool             int
	candidates       []routingCandidateRecord
	candidatesFresh  bool
	eligibleOverflow int
	sticky           bool

	selectFailed bool
	selectErr    string
	queueWaitMs  int64

	attempts        []RoutingAttempt
	attemptCount    int
	inFlight        bool
	inFlightStart   time.Time
	inFlightAccount int64
	inFlightName    string
	pendingFailed   bool
	pendingCooldown *time.Time
	anyFailed       bool
	lastAccountID   int64

	finished bool
}

// NewRoutingTrace creates an empty trace for one request.
func NewRoutingTrace(meta RoutingTraceMeta) *RoutingTrace {
	return &RoutingTrace{meta: meta, now: time.Now}
}

type routingTraceContextKey struct{}

// WithRoutingTrace attaches t to ctx.
func WithRoutingTrace(ctx context.Context, t *RoutingTrace) context.Context {
	if ctx == nil || t == nil {
		return ctx
	}
	return context.WithValue(ctx, routingTraceContextKey{}, t)
}

// RoutingTraceFromContext returns the trace attached to ctx, or nil.
func RoutingTraceFromContext(ctx context.Context) *RoutingTrace {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(routingTraceContextKey{}).(*RoutingTrace)
	return t
}

func (t *RoutingTrace) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// EmitStart publishes request_start.
func (t *RoutingTrace) EmitStart() {
	if t == nil {
		return
	}
	EmitRoutingEvent(RoutingEvent{
		Type:      RoutingEventRequestStart,
		RequestID: t.meta.RequestID,
		APIKeyID:  t.meta.APIKeyID,
		GroupID:   t.meta.GroupID,
		At:        t.clock(),
	})
}

// ---------------------------------------------------------------------------
// Selection hooks (called by the schedulers).
// ---------------------------------------------------------------------------

// beginSelection marks the start of one account selection call. Candidates of
// a previous selection are dropped by finishSelection unless this selection
// enumerates its own pool.
func (t *RoutingTrace) beginSelection() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.candidatesFresh = false
	t.sticky = false
	t.mu.Unlock()
}

// beginCandidates starts a fresh candidate-filter pass over pool accounts.
func (t *RoutingTrace) beginCandidates(pool int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.pool = pool
	t.candidates = t.candidates[:0]
	t.candidatesFresh = true
	t.eligibleOverflow = 0
	t.sticky = false
	t.mu.Unlock()
}

// exclude records an account rejected with the given filter reason.
func (t *RoutingTrace) exclude(account *Account, reason string) {
	if t == nil || account == nil {
		return
	}
	t.excludeUntil(account, reason, routingAccountRecoverAt(account, t.clock()))
}

// excludeUntil is exclude with an explicit recovery time.
func (t *RoutingTrace) excludeUntil(account *Account, reason string, recoverAt *time.Time) {
	if t == nil || account == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.candidates) >= routingTraceMaxRecordedCandidates {
		return
	}
	t.candidates = append(t.candidates, routingCandidateRecord{
		accountID: account.ID,
		name:      account.Name,
		platform:  account.Platform,
		reason:    reason,
		failGate:  routingGateIndexForReason(reason),
		recoverAt: recoverAt,
	})
}

// pass records an account that passed every filter gate.
func (t *RoutingTrace) pass(account *Account) {
	if t == nil || account == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.candidates) >= routingTraceMaxRecordedCandidates {
		t.eligibleOverflow++
		return
	}
	t.candidates = append(t.candidates, routingCandidateRecord{
		accountID: account.ID,
		name:      account.Name,
		platform:  account.Platform,
		failGate:  routingGatePassedAll,
		loadMax:   account.EffectiveLoadFactor(),
	})
}

// setLoads copies the load snapshot the scheduler fetched onto the eligible
// candidates.
func (t *RoutingTrace) setLoads(loadMap map[int64]*AccountLoadInfo) {
	if t == nil || len(loadMap) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.candidates {
		rec := &t.candidates[i]
		if !rec.eligible() {
			continue
		}
		if info := loadMap[rec.accountID]; info != nil {
			rec.loadCurrent = info.CurrentConcurrency
			rec.loadKnown = true
		}
	}
}

// markSticky records that the selection reused the session's bound account.
func (t *RoutingTrace) markSticky() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.sticky = true
	t.mu.Unlock()
}

// finishSelection closes one selection call.
func (t *RoutingTrace) finishSelection(selection *AccountSelectionResult, sticky bool, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.candidatesFresh {
		// This selection never enumerated a pool (sticky hit, empty pool,
		// early gate): candidates of an earlier selection no longer apply.
		t.candidates = t.candidates[:0]
		t.eligibleOverflow = 0
		t.pool = 0
	}
	if sticky {
		t.sticky = true
	}
	if err != nil {
		t.selectFailed = true
		t.selectErr = truncateRoutingRaw(err.Error())
		return
	}
	if selection == nil || selection.Account == nil {
		t.selectFailed = true
		t.selectErr = ""
		return
	}
	t.selectFailed = false
	t.selectErr = ""
	account := selection.Account
	for i := range t.candidates {
		if t.candidates[i].accountID == account.ID {
			t.candidates[i].chosen = true
			t.candidates[i].failGate = routingGatePassedAll
			t.candidates[i].reason = ""
			return
		}
	}
	t.candidates = append(t.candidates, routingCandidateRecord{
		accountID: account.ID,
		name:      account.Name,
		platform:  account.Platform,
		failGate:  routingGatePassedAll,
		loadMax:   account.EffectiveLoadFactor(),
		chosen:    true,
	})
}

// ---------------------------------------------------------------------------
// Request hooks (called by the handlers).
// ---------------------------------------------------------------------------

// AddQueueWait records time spent waiting for an account slot (WaitPlan path).
func (t *RoutingTrace) AddQueueWait(d time.Duration) {
	if t == nil || d <= 0 {
		return
	}
	t.mu.Lock()
	t.queueWaitMs += d.Milliseconds()
	t.mu.Unlock()
}

// BeginAttempt records the start of one upstream attempt and emits attempt.
func (t *RoutingTrace) BeginAttempt(account *Account) {
	if t == nil || account == nil {
		return
	}
	t.mu.Lock()
	if t.inFlight {
		// The previous attempt was never closed; treat it as a silent switch.
		t.endAttemptLocked(0, true, nil)
		t.attempts[len(t.attempts)-1].Next = RoutingNextSwitch
		t.pendingFailed = false
		t.pendingCooldown = nil
	}
	now := t.clock()
	t.attemptCount++
	t.inFlight = true
	t.inFlightStart = now
	t.inFlightAccount = account.ID
	t.inFlightName = account.Name
	t.lastAccountID = account.ID
	ev := RoutingEvent{
		Type:      RoutingEventAttempt,
		RequestID: t.meta.RequestID,
		APIKeyID:  t.meta.APIKeyID,
		GroupID:   t.meta.GroupID,
		AccountID: account.ID,
		Attempt:   t.attemptCount,
		At:        now,
	}
	t.mu.Unlock()
	EmitRoutingEvent(ev)
}

// EndAttempt closes the current attempt. A failed attempt is provisionally
// give_up until ResolveAttempt says the handler retries.
func (t *RoutingTrace) EndAttempt(statusCode int, failed bool, cooldownUntil *time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.endAttemptLocked(statusCode, failed, cooldownUntil)
	t.mu.Unlock()
}

func (t *RoutingTrace) endAttemptLocked(statusCode int, failed bool, cooldownUntil *time.Time) {
	if !t.inFlight {
		return
	}
	t.inFlight = false
	attempt := RoutingAttempt{
		Attempt:     t.attemptCount,
		AccountID:   t.inFlightAccount,
		AccountName: t.inFlightName,
		StatusCode:  statusCode,
		DurationMs:  t.clock().Sub(t.inFlightStart).Milliseconds(),
	}
	if failed {
		attempt.Next = RoutingNextGiveUp
		t.pendingFailed = true
		t.pendingCooldown = cooldownUntil
		t.anyFailed = true
	} else {
		t.pendingFailed = false
		t.pendingCooldown = nil
	}
	if len(t.attempts) >= routingTraceMaxRecordedAttempts {
		copy(t.attempts, t.attempts[1:])
		t.attempts = t.attempts[:len(t.attempts)-1]
	}
	t.attempts = append(t.attempts, attempt)
}

// ResolveAttempt sets what happens after the last failed attempt
// (same_account / switch / give_up) and emits attempt_failed.
func (t *RoutingTrace) ResolveAttempt(next string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	ev, ok := t.resolvePendingLocked(next)
	t.mu.Unlock()
	if ok {
		EmitRoutingEvent(ev)
	}
}

func (t *RoutingTrace) resolvePendingLocked(next string) (RoutingEvent, bool) {
	if !t.pendingFailed || len(t.attempts) == 0 {
		return RoutingEvent{}, false
	}
	last := &t.attempts[len(t.attempts)-1]
	last.Next = next
	ev := RoutingEvent{
		Type:          RoutingEventAttemptFailed,
		RequestID:     t.meta.RequestID,
		APIKeyID:      t.meta.APIKeyID,
		GroupID:       t.meta.GroupID,
		AccountID:     last.AccountID,
		Attempt:       last.Attempt,
		StatusCode:    last.StatusCode,
		Next:          next,
		CooldownUntil: t.pendingCooldown,
		At:            t.clock(),
	}
	t.pendingFailed = false
	t.pendingCooldown = nil
	return ev, true
}

// Finish ends the request: a still-pending failed attempt becomes give_up,
// a failed selection emits blocked, and request_done is published. A
// statusCode <= 0 is derived from the trace. Only the first call has effect.
func (t *RoutingTrace) Finish(statusCode int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	events := make([]RoutingEvent, 0, 3)
	if t.inFlight {
		// Still open at the end (e.g. WebSocket connection): it served the request.
		t.endAttemptLocked(http.StatusOK, false, nil)
	}
	if ev, ok := t.resolvePendingLocked(RoutingNextGiveUp); ok {
		events = append(events, ev)
	}
	now := t.clock()
	if t.selectFailed {
		events = append(events, RoutingEvent{
			Type:      RoutingEventBlocked,
			RequestID: t.meta.RequestID,
			APIKeyID:  t.meta.APIKeyID,
			GroupID:   t.meta.GroupID,
			Reason:    t.dominantReasonLocked(),
			At:        now,
		})
	}
	if statusCode <= 0 {
		statusCode = t.derivedStatusLocked()
	}
	events = append(events, RoutingEvent{
		Type:       RoutingEventRequestDone,
		RequestID:  t.meta.RequestID,
		APIKeyID:   t.meta.APIKeyID,
		GroupID:    t.meta.GroupID,
		AccountID:  t.lastAccountID,
		StatusCode: statusCode,
		At:         now,
	})
	t.mu.Unlock()
	for _, ev := range events {
		EmitRoutingEvent(ev)
	}
}

func (t *RoutingTrace) derivedStatusLocked() int {
	if t.selectFailed {
		return http.StatusServiceUnavailable
	}
	if n := len(t.attempts); n > 0 {
		last := t.attempts[n-1]
		if last.Next == "" {
			if last.StatusCode > 0 {
				return last.StatusCode
			}
			return http.StatusOK
		}
		if last.StatusCode > 0 {
			return last.StatusCode
		}
		return http.StatusBadGateway
	}
	return 0
}

// dominantReasonLocked returns the main reason no account was usable.
func (t *RoutingTrace) dominantReasonLocked() string {
	if t.selectErr != "" {
		return ParseNoAccountReason(t.selectErr)
	}
	counts := make(map[string]int, 4)
	best, bestCount := "", 0
	for _, rec := range t.candidates {
		if rec.reason == "" {
			continue
		}
		counts[rec.reason]++
		if c := counts[rec.reason]; c > bestCount || (c == bestCount && rec.reason < best) {
			best, bestCount = rec.reason, c
		}
	}
	if best != "" {
		return best
	}
	if t.pool == 0 && len(t.candidates) == 0 {
		return OpsNoAccountReasonEmptyPool
	}
	return OpsNoAccountReasonUnknown
}

// ---------------------------------------------------------------------------
// Output.
// ---------------------------------------------------------------------------

// Failed reports whether the request had no usable account or an upstream
// attempt failed — i.e. whether Diagnosis has something to say.
func (t *RoutingTrace) Failed() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.selectFailed || t.anyFailed
}

// Diagnosis snapshots the trace for a failed request; nil when the request
// neither failed selection nor had a failed upstream attempt. reason, when
// non-empty, overrides the derived dominant reason (keeps it equal to
// ops_error_logs.error_reason).
func (t *RoutingTrace) Diagnosis(reason string) *RoutingDiagnosis {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selectFailed && !t.anyFailed {
		return nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" && (t.selectFailed || len(t.candidates) > 0) {
		reason = t.dominantReasonLocked()
	}
	n := len(t.candidates)
	if n > RoutingDiagnosisMaxCandidates {
		n = RoutingDiagnosisMaxCandidates
	}
	candidates := make([]RoutingCandidate, 0, n)
	for i := 0; i < n; i++ {
		candidates = append(candidates, t.candidates[i].toCandidate())
	}
	return &RoutingDiagnosis{
		GroupID:    t.meta.GroupID,
		GroupName:  t.meta.GroupName,
		Model:      t.meta.Model,
		PoolSize:   t.pool,
		Candidates: candidates,
		Attempts:   lastRoutingAttempts(t.attempts, RoutingDiagnosisMaxAttempts),
		Reason:     reason,
		Raw:        t.selectErr,
	}
}

// Info snapshots how the request picked its account; nil when nothing was
// selected. An attempt still in flight (WebSocket turns) is reported as
// serving with status 200.
func (t *RoutingTrace) Info() *RoutingInfo {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	eligibleCount := t.eligibleOverflow
	hasChosen := false
	for _, rec := range t.candidates {
		if rec.eligible() {
			eligibleCount++
		}
		if rec.chosen {
			hasChosen = true
		}
	}
	if !hasChosen && t.attemptCount == 0 {
		return nil
	}
	if eligibleCount == 0 && hasChosen {
		eligibleCount = 1
	}
	method := RoutingMethodLoadBalance
	switch {
	case t.sticky:
		method = RoutingMethodSticky
	case eligibleCount <= 1:
		method = RoutingMethodSingle
	}

	candidates := make([]RoutingCandidate, 0, minInt(eligibleCount, RoutingInfoMaxCandidates))
	for _, rec := range t.candidates {
		if rec.chosen {
			candidates = append(candidates, rec.toCandidate())
			break
		}
	}
	for _, rec := range t.candidates {
		if len(candidates) >= RoutingInfoMaxCandidates {
			break
		}
		if rec.eligible() && !rec.chosen {
			candidates = append(candidates, rec.toCandidate())
		}
	}

	attempts := lastRoutingAttempts(t.attempts, RoutingInfoMaxAttempts)
	if t.inFlight {
		inFlight := RoutingAttempt{
			Attempt:     t.attemptCount,
			AccountID:   t.inFlightAccount,
			AccountName: t.inFlightName,
			StatusCode:  http.StatusOK,
			DurationMs:  t.clock().Sub(t.inFlightStart).Milliseconds(),
		}
		if len(attempts) >= RoutingInfoMaxAttempts {
			attempts = attempts[1:]
		}
		attempts = append(attempts, inFlight)
	}
	if len(candidates) == 0 {
		candidates = nil
	}
	return &RoutingInfo{
		Method:         method,
		CandidateCount: eligibleCount,
		Candidates:     candidates,
		QueueWaitMs:    t.queueWaitMs,
		Attempts:       attempts,
	}
}

func (r routingCandidateRecord) toCandidate() RoutingCandidate {
	out := RoutingCandidate{
		AccountID:   r.accountID,
		AccountName: r.name,
		Platform:    r.platform,
		Reason:      r.reason,
		RecoverAt:   r.recoverAt,
		LoadCurrent: r.loadCurrent,
		LoadMax:     r.loadMax,
		Chosen:      r.chosen,
	}
	switch {
	case r.failGate == routingGateUnevaluated:
		// Skipped before any gate: no checks.
	case r.failGate >= 0:
		out.Checks = make(map[string]bool, int(r.failGate)+1)
		for i := int8(0); i < r.failGate; i++ {
			out.Checks[routingGateOrder[i]] = true
		}
		out.Checks[routingGateOrder[r.failGate]] = false
	default:
		out.Checks = make(map[string]bool, len(routingGateOrder))
		for i := 0; i < len(routingGateOrder)-1; i++ {
			out.Checks[routingGateOrder[i]] = true
		}
		if r.chosen || r.loadKnown {
			capacityOK := r.chosen || r.loadMax <= 0 || r.loadCurrent < r.loadMax
			out.Checks[RoutingGateCapacity] = capacityOK
			if !r.chosen && out.Reason == "" && !capacityOK {
				out.Reason = RoutingReasonCapacityFull
			}
		}
		if !r.chosen && out.Reason == "" {
			out.Reason = RoutingReasonNotSelected
		}
	}
	return out
}

func lastRoutingAttempts(attempts []RoutingAttempt, max int) []RoutingAttempt {
	if len(attempts) == 0 {
		return nil
	}
	start := 0
	if len(attempts) > max {
		start = len(attempts) - max
	}
	out := make([]RoutingAttempt, len(attempts)-start)
	copy(out, attempts[start:])
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncateRoutingRaw(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= routingTraceRawMaxLen {
		return s
	}
	cut := routingTraceRawMaxLen
	// Avoid splitting a multi-byte rune.
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}

// routingAccountRecoverAt returns when the account's active schedulability
// block ends (the latest of overload / rate-limit / temp-unschedulable), or
// nil when none is active.
func routingAccountRecoverAt(account *Account, now time.Time) *time.Time {
	if account == nil {
		return nil
	}
	var best *time.Time
	consider := func(ts *time.Time) {
		if ts == nil || !ts.After(now) {
			return
		}
		if best == nil || ts.After(*best) {
			v := *ts
			best = &v
		}
	}
	consider(account.OverloadUntil)
	consider(account.RateLimitResetAt)
	consider(account.TempUnschedulableUntil)
	return best
}

// routingModelRecoverAt returns when a model-level rate limit on account ends.
func routingModelRecoverAt(ctx context.Context, account *Account, model string, now time.Time) *time.Time {
	if account == nil {
		return nil
	}
	if remaining := account.GetModelRateLimitRemainingTimeWithContext(ctx, model); remaining > 0 {
		v := now.Add(remaining)
		return &v
	}
	return routingAccountRecoverAt(account, now)
}

// RoutingCooldownFromHeaders derives a cooldown end from an upstream 429's
// Retry-After header (delta seconds or HTTP date). nil when not a 429 or
// unknown.
func RoutingCooldownFromHeaders(statusCode int, headers http.Header, now time.Time) *time.Time {
	if statusCode != http.StatusTooManyRequests || headers == nil {
		return nil
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return nil
		}
		v := now.Add(time.Duration(secs) * time.Second)
		return &v
	}
	if ts, err := http.ParseTime(raw); err == nil && ts.After(now) {
		return &ts
	}
	return nil
}
