//go:build unit

package admin

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type topologyHandlerInventory struct {
	keys     []service.APIKey
	groups   []service.Group
	accounts []service.Account
}

func (f *topologyHandlerInventory) ListTopologyAPIKeys(context.Context) ([]service.APIKey, error) {
	return f.keys, nil
}

func (f *topologyHandlerInventory) ListTopologyGroups(context.Context) ([]service.Group, error) {
	return f.groups, nil
}

func (f *topologyHandlerInventory) ListTopologyAccounts(context.Context) ([]service.Account, error) {
	return f.accounts, nil
}

func newTopologyTestRouter(h *TopologyHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/topology/snapshot", h.GetSnapshot)
	r.GET("/api/v1/admin/topology/events", h.StreamEvents)
	return r
}

func TestTopologyHandler_GetSnapshot(t *testing.T) {
	groupID := int64(10)
	inv := &topologyHandlerInventory{
		keys:   []service.APIKey{{ID: 1, Name: "k1", GroupID: &groupID, Status: service.StatusActive}},
		groups: []service.Group{{ID: 10, Name: "g", Platform: service.PlatformOpenAI}},
		accounts: []service.Account{
			{
				ID:          5,
				Name:        "acc",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 3,
				GroupIDs:    []int64{10},
				Extra: map[string]any{
					"codex_5h_used_percent":   10.0,
					"codex_5h_window_minutes": 0,
					"codex_7d_used_percent":   50.0,
				},
			},
		},
	}
	hub := service.NewTopologyHub(nil)
	hub.OnRoutingEvent(service.RoutingEvent{Type: service.RoutingEventBlocked, Reason: "not_schedulable"})
	h := NewTopologyHandler(service.NewTopologyService(hub, inv, nil, nil))
	r := newTopologyTestRouter(h)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/topology/snapshot", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Code int `json:"code"`
		Data map[string]json.RawMessage
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, 0, env.Code)
	for _, field := range []string{"generated_at", "totals", "gate_blocks_5m", "keys", "groups", "accounts"} {
		require.Contains(t, env.Data, field)
	}

	var totals map[string]any
	require.NoError(t, json.Unmarshal(env.Data["totals"], &totals))
	for _, field := range []string{"in_flight", "queued", "requests_5m", "failed_5m", "retries_same_5m", "retries_switch_5m", "failed_after_retry_5m", "accounts_available", "accounts_total"} {
		require.Contains(t, totals, field)
	}
	require.EqualValues(t, 1, totals["accounts_available"])

	var blocks map[string]int64
	require.NoError(t, json.Unmarshal(env.Data["gate_blocks_5m"], &blocks))
	require.Equal(t, int64(1), blocks["not_schedulable"])

	var accounts []map[string]any
	require.NoError(t, json.Unmarshal(env.Data["accounts"], &accounts))
	require.Len(t, accounts, 1)
	acc := accounts[0]
	require.Equal(t, "ok", acc["status"])
	require.Equal(t, "pay_as_you_go", acc["billing"])
	require.Contains(t, acc, "quota_5h")
	require.Nil(t, acc["quota_5h"], "window 0 must serialize as null")
	require.Contains(t, acc, "recover_at")
	require.Nil(t, acc["recover_at"])
	q7d, ok := acc["quota_7d"].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 50, q7d["used_percent"])
	require.EqualValues(t, 3, acc["concurrency_max"])

	var groups []map[string]any
	require.NoError(t, json.Unmarshal(env.Data["groups"], &groups))
	require.Len(t, groups, 1)
	require.Equal(t, "load_balance", groups[0]["strategy"])
	require.Equal(t, []any{float64(5)}, groups[0]["account_ids"])

	var keys []map[string]any
	require.NoError(t, json.Unmarshal(env.Data["keys"], &keys))
	require.Len(t, keys, 1)
	require.Equal(t, "g", keys[0]["group_name"])
}

func TestTopologyHandler_GetSnapshotUnavailable(t *testing.T) {
	r := newTopologyTestRouter(NewTopologyHandler(nil))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/topology/snapshot", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestTopologyHandler_StreamEvents(t *testing.T) {
	hub := service.NewTopologyHub(nil)
	h := NewTopologyHandler(service.NewTopologyService(hub, &topologyHandlerInventory{}, nil, nil))
	h.heartbeat = 50 * time.Millisecond
	srv := httptest.NewServer(newTopologyTestRouter(h))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/admin/topology/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	require.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	require.Eventually(t, func() bool { return hub.SubscriberCount() == 1 }, 2*time.Second, 10*time.Millisecond)

	go func() {
		hub.OnRoutingEvent(service.RoutingEvent{Type: service.RoutingEventRequestStart, RequestID: "req-1", APIKeyID: 7, At: time.Unix(1_700_000_000, 0).UTC()})
		hub.OnRoutingEvent(service.RoutingEvent{Type: service.RoutingEventRequestDone, RequestID: "req-1", StatusCode: 200, At: time.Unix(1_700_000_001, 0).UTC()})
	}()

	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	var (
		events    []service.RoutingEvent
		sawPing   bool
		eventName string
	)
	timeout := time.After(3 * time.Second)
readLoop:
	for len(events) < 2 || !sawPing {
		select {
		case line, ok := <-lines:
			if !ok {
				break readLoop
			}
			switch {
			case line == ": ping":
				sawPing = true
			case strings.HasPrefix(line, "event: "):
				eventName = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				require.Equal(t, "routing", eventName)
				var ev service.RoutingEvent
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev))
				events = append(events, ev)
			}
		case <-timeout:
			break readLoop
		}
	}

	require.Len(t, events, 2)
	require.Equal(t, service.RoutingEventRequestStart, events[0].Type)
	require.Equal(t, "req-1", events[0].RequestID)
	require.Equal(t, int64(7), events[0].APIKeyID)
	require.Equal(t, service.RoutingEventRequestDone, events[1].Type)
	require.Equal(t, 200, events[1].StatusCode)
	require.True(t, sawPing, "expected a heartbeat comment")

	// 客户端断开后订阅者被回收
	cancel()
	require.Eventually(t, func() bool { return hub.SubscriberCount() == 0 }, 2*time.Second, 10*time.Millisecond)
}

func TestTopologyHandler_StreamEventsSubscriberCap(t *testing.T) {
	hub := service.NewTopologyHub(nil)
	for i := 0; i < service.TopologyMaxSubscribers; i++ {
		sub, err := hub.Subscribe()
		require.NoError(t, err)
		defer sub.Close()
	}
	h := NewTopologyHandler(service.NewTopologyService(hub, &topologyHandlerInventory{}, nil, nil))
	r := newTopologyTestRouter(h)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/topology/events", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}
