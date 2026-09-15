package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// API Key 当日用量阈值提醒。
//
// 和账号用量类指标一样是「一条规则 × N 个目标」，但拆分维度换成了 API Key：事件的
// dimensions 里放 api_key_id / user_id，评估器靠 opsAlertTargetKindAPIKey 区分这是密钥
// 目标还是账号目标，各自查活动事件、各自冷却、各自恢复。
//
// 取值口径固定为 EffectiveUsage1d() ÷ RateLimit1d × 100：
//   - 必须用 EffectiveUsage1d() 而不是裸的 Usage1d —— 1 天窗口过期后裸字段里留着的是上个
//     窗口的旧值，直接读会让告警拿着昨天的用量一直报，窗口滚动后也不会自动恢复；
//   - RateLimit1d <= 0 表示这把 key 压根没配日限额，百分比无从谈起，一律跳过（当 0 处理
//     会让 `>=` 型规则永不触发、当分母会除零）。
//
// 这一版密钥指标不支持按对象过滤（没有平台 / 分组 / 指定 key 的概念），规则一旦启用就对
// 全站配了日限额的启用中密钥做全量评估。
const OpsAlertMetricAPIKeyDailyUsedPercent = "apikey_daily_used_percent"

// IsOpsAlertAPIKeyMetric 判断指标是否为按 API Key 拆分评估的密钥用量类指标。
func IsOpsAlertAPIKeyMetric(metricType string) bool {
	return strings.TrimSpace(metricType) == OpsAlertMetricAPIKeyDailyUsedPercent
}

// opsAlertAPIKeyDailyLimitLister 评估器读「配了日限额的密钥」的最小口子。
//
// 故意不塞进 APIKeyRepository：那个接口有一堆实现与测试替身，加方法会牵动无关代码。
// 这里按仓库里已有的窄接口断言惯例（见 accountWindowStatsBatchReader）声明，注入进来的
// APIKeyRepository 实现了就用，没实现就当"无数据"跳过。
type opsAlertAPIKeyDailyLimitLister interface {
	ListAPIKeysWithDailyRateLimit(ctx context.Context) ([]*APIKey, error)
}

// apiKeyMetricSample 一把密钥在某条规则下的取值。
// Used / Limit 只用于推送正文里的「（425.00 / 500.00 USD）」补充说明，不参与比较。
type apiKeyMetricSample struct {
	APIKeyID   int64
	APIKeyName string
	UserID     int64
	UserName   string
	Value      float64
	Used       float64
	Limit      float64
}

// collectAPIKeyMetricSamples 取全站配了日限额的密钥并逐个算当日用量百分比。
// 返回的样本按密钥 ID 升序，保证同一轮内评估顺序稳定。
func (s *OpsAlertEvaluatorService) collectAPIKeyMetricSamples(ctx context.Context, rule *OpsAlertRule) ([]apiKeyMetricSample, error) {
	if s == nil || rule == nil {
		return nil, nil
	}
	if !IsOpsAlertAPIKeyMetric(rule.MetricType) {
		return nil, nil
	}
	if s.apiKeyRepo == nil {
		return nil, nil
	}
	lister, ok := s.apiKeyRepo.(opsAlertAPIKeyDailyLimitLister)
	if !ok {
		return nil, nil
	}
	keys, err := lister.ListAPIKeysWithDailyRateLimit(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}

	samples := make([]apiKeyMetricSample, 0, len(keys))
	for _, key := range keys {
		if key == nil || key.ID <= 0 {
			continue
		}
		percent, used, limit, ok := readAPIKeyDailyUsedPercent(key)
		if !ok {
			continue
		}
		sample := apiKeyMetricSample{
			APIKeyID:   key.ID,
			APIKeyName: strings.TrimSpace(key.Name),
			UserID:     key.UserID,
			Value:      percent,
			Used:       used,
			Limit:      limit,
		}
		if key.User != nil {
			sample.UserName = strings.TrimSpace(key.User.Username)
		}
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].APIKeyID < samples[j].APIKeyID })
	return samples, nil
}

// readAPIKeyDailyUsedPercent 当日已用金额 ÷ 日限额 × 100；没配日限额（<= 0）视为无数据。
// 用量走 EffectiveUsage1d()：窗口过期时它返回 0，新的一天从头开始算，告警随之自动恢复。
func readAPIKeyDailyUsedPercent(key *APIKey) (percent float64, used float64, limit float64, ok bool) {
	if key == nil || key.RateLimit1d <= 0 {
		return 0, 0, 0, false
	}
	used = key.EffectiveUsage1d()
	if used < 0 {
		used = 0
	}
	limit = key.RateLimit1d
	return used / limit * 100, used, limit, true
}

