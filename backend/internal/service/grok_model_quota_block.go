package service

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// Process-local per-account model soft-blocks for Grok free-usage that names a
// model (e.g. "used all free usage for model grok-4.5"). Sibling models on the
// same account stay schedulable. Multi-instance: each process learns from its
// own upstream errors.
type grokModelQuotaBlock struct {
	Until time.Time
}

type grokModelQuotaBlockStore struct {
	mu    sync.Mutex
	items map[string]grokModelQuotaBlock // key: accountID|model
}

var globalGrokModelQuotaBlocks = &grokModelQuotaBlockStore{
	items: make(map[string]grokModelQuotaBlock),
}

const (
	grokModelQuotaBlockDefaultTTL = 2 * time.Hour
	grokModelQuotaBlockMaxTTL     = 6 * time.Hour
	grokModelQuotaBlockMinTTL     = 20 * time.Minute
)

func grokModelQuotaBlockKey(accountID int64, model string) string {
	return strings.TrimSpace(strings.ToLower(model)) + "|" + strconv.FormatInt(accountID, 10)
}

// markGrokModelQuotaBlock soft-blocks accountID for model until the given time.
func markGrokModelQuotaBlock(accountID int64, model string, until time.Time) {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" || until.IsZero() {
		return
	}
	now := time.Now()
	if !until.After(now.Add(grokModelQuotaBlockMinTTL)) {
		until = now.Add(grokModelQuotaBlockDefaultTTL)
	}
	if max := now.Add(grokModelQuotaBlockMaxTTL); until.After(max) {
		until = max
	}
	storeGrokModelQuotaBlock(accountID, model, until, now)
}

// peekGrokModelQuotaBlocks 列出该账号当前仍生效的单模型软封锁（model -> 解除时间）。
// 严格只读：不清理过期项、不改任何计时器。isGrokModelQuotaBlocked 走的是调度路径，
// 诊断不能复用它——那条路径会顺手删过期项，把"看一眼"变成"改状态"。
func peekGrokModelQuotaBlocks(accountID int64, now time.Time) map[string]time.Time {
	if accountID <= 0 {
		return nil
	}
	suffix := "|" + strconv.FormatInt(accountID, 10)
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	var out map[string]time.Time
	for key, block := range globalGrokModelQuotaBlocks.items {
		if !strings.HasSuffix(key, suffix) || !now.Before(block.Until) {
			continue
		}
		if out == nil {
			out = make(map[string]time.Time, 1)
		}
		out[strings.TrimSuffix(key, suffix)] = block.Until
	}
	return out
}

const (
	grokModelTransientBlockMinTTL = 500 * time.Millisecond
	grokModelTransientBlockMaxTTL = 5 * time.Minute
)

// markGrokModelTransientBlock soft-blocks a single model for a short capacity
// burst without the free-usage 20m floor (and without unscheduling the account).
func markGrokModelTransientBlock(accountID int64, model string, until time.Time) {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" || until.IsZero() {
		return
	}
	now := time.Now()
	if !until.After(now.Add(grokModelTransientBlockMinTTL)) {
		until = now.Add(grokModelTransientBlockMinTTL)
	}
	if max := now.Add(grokModelTransientBlockMaxTTL); until.After(max) {
		until = max
	}
	storeGrokModelQuotaBlock(accountID, model, until, now)
}

func storeGrokModelQuotaBlock(accountID int64, model string, until, now time.Time) {
	key := grokModelQuotaBlockKey(accountID, model)
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	if cur, ok := globalGrokModelQuotaBlocks.items[key]; ok && cur.Until.After(until) {
		return
	}
	globalGrokModelQuotaBlocks.items[key] = grokModelQuotaBlock{Until: until}
	for k, v := range globalGrokModelQuotaBlocks.items {
		if !v.Until.After(now) {
			delete(globalGrokModelQuotaBlocks.items, k)
		}
	}
}

// isGrokModelQuotaBlocked reports whether this account cannot serve model now.
func isGrokModelQuotaBlocked(accountID int64, model string, now time.Time) bool {
	model = strings.TrimSpace(model)
	if accountID <= 0 || model == "" {
		return false
	}
	key := grokModelQuotaBlockKey(accountID, model)
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	cur, ok := globalGrokModelQuotaBlocks.items[key]
	if !ok {
		return false
	}
	if !cur.Until.After(now) {
		delete(globalGrokModelQuotaBlocks.items, key)
		return false
	}
	return true
}

// clearGrokModelQuotaBlocks 无条件删掉 Grok 单模型软封锁，accountIDs 为空表示全量。
// 返回真正删掉的条目数，含尚未被顺手清扫掉的过期条目。
//
// 键的形状是 "model|accountID"（见 grokModelQuotaBlockKey），所以按账号过滤是靠后缀
// "|<id>" 匹配的，和只读的 peekGrokModelQuotaBlocks 用的是同一套判据。
//
// 服务于运维逃生口：这张表是进程级全局变量，数据库里没有任何对应记录，后台页面看不见它，
// 也没有任何按钮能清它。2026-09-11 线上就出现过库里干干净净、账号却一个请求都不接的情况。
func clearGrokModelQuotaBlocks(accountIDs []int64) int {
	globalGrokModelQuotaBlocks.mu.Lock()
	defer globalGrokModelQuotaBlocks.mu.Unlock()
	if len(globalGrokModelQuotaBlocks.items) == 0 {
		return 0
	}
	if len(accountIDs) == 0 {
		cleared := len(globalGrokModelQuotaBlocks.items)
		globalGrokModelQuotaBlocks.items = make(map[string]grokModelQuotaBlock)
		return cleared
	}

	suffixes := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID > 0 {
			suffixes = append(suffixes, "|"+strconv.FormatInt(accountID, 10))
		}
	}
	cleared := 0
	for key := range globalGrokModelQuotaBlocks.items {
		for _, suffix := range suffixes {
			if !strings.HasSuffix(key, suffix) {
				continue
			}
			delete(globalGrokModelQuotaBlocks.items, key)
			cleared++
			break
		}
	}
	return cleared
}

func filterGrokModelQuotaBlockedAccounts(accounts []Account, model string, now time.Time) []Account {
	if len(accounts) == 0 || strings.TrimSpace(model) == "" {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		upstreamModel := canonicalOpenAIAccountSchedulingModel(&accounts[i], model)
		if isGrokModelQuotaBlocked(accounts[i].ID, upstreamModel, now) {
			continue
		}
		out = append(out, accounts[i])
	}
	return out
}

// isGrokModelSpecificFreeUsage is true when free-usage exhaustion is scoped to
// a named model (account may still serve other models).
func isGrokModelSpecificFreeUsage(low, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || low == "" {
		return false
	}
	if strings.Contains(low, "for model") || strings.Contains(low, "模型") {
		return true
	}
	// "used all the included free usage for model grok-4.5"
	if strings.Contains(low, "free usage") && strings.Contains(low, model) {
		return true
	}
	return false
}
