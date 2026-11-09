package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// 调度拓扑页的实时数据中枢：作为 RoutingObserver 接收网关请求路径上的路由事件，
// 维护近 5 分钟的滚动计数、账号冷却期，并把原始事件扇出给 SSE 订阅者。
// OnRoutingEvent 运行在请求路径上，只做 O(1) 的计数和非阻塞投递。

const (
	topologyWindow           = 5 * time.Minute
	topologyBucketWidth      = 5 * time.Second
	topologyBucketCount      = int(topologyWindow / topologyBucketWidth)
	topologySubscriberBuffer = 256
	topologyLiveRequestTTL   = 30 * time.Minute
	topologyMaxLiveRequests  = 20000
	topologyLivePruneEvery   = time.Minute

	// TopologyMaxSubscribers 同时在线的拓扑事件流订阅者上限。
	TopologyMaxSubscribers = 20
	// TopologyBlockedReasonUnknown 拦下事件没带原因时的归类。
	TopologyBlockedReasonUnknown = "unknown"
)

// ErrTopologyTooManySubscribers 订阅者已满。
var ErrTopologyTooManySubscribers = errors.New("topology: too many subscribers")

// TopologyCounters 近 5 分钟的整体计数。
type TopologyCounters struct {
	Requests         int64 // 进入调度的请求数（request_start）
	Failed           int64 // 最终返回错误的请求数（request_done 且状态码 >= 400）
	Blocked          int64 // 被关卡拦下的次数（blocked）
	RetriesSame      int64 // 上游报错后原账号重试（attempt_failed, next=same_account）
	RetriesSwitch    int64 // 上游报错后换号重试（attempt_failed, next=switch）
	FailedAfterRetry int64 // 重试过仍然失败的请求数
}

func (c *TopologyCounters) add(o TopologyCounters) {
	c.Requests += o.Requests
	c.Failed += o.Failed
	c.Blocked += o.Blocked
	c.RetriesSame += o.RetriesSame
	c.RetriesSwitch += o.RetriesSwitch
	c.FailedAfterRetry += o.FailedAfterRetry
}

// TopologyStats 某一时刻的近 5 分钟统计快照（调用方独占，可随意修改）。
type TopologyStats struct {
	Totals        TopologyCounters
	KeyRequests   map[int64]int64
	KeyFailed     map[int64]int64
	GroupRequests map[int64]int64
	GroupFailed   map[int64]int64
	GateBlocks    map[string]int64
	// Cooldowns 仍在冷却期内的账号 -> 冷却结束时间（来自 attempt_failed.CooldownUntil）。
	Cooldowns map[int64]time.Time
	// KeyPending 已进入调度但还没发往任何上游账号的请求数（按 API Key）。
	KeyPending map[int64]int
	// LiveRequests 已开始未结束的请求数（按事件推算）。
	LiveRequests int
}

type topologyBucket struct {
	idx           int64 // 桶序号 = unix 纳秒 / 桶宽；用于判断桶是否过期
	totals        TopologyCounters
	keyRequests   map[int64]int64
	keyFailed     map[int64]int64
	groupRequests map[int64]int64
	groupFailed   map[int64]int64
	blocks        map[string]int64
}

func (b *topologyBucket) reset(idx int64) {
	b.idx = idx
	b.totals = TopologyCounters{}
	clear(b.keyRequests)
	clear(b.keyFailed)
	clear(b.groupRequests)
	clear(b.groupFailed)
	clear(b.blocks)
}

type topologyLiveRequest struct {
	apiKeyID  int64
	groupID   int64
	attempted bool
	retries   int
	startedAt time.Time
}

// TopologyHub 实现 RoutingObserver。
type TopologyHub struct {
	now func() time.Time

	mu        sync.Mutex
	buckets   []topologyBucket
	live      map[string]*topologyLiveRequest
	lastPrune time.Time
	cooldowns map[int64]time.Time

	subMu   sync.RWMutex
	subs    map[*TopologySubscription]struct{}
	maxSubs int
}

// NewTopologyHub 创建中枢；now 为 nil 时使用 time.Now。不会自动注册为观察者。
func NewTopologyHub(now func() time.Time) *TopologyHub {
	if now == nil {
		now = time.Now
	}
	h := &TopologyHub{
		now:       now,
		buckets:   make([]topologyBucket, topologyBucketCount),
		live:      make(map[string]*topologyLiveRequest),
		cooldowns: make(map[int64]time.Time),
		subs:      make(map[*TopologySubscription]struct{}),
		maxSubs:   TopologyMaxSubscribers,
	}
	for i := range h.buckets {
		h.buckets[i] = topologyBucket{
			idx:           -1,
			keyRequests:   make(map[int64]int64),
			keyFailed:     make(map[int64]int64),
			groupRequests: make(map[int64]int64),
			groupFailed:   make(map[int64]int64),
			blocks:        make(map[string]int64),
		}
	}
	return h
}

