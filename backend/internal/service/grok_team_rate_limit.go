package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// In-memory team+model rate-limit overlay for Grok OAuth. When xAI rate-limits
// one account in a team for a model, sibling accounts sharing team_id skip the
// same model until the cooldown expires (mirrors grok2api teamModelRateLimit).
//
// Process-local only: multi-instance deployments each learn the block from their
// own 429s. Prefer short TTLs so drift self-heals.
type grokTeamModelRateLimit struct {
	Until time.Time
}

type grokTeamModelRateLimitStore struct {
	mu    sync.Mutex
	items map[string]grokTeamModelRateLimit
}

var globalGrokTeamModelRateLimits = &grokTeamModelRateLimitStore{
	items: make(map[string]grokTeamModelRateLimit),
}

const (
	grokTeamRateLimitDefaultTTL = 10 * time.Minute
	grokTeamRateLimitMaxTTL     = time.Hour
	grokTeamRateLimitMinTTL     = 30 * time.Second
)

func grokTeamFingerprint(teamID string) string {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(teamID)))
	return hex.EncodeToString(sum[:8])
}

func grokTeamModelRateLimitKey(teamFingerprint, model string) string {
	return teamFingerprint + "|" + strings.ToLower(strings.TrimSpace(model))
}

func accountGrokTeamID(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetCredential("team_id"))
}

// peekGrokTeamModelRateLimits 列出该账号所在 team 当前仍生效的单模型限流
// （model -> 解除时间）。严格只读，不清理过期项。
//
// team 维度是跨账号连坐的：本账号自己一次都没失败，也可能因为同 team 的兄弟账号
// 撞了 429 而在这张表里。这正是它必须被展示出来的原因——页面上本账号毫无异常痕迹。
func peekGrokTeamModelRateLimits(account *Account, now time.Time) map[string]time.Time {
	fp := grokTeamFingerprint(accountGrokTeamID(account))
	if fp == "" {
		return nil
	}
	prefix := fp + "|"
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	var out map[string]time.Time
	for key, limit := range globalGrokTeamModelRateLimits.items {
		if !strings.HasPrefix(key, prefix) || !now.Before(limit.Until) {
			continue
		}
		if out == nil {
			out = make(map[string]time.Time, 1)
		}
		out[strings.TrimPrefix(key, prefix)] = limit.Until
	}
	return out
}

// markGrokTeamModelRateLimit records that this team+model pair should be skipped
// until until. No-op when team_id or model is empty.
func markGrokTeamModelRateLimit(account *Account, model string, until time.Time) {
	if account == nil || !account.IsGrokOAuth() {
		return
	}
	fp := grokTeamFingerprint(accountGrokTeamID(account))
	model = strings.TrimSpace(model)
	if fp == "" || model == "" || until.IsZero() {
		return
	}
	now := time.Now()
	if !until.After(now) {
		until = now.Add(grokTeamRateLimitDefaultTTL)
	}
	maxUntil := now.Add(grokTeamRateLimitMaxTTL)
	if until.After(maxUntil) {
		until = maxUntil
	}
	key := grokTeamModelRateLimitKey(fp, model)
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	if cur, ok := globalGrokTeamModelRateLimits.items[key]; ok && cur.Until.After(until) {
		return
	}
	globalGrokTeamModelRateLimits.items[key] = grokTeamModelRateLimit{Until: until}
	// Opportunistic prune of expired entries.
	for k, v := range globalGrokTeamModelRateLimits.items {
		if !v.Until.After(now) {
			delete(globalGrokTeamModelRateLimits.items, k)
		}
	}
}

// isGrokTeamModelRateLimited reports whether the account's team is currently
// blocked for the requested model.
func isGrokTeamModelRateLimited(account *Account, model string, now time.Time) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
	}
	fp := grokTeamFingerprint(accountGrokTeamID(account))
	model = strings.TrimSpace(model)
	if fp == "" || model == "" {
		return false
	}
	key := grokTeamModelRateLimitKey(fp, model)
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	cur, ok := globalGrokTeamModelRateLimits.items[key]
	if !ok {
		return false
	}
	if !cur.Until.After(now) {
		delete(globalGrokTeamModelRateLimits.items, key)
		return false
	}
	return true
}

// clearAllGrokTeamModelRateLimits 无条件清空 team×模型限流叠加表。
// 返回真正删掉的条目数，含尚未被顺手清扫掉的过期条目。
//
// 只有运维逃生口的**全量**模式会调它。键是 "teamFingerprint|model"，而 teamFingerprint
// 是 team_id 的 sha256 前缀——想从账号 ID 反推，得先去数据库把 team_id 读出来。这个逃生口
// 的前提正是"不信任、也不依赖数据库状态"，所以"指定账号"模式下这一类一律跳过，计数如实
// 返回 0，不假装清过。
//
// 这一类还额外隐蔽：它是跨账号连坐的，本账号一次都没失败也可能被同 team 的兄弟账号
// 撞的 429 关在外面，页面上本账号毫无异常痕迹（同 peekGrokTeamModelRateLimits 的理由）。
func clearAllGrokTeamModelRateLimits() int {
	globalGrokTeamModelRateLimits.mu.Lock()
	defer globalGrokTeamModelRateLimits.mu.Unlock()
	cleared := len(globalGrokTeamModelRateLimits.items)
	if cleared == 0 {
		return 0
	}
	globalGrokTeamModelRateLimits.items = make(map[string]grokTeamModelRateLimit)
	return cleared
}

// filterGrokTeamModelRateLimitedAccounts drops candidates whose team is under a
// model-scoped rate-limit cool. Accounts without team_id pass through.
func filterGrokTeamModelRateLimitedAccounts(accounts []Account, model string, now time.Time) []Account {
	if len(accounts) == 0 || strings.TrimSpace(model) == "" {
		return accounts
	}
	out := accounts[:0]
	kept := false
	for i := range accounts {
		upstreamModel := canonicalOpenAIAccountSchedulingModel(&accounts[i], model)
		if isGrokTeamModelRateLimited(&accounts[i], upstreamModel, now) {
			continue
		}
		out = append(out, accounts[i])
		kept = true
	}
	if !kept && len(out) == 0 {
		// All filtered — return empty (caller treats as no capacity).
		return nil
	}
	return out
}

// resolveGrokTeamRateLimitUntil derives a team cool window from an observed
// account rate-limit reset, with sane clamps.
func resolveGrokTeamRateLimitUntil(resetAt, now time.Time) time.Time {
	if resetAt.After(now.Add(grokTeamRateLimitMinTTL)) {
		maxUntil := now.Add(grokTeamRateLimitMaxTTL)
		if resetAt.After(maxUntil) {
			return maxUntil
		}
		return resetAt
	}
	return now.Add(grokTeamRateLimitDefaultTTL)
}
