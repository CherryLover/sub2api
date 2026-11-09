package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"go.uber.org/zap"
)

// 调度拓扑快照：API Key -> 分组 -> 上游账号 三栏 + 近 5 分钟计数。
// 管理员页面约每 5 秒轮询一次，只用现成的全量列表查询和 Redis 并发批量读。

// Topology account status values.
const (
	TopologyAccountOK          = "ok"
	TopologyAccountRateLimited = "rate_limited"
	TopologyAccountCooldown    = "cooldown"
	TopologyAccountError       = "error"
	TopologyAccountDisabled    = "disabled"
)

// Topology account billing values.
const (
	TopologyBillingSubscription = "subscription"
	TopologyBillingPayAsYouGo   = "pay_as_you_go"
)

// TopologyStrategyLoadBalance 目前所有分组都按负载均衡选号（同会话粘性在选号内部处理）。
const TopologyStrategyLoadBalance = "load_balance"

const topologyAPIKeyPageSize = 1000

// TopologySnapshot GET /admin/topology/snapshot 的返回体。
type TopologySnapshot struct {
	GeneratedAt  time.Time         `json:"generated_at"`
	Totals       TopologyTotals    `json:"totals"`
	GateBlocks5m map[string]int64  `json:"gate_blocks_5m"`
	Keys         []TopologyKey     `json:"keys"`
	Groups       []TopologyGroup   `json:"groups"`
	Accounts     []TopologyAccount `json:"accounts"`
}

// TopologyTotals 顶部汇总。
type TopologyTotals struct {
	InFlight           int   `json:"in_flight"`
	Queued             int   `json:"queued"`
	Requests5m         int64 `json:"requests_5m"`
	Failed5m           int64 `json:"failed_5m"`
	Blocked5m          int64 `json:"blocked_5m"`
	RetriesSame5m      int64 `json:"retries_same_5m"`
	RetriesSwitch5m    int64 `json:"retries_switch_5m"`
	FailedAfterRetry5m int64 `json:"failed_after_retry_5m"`
	AccountsAvailable  int   `json:"accounts_available"`
	AccountsTotal      int   `json:"accounts_total"`
}

// TopologyKey 左栏：一个启用中的 API Key。
type TopologyKey struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	GroupID    int64  `json:"group_id"`
	GroupName  string `json:"group_name"`
	InFlight   int    `json:"in_flight"`
	Queued     int    `json:"queued"`
	Requests5m int64  `json:"requests_5m"`
	Failed5m   int64  `json:"failed_5m"`
}

// TopologyGroup 中栏：一个启用中的分组（平台卡片）。
type TopologyGroup struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Strategy   string  `json:"strategy"`
	InFlight   int     `json:"in_flight"`
	Requests5m int64   `json:"requests_5m"`
	Failed5m   int64   `json:"failed_5m"`
	AccountIDs []int64 `json:"account_ids"`
}

// TopologyQuota 一个用量窗口（5 小时 / 7 天）。
type TopologyQuota struct {
	UsedPercent float64    `json:"used_percent"`
	ResetAt     *time.Time `json:"reset_at"`
}

// TopologyAccount 右栏：一个上游账号。
type TopologyAccount struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	Platform           string         `json:"platform"`
	Status             string         `json:"status"`
	StatusText         string         `json:"status_text"`
	RecoverAt          *time.Time     `json:"recover_at"`
	ConcurrencyCurrent int            `json:"concurrency_current"`
	ConcurrencyMax     int            `json:"concurrency_max"`
	Waiting            int            `json:"waiting"`
	Quota5h            *TopologyQuota `json:"quota_5h"`
	Quota7d            *TopologyQuota `json:"quota_7d"`
	Billing            string         `json:"billing"`
	GroupIDs           []int64        `json:"group_ids"`
}

// TopologyInventory 快照所需的库存数据来源。
type TopologyInventory interface {
	ListTopologyAPIKeys(ctx context.Context) ([]APIKey, error)
	ListTopologyGroups(ctx context.Context) ([]Group, error)
	ListTopologyAccounts(ctx context.Context) ([]Account, error)
}

// TopologyLoadReader 实时并发数来源（ConcurrencyService 满足该接口）。
type TopologyLoadReader interface {
	GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error)
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
}

// adminTopologyInventory 基于 AdminService 的现成列表查询。
type adminTopologyInventory struct {
	admin AdminService
}