// ─── 文案 ───

// opsAlertAPIKeyMetricLabel 密钥用量类指标的人话名字，用于事件描述与推送正文。
func opsAlertAPIKeyMetricLabel(metricType string) string {
	if IsOpsAlertAPIKeyMetric(metricType) {
		return "API Key 当日用量"
	}
	return strings.TrimSpace(metricType)
}

// opsAlertAPIKeyMetricUnit 取值后缀：当日用量是百分比。
func opsAlertAPIKeyMetricUnit(metricType string) string {
	if IsOpsAlertAPIKeyMetric(metricType) {
		return "%"
	}
	return ""
}

// formatOpsAlertAPIKeyName 「用户名 / 密钥名」，缺哪个就退化成另一个，都缺时用「#id」。
func formatOpsAlertAPIKeyName(sample apiKeyMetricSample) string {
	name := strings.TrimSpace(sample.APIKeyName)
	if name == "" {
		name = fmt.Sprintf("#%d", sample.APIKeyID)
	}
	user := strings.TrimSpace(sample.UserName)
	if user == "" {
		return name
	}
	return user + " / " + name
}

// formatOpsAlertUSD 金额固定两位小数，让「425.00 / 500.00 USD」这类对照读起来齐整。
func formatOpsAlertUSD(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// formatOpsAlertAPIKeyLine 推送正文里的密钥行：「张三 / dev-key：85%（425.00 / 500.00 USD）」，
// 一眼看得出是谁的哪把 key、用掉多少、限额多少。
func formatOpsAlertAPIKeyLine(sample apiKeyMetricSample) string {
	return fmt.Sprintf("%s：%s%%（%s / %s USD）",
		formatOpsAlertAPIKeyName(sample),
		formatBarkNumber(sample.Value),
		formatOpsAlertUSD(sample.Used),
		formatOpsAlertUSD(sample.Limit),
	)
}

// buildOpsAlertAPIKeyDescription 事件描述（人话），例如：
// 「API Key 当日用量：张三 / dev-key 当前 85%，阈值 >= 80%」。
func buildOpsAlertAPIKeyDescription(rule *OpsAlertRule, sample apiKeyMetricSample) string {
	if rule == nil {
		return ""
	}
	unit := opsAlertAPIKeyMetricUnit(rule.MetricType)
	return fmt.Sprintf("%s：%s 当前 %s%s，阈值 %s %s%s",
		opsAlertAPIKeyMetricLabel(rule.MetricType),
		formatOpsAlertAPIKeyName(sample),
		formatBarkNumber(sample.Value), unit,
		strings.TrimSpace(rule.Operator),
		formatBarkNumber(rule.Threshold), unit,
	)
}

// buildOpsAlertAPIKeyDimensions 事件 dimensions：api_key_id 是按密钥查活动事件的键，
// 另外带上 key 名与所属用户，方便事件列表里不查库也能看懂是谁的哪把 key。
func buildOpsAlertAPIKeyDimensions(sample apiKeyMetricSample) map[string]any {
	dims := map[string]any{
		"api_key_id":   sample.APIKeyID,
		"api_key_name": sample.APIKeyName,
	}
	if sample.UserID > 0 {
		dims["user_id"] = sample.UserID
	}
	if sample.UserName != "" {
		dims["user_name"] = sample.UserName
	}
	return dims
}

// buildOpsAlertAPIKeyManualDetails 手动试发正文里的密钥行：最多 5 行，其余折成「另有 N 把密钥」。
func buildOpsAlertAPIKeyManualDetails(samples []apiKeyMetricSample, rule *OpsAlertRule) []string {
	const maxLines = 5
	if len(samples) == 0 || rule == nil {
		return nil
	}
	lines := make([]string, 0, maxLines+1)
	for i, sample := range samples {
		if i >= maxLines {
			lines = append(lines, fmt.Sprintf("另有 %d 把密钥", len(samples)-maxLines))
			break
		}
		line := formatOpsAlertAPIKeyLine(sample)
		if compareMetric(sample.Value, rule.Operator, rule.Threshold) {
			line += "（越阈）"
		}
		lines = append(lines, line)
	}
	return lines
}
