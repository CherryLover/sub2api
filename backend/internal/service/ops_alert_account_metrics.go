package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// 账号用量阈值提醒规则（批次 6 / A6-2 第二步）。
//
// 四个指标都是「一条规则 × N 个账号 = N 个评估目标」：每个账号各自越阈、各自恢复，
// 事件用 dimensions.account_id 区分。取值全部来自 accounts.extra 里已有的快照，
// 没有快照 / 没有 limit / 没有余额键的账号一律视为"无数据"跳过，绝不当 0 处理
// （否则 `<` 型规则会误触发）。

const (
	OpsAlertMetricAccountWindowUsedPercent = "account_window_used_percent"
	OpsAlertMetricAccountQuotaUsedPercent  = "account_quota_used_percent"
	OpsAlertMetricAccountBalance           = "account_balance"
	OpsAlertMetricAccountTodayCost         = "account_today_cost"

	opsAlertAccountWindow5h = "5h"
	opsAlertAccountWindow7d = "7d"

	opsAlertQuotaDimensionDaily  = "daily"
	opsAlertQuotaDimensionWeekly = "weekly"
	opsAlertQuotaDimensionTotal  = "total"

	// OpsAlertAccountIDsMax filters.account_ids 最多允许指定的账号数。
	OpsAlertAccountIDsMax = 200
)

// IsOpsAlertAccountMetric 判断指标是否为按账号拆分评估的账号用量类指标。
func IsOpsAlertAccountMetric(metricType string) bool {
	switch strings.TrimSpace(metricType) {
	case OpsAlertMetricAccountWindowUsedPercent,
		OpsAlertMetricAccountQuotaUsedPercent,
		OpsAlertMetricAccountBalance,
		OpsAlertMetricAccountTodayCost:
		return true
	default:
		return false
	}
}

// opsAlertAccountFilters 账号用量类规则的 filters 解析结果。
type opsAlertAccountFilters struct {
	Platform   string
	GroupID    *int64
	Window     string // 5h | 7d（account_window_used_percent）
	Dimension  string // daily | weekly | total（account_quota_used_percent）
	Provider   string // kimi | deepseek（account_balance）
	AccountIDs []int64
}

// effectivePlatform 余额指标指定了 provider 时按 provider 取账号，否则用 platform。
func (f opsAlertAccountFilters) effectivePlatform() string {
	if f.Provider != "" {
		return f.Provider
	}
	return f.Platform
}

func parseOpsAlertAccountFilters(filters map[string]any) opsAlertAccountFilters {
	platform, groupID, _ := parseOpsAlertRuleScope(filters)
	out := opsAlertAccountFilters{Platform: platform, GroupID: groupID}
	if filters == nil {
		return out
	}
	out.Window = strings.TrimSpace(stringValue(filters["window"]))
	out.Dimension = strings.TrimSpace(stringValue(filters["dimension"]))
	out.Provider = strings.TrimSpace(stringValue(filters["provider"]))
	out.AccountIDs = ParseOpsAlertAccountIDs(filters["account_ids"])
	return out
}

