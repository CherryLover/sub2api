//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeTopologyInventory struct {
	keys     []APIKey
	groups   []Group
	accounts []Account
	err      error
}

func (f *fakeTopologyInventory) ListTopologyAPIKeys(context.Context) ([]APIKey, error) {
	return f.keys, f.err
}

func (f *fakeTopologyInventory) ListTopologyGroups(context.Context) ([]Group, error) {
	return f.groups, nil
}

func (f *fakeTopologyInventory) ListTopologyAccounts(context.Context) ([]Account, error) {
	return f.accounts, nil
}

type fakeTopologyLoads struct {
	keys     map[int64]int
	accounts map[int64]*AccountLoadInfo
	keyErr   error
	gotKeys  []int64
	gotAccts []AccountWithConcurrency
}

func (f *fakeTopologyLoads) GetAPIKeyConcurrencyBatch(_ context.Context, ids []int64) (map[int64]int, error) {
	f.gotKeys = ids
	if f.keyErr != nil {
		return nil, f.keyErr
	}
	return f.keys, nil
}

func (f *fakeTopologyLoads) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	f.gotAccts = accounts
	return f.accounts, nil
}

func topologyFindAccount(t *testing.T, snap *TopologySnapshot, id int64) TopologyAccount {
	t.Helper()
	for _, a := range snap.Accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("account %d not in snapshot", id)
	return TopologyAccount{}
}