func (i adminTopologyInventory) ListTopologyAPIKeys(ctx context.Context) ([]APIKey, error) {
	var out []APIKey
	filters := AdminAPIKeyListFilters{Status: StatusActive}
	for page := 1; ; page++ {
		keys, result, err := i.admin.AdminListAPIKeys(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  topologyAPIKeyPageSize,
			SortBy:    "created_at",
			SortOrder: pagination.SortOrderAsc,
		}, filters)
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
		if len(keys) == 0 || len(keys) < topologyAPIKeyPageSize || result == nil || page >= result.Pages {
			break
		}
	}
	return out, nil
}

func (i adminTopologyInventory) ListTopologyGroups(ctx context.Context) ([]Group, error) {
	return i.admin.GetAllGroups(ctx)
}

func (i adminTopologyInventory) ListTopologyAccounts(ctx context.Context) ([]Account, error) {
	return i.admin.ListAccountsForSchedulerScoreFilter(ctx, "", "", "", "", 0, "")
}

// TopologyService 组装拓扑快照并提供事件订阅。
type TopologyService struct {
	hub       *TopologyHub
	inventory TopologyInventory
	loads     TopologyLoadReader
	now       func() time.Time
}

// NewTopologyService 创建服务；now 为 nil 时使用 time.Now。
func NewTopologyService(hub *TopologyHub, inventory TopologyInventory, loads TopologyLoadReader, now func() time.Time) *TopologyService {
	if now == nil {
		now = time.Now
	}
	return &TopologyService{hub: hub, inventory: inventory, loads: loads, now: now}
}

// ProvideTopologyService wire 装配入口。
func ProvideTopologyService(hub *TopologyHub, adminService AdminService, concurrencyService *ConcurrencyService) *TopologyService {
	var loads TopologyLoadReader
	if concurrencyService != nil {
		loads = concurrencyService
	}
	return NewTopologyService(hub, adminTopologyInventory{admin: adminService}, loads, nil)
}

// Subscribe 订阅实时路由事件。
func (s *TopologyService) Subscribe() (*TopologySubscription, error) {
	if s == nil || s.hub == nil {
		return nil, fmt.Errorf("topology: hub not configured")
	}
	return s.hub.Subscribe()
}

// Snapshot 组装当前拓扑快照。
func (s *TopologyService) Snapshot(ctx context.Context) (*TopologySnapshot, error) {
	if s == nil || s.inventory == nil {
		return nil, fmt.Errorf("topology: service not configured")
	}
	now := s.now()

	keys, err := s.inventory.ListTopologyAPIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	groups, err := s.inventory.ListTopologyGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	accounts, err := s.inventory.ListTopologyAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	stats := s.hub.Stats() // nil hub 返回空统计

	keyIDs := make([]int64, 0, len(keys))
	for i := range keys {
		keyIDs = append(keyIDs, keys[i].ID)
	}
	keyInFlight := map[int64]int{}
	accountLoads := map[int64]*AccountLoadInfo{}
	if s.loads != nil {
		if len(keyIDs) > 0 {
			if m, err := s.loads.GetAPIKeyConcurrencyBatch(ctx, keyIDs); err != nil {
				logger.L().Warn("topology_api_key_concurrency_failed", zap.Error(err))
			} else if m != nil {
				keyInFlight = m
			}
		}
		if len(accounts) > 0 {
			batch := make([]AccountWithConcurrency, 0, len(accounts))
			for i := range accounts {
				batch = append(batch, AccountWithConcurrency{ID: accounts[i].ID, MaxConcurrency: accounts[i].Concurrency})
			}
			if m, err := s.loads.GetAccountsLoadBatch(ctx, batch); err != nil {
				logger.L().Warn("topology_account_load_failed", zap.Error(err))
			} else if m != nil {
				accountLoads = m
			}
		}
	}

	return buildTopologySnapshot(now, keys, groups, accounts, stats, keyInFlight, accountLoads), nil
}