var (
	topologyHubOnce     sync.Once
	topologyHubInstance *TopologyHub
)

// ProvideTopologyHub 返回进程级唯一的拓扑中枢，并在首次调用时注册为路由观察者。
func ProvideTopologyHub() *TopologyHub {
	topologyHubOnce.Do(func() {
		topologyHubInstance = NewTopologyHub(nil)
		RegisterRoutingObserver(topologyHubInstance)
	})
	return topologyHubInstance
}

func topologyBucketIndex(t time.Time) int64 {
	return t.UnixNano() / int64(topologyBucketWidth)
}

// bucketLocked 返回当前时刻所在的桶，必要时重置复用的旧桶。调用方持有 h.mu。
func (h *TopologyHub) bucketLocked(now time.Time) *topologyBucket {
	idx := topologyBucketIndex(now)
	slot := int(idx % int64(topologyBucketCount))
	if slot < 0 {
		slot += topologyBucketCount
	}
	b := &h.buckets[slot]
	if b.idx != idx {
		b.reset(idx)
	}
	return b
}

func topologyRequestFailed(statusCode int) bool {
	return statusCode >= 400
}

// OnRoutingEvent 实现 RoutingObserver：计数 + 扇出，绝不阻塞。
func (h *TopologyHub) OnRoutingEvent(ev RoutingEvent) {
	if h == nil {
		return
	}
	now := h.now()
	if ev.At.IsZero() {
		ev.At = now
	}

	h.mu.Lock()
	b := h.bucketLocked(now)
	switch ev.Type {
	case RoutingEventRequestStart:
		b.totals.Requests++
		if ev.APIKeyID > 0 {
			b.keyRequests[ev.APIKeyID]++
		}
		if ev.GroupID > 0 {
			b.groupRequests[ev.GroupID]++
		}
		h.pruneLiveLocked(now)
		if ev.RequestID != "" && len(h.live) < topologyMaxLiveRequests {
			h.live[ev.RequestID] = &topologyLiveRequest{
				apiKeyID:  ev.APIKeyID,
				groupID:   ev.GroupID,
				startedAt: now,
			}
		}
	case RoutingEventBlocked:
		reason := ev.Reason
		if reason == "" {
			reason = TopologyBlockedReasonUnknown
		}
		b.totals.Blocked++
		b.blocks[reason]++
	case RoutingEventAttempt:
		if lr := h.live[ev.RequestID]; lr != nil {
			lr.attempted = true
		}
	case RoutingEventAttemptFailed:
		lr := h.live[ev.RequestID]
		if lr != nil {
			lr.attempted = true
		}
		switch ev.Next {
		case RoutingNextSameAccount:
			b.totals.RetriesSame++
			if lr != nil {
				lr.retries++
			}
		case RoutingNextSwitch:
			b.totals.RetriesSwitch++
			if lr != nil {
				lr.retries++
			}
		}
		if ev.AccountID > 0 && ev.CooldownUntil != nil && ev.CooldownUntil.After(now) {
			if cur, ok := h.cooldowns[ev.AccountID]; !ok || ev.CooldownUntil.After(cur) {
				h.cooldowns[ev.AccountID] = *ev.CooldownUntil
			}
		}
	case RoutingEventRequestDone:
		lr := h.live[ev.RequestID]
		if lr != nil {
			delete(h.live, ev.RequestID)
		}
		if topologyRequestFailed(ev.StatusCode) {
			keyID, groupID := ev.APIKeyID, ev.GroupID
			if lr != nil {
				if keyID == 0 {
					keyID = lr.apiKeyID
				}
				if groupID == 0 {
					groupID = lr.groupID
				}
			}
			b.totals.Failed++
			if keyID > 0 {
				b.keyFailed[keyID]++
			}
			if groupID > 0 {
				b.groupFailed[groupID]++
			}
			if lr != nil && lr.retries > 0 {
				b.totals.FailedAfterRetry++
			}
		}
	}
	h.mu.Unlock()

	h.broadcast(ev)
}

// pruneLiveLocked 定期清掉没收到 request_done 的陈旧请求，防止内存无限增长。
func (h *TopologyHub) pruneLiveLocked(now time.Time) {
	if now.Sub(h.lastPrune) < topologyLivePruneEvery && len(h.live) < topologyMaxLiveRequests {
		return
	}
	h.lastPrune = now
	cutoff := now.Add(-topologyLiveRequestTTL)
	for id, lr := range h.live {
		if lr.startedAt.Before(cutoff) {
			delete(h.live, id)
		}
	}
}

