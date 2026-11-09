//go:build unit

package service

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type topologyTestClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTopologyTestClock() *topologyTestClock {
	return &topologyTestClock{t: time.Unix(1_700_000_000, 0)}
}

func (c *topologyTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *topologyTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestTopologyHub_CountsRequestLifecycle(t *testing.T) {
	clock := newTopologyTestClock()
	hub := NewTopologyHub(clock.Now)

	// r1：一次成功
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r1", APIKeyID: 1, GroupID: 10})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttempt, RequestID: "r1", AccountID: 100, Attempt: 1})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "r1", APIKeyID: 1, GroupID: 10, StatusCode: 200})

	// r2：原地重试 + 换号后仍失败
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r2", APIKeyID: 1, GroupID: 10})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttempt, RequestID: "r2", AccountID: 100, Attempt: 1})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, RequestID: "r2", AccountID: 100, Attempt: 1, StatusCode: 529, Next: RoutingNextSameAccount})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, RequestID: "r2", AccountID: 100, Attempt: 2, StatusCode: 529, Next: RoutingNextSwitch})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, RequestID: "r2", AccountID: 101, Attempt: 3, StatusCode: 500, Next: RoutingNextGiveUp})
	// done 不带 key/group：从开始事件回填
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "r2", StatusCode: 502})

	// r3：被关卡拦下，直接失败（没有重试）
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r3", APIKeyID: 2, GroupID: 20})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventBlocked, RequestID: "r3", APIKeyID: 2, GroupID: 20, Reason: "model_not_supported"})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventBlocked, RequestID: "r3b"})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "r3", APIKeyID: 2, GroupID: 20, StatusCode: 503})

	// r4：还在进行中，尚未发往上游
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r4", APIKeyID: 2, GroupID: 20})

	stats := hub.Stats()
	require.Equal(t, TopologyCounters{
		Requests:         4,
		Failed:           2,
		Blocked:          2,
		RetriesSame:      1,
		RetriesSwitch:    1,
		FailedAfterRetry: 1,
	}, stats.Totals)
	require.Equal(t, int64(2), stats.KeyRequests[1])
	require.Equal(t, int64(2), stats.KeyRequests[2])
	require.Equal(t, int64(1), stats.KeyFailed[1])
	require.Equal(t, int64(1), stats.KeyFailed[2])
	require.Equal(t, int64(2), stats.GroupRequests[10])
	require.Equal(t, int64(1), stats.GroupFailed[10])
	require.Equal(t, int64(1), stats.GroupFailed[20])
	require.Equal(t, map[string]int64{"model_not_supported": 1, TopologyBlockedReasonUnknown: 1}, stats.GateBlocks)
	require.Equal(t, 1, stats.LiveRequests)
	require.Equal(t, 1, stats.KeyPending[2])
	require.Zero(t, stats.KeyPending[1])
}

func TestTopologyHub_FiveMinuteExpiry(t *testing.T) {
	clock := newTopologyTestClock()
	hub := NewTopologyHub(clock.Now)

	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "a", APIKeyID: 1, GroupID: 10})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "a", APIKeyID: 1, GroupID: 10, StatusCode: 500})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventBlocked, Reason: "not_schedulable"})

	clock.Advance(2 * time.Minute)
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "b", APIKeyID: 1, GroupID: 10})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "b", APIKeyID: 1, GroupID: 10, StatusCode: 200})

	clock.Advance(2 * time.Minute) // t0+4m：两次都还在窗口里
	stats := hub.Stats()
	require.Equal(t, int64(2), stats.Totals.Requests)
	require.Equal(t, int64(1), stats.Totals.Failed)
	require.Equal(t, int64(2), stats.KeyRequests[1])
	require.Equal(t, int64(1), stats.GateBlocks["not_schedulable"])

	clock.Advance(time.Minute + time.Second) // t0+5m01s：第一次过期
	stats = hub.Stats()
	require.Equal(t, int64(1), stats.Totals.Requests)
	require.Zero(t, stats.Totals.Failed)
	require.Zero(t, stats.KeyFailed[1])
	require.Equal(t, int64(1), stats.KeyRequests[1])
	require.Empty(t, stats.GateBlocks)

	clock.Advance(3 * time.Minute) // 全部过期
	stats = hub.Stats()
	require.Zero(t, stats.Totals.Requests)
	require.Empty(t, stats.KeyRequests)

	// 环形桶复用后重新计数正常
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "c", APIKeyID: 3})
	stats = hub.Stats()
	require.Equal(t, int64(1), stats.Totals.Requests)
	require.Equal(t, int64(1), stats.KeyRequests[3])
	require.Zero(t, stats.KeyRequests[1])
}

