package service

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// openAIModelTransientStreakTTL bounds how long a failure streak survives
	// without a new failure. It exists only so the map does not keep state for
	// account+model pairs that stopped being used; a streak is otherwise reset
	// by recordSuccess alone.
	//
	// It must stay well above the cooldowns. Resetting the streak on a short
	// wall-clock window makes the breaker's sensitivity depend on request rate:
	// a gateway called less often than the window never reaches streak 2, so a
	// broken upstream is never cooled down and every request pays a failed
	// attempt plus a failover before reaching a healthy account. Low-traffic
	// deployments were hit hardest, which is the opposite of what a breaker
	// should do.
	openAIModelTransientStreakTTL     = 30 * time.Minute
	openAIModelTransientShortCooldown = 10 * time.Second
	openAIModelTransientLongCooldown  = 45 * time.Second
	openAIModelTransientDefaultMax    = 4096
	openAIModelTransientMaxModelBytes = 512
)

type openAIAccountModelKey struct {
	AccountID int64
	Model     string
}

type openAIAccountModelTransientEntry struct {
	failureStreak int
	lastFailure   time.Time
	blockUntil    time.Time
	lastTouched   time.Time
}

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
}

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
	}
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		maxEntries: maxEntries,
	}
}

func normalizeOpenAIAccountModelTransientModel(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > openAIModelTransientMaxModelBytes {
		return ""
	}
	return strings.ToLower(model)
}

func openAIAccountModelTransientKey(accountID int64, model string) (openAIAccountModelKey, bool) {
	model = normalizeOpenAIAccountModelTransientModel(model)
	if accountID <= 0 || model == "" {
		return openAIAccountModelKey{}, false
	}
	return openAIAccountModelKey{AccountID: accountID, Model: model}, true
}

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}

	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
	}
	// The streak is cleared by recordSuccess. Only drop it here when the entry
	// is stale beyond the TTL, or when the clock moved backwards.
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
	}
	entry.failureStreak++
	entry.lastFailure = now
	entry.lastTouched = now

	cooldown := time.Duration(0)
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
	}
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
	} else {
		entry.blockUntil = time.Time{}
	}
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockUntil,
	}
}

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

func (s *openAIAccountModelTransientState) isBlocked(accountID int64, model string, now time.Time) bool {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL {
		delete(s.entries, key)
		return false
	}
	entry.lastTouched = now
	s.entries[key] = entry
	return !entry.blockUntil.IsZero() && now.Before(entry.blockUntil)
}

// openAIAccountModelTransientBlock is a read-only view of an active
// account+model cooldown, used by scheduling diagnostics.
type openAIAccountModelTransientBlock struct {
	model      string
	blockUntil time.Time
}

// activeBlocks lists, per account, the models whose cooldown is still active
// at now (blockUntil in the future and the streak not past
// openAIModelTransientStreakTTL). Unlike isBlocked it never deletes stale
// entries and never refreshes lastTouched, so calling it cannot change what
// the scheduler sees or which entry the LRU evicts next. Models are sorted.
func (s *openAIAccountModelTransientState) activeBlocks(accountIDs []int64, now time.Time) map[int64][]openAIAccountModelTransientBlock {
	if s == nil || len(accountIDs) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	wanted := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id > 0 {
			wanted[id] = struct{}{}
		}
	}

	out := make(map[int64][]openAIAccountModelTransientBlock)
	s.mu.Lock()
	for key, entry := range s.entries {
		if _, ok := wanted[key.AccountID]; !ok {
			continue
		}
		if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL {
			continue
		}
		if entry.blockUntil.IsZero() || !now.Before(entry.blockUntil) {
			continue
		}
		out[key.AccountID] = append(out[key.AccountID], openAIAccountModelTransientBlock{
			model:      key.Model,
			blockUntil: entry.blockUntil,
		})
	}
	s.mu.Unlock()

	for _, blocks := range out {
		sort.Slice(blocks, func(i, j int) bool { return blocks[i].model < blocks[j].model })
	}
	return out
}

// clearBlocks 无条件删掉账号×模型的瞬时冷却条目，返回真正删掉的条目数。
// accountIDs 为空表示全量，否则只删这些账号名下的（键里带 AccountID，能精确过滤）。
//
// 服务于运维逃生口：这些冷却只存在于进程内存里，数据库上看不见也改不动，账号页显示一切
// 正常但请求就是进不来时（2026-09-11 线上账号 5、6），必须能从服务自己身上一键抹掉。
// 走 s.mu，和 recordFailure / isBlocked / activeBlocks 互斥，并发下不会撕裂。
func (s *openAIAccountModelTransientState) clearBlocks(accountIDs []int64) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return 0
	}
	if len(accountIDs) == 0 {
		cleared := len(s.entries)
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
		return cleared
	}

	wanted := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID > 0 {
			wanted[accountID] = struct{}{}
		}
	}
	cleared := 0
	for key := range s.entries {
		if _, ok := wanted[key.AccountID]; !ok {
			continue
		}
		delete(s.entries, key)
		cleared++
	}
	return cleared
}

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if !found || entry.lastTouched.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastTouched
			found = true
		}
	}
	if found {
		delete(s.entries, oldestKey)
	}
}