// Stats 汇总近 5 分钟的计数（以 hub 时钟为准）。
func (h *TopologyHub) Stats() TopologyStats {
	out := TopologyStats{
		KeyRequests:   map[int64]int64{},
		KeyFailed:     map[int64]int64{},
		GroupRequests: map[int64]int64{},
		GroupFailed:   map[int64]int64{},
		GateBlocks:    map[string]int64{},
		Cooldowns:     map[int64]time.Time{},
		KeyPending:    map[int64]int{},
	}
	if h == nil {
		return out
	}
	now := h.now()
	cur := topologyBucketIndex(now)
	oldest := cur - int64(topologyBucketCount) // 不含

	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.buckets {
		b := &h.buckets[i]
		if b.idx <= oldest || b.idx > cur {
			continue
		}
		out.Totals.add(b.totals)
		for k, v := range b.keyRequests {
			out.KeyRequests[k] += v
		}
		for k, v := range b.keyFailed {
			out.KeyFailed[k] += v
		}
		for k, v := range b.groupRequests {
			out.GroupRequests[k] += v
		}
		for k, v := range b.groupFailed {
			out.GroupFailed[k] += v
		}
		for k, v := range b.blocks {
			out.GateBlocks[k] += v
		}
	}
	for id, until := range h.cooldowns {
		if !until.After(now) {
			delete(h.cooldowns, id)
			continue
		}
		out.Cooldowns[id] = until
	}
	cutoff := now.Add(-topologyLiveRequestTTL)
	for id, lr := range h.live {
		if lr.startedAt.Before(cutoff) {
			delete(h.live, id)
			continue
		}
		out.LiveRequests++
		if !lr.attempted && lr.apiKeyID > 0 {
			out.KeyPending[lr.apiKeyID]++
		}
	}
	return out
}

// CooldownUntil 返回账号当前的冷却结束时间（不在冷却期返回 false）。
func (h *TopologyHub) CooldownUntil(accountID int64) (time.Time, bool) {
	if h == nil {
		return time.Time{}, false
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.cooldowns[accountID]
	if !ok {
		return time.Time{}, false
	}
	if !until.After(now) {
		delete(h.cooldowns, accountID)
		return time.Time{}, false
	}
	return until, true
}

// TopologySubscription 一个事件流订阅者。C 在 Close 后会被关闭。
type TopologySubscription struct {
	C       <-chan RoutingEvent
	ch      chan RoutingEvent
	hub     *TopologyHub
	dropped atomic.Int64
	once    sync.Once
}

// Dropped 因订阅者消费太慢而丢弃的事件数。
func (s *TopologySubscription) Dropped() int64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

// Close 取消订阅（可重复调用）。
func (s *TopologySubscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.hub.subMu.Lock()
		delete(s.hub.subs, s)
		close(s.ch)
		s.hub.subMu.Unlock()
	})
}

// Subscribe 注册一个事件订阅者；超过上限返回 ErrTopologyTooManySubscribers。
func (h *TopologyHub) Subscribe() (*TopologySubscription, error) {
	return h.subscribe(topologySubscriberBuffer)
}

func (h *TopologyHub) subscribe(buffer int) (*TopologySubscription, error) {
	if h == nil {
		return nil, errors.New("topology: hub not configured")
	}
	if buffer <= 0 {
		buffer = 1
	}
	h.subMu.Lock()
	defer h.subMu.Unlock()
	if len(h.subs) >= h.maxSubs {
		return nil, ErrTopologyTooManySubscribers
	}
	ch := make(chan RoutingEvent, buffer)
	sub := &TopologySubscription{C: ch, ch: ch, hub: h}
	h.subs[sub] = struct{}{}
	return sub, nil
}

// SubscriberCount 当前订阅者数量。
func (h *TopologyHub) SubscriberCount() int {
	if h == nil {
		return 0
	}
	h.subMu.RLock()
	defer h.subMu.RUnlock()
	return len(h.subs)
}

// broadcast 非阻塞投递：订阅者通道满了就丢弃该事件。
// 持读锁发送，Close 持写锁关闭通道，保证不会向已关闭的通道发送。
func (h *TopologyHub) broadcast(ev RoutingEvent) {
	h.subMu.RLock()
	defer h.subMu.RUnlock()
	for sub := range h.subs {
		select {
		case sub.ch <- ev:
		default:
			sub.dropped.Add(1)
		}
	}
}