// ParseOpsAlertAccountIDs 兼容 JSON 解出来的 []any{float64}、手写的 []int64 / []int 等形态；
// 非正数与重复项丢弃，保持首次出现的顺序。
func ParseOpsAlertAccountIDs(raw any) []int64 {
	var candidates []int64
	appendID := func(id int64) {
		if id > 0 {
			candidates = append(candidates, id)
		}
	}
	switch v := raw.(type) {
	case nil:
		return nil
	case []int64:
		for _, id := range v {
			appendID(id)
		}
	case []int:
		for _, id := range v {
			appendID(int64(id))
		}
	case []float64:
		for _, id := range v {
			appendID(int64(id))
		}
	case []any:
		for _, item := range v {
			switch t := item.(type) {
			case float64:
				appendID(int64(t))
			case float32:
				appendID(int64(t))
			case int:
				appendID(int64(t))
			case int64:
				appendID(t)
			case json.Number:
				if n, err := t.Int64(); err == nil {
					appendID(n)
				}
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
					appendID(n)
				}
			}
		}
	default:
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(candidates))
	out := make([]int64, 0, len(candidates))
	for _, id := range candidates {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// accountMetricSample 一个账号在某条规则下的取值。
type accountMetricSample struct {
	AccountID   int64
	AccountName string
	Platform    string
	Value       float64
	// Currency 仅余额指标有值（<provider>_balance_currency）。
	Currency string
}

// collectAccountMetricSamples 按规则 filters 取账号并逐个读取指标；读不到的账号直接跳过。
// 返回的样本按账号 ID 升序，保证同一轮内评估顺序稳定。
func (s *OpsAlertEvaluatorService) collectAccountMetricSamples(ctx context.Context, rule *OpsAlertRule, now time.Time) ([]accountMetricSample, error) {
	if s == nil || s.opsService == nil || rule == nil {
		return nil, nil
	}
	filters := parseOpsAlertAccountFilters(rule.Filters)
	accounts, err := s.opsService.ListAccountsForAlerts(ctx, filters.effectivePlatform(), filters.GroupID, filters.AccountIDs)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}

	metricType := strings.TrimSpace(rule.MetricType)
	var todayCosts map[int64]float64
	if metricType == OpsAlertMetricAccountTodayCost {
		costs, ok := s.readAccountTodayCosts(ctx, accounts)
		if !ok {
			return nil, nil
		}
		todayCosts = costs
	}

	samples := make([]accountMetricSample, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		sample := accountMetricSample{
			AccountID:   account.ID,
			AccountName: strings.TrimSpace(account.Name),
			Platform:    strings.ToLower(strings.TrimSpace(account.Platform)),
		}
		var ok bool
		switch metricType {
		case OpsAlertMetricAccountWindowUsedPercent:
			sample.Value, ok = readAccountWindowUsedPercent(account, filters.Window, now)
		case OpsAlertMetricAccountQuotaUsedPercent:
			sample.Value, ok = readAccountQuotaUsedPercent(account, filters.Dimension)
		case OpsAlertMetricAccountBalance:
			sample.Value, sample.Currency, ok = readAccountBalance(account)
		case OpsAlertMetricAccountTodayCost:
			sample.Value, ok = todayCosts[account.ID]
		}
		if !ok {
			continue
		}
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].AccountID < samples[j].AccountID })
	return samples, nil
}

// readAccountWindowUsedPercent 读取账号滚动窗口用量百分比（0-100）。
//
// 复用调度阈值停调的读取器：openai 的 codex_<window>_used_percent 本身就是 0-100；
// anthropic 存的是 0-1 小数，必须经 utilizationAsPercent 换算；grok 只有一个滚动
// 配额窗口，挂在 5h 下；国产 kimi / zhipu 的 7d 对应 weekly 键；gemini / antigravity
// 等没有窗口快照的平台直接跳过。
//
// 三种情况分开处理：没有快照或快照已过期 → 无数据跳过（账号可能只是闲置，用量仍然高，
// 不能当 0）；窗口重置时间已过 → 新窗口用量记 0（窗口一重置告警就恢复）；其余用快照值。
func readAccountWindowUsedPercent(account *Account, window string, now time.Time) (float64, bool) {
	if account == nil {
		return 0, false
	}
	window = strings.TrimSpace(window)
	if window != opsAlertAccountWindow5h && window != opsAlertAccountWindow7d {
		return 0, false
	}

	var candidate *accountSchedulingThresholdCandidate
	switch strings.ToLower(strings.TrimSpace(account.Platform)) {
	case PlatformOpenAI:
		if !openAICodexSnapshotIdentityTrusted(account) {
			return 0, false
		}
		if _, ok := resolveAccountExtraNumber(account.Extra, "codex_"+window+"_used_percent"); !ok {
			return 0, false
		}
		if openAIQuotaWindowReset(account.Extra, window, now) {
			return 0, true
		}
		// 读取器内部还会做快照过期判断（codex_usage_updated_at 超 2 小时 → nil）。
		candidate = openAIThresholdCandidate(account.Extra, window, now)
	case PlatformAnthropic:
		candidate = anthropicWindowCandidate(account, window)
	case PlatformGrok:
		if window != opsAlertAccountWindow5h {
			return 0, false
		}
		if _, ok := resolveAccountExtraNumber(account.Extra, "grok_sched_utilization"); !ok {
			return 0, false
		}
		candidate = grokThresholdCandidates(account)[0]
	case PlatformKimi, PlatformZhipu:
		cnWindow := window
		if window == opsAlertAccountWindow7d {
			cnWindow = "weekly"
		}
		candidate = cnThresholdCandidate(account.Extra, strings.ToLower(strings.TrimSpace(account.Platform)), cnWindow)
	default:
		return 0, false
	}
	if candidate == nil {
		return 0, false
	}
	// 窗口已重置：快照里的用量是上个窗口的，新窗口从 0 开始（openai 已在上面单独处理）。
	if candidate.until != nil && !candidate.until.After(now) {
		return 0, true
	}
	return candidate.usedPercent, true
}

