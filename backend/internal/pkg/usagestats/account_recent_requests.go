package usagestats

// AccountRecentAPIKeyStat 账号最近窗口内按 API Key 聚合的请求数与费用。
//
// cost 口径与账号列表「今日费用」一致：COALESCE(account_stats_cost, total_cost) × account_rate_multiplier。
type AccountRecentAPIKeyStat struct {
	APIKeyID int64   `json:"api_key_id"`
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	Cost     float64 `json:"cost"`
}

// AccountRecentModelStat 账号最近窗口内按展示模型（requested_model 回退 model）聚合的请求数与费用。
type AccountRecentModelStat struct {
	Model string  `json:"model"`
	Count int64   `json:"count"`
	Cost  float64 `json:"cost"`
}

// AccountRecentRequestBreakdown 账号最近窗口的整体聚合结果。
// TotalRequests 统计整个窗口，不受明细条数上限截断。
type AccountRecentRequestBreakdown struct {
	TotalRequests int64
	ByAPIKey      []AccountRecentAPIKeyStat
	ByModel       []AccountRecentModelStat
}