func TestTopologyService_SnapshotAssembly(t *testing.T) {
	clock := newTopologyTestClock()
	now := clock.Now()
	hub := NewTopologyHub(clock.Now)

	g10 := int64(10)
	future := now.Add(10 * time.Minute)
	soon := now.Add(2 * time.Minute)
	past := now.Add(-time.Minute)
	reset5h := now.Add(90 * time.Minute).UTC().Format(time.RFC3339)
	reset7d := now.Add(-time.Hour).UTC().Format(time.RFC3339) // 已过期 → 用量归零
	cooldown := now.Add(45 * time.Second)

	inv := &fakeTopologyInventory{
		keys: []APIKey{
			{ID: 2, Name: "beta", GroupID: &g10, Status: StatusActive},
			{ID: 1, Name: "alpha", GroupID: &g10, Status: StatusActive},
			{ID: 3, Name: "nogroup", Status: StatusActive},
		},
		groups: []Group{{ID: 10, Name: "codex", Platform: PlatformOpenAI, Status: StatusActive}},
		accounts: []Account{
			// ok, subscription, codex quotas
			{
				ID:          100,
				Name:        "ok",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 5,
				GroupIDs:    []int64{10},
				Extra: map[string]any{
					"codex_5h_used_percent":   42.5,
					"codex_5h_reset_at":       reset5h,
					"codex_5h_window_minutes": 300,
					"codex_7d_used_percent":   "80",
					"codex_7d_reset_at":       reset7d,
					"codex_7d_window_minutes": float64(10080),
				},
			},
			// rate limited; 5h window doesn't exist (window 0) -> null
			{
				ID:               101,
				Name:             "limited",
				Platform:         PlatformOpenAI,
				Type:             AccountTypeOAuth,
				Status:           StatusActive,
				Schedulable:      true,
				Concurrency:      3,
				RateLimitResetAt: &future,
				OverloadUntil:    &soon,
				AccountGroups:    []AccountGroup{{AccountID: 101, GroupID: 10}},
				Extra: map[string]any{
					"codex_5h_used_percent":   0,
					"codex_5h_window_minutes": 0,
					"codex_7d_used_percent":   12,
					"codex_7d_window_minutes": 10080,
				},
			},
			// overload (expired rate limit) -> cooldown
			{
				ID:               102,
				Name:             "hot",
				Platform:         PlatformOpenAI,
				Type:             AccountTypeAPIKey,
				Status:           StatusActive,
				Schedulable:      true,
				Concurrency:      2,
				OverloadUntil:    &soon,
				RateLimitResetAt: &past,
				GroupIDs:         []int64{10},
			},
			// hub cooldown only, pay-as-you-go
			{
				ID:          103,
				Name:        "cool",
				Platform:    PlatformAnthropic,
				Type:        AccountTypeUpstream,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
			},
			{ID: 104, Name: "broken", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, Schedulable: true},
			{ID: 105, Name: "off", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled},
			{ID: 106, Name: "paused", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
			// anthropic session window fallback
			{
				ID:               107,
				Name:             "claude",
				Platform:         PlatformAnthropic,
				Type:             AccountTypeSetupToken,
				Status:           StatusActive,
				Schedulable:      true,
				Concurrency:      4,
				SessionWindowEnd: &future,
				Extra:            map[string]any{"session_window_utilization": 0.25},
			},
		},
	}
	loads := &fakeTopologyLoads{
		keys: map[int64]int{1: 2, 2: 1, 3: 0},
		accounts: map[int64]*AccountLoadInfo{
			100: {AccountID: 100, CurrentConcurrency: 2, WaitingCount: 1},
			101: {AccountID: 101, CurrentConcurrency: 1, WaitingCount: 0},
		},
	}

	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "a", APIKeyID: 1, GroupID: 10})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventAttemptFailed, RequestID: "a", AccountID: 103, Next: RoutingNextSwitch, CooldownUntil: &cooldown})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestDone, RequestID: "a", APIKeyID: 1, GroupID: 10, StatusCode: 502})
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventBlocked, Reason: "model_rate_limited"})

	svc := NewTopologyService(hub, inv, loads, clock.Now)
	snap, err := svc.Snapshot(context.Background())
	require.NoError(t, err)

	require.True(t, snap.GeneratedAt.Equal(now))
	require.Equal(t, TopologyTotals{
		InFlight:           3,
		Queued:             1,
		Requests5m:         1,
		Failed5m:           1,
		Blocked5m:          1,
		RetriesSwitch5m:    1,
		FailedAfterRetry5m: 1,
		AccountsAvailable:  2, // 100, 107
		AccountsTotal:      8,
	}, snap.Totals)
	require.Equal(t, map[string]int64{"model_rate_limited": 1}, snap.GateBlocks5m)

	// keys sorted by id, group name resolved
	require.Len(t, snap.Keys, 3)
	require.Equal(t, int64(1), snap.Keys[0].ID)
	require.Equal(t, "codex", snap.Keys[0].GroupName)
	require.Equal(t, 2, snap.Keys[0].InFlight)
	require.Equal(t, int64(1), snap.Keys[0].Requests5m)
	require.Equal(t, int64(1), snap.Keys[0].Failed5m)
	require.Equal(t, int64(0), snap.Keys[2].GroupID)
	require.Equal(t, "", snap.Keys[2].GroupName)
	require.ElementsMatch(t, []int64{2, 1, 3}, loads.gotKeys)

	require.Len(t, snap.Groups, 1)
	require.Equal(t, TopologyGroup{
		ID:         10,
		Name:       "codex",
		Platform:   PlatformOpenAI,
		Strategy:   TopologyStrategyLoadBalance,
		InFlight:   3,
		Requests5m: 1,
		Failed5m:   1,
		AccountIDs: []int64{100, 101, 102},
	}, snap.Groups[0])

	ok := topologyFindAccount(t, snap, 100)
	require.Equal(t, TopologyAccountOK, ok.Status)
	require.Equal(t, "正常", ok.StatusText)
	require.Nil(t, ok.RecoverAt)
	require.Equal(t, 2, ok.ConcurrencyCurrent)
	require.Equal(t, 5, ok.ConcurrencyMax)
	require.Equal(t, 1, ok.Waiting)
	require.Equal(t, TopologyBillingSubscription, ok.Billing)
	require.NotNil(t, ok.Quota5h)
	require.InDelta(t, 42.5, ok.Quota5h.UsedPercent, 0.001)
	require.NotNil(t, ok.Quota5h.ResetAt)
	require.NotNil(t, ok.Quota7d)
	require.Zero(t, ok.Quota7d.UsedPercent, "expired window resets to 0")

	limited := topologyFindAccount(t, snap, 101)
	require.Equal(t, TopologyAccountRateLimited, limited.Status)
	require.True(t, limited.RecoverAt.Equal(future))
	require.Nil(t, limited.Quota5h, "window_minutes 0 means no 5h window")
	require.NotNil(t, limited.Quota7d)
	require.InDelta(t, 12, limited.Quota7d.UsedPercent, 0.001)
	require.Nil(t, limited.Quota7d.ResetAt)
	require.Equal(t, []int64{10}, limited.GroupIDs)

	hot := topologyFindAccount(t, snap, 102)
	require.Equal(t, TopologyAccountCooldown, hot.Status)
	require.True(t, hot.RecoverAt.Equal(soon))
	require.Equal(t, TopologyBillingPayAsYouGo, hot.Billing)
	require.Nil(t, hot.Quota5h)
	require.Nil(t, hot.Quota7d)

	cool := topologyFindAccount(t, snap, 103)
	require.Equal(t, TopologyAccountCooldown, cool.Status)
	require.Equal(t, "冷却中", cool.StatusText)
	require.True(t, cool.RecoverAt.Equal(cooldown))
	require.Equal(t, TopologyBillingPayAsYouGo, cool.Billing)
	require.Equal(t, []int64{}, cool.GroupIDs)

	require.Equal(t, TopologyAccountError, topologyFindAccount(t, snap, 104).Status)
	require.Equal(t, "授权失效", topologyFindAccount(t, snap, 104).StatusText)
	require.Equal(t, TopologyAccountDisabled, topologyFindAccount(t, snap, 105).Status)
	require.Equal(t, TopologyAccountDisabled, topologyFindAccount(t, snap, 106).Status)

	claude := topologyFindAccount(t, snap, 107)
	require.Equal(t, TopologyBillingSubscription, claude.Billing)
	require.NotNil(t, claude.Quota5h)
	require.InDelta(t, 25, claude.Quota5h.UsedPercent, 0.001)
	require.Nil(t, claude.Quota7d)

	// 冷却到期后恢复正常
	clock.Advance(46 * time.Second)
	snap, err = svc.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, TopologyAccountOK, topologyFindAccount(t, snap, 103).Status)
}