// anthropicWindowCandidate 读取 Claude 账号的 5h / 7d 利用率快照（0-1 小数，换算成百分比）。
// 键不存在或为 nil 视为无快照。
func anthropicWindowCandidate(account *Account, window string) *accountSchedulingThresholdCandidate {
	if account == nil || len(account.Extra) == 0 {
		return nil
	}
	var (
		key   string
		until *time.Time
	)
	switch window {
	case opsAlertAccountWindow5h:
		key = "session_window_utilization"
		until = cloneTimePtr(account.SessionWindowEnd)
	case opsAlertAccountWindow7d:
		key = "passive_usage_7d_utilization"
		until = parseSchedulingResetAt(account.Extra["passive_usage_7d_reset"])
	default:
		return nil
	}
	raw, ok := account.Extra[key]
	if !ok || raw == nil {
		return nil
	}
	return &accountSchedulingThresholdCandidate{
		window:      window,
		usedPercent: utilizationAsPercent(raw),
		until:       until,
	}
}

// readAccountQuotaUsedPercent 读取额度已用百分比 = used / limit × 100；limit ≤ 0（未启用）视为无数据。
func readAccountQuotaUsedPercent(account *Account, dimension string) (float64, bool) {
	if account == nil {
		return 0, false
	}
	var limit, used float64
	switch strings.TrimSpace(dimension) {
	case opsAlertQuotaDimensionDaily:
		limit, used = account.GetQuotaDailyLimit(), account.GetQuotaDailyUsed()
	case opsAlertQuotaDimensionWeekly:
		limit, used = account.GetQuotaWeeklyLimit(), account.GetQuotaWeeklyUsed()
	case opsAlertQuotaDimensionTotal:
		limit, used = account.GetQuotaLimit(), account.GetQuotaUsed()
	default:
		return 0, false
	}
	if limit <= 0 {
		return 0, false
	}
	if used < 0 {
		used = 0
	}
	return used / limit * 100, true
}

// readAccountBalance 读取国产 payg 账号的余额快照（<provider>_balance）；键不存在视为无数据。
func readAccountBalance(account *Account) (float64, string, bool) {
	if account == nil || len(account.Extra) == 0 {
		return 0, "", false
	}
	provider := strings.ToLower(strings.TrimSpace(account.Platform))
	if provider != PlatformKimi && provider != PlatformDeepseek {
		return 0, "", false
	}
	balance, ok := resolveAccountExtraNumber(account.Extra, cnExtraKey(provider, cnBalanceExtraSuffixBalance))
	if !ok {
		return 0, "", false
	}
	currency := strings.TrimSpace(stringValue(account.Extra[cnExtraKey(provider, cnBalanceExtraSuffixCurrency)]))
	return balance, currency, true
}

// readAccountTodayCosts 用 GetTodayStatsBatch 同一条路径（timezone.Today + 批量 SQL）读账号今日费用。
// 没有用量日志仓储或批量查询失败时整体视为无数据；今天没有请求的账号费用就是 0，不算无数据。
func (s *OpsAlertEvaluatorService) readAccountTodayCosts(ctx context.Context, accounts []*Account) (map[int64]float64, bool) {
	if s == nil || s.usageLogRepo == nil {
		return nil, false
	}
	reader, ok := s.usageLogRepo.(accountWindowStatsBatchReader)
	if !ok {
		return nil, false
	}
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil && account.ID > 0 {
			ids = append(ids, account.ID)
		}
	}
	if len(ids) == 0 {
		return map[int64]float64{}, true
	}
	statsByAccount, err := reader.GetAccountWindowStatsBatch(ctx, ids, timezone.Today())
	if err != nil {
		slog.Warn("ops_alert_account_today_cost_failed", "error", err)
		return nil, false
	}
	out := make(map[int64]float64, len(ids))
	for _, id := range ids {
		out[id] = windowStatsFromAccountStats(statsByAccount[id]).Cost
	}
	return out, true
}