func TestTopologyHub_TracksCooldowns(t *testing.T) {
	clock := newTopologyTestClock()
	hub := NewTopologyHub(clock.Now)

	until := clock.Now().Add(30 * time.Second)
	later := clock.Now().Add(60 * time.Second)
	past := clock.Now().Add(-time.Second)

	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, AccountID: 7, Next: RoutingNextSwitch, CooldownUntil: &until})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, AccountID: 8, Next: RoutingNextSwitch, CooldownUntil: &past})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, AccountID: 9, Next: RoutingNextSwitch})

	got, ok := hub.CooldownUntil(7)
	require.True(t, ok)
	require.True(t, got.Equal(until))
	_, ok = hub.CooldownUntil(8)
	require.False(t, ok)
	_, ok = hub.CooldownUntil(9)
	require.False(t, ok)

	// 更晚的冷却覆盖更早的，更早的不会缩短
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, AccountID: 7, CooldownUntil: &later})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, AccountID: 7, CooldownUntil: &until})
	got, _ = hub.CooldownUntil(7)
	require.True(t, got.Equal(later))
	require.Len(t, hub.Stats().Cooldowns, 1)

	clock.Advance(61 * time.Second)
	_, ok = hub.CooldownUntil(7)
	require.False(t, ok)
	require.Empty(t, hub.Stats().Cooldowns)
}

func TestTopologyHub_AttemptFailedClearsPending(t *testing.T) {
	hub := NewTopologyHub(nil)
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "r1", APIKeyID: 7})
	require.Equal(t, 1, hub.Stats().KeyPending[7])

	hub.OnRoutingEvent(RoutingEvent{
		Type:      RoutingEventAttemptFailed,
		RequestID: "r1",
		AccountID: 9,
		Next:      RoutingNextGiveUp,
	})
	stats := hub.Stats()
	require.Zero(t, stats.KeyPending[7])
	require.Equal(t, 1, stats.LiveRequests)
}

func TestTopologyHub_FanOutDropsWhenSubscriberFull(t *testing.T) {
	hub := NewTopologyHub(nil)

	slow, err := hub.subscribe(2)
	require.NoError(t, err)
	defer slow.Close()
	fast, err := hub.subscribe(16)
	require.NoError(t, err)
	defer fast.Close()

	for i := 0; i < 5; i++ {
		hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "x", Attempt: i})
	}

	require.Equal(t, int64(3), slow.Dropped())
	require.Zero(t, fast.Dropped())
	require.Len(t, fast.C, 5)
	require.Len(t, slow.C, 2)
	first := <-slow.C
	require.Equal(t, RoutingEventRequestStart, first.Type)
	require.Equal(t, 0, first.Attempt)
}

func TestTopologyHub_SubscriberCapAndClose(t *testing.T) {
	hub := NewTopologyHub(nil)

	subs := make([]*TopologySubscription, 0, TopologyMaxSubscribers)
	for i := 0; i < TopologyMaxSubscribers; i++ {
		sub, err := hub.Subscribe()
		require.NoError(t, err)
		subs = append(subs, sub)
	}
	require.Equal(t, TopologyMaxSubscribers, hub.SubscriberCount())

	_, err := hub.Subscribe()
	require.ErrorIs(t, err, ErrTopologyTooManySubscribers)

	subs[0].Close()
	subs[0].Close() // 重复关闭无害
	_, ok := <-subs[0].C
	require.False(t, ok, "closed subscription channel must be closed")
	require.Equal(t, TopologyMaxSubscribers-1, hub.SubscriberCount())

	again, err := hub.Subscribe()
	require.NoError(t, err)
	again.Close()

	// 关闭后的订阅者不再收事件，也不会 panic
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "y"})
	for _, s := range subs[1:] {
		require.Len(t, s.C, 1)
		s.Close()
	}
	require.Zero(t, hub.SubscriberCount())
}

func TestTopologyHub_ConcurrentEventsAndSubscribers(t *testing.T) {
	hub := NewTopologyHub(nil)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				sub, err := hub.Subscribe()
				if err == nil {
					sub.Close()
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "z", APIKeyID: 1})
				hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "z", APIKeyID: 1, StatusCode: 200})
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(800), hub.Stats().Totals.Requests)
}

func TestProvideTopologyHub_RegistersOnce(t *testing.T) {
	a := ProvideTopologyHub()
	b := ProvideTopologyHub()
	require.Same(t, a, b)

	sub, err := a.Subscribe()
	require.NoError(t, err)
	defer sub.Close()

	EmitRoutingEvent(RoutingEvent{Type: RoutingEventBlocked, RequestID: "provide-test", Reason: "provide_test"})
	deadline := time.After(time.Second)
	for {
		select {
		case ev := <-sub.C:
			// 其它并行测试也可能发事件，只认自己的那条
			if ev.RequestID == "provide-test" {
				return
			}
		case <-deadline:
			t.Fatal("registered hub did not receive emitted event")
		}
	}
}
