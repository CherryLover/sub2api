package usagestats

import "strings"

// Key 排行榜支持的排序指标。放在 usagestats 里是为了让 repository 与 service
// 共用同一套字面量，避免两层各写一份字符串导致口径漂移。
const (
	// KeyRankingMetricCost 按实际扣费（actual_cost）排序。
	KeyRankingMetricCost = "cost"
	// KeyRankingMetricTokens 按 token 总数（输入+输出+缓存写+缓存读）排序。
	KeyRankingMetricTokens = "tokens"
	// KeyRankingMetricRequests 按请求次数排序。
	KeyRankingMetricRequests = "requests"
)

// NormalizeKeyRankingMetric 归一化排序指标，非法或缺省值一律回落到 cost。
// 免登录端点的入参不可信，这里 fail-safe 而不是报错，保证页面永远有数据可看。
func NormalizeKeyRankingMetric(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case KeyRankingMetricTokens:
		return KeyRankingMetricTokens
	case KeyRankingMetricRequests:
		return KeyRankingMetricRequests
	default:
		return KeyRankingMetricCost
	}
}