// ─── 文案 ───

// opsAlertAccountMetricLabel 账号用量类指标的人话名字，用于事件描述与推送正文。
func opsAlertAccountMetricLabel(metricType string, filters opsAlertAccountFilters) string {
	switch strings.TrimSpace(metricType) {
	case OpsAlertMetricAccountWindowUsedPercent:
		if filters.Window == opsAlertAccountWindow7d {
			return "账号 7 天窗口用量"
		}
		return "账号 5 小时窗口用量"
	case OpsAlertMetricAccountQuotaUsedPercent:
		switch filters.Dimension {
		case opsAlertQuotaDimensionWeekly:
			return "账号周额度用量"
		case opsAlertQuotaDimensionTotal:
			return "账号总额度用量"
		default:
			return "账号日额度用量"
		}
	case OpsAlertMetricAccountBalance:
		return "账号余额"
	case OpsAlertMetricAccountTodayCost:
		return "账号今日费用"
	default:
		return strings.TrimSpace(metricType)
	}
}

// opsAlertAccountMetricUnit 取值后缀：百分比类是 "%"，余额跟币种，今日费用按美元计。
func opsAlertAccountMetricUnit(metricType string, currency string) string {
	switch strings.TrimSpace(metricType) {
	case OpsAlertMetricAccountWindowUsedPercent, OpsAlertMetricAccountQuotaUsedPercent:
		return "%"
	case OpsAlertMetricAccountBalance:
		if c := strings.TrimSpace(currency); c != "" {
			return " " + c
		}
		return ""
	case OpsAlertMetricAccountTodayCost:
		return " USD"
	default:
		return ""
	}
}

// formatOpsAlertAccountLine 推送正文里的账号行：「账号：<名>（openai）」。
func formatOpsAlertAccountLine(sample accountMetricSample) string {
	name := sample.AccountName
	if name == "" {
		name = fmt.Sprintf("#%d", sample.AccountID)
	}
	if sample.Platform == "" {
		return "账号：" + name
	}
	return fmt.Sprintf("账号：%s（%s）", name, sample.Platform)
}

// buildOpsAlertAccountDescription 事件描述（人话），例如：
// 「账号 5 小时窗口用量：账号 codex-01（openai）当前 85%，阈值 >= 80%」。
func buildOpsAlertAccountDescription(rule *OpsAlertRule, sample accountMetricSample, filters opsAlertAccountFilters) string {
	if rule == nil {
		return ""
	}
	unit := opsAlertAccountMetricUnit(rule.MetricType, sample.Currency)
	name := sample.AccountName
	if name == "" {
		name = fmt.Sprintf("#%d", sample.AccountID)
	}
	return fmt.Sprintf("%s：账号 %s（%s）当前 %s%s，阈值 %s %s%s",
		opsAlertAccountMetricLabel(rule.MetricType, filters),
		name,
		sample.Platform,
		formatBarkNumber(sample.Value), unit,
		strings.TrimSpace(rule.Operator),
		formatBarkNumber(rule.Threshold), unit,
	)
}

// buildOpsAlertAccountDimensions 事件 dimensions：账号身份 + 规则作用域，account_id 是按账号查活动事件的键。
func buildOpsAlertAccountDimensions(sample accountMetricSample, filters opsAlertAccountFilters) map[string]any {
	dims := map[string]any{
		"account_id":   sample.AccountID,
		"account_name": sample.AccountName,
	}
	if sample.Platform != "" {
		dims["platform"] = sample.Platform
	}
	if filters.GroupID != nil && *filters.GroupID > 0 {
		dims["group_id"] = *filters.GroupID
	}
	if filters.Window != "" {
		dims["window"] = filters.Window
	}
	if filters.Dimension != "" {
		dims["dimension"] = filters.Dimension
	}
	if filters.Provider != "" {
		dims["provider"] = filters.Provider
	}
	if sample.Currency != "" {
		dims["currency"] = sample.Currency
	}
	return dims
}