func buildTopologySnapshot(
	now time.Time,
	keys []APIKey,
	groups []Group,
	accounts []Account,
	stats TopologyStats,
	keyInFlight map[int64]int,
	accountLoads map[int64]*AccountLoadInfo,
) *TopologySnapshot {
	snap := &TopologySnapshot{
		GeneratedAt:  now,
		GateBlocks5m: map[string]int64{},
		Keys:         make([]TopologyKey, 0, len(keys)),
		Groups:       make([]TopologyGroup, 0, len(groups)),
		Accounts:     make([]TopologyAccount, 0, len(accounts)),
	}
	for k, v := range stats.GateBlocks {
		snap.GateBlocks5m[k] = v
	}
	snap.Totals.Requests5m = stats.Totals.Requests
	snap.Totals.Failed5m = stats.Totals.Failed
	snap.Totals.Blocked5m = stats.Totals.Blocked
	snap.Totals.RetriesSame5m = stats.Totals.RetriesSame
	snap.Totals.RetriesSwitch5m = stats.Totals.RetriesSwitch
	snap.Totals.FailedAfterRetry5m = stats.Totals.FailedAfterRetry

	groupNames := make(map[int64]string, len(groups))
	for i := range groups {
		groupNames[groups[i].ID] = groups[i].Name
	}

	// Keys
	groupInFlight := map[int64]int{}
	for i := range keys {
		k := &keys[i]
		item := TopologyKey{
			ID:         k.ID,
			Name:       k.Name,
			InFlight:   keyInFlight[k.ID],
			Queued:     stats.KeyPending[k.ID],
			Requests5m: stats.KeyRequests[k.ID],
			Failed5m:   stats.KeyFailed[k.ID],
		}
		if k.GroupID != nil {
			item.GroupID = *k.GroupID
		}
		if name, ok := groupNames[item.GroupID]; ok {
			item.GroupName = name
		} else if k.Group != nil {
			item.GroupName = k.Group.Name
		}
		if item.GroupID > 0 {
			groupInFlight[item.GroupID] += item.InFlight
		}
		snap.Totals.InFlight += item.InFlight
		snap.Keys = append(snap.Keys, item)
	}
	sort.SliceStable(snap.Keys, func(a, b int) bool { return snap.Keys[a].ID < snap.Keys[b].ID })

	// Accounts
	groupAccounts := map[int64][]int64{}
	accountQueued := 0
	for i := range accounts {
		acc := &accounts[i]
		item := topologyAccountFromService(acc, now, stats.Cooldowns)
		if load := accountLoads[acc.ID]; load != nil {
			item.ConcurrencyCurrent = load.CurrentConcurrency
			item.Waiting = load.WaitingCount
		}
		accountQueued += item.Waiting
		snap.Totals.AccountsTotal++
		if item.Status == TopologyAccountOK {
			snap.Totals.AccountsAvailable++
		}
		for _, gid := range item.GroupIDs {
			groupAccounts[gid] = append(groupAccounts[gid], acc.ID)
		}
		snap.Accounts = append(snap.Accounts, item)
	}
	sort.SliceStable(snap.Accounts, func(a, b int) bool { return snap.Accounts[a].ID < snap.Accounts[b].ID })

	// Concurrency load already reports requests waiting for an account slot. Use
	// hub pending state only when no account waiting count is reported, so the
	// same queue wait is not counted twice.
	keyPending := 0
	for _, n := range stats.KeyPending {
		keyPending += n
	}
	if accountQueued > 0 {
		snap.Totals.Queued = accountQueued
	} else {
		snap.Totals.Queued = keyPending
	}

	// Groups
	for i := range groups {
		g := &groups[i]
		ids := groupAccounts[g.ID]
		if ids == nil {
			ids = []int64{}
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
		snap.Groups = append(snap.Groups, TopologyGroup{
			ID:         g.ID,
			Name:       g.Name,
			Platform:   g.Platform,
			Strategy:   TopologyStrategyLoadBalance,
			InFlight:   groupInFlight[g.ID],
			Requests5m: stats.GroupRequests[g.ID],
			Failed5m:   stats.GroupFailed[g.ID],
			AccountIDs: ids,
		})
	}
	sort.SliceStable(snap.Groups, func(a, b int) bool { return snap.Groups[a].ID < snap.Groups[b].ID })
	return snap
}

func topologyAccountGroupIDs(acc *Account) []int64 {
	seen := map[int64]struct{}{}
	out := []int64{}
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range acc.GroupIDs {
		add(id)
	}
	for _, ag := range acc.AccountGroups {
		add(ag.GroupID)
	}
	for _, g := range acc.Groups {
		if g != nil {
			add(g.ID)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func topologyAccountFromService(acc *Account, now time.Time, cooldowns map[int64]time.Time) TopologyAccount {
	item := TopologyAccount{
		ID:             acc.ID,
		Name:           acc.Name,
		Platform:       acc.Platform,
		ConcurrencyMax: acc.Concurrency,
		Billing:        TopologyBillingPayAsYouGo,
		GroupIDs:       topologyAccountGroupIDs(acc),
	}
	if acc.IsOAuth() {
		item.Billing = TopologyBillingSubscription
	}
	item.Status, item.StatusText, item.RecoverAt = topologyAccountStatus(acc, now, cooldowns)
	item.Quota5h, item.Quota7d = topologyAccountQuotas(acc, now)
	return item
}

func topologyFuture(t *time.Time, now time.Time) bool {
	return t != nil && t.After(now)
}

func topologyLater(cur *time.Time, t time.Time) *time.Time {
	if cur == nil || t.After(*cur) {
		v := t
		return &v
	}
	return cur
}

// topologyAccountStatus 把账号状态折算成页面上的四种状态 + 已停用。
// 优先级：停用 > 授权失效 > 暂停调度 > 限流中 > 冷却中 > 正常。
func topologyAccountStatus(acc *Account, now time.Time, cooldowns map[int64]time.Time) (string, string, *time.Time) {
	switch acc.Status {
	case StatusDisabled:
		return TopologyAccountDisabled, "已停用", nil
	case StatusError:
		return TopologyAccountError, "授权失效", nil
	}
	if acc.Status != "" && acc.Status != StatusActive {
		return TopologyAccountDisabled, "已停用", nil
	}
	if acc.AutoPauseOnExpired && acc.ExpiresAt != nil && !now.Before(*acc.ExpiresAt) {
		return TopologyAccountError, "已过期", nil
	}
	if !acc.Schedulable {
		return TopologyAccountDisabled, "暂停调度", nil
	}
	if topologyFuture(acc.RateLimitResetAt, now) {
		v := *acc.RateLimitResetAt
		return TopologyAccountRateLimited, "限流中", &v
	}
	var recoverAt *time.Time
	if topologyFuture(acc.OverloadUntil, now) {
		recoverAt = topologyLater(recoverAt, *acc.OverloadUntil)
	}
	if topologyFuture(acc.TempUnschedulableUntil, now) {
		recoverAt = topologyLater(recoverAt, *acc.TempUnschedulableUntil)
	}
	if until, ok := cooldowns[acc.ID]; ok && until.After(now) {
		recoverAt = topologyLater(recoverAt, until)
	}
	if recoverAt != nil {
		return TopologyAccountCooldown, "冷却中", recoverAt
	}
	return TopologyAccountOK, "正常", nil
}

// topologyAccountQuotas 读取账号 extra 里已同步的 5h / 7d 用量窗口。
// OpenAI(Codex) 用 codex_* 键；window_minutes 为 0 表示该窗口不存在 → nil。
// 没有 codex 键时回退到 Anthropic 的会话窗口 / 被动 7 天用量。
func topologyAccountQuotas(acc *Account, now time.Time) (*TopologyQuota, *TopologyQuota) {
	extra := acc.Extra
	if len(extra) == 0 {
		return nil, nil
	}
	_, has5h := extra["codex_5h_used_percent"]
	_, has7d := extra["codex_7d_used_percent"]
	if has5h || has7d {
		return topologyCodexQuota(extra, "5h", now), topologyCodexQuota(extra, "7d", now)
	}

	var q5h, q7d *TopologyQuota
	if raw, ok := extra["session_window_utilization"]; ok {
		q5h = topologyQuota(utilizationAsPercent(raw), acc.SessionWindowEnd, now)
	}
	if raw, ok := extra["passive_usage_7d_utilization"]; ok {
		q7d = topologyQuota(utilizationAsPercent(raw), parseSchedulingResetAt(extra["passive_usage_7d_reset"]), now)
	}
	return q5h, q7d
}

func topologyCodexQuota(extra map[string]any, window string, now time.Time) *TopologyQuota {
	usedRaw, ok := extra["codex_"+window+"_used_percent"]
	if !ok {
		return nil
	}
	if raw, ok := extra["codex_"+window+"_window_minutes"]; ok && parseExtraInt(raw) <= 0 {
		return nil
	}
	var resetAt *time.Time
	if raw, ok := extra["codex_"+window+"_reset_at"]; ok && raw != nil {
		if t, err := parseTime(fmt.Sprint(raw)); err == nil {
			resetAt = &t
		}
	}
	return topologyQuota(parseExtraFloat64(usedRaw), resetAt, now)
}

func topologyQuota(used float64, resetAt *time.Time, now time.Time) *TopologyQuota {
	q := &TopologyQuota{UsedPercent: used}
	if resetAt != nil {
		v := *resetAt
		q.ResetAt = &v
		// 窗口已经重置：用量归零。
		if !now.Before(v) {
			q.UsedPercent = 0
		}
	}
	if q.UsedPercent < 0 {
		q.UsedPercent = 0
	}
	return q
}
