package service

import (
	"sync"
	"time"
)

// 调度可视化的共享契约：选号诊断（失败请求）、选号信息（成功请求）和
// 实时路由事件。错误请求 / 用量明细 / 调度拓扑三处都只依赖这里的结构，
// 字段名即 JSON 字段名，前端按同名读取。

// Routing event types published to RoutingObservers.
const (
	RoutingEventRequestStart  = "request_start"  // 请求进入调度
	RoutingEventBlocked       = "blocked"        // 关卡拦下，没有可用账号（Reason 为拦下原因）
	RoutingEventAttempt       = "attempt"        // 发往某个上游账号（Attempt 从 1 开始）
	RoutingEventAttemptFailed = "attempt_failed" // 上游报错（Next 说明接下来怎么办）
	RoutingEventRequestDone   = "request_done"   // 请求结束（StatusCode 为返回给客户端的状态码）
)

// Values of RoutingEvent.Next / RoutingAttempt.Next after an upstream failure.
const (
	RoutingNextSameAccount = "same_account" // 原账号等一会儿再试
	RoutingNextSwitch      = "switch"       // 排除这个账号，换下一个
	RoutingNextGiveUp      = "give_up"      // 不再重试，错误返回给客户端
)

// Values of RoutingInfo.Method.
const (
	RoutingMethodLoadBalance = "load_balance" // 按负载在多个候选里挑
	RoutingMethodSticky      = "sticky"       // 沿用同一会话上次的账号
	RoutingMethodSingle      = "single"       // 只有一个候选
)

// RoutingEvent is one step of a request's routing lifecycle.
type RoutingEvent struct {
	Type          string     `json:"type"`
	RequestID     string     `json:"request_id"`
	APIKeyID      int64      `json:"api_key_id,omitempty"`
	GroupID       int64      `json:"group_id,omitempty"`
	AccountID     int64      `json:"account_id,omitempty"`
	Attempt       int        `json:"attempt,omitempty"`
	StatusCode    int        `json:"status_code,omitempty"`
	Next          string     `json:"next,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
	At            time.Time  `json:"at"`
}

// RoutingObserver receives routing events. Implementations must be fast and
// non-blocking: OnRoutingEvent runs on the request path.
type RoutingObserver interface {
	OnRoutingEvent(RoutingEvent)
}

var (
	routingObserversMu sync.RWMutex
	routingObservers   []RoutingObserver
)

// RegisterRoutingObserver adds an observer; it returns a function that removes it.
func RegisterRoutingObserver(o RoutingObserver) (unregister func()) {
	if o == nil {
		return func() {}
	}
	routingObserversMu.Lock()
	routingObservers = append(routingObservers, o)
	routingObserversMu.Unlock()
	return func() {
		routingObserversMu.Lock()
		defer routingObserversMu.Unlock()
		for i, cur := range routingObservers {
			if cur == o {
				routingObservers = append(routingObservers[:i:i], routingObservers[i+1:]...)
				return
			}
		}
	}
}

// EmitRoutingEvent delivers ev to every registered observer. A panicking
// observer never breaks the request path.
func EmitRoutingEvent(ev RoutingEvent) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	routingObserversMu.RLock()
	observers := routingObservers
	routingObserversMu.RUnlock()
	for _, o := range observers {
		func() {
			defer func() { _ = recover() }()
			o.OnRoutingEvent(ev)
		}()
	}
}

// RoutingCandidate is one account the scheduler looked at.
type RoutingCandidate struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Platform    string `json:"platform,omitempty"`
	// Checks: gate name -> passed. A gate that was never evaluated (an earlier
	// gate already excluded the account) is absent. Gate names:
	// platform, schedulable, quota, model, capacity.
	Checks map[string]bool `json:"checks,omitempty"`
	// Reason is the machine code of why the account was not chosen; empty for
	// the chosen account. Uses the same codes as the scheduler filter stats
	// (model_not_supported, not_schedulable, model_rate_limited, excluded, ...).
	Reason      string     `json:"reason,omitempty"`
	RecoverAt   *time.Time `json:"recover_at,omitempty"`
	LoadCurrent int        `json:"load_current"`
	LoadMax     int        `json:"load_max"`
	Chosen      bool       `json:"chosen,omitempty"`
}

// RoutingAttempt is one upstream attempt during failover.
type RoutingAttempt struct {
	Attempt     int    `json:"attempt"`
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	StatusCode  int    `json:"status_code"` // 0 = network error / no response
	DurationMs  int64  `json:"duration_ms"`
	Next        string `json:"next,omitempty"` // same_account | switch | give_up; empty when it succeeded
}

// RoutingDiagnosis explains a request that ended without a usable account
// (stored on ops_error_logs.routing_diagnosis).
type RoutingDiagnosis struct {
	GroupID    int64              `json:"group_id,omitempty"`
	GroupName  string             `json:"group_name,omitempty"`
	Model      string             `json:"model,omitempty"`
	PoolSize   int                `json:"pool_size"`
	Candidates []RoutingCandidate `json:"candidates"`
	Attempts   []RoutingAttempt   `json:"attempts,omitempty"`
	Reason     string             `json:"reason,omitempty"` // dominant reason code, same as ops_error_logs.error_reason
	Raw        string             `json:"raw,omitempty"`    // original scheduler error text
}

// RoutingInfo explains how a successful request picked its account
// (stored on usage_logs.routing_info).
type RoutingInfo struct {
	Method         string             `json:"method"` // load_balance | sticky | single
	CandidateCount int                `json:"candidate_count"`
	Candidates     []RoutingCandidate `json:"candidates,omitempty"`
	QueueWaitMs    int64              `json:"queue_wait_ms,omitempty"`
	Attempts       []RoutingAttempt   `json:"attempts,omitempty"`
}