func TestTopologyService_DegradesWhenConcurrencyFails(t *testing.T) {
	inv := &fakeTopologyInventory{
		keys:     []APIKey{{ID: 1, Name: "k", Status: StatusActive}},
		accounts: []Account{{ID: 1, Name: "a", Status: StatusActive, Schedulable: true, Concurrency: 2}},
	}
	loads := &fakeTopologyLoads{keyErr: errors.New("redis down")}
	svc := NewTopologyService(NewTopologyHub(nil), inv, loads, nil)

	snap, err := svc.Snapshot(context.Background())
	require.NoError(t, err)
	require.Zero(t, snap.Keys[0].InFlight)
	require.Zero(t, snap.Accounts[0].ConcurrencyCurrent)
	require.Equal(t, 2, snap.Accounts[0].ConcurrencyMax)
	require.Equal(t, []AccountWithConcurrency{{ID: 1, MaxConcurrency: 2}}, loads.gotAccts)
	require.NotNil(t, snap.Groups)
	require.NotNil(t, snap.GateBlocks5m)
}

func TestTopologyService_QueuedFallsBackToPendingWithoutDoubleCounting(t *testing.T) {
	inv := &fakeTopologyInventory{
		keys:     []APIKey{{ID: 1, Name: "k", Status: StatusActive}},
		accounts: []Account{{ID: 1, Name: "a", Status: StatusActive, Schedulable: true, Concurrency: 2}},
	}
	hub := NewTopologyHub(nil)
	hub.OnRoutingEvent(RoutingEvent{Type: RoutingEventRequestStart, RequestID: "pending", APIKeyID: 1})

	svc := NewTopologyService(hub, inv, nil, nil)
	snap, err := svc.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, snap.Totals.Queued)
	require.Equal(t, 1, snap.Keys[0].Queued)

	loads := &fakeTopologyLoads{
		accounts: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, WaitingCount: 2},
		},
	}
	svc = NewTopologyService(hub, inv, loads, nil)
	snap, err = svc.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, snap.Totals.Queued, "account waiting count replaces pending estimate")
	require.Equal(t, 1, snap.Keys[0].Queued)
}

func TestTopologyService_InventoryErrorAndNilDeps(t *testing.T) {
	svc := NewTopologyService(nil, &fakeTopologyInventory{err: errors.New("db down")}, nil, nil)
	_, err := svc.Snapshot(context.Background())
	require.Error(t, err)

	// 没有 hub、没有并发来源也能出快照
	svc = NewTopologyService(nil, &fakeTopologyInventory{}, nil, nil)
	snap, err := svc.Snapshot(context.Background())
	require.NoError(t, err)
	require.Empty(t, snap.Keys)
	_, err = svc.Subscribe()
	require.Error(t, err)
}
