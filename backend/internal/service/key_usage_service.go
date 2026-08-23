package service

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultKeyUsageTokenTTL 用量页 URL 令牌默认有效期（30 天），可由配置覆盖。
	DefaultKeyUsageTokenTTL = 30 * 24 * time.Hour
	// DefaultKeyUsageSiteRankingCacheTTL 全站排行榜默认缓存时长，可由配置覆盖。
	DefaultKeyUsageSiteRankingCacheTTL = 120 * time.Second
	// keyUsageSiteRankingLoadTimeout 单次全站聚合的硬超时。
	// 全站榜走 singleflight，加载用的 ctx 与首个调用方的请求 ctx 解绑（见 buildSiteRanking），
	// 所以必须自带超时，否则一条卡住的聚合会把这个 cache key 上的所有等待者一起挂住。
	keyUsageSiteRankingLoadTimeout = 30 * time.Second
	// keyUsageSiteCacheMaxEntries 全站榜缓存的条目上限（兜底，正常只有 3 metric × 3 window = 9 条）。
	keyUsageSiteCacheMaxEntries = 32
	// KeyUsageTopN 排行榜返回的名次数量上限（金银铜取前三）。
	KeyUsageTopN = 10
	// keyUsageUnknownKeyName Key 记录已被硬删除时的占位名称。
	keyUsageUnknownKeyName = "unknown"

	// 窗口标识，与前端契约一致。
	KeyUsageWindowToday   = "today"
	KeyUsageWindowLast7d  = "last_7d"
	KeyUsageWindowLast30d = "last_30d"
)

// ErrKeyUsageUnauthorized 是免登录用量页所有鉴权失败的唯一出口。
// key 不存在、key 被禁用、令牌过期、令牌被篡改都返回它：任何差异化的错误都会把
// 这个端点变成"批量验证 API Key 是否存在"的探测器。
var ErrKeyUsageUnauthorized = infraerrors.Unauthorized("KEY_USAGE_UNAUTHORIZED", "Invalid or expired key")

// keyUsageAPIKeyResolver 是本服务需要的最小 API Key 查询能力（由 *APIKeyService 实现）。
type keyUsageAPIKeyResolver interface {
	GetByKey(ctx context.Context, key string) (*APIKey, error)
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

// keyUsageModelStatsProvider 复用既有的按模型统计口径（由 *UsageService 实现），
// 保证页面上"窗口汇总"和已有的 model_stats 永远来自同一条 SQL 口径。
type keyUsageModelStatsProvider interface {
	GetAPIKeyModelStats(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]usagestats.ModelStat, error)
}

// KeyUsageRankingRepository 排行榜聚合所需的仓储能力（由 usageLogRepository 实现）。
type KeyUsageRankingRepository interface {
	GetAPIKeyUsageAggregates(ctx context.Context, startTime, endTime time.Time, userID int64, metric string) ([]usagestats.APIKeyUsageAggregate, error)
	GetAPIKeyNamesByIDs(ctx context.Context, ids []int64) (map[int64]string, error)
}

// KeyUsageModelStat 单个模型在窗口内的用量。
type KeyUsageModelStat struct {
	Model    string  `json:"model"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

// KeyUsageWindowStat 单个时间窗口内本 Key 的用量汇总。
type KeyUsageWindowStat struct {
	Requests int64               `json:"requests"`
	Tokens   int64               `json:"tokens"`
	CostUSD  float64             `json:"cost_usd"`
	Models   []KeyUsageModelStat `json:"models"`
}

// KeyUsageWindows 三个固定窗口的用量汇总。用结构体而不是 map，保证三个字段恒存在。
type KeyUsageWindows struct {
	Today   KeyUsageWindowStat `json:"today"`
	Last7d  KeyUsageWindowStat `json:"last_7d"`
	Last30d KeyUsageWindowStat `json:"last_30d"`
}

// KeyUsageRankEntry 排行榜中的一行。
type KeyUsageRankEntry struct {
	// APIKeyID 仅用于服务端标记 is_self，不下发（避免把可枚举的自增 ID 暴露给前端）。
	APIKeyID int64   `json:"-"`
	Rank     int     `json:"rank"`
	KeyName  string  `json:"key_name"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
	IsSelf   bool    `json:"is_self"`
}

// KeyUsageRankingWindow 单个窗口的排名结果。
type KeyUsageRankingWindow struct {
	TotalKeys int                 `json:"total_keys"`
	SelfRank  int                 `json:"self_rank"`
	Top       []KeyUsageRankEntry `json:"top"`
	Self      KeyUsageRankEntry   `json:"self"`
}

// KeyUsageRankingScope 一个维度（账户内 / 全站）下三个窗口的排名。
type KeyUsageRankingScope struct {
	Today   KeyUsageRankingWindow `json:"today"`
	Last7d  KeyUsageRankingWindow `json:"last_7d"`
	Last30d KeyUsageRankingWindow `json:"last_30d"`
}

// KeyUsageRankings 两个排名维度。
type KeyUsageRankings struct {
	Account KeyUsageRankingScope `json:"account"`
	Site    KeyUsageRankingScope `json:"site"`
}

// KeyUsageKeyInfo 页面顶部展示的 Key 元信息。
// CreatedAt 用指针：缺值时序列化为 null，而不是 0001-01-01 这种前端没法渲染的零时间。
type KeyUsageKeyInfo struct {
	Name      string     `json:"name"`
	CreatedAt *time.Time `json:"created_at"`
	Status    string     `json:"status"`
}

// KeyUsageReportData 是 /api/v1/key-usage/report 中除 usage（复用 /v1/usage 原样 payload）
// 之外的全部内容，由 handler 负责拼装最终响应。
type KeyUsageReportData struct {
	Key      KeyUsageKeyInfo  `json:"key"`
	Windows  KeyUsageWindows  `json:"windows"`
	Rankings KeyUsageRankings `json:"rankings"`
	Metric   string           `json:"metric"`
}

// keyUsageWindow 一个自然日对齐的时间窗口 [Start, End)。
type keyUsageWindow struct {
	Name  string
	Start time.Time
	End   time.Time
}

// KeyUsageService 免登录用量页的业务逻辑：令牌换取、窗口汇总、两个维度的排名。
type KeyUsageService struct {
	apiKeys    keyUsageAPIKeyResolver
	modelStats keyUsageModelStatsProvider
	ranking    KeyUsageRankingRepository
	tokens     *KeyUsageTokenService
	siteCache  *keyUsageSiteCache
}

// NewKeyUsageService 创建服务。ranking / modelStats 允许为 nil（缺依赖时对应板块返回零值），
// 保证这个只读页面不会因为某个可选依赖缺失而整体不可用。
func NewKeyUsageService(
	apiKeys keyUsageAPIKeyResolver,
	modelStats keyUsageModelStatsProvider,
	ranking KeyUsageRankingRepository,
	tokens *KeyUsageTokenService,
	siteCacheTTL time.Duration,
) *KeyUsageService {
	if siteCacheTTL <= 0 {
		siteCacheTTL = DefaultKeyUsageSiteRankingCacheTTL
	}
	return &KeyUsageService{
		apiKeys:    apiKeys,
		modelStats: modelStats,
		ranking:    ranking,
		tokens:     tokens,
		siteCache:  newKeyUsageSiteCache(siteCacheTTL),
	}
}

// TokenTTL 返回令牌有效期。
func (s *KeyUsageService) TokenTTL() time.Duration {
	return s.tokens.TTL()
}

// IssueToken 用原始 API Key 换取只读用量令牌。
// clientIP 参与 Key 的 IP 白名单/黑名单校验，口径与网关中间件一致。
func (s *KeyUsageService) IssueToken(ctx context.Context, rawKey, clientIP string) (string, time.Time, error) {
	apiKey, err := s.ResolveRawKey(ctx, rawKey, clientIP)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.tokens.Issue(apiKey.ID, apiKey.Key, timezone.Now())
}

// ResolveRawKey 校验原始 API Key（Bearer 直连路径）。
func (s *KeyUsageService) ResolveRawKey(ctx context.Context, rawKey, clientIP string) (*APIKey, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" || s.apiKeys == nil {
		return nil, ErrKeyUsageUnauthorized
	}
	apiKey, err := s.apiKeys.GetByKey(ctx, rawKey)
	if err != nil || apiKey == nil {
		return nil, ErrKeyUsageUnauthorized
	}
	if !keyUsageViewable(apiKey) {
		return nil, ErrKeyUsageUnauthorized
	}
	if !keyUsageAllowedFromIP(apiKey, clientIP) {
		return nil, ErrKeyUsageUnauthorized
	}
	return apiKey, nil
}

// ResolveToken 校验令牌并回到当前的 Key 记录。
//
// 令牌是无状态签名，所以"失效"必须在解析时重新体检：
//  1. 签名/类型/过期 —— KeyUsageTokenService.Parse；
//  2. Key 是否还存在（软删除后 GetByID 直接查不到）；
//  3. Key 当前状态是否仍可查看（禁用即刻失效）；
//  4. Key 明文是否还是签发时那一把（指纹比对，key 轮换后旧令牌失效）；
//  5. 当前来源 IP 是否仍被这把 Key 的 IP 白名单/黑名单允许 —— 令牌不能变成一张
//     绕开 IP ACL 的长期旁路凭证。
func (s *KeyUsageService) ResolveToken(ctx context.Context, token, clientIP string) (*APIKey, error) {
	if s.apiKeys == nil {
		return nil, ErrKeyUsageUnauthorized
	}
	claims, err := s.tokens.Parse(token, timezone.Now())
	if err != nil {
		return nil, ErrKeyUsageUnauthorized
	}
	apiKey, err := s.apiKeys.GetByID(ctx, claims.APIKeyID)
	if err != nil || apiKey == nil {
		return nil, ErrKeyUsageUnauthorized
	}
	if !keyUsageViewable(apiKey) {
		return nil, ErrKeyUsageUnauthorized
	}
	fingerprint := s.tokens.Fingerprint(apiKey.Key)
	if fingerprint == "" || !hmac.Equal([]byte(fingerprint), []byte(claims.Fingerprint)) {
		return nil, ErrKeyUsageUnauthorized
	}
	if !keyUsageAllowedFromIP(apiKey, clientIP) {
		return nil, ErrKeyUsageUnauthorized
	}
	return apiKey, nil
}

// keyUsageAllowedFromIP 复用网关 API Key 中间件的 IP 白名单/黑名单口径
// （ip.CheckIPRestrictionWithCompiledRules）。
//
// 免登录用量页不经过 API Key 中间件，如果这里不查，一把配了 IP 白名单的 Key 一旦泄露，
// 攻击者就能从任意 IP 读到余额、额度、套餐、逐日曲线与模型统计——站长明确配置过的
// 访问控制被静默绕过。原始 Key 路径与令牌路径都要查。
//
// 预编译规则缺失时就地编译：宁可多花一次编译，也不能因为调用方忘了编译而 fail-open。
func keyUsageAllowedFromIP(apiKey *APIKey, clientIP string) bool {
	if apiKey == nil {
		return false
	}
	if len(apiKey.IPWhitelist) == 0 && len(apiKey.IPBlacklist) == 0 {
		return true
	}
	whitelist := apiKey.CompiledIPWhitelist
	if whitelist == nil && len(apiKey.IPWhitelist) > 0 {
		whitelist = ip.CompileIPRules(apiKey.IPWhitelist)
	}
	blacklist := apiKey.CompiledIPBlacklist
	if blacklist == nil && len(apiKey.IPBlacklist) > 0 {
		blacklist = ip.CompileIPRules(apiKey.IPBlacklist)
	}
	allowed, _ := ip.CheckIPRestrictionWithCompiledRules(clientIP, whitelist, blacklist)
	return allowed
}

// keyUsageViewable 判断该 Key 当前是否允许查看用量。
// 与 /v1/usage 的 isValid 口径保持一致：额度耗尽/已过期仍然能看自己的用量，
// 只有被管理员禁用（或已删除）才彻底关闭入口；
// owner 被封禁时同样关闭（纵深防御：封号后不该还能靠旧令牌看数据）。
func keyUsageViewable(apiKey *APIKey) bool {
	if apiKey.User != nil && !apiKey.User.IsActive() {
		return false
	}
	switch apiKey.Status {
	case StatusAPIKeyActive, StatusAPIKeyQuotaExhausted, StatusAPIKeyExpired:
		return true
	default:
		return false
	}
}

// BuildReport 组装窗口汇总 + 两个维度的排名。
//
// userTZ 是前端传来的浏览器时区（与 /v1/usage 的 timezone query param 同一个值）：
// 窗口边界必须跟着它切，否则 UTC+8 的访客在 UTC 服务器上会看到"今日汇总"与按天曲线
// 最后一行差 8 小时。空串表示回落到服务端全局时区。
func (s *KeyUsageService) BuildReport(ctx context.Context, apiKey *APIKey, metric, userTZ string) *KeyUsageReportData {
	metric = usagestats.NormalizeKeyRankingMetric(metric)
	windows := keyUsageWindows(timezone.NowInUserLocation(userTZ), userTZ)

	keyInfo := KeyUsageKeyInfo{Name: apiKey.Name, Status: apiKey.Status}
	if !apiKey.CreatedAt.IsZero() {
		createdAt := apiKey.CreatedAt
		keyInfo.CreatedAt = &createdAt
	}
	report := &KeyUsageReportData{Key: keyInfo, Metric: metric}

	stats := make([]KeyUsageWindowStat, len(windows))
	for i, window := range windows {
		stats[i] = s.buildWindowStat(ctx, apiKey.ID, window)
	}
	report.Windows = KeyUsageWindows{Today: stats[0], Last7d: stats[1], Last30d: stats[2]}

	accountRanks := make([]KeyUsageRankingWindow, len(windows))
	siteRanks := make([]KeyUsageRankingWindow, len(windows))
	for i, window := range windows {
		accountRanks[i] = s.buildAccountRanking(ctx, apiKey, metric, window, stats[i])
		siteRanks[i] = s.buildSiteRanking(ctx, apiKey, metric, window, stats[i])
	}
	report.Rankings = KeyUsageRankings{
		Account: KeyUsageRankingScope{Today: accountRanks[0], Last7d: accountRanks[1], Last30d: accountRanks[2]},
		Site:    KeyUsageRankingScope{Today: siteRanks[0], Last7d: siteRanks[1], Last30d: siteRanks[2]},
	}
	return report
}

// keyUsageWindows 返回 today / last_7d / last_30d 三个窗口。
//
// 全部按 userTZ 的自然日边界切分（与 buildAPIKeyDailyUsage 用的 apiKeyDailyUsageRange
// 完全同源：起点 = StartOfDayInUserLocation(now-(days-1))，终点 = 明天 00:00），
// 这样窗口汇总和页面上的按天曲线能对得上；不使用 now-7*24h 这种滚动窗口。
// userTZ 为空时 timezone.* 会回落到服务端全局时区。
func keyUsageWindows(now time.Time, userTZ string) []keyUsageWindow {
	end := timezone.StartOfDayInUserLocation(now.AddDate(0, 0, 1), userTZ)
	return []keyUsageWindow{
		{Name: KeyUsageWindowToday, Start: timezone.StartOfDayInUserLocation(now, userTZ), End: end},
		{Name: KeyUsageWindowLast7d, Start: timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -6), userTZ), End: end},
		{Name: KeyUsageWindowLast30d, Start: timezone.StartOfDayInUserLocation(now.AddDate(0, 0, -29), userTZ), End: end},
	}
}

// KeyUsageWindowRanges 返回三个窗口的时间边界（窗口名 → [start, end)），
// 供测试与线上排查核对口径：它必须与 handler 里按天聚合用的 apiKeyDailyUsageRange
// 在同一个 userTZ 下完全一致。
func KeyUsageWindowRanges(now time.Time, userTZ string) map[string][2]time.Time {
	ranges := make(map[string][2]time.Time, 3)
	for _, window := range keyUsageWindows(now, userTZ) {
		ranges[window.Name] = [2]time.Time{window.Start, window.End}
	}
	return ranges
}

// buildWindowStat 复用既有的 GetAPIKeyModelStats 口径：窗口总量直接由各模型行相加得到，
// 避免再写一条"总量 SQL"导致同一页面上两个数字对不上。查询走 (api_key_id, created_at) 索引。
func (s *KeyUsageService) buildWindowStat(ctx context.Context, apiKeyID int64, window keyUsageWindow) KeyUsageWindowStat {
	stat := KeyUsageWindowStat{Models: []KeyUsageModelStat{}}
	if s.modelStats == nil {
		return stat
	}
	rows, err := s.modelStats.GetAPIKeyModelStats(ctx, apiKeyID, window.Start, window.End)
	if err != nil {
		// 尽力而为：单个窗口查询失败不影响页面其余部分，返回零值。
		slog.Warn("key usage window stats failed", "window", window.Name, "api_key_id", apiKeyID, "error", err)
		return stat
	}
	for _, row := range rows {
		stat.Requests += row.Requests
		stat.Tokens += row.TotalTokens
		stat.CostUSD += row.ActualCost
		stat.Models = append(stat.Models, KeyUsageModelStat{
			Model:    row.Model,
			Requests: row.Requests,
			Tokens:   row.TotalTokens,
			CostUSD:  row.ActualCost,
		})
	}
	return stat
}

// buildAccountRanking 账户内排名：数据量小（一个账户的 Key 数量有限），每次实时查，不缓存。
func (s *KeyUsageService) buildAccountRanking(ctx context.Context, apiKey *APIKey, metric string, window keyUsageWindow, self KeyUsageWindowStat) KeyUsageRankingWindow {
	if s.ranking == nil {
		return emptyKeyUsageRanking(apiKey, self)
	}
	rows, err := s.ranking.GetAPIKeyUsageAggregates(ctx, window.Start, window.End, apiKey.UserID, metric)
	if err != nil {
		slog.Warn("key usage account ranking failed", "window", window.Name, "user_id", apiKey.UserID, "error", err)
		return emptyKeyUsageRanking(apiKey, self)
	}
	values := keyUsageMetricValues(rows, metric)
	selfValue, selfFound := keyUsageSelfValueByID(keyUsageAggregateIDs(rows), values, apiKey.ID)
	top := s.resolveTopEntries(ctx, rows, values, apiKey.ID)
	return assembleKeyUsageRanking(top, values, len(rows), apiKey, self, metric, selfValue, selfFound)
}

// buildSiteRanking 全站排名：与具体 Key 无关，全站共用一份缓存（key = metric + 窗口边界）。
// 免登录端点必须挡住"任何人都能触发一次全表 30 天聚合"这条路径。
func (s *KeyUsageService) buildSiteRanking(ctx context.Context, apiKey *APIKey, metric string, window keyUsageWindow, self KeyUsageWindowStat) KeyUsageRankingWindow {
	if s.ranking == nil {
		return emptyKeyUsageRanking(apiKey, self)
	}
	// singleflight 会让所有等待者共享首个调用方的加载过程：如果直接用请求 ctx，
	// 首个请求断连（浏览器刷新/超时）会把所有等待者一起打成 context.Canceled，
	// 整块排名退化。这里与请求生命周期解绑，改用自带超时的 ctx。
	loadCtx := context.WithoutCancel(ctx)
	snapshot, err := s.siteCache.GetOrLoad(keyUsageSiteCacheKey(metric, window), func() (*keyUsageSiteSnapshot, error) {
		queryCtx, cancel := context.WithTimeout(loadCtx, keyUsageSiteRankingLoadTimeout)
		defer cancel()
		rows, loadErr := s.ranking.GetAPIKeyUsageAggregates(queryCtx, window.Start, window.End, 0, metric)
		if loadErr != nil {
			return nil, loadErr
		}
		values := keyUsageMetricValues(rows, metric)
		return &keyUsageSiteSnapshot{
			Top:       s.resolveTopEntries(queryCtx, rows, values, 0),
			IDs:       keyUsageAggregateIDs(rows),
			Values:    values,
			TotalKeys: len(rows),
		}, nil
	})
	if err != nil || snapshot == nil {
		slog.Warn("key usage site ranking failed", "window", window.Name, "error", err)
		return emptyKeyUsageRanking(apiKey, self)
	}

	// 快照里的 is_self 是按"无自身视角"生成的（全站共用），这里按当前 Key 重新打标记。
	top := make([]KeyUsageRankEntry, len(snapshot.Top))
	copy(top, snapshot.Top)
	for i := range top {
		top[i].IsSelf = top[i].APIKeyID == apiKey.ID
	}
	selfValue, selfFound := keyUsageSelfValueByID(snapshot.IDs, snapshot.Values, apiKey.ID)
	return assembleKeyUsageRanking(top, snapshot.Values, snapshot.TotalKeys, apiKey, self, metric, selfValue, selfFound)
}

// resolveTopEntries 取前 KeyUsageTopN 行并批量补齐 Key 名称。
func (s *KeyUsageService) resolveTopEntries(ctx context.Context, rows []usagestats.APIKeyUsageAggregate, values []float64, selfID int64) []KeyUsageRankEntry {
	limit := min(len(rows), KeyUsageTopN)
	entries := make([]KeyUsageRankEntry, 0, limit)
	if limit == 0 {
		return entries
	}

	ids := make([]int64, 0, limit)
	for _, row := range rows[:limit] {
		ids = append(ids, row.APIKeyID)
	}
	names, err := s.ranking.GetAPIKeyNamesByIDs(ctx, ids)
	if err != nil {
		slog.Warn("key usage ranking name lookup failed", "error", err)
		names = map[int64]string{}
	}

	for i, row := range rows[:limit] {
		name := strings.TrimSpace(names[row.APIKeyID])
		if name == "" {
			name = keyUsageUnknownKeyName
		}
		entries = append(entries, KeyUsageRankEntry{
			APIKeyID: row.APIKeyID,
			Rank:     keyUsageRankForValue(values, values[i]),
			KeyName:  name,
			Requests: row.Requests,
			Tokens:   row.Tokens,
			CostUSD:  row.Cost,
			IsSelf:   selfID > 0 && row.APIKeyID == selfID,
		})
	}
	return entries
}

// assembleKeyUsageRanking 把 top 列表与"自己"的名次拼成最终结果。
//
// 名次策略：标准竞赛排名（1224）—— 名次 = 指标严格大于自己的 Key 数 + 1，
// 并列的 Key 共享同一名次，其后名次跳号。展示顺序在并列时按 api_key_id 升序（SQL 已保证）。
//
// selfValue 必须来自榜单同一份聚合结果（selfFound == true 时由调用方按 api_key_id 取出），
// 不能在 Go 里重新把各模型行的 cost 累加一遍：浮点求和顺序不同会让末位比特不同，
// keyUsageRankForValue 里的 `values[i] <= value` 比较就会把自己算进"严格大于自己"，
// 名次凭空 +1（能复现出"金牌是自己、同时显示第 2 名 / 共 2 个"这种自相矛盾的渲染）。
// 只有在聚合结果里确实没有自己这一行时，才回落到实时窗口值定位名次。
func assembleKeyUsageRanking(
	top []KeyUsageRankEntry,
	values []float64,
	totalKeys int,
	apiKey *APIKey,
	self KeyUsageWindowStat,
	metric string,
	selfValue float64,
	selfFound bool,
) KeyUsageRankingWindow {
	if !selfFound {
		// 本 Key 在窗口内没有聚合行（没有用量，或实时值与聚合口径之间存在时间差）：
		// 用实时窗口值定位名次，并把自己补进总数，避免 self_rank > total_keys。
		selfValue = keyUsageWindowMetricValue(self, metric)
		totalKeys++
	}
	selfRank := keyUsageRankForValue(values, selfValue)
	if top == nil {
		top = []KeyUsageRankEntry{}
	}
	return KeyUsageRankingWindow{
		TotalKeys: totalKeys,
		SelfRank:  selfRank,
		Top:       top,
		Self: KeyUsageRankEntry{
			APIKeyID: apiKey.ID,
			Rank:     selfRank,
			KeyName:  apiKey.Name,
			Requests: self.Requests,
			Tokens:   self.Tokens,
			CostUSD:  self.CostUSD,
			IsSelf:   true,
		},
	}
}

// emptyKeyUsageRanking 排名不可用（依赖缺失 / 聚合查询失败）时的结果。
//
// self_rank 与 total_keys 都是 0，表示"排名暂不可用"：绝不能返回 1/1，
// 那会让每一个访客在 DB 抖动时都被渲染成"全站第 1 / 共 1 个 Key"，
// 而且与真实数据在视觉上完全无法区分。前端据 self_rank == 0 走单独的降级文案。
// top 仍是空数组而不是 null，self 依然返回，前端渲染路径唯一。
func emptyKeyUsageRanking(apiKey *APIKey, self KeyUsageWindowStat) KeyUsageRankingWindow {
	return KeyUsageRankingWindow{
		TotalKeys: 0,
		SelfRank:  0,
		Top:       []KeyUsageRankEntry{},
		Self: KeyUsageRankEntry{
			APIKeyID: apiKey.ID,
			Rank:     0,
			KeyName:  apiKey.Name,
			Requests: self.Requests,
			Tokens:   self.Tokens,
			CostUSD:  self.CostUSD,
			IsSelf:   true,
		},
	}
}

// keyUsageAggregateIDs 抽出聚合结果的 api_key_id 序列（与 keyUsageMetricValues 同序）。
func keyUsageAggregateIDs(rows []usagestats.APIKeyUsageAggregate) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.APIKeyID
	}
	return ids
}

// keyUsageSelfValueByID 在聚合结果里按 api_key_id 找出自己那一行的指标值。
// 线性扫描：全站榜上万行时也只是一次 int64 比较循环（远快于一次 SQL 往返），
// 换来的是"名次与榜单来自同一份浮点数"这个必须成立的性质。
func keyUsageSelfValueByID(ids []int64, values []float64, apiKeyID int64) (float64, bool) {
	if apiKeyID <= 0 {
		return 0, false
	}
	for i, id := range ids {
		if id != apiKeyID {
			continue
		}
		if i >= len(values) {
			return 0, false
		}
		return values[i], true
	}
	return 0, false
}

// keyUsageMetricValues 抽出排序指标序列（SQL 已按该指标降序）。
func keyUsageMetricValues(rows []usagestats.APIKeyUsageAggregate, metric string) []float64 {
	values := make([]float64, len(rows))
	for i, row := range rows {
		switch metric {
		case usagestats.KeyRankingMetricTokens:
			values[i] = float64(row.Tokens)
		case usagestats.KeyRankingMetricRequests:
			values[i] = float64(row.Requests)
		default:
			values[i] = row.Cost
		}
	}
	return values
}

func keyUsageWindowMetricValue(stat KeyUsageWindowStat, metric string) float64 {
	switch metric {
	case usagestats.KeyRankingMetricTokens:
		return float64(stat.Tokens)
	case usagestats.KeyRankingMetricRequests:
		return float64(stat.Requests)
	default:
		return stat.CostUSD
	}
}

// keyUsageRankForValue 在降序序列里二分出"严格大于 value 的元素个数 + 1"。
func keyUsageRankForValue(sortedDesc []float64, value float64) int {
	greater := sort.Search(len(sortedDesc), func(i int) bool {
		return sortedDesc[i] <= value
	})
	return greater + 1
}

func keyUsageSiteCacheKey(metric string, window keyUsageWindow) string {
	// 把窗口边界也编进 key：跨天时 today/last_7d 的实际区间会变，
	// 只用窗口名做 key 会在午夜后继续命中昨天的榜单。
	return metric + "|" + window.Name + "|" +
		strconv.FormatInt(window.Start.Unix(), 10) + "|" +
		strconv.FormatInt(window.End.Unix(), 10)
}

// keyUsageSiteSnapshot 全站榜快照（与具体 Key 无关，可全站共享）。
// 只保留前十名明细 + 全部 Key 的 id / 指标值序列（各 8 字节/Key），
// 不缓存整张榜的明细，避免大站上把几十 MB 常驻在内存里。
// IDs 与 Values 同序，用来按 api_key_id 取出"自己"在榜上的原始值（见 keyUsageSelfValueByID）。
type keyUsageSiteSnapshot struct {
	Top       []KeyUsageRankEntry
	IDs       []int64
	Values    []float64
	TotalKeys int
}

type keyUsageSiteCacheEntry struct {
	snapshot  *keyUsageSiteSnapshot
	expiresAt time.Time
}

// keyUsageSiteCache 进程内 TTL 缓存 + singleflight：
// 同一时刻只允许一个 goroutine 触发全站聚合，其余请求等待同一份结果。
type keyUsageSiteCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]keyUsageSiteCacheEntry
	sf    singleflight.Group
}

func newKeyUsageSiteCache(ttl time.Duration) *keyUsageSiteCache {
	if ttl <= 0 {
		ttl = DefaultKeyUsageSiteRankingCacheTTL
	}
	return &keyUsageSiteCache{ttl: ttl, items: make(map[string]keyUsageSiteCacheEntry)}
}

func (c *keyUsageSiteCache) get(key string) (*keyUsageSiteSnapshot, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		// 双检：可能已被其它 goroutine 刷新，别把新值删掉。
		if current, exists := c.items[key]; exists && !time.Now().Before(current.expiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	return entry.snapshot, true
}

// set 写入快照，并顺手回收过期项。
//
// 必须在写入时回收：cache key 里编了窗口边界的 unix 时间戳，每天会产生 9 个全新 key
// （3 metric × 3 window），而昨天的窗口边界此后永远不会被再次读取——只靠 get 的惰性
// 删除，昨天的条目会永久留在 map 里（每条持有一个长度=全站有用量 Key 数的 []float64
// 和 []int64），几百天下来就是几个 GB 的纯泄漏。
func (c *keyUsageSiteCache) set(key string, snapshot *keyUsageSiteSnapshot) {
	if c == nil {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	for existing, entry := range c.items {
		if !now.Before(entry.expiresAt) {
			delete(c.items, existing)
		}
	}
	c.items[key] = keyUsageSiteCacheEntry{snapshot: snapshot, expiresAt: now.Add(c.ttl)}

	// 兜底上限：即使 TTL 被配得很长（所有条目都还没过期），也不允许无界增长。
	for len(c.items) > keyUsageSiteCacheMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for existing, entry := range c.items {
			if existing == key {
				continue
			}
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = existing, entry.expiresAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(c.items, oldestKey)
	}
}

// Len 返回当前缓存条目数（供测试断言回收行为）。
func (c *keyUsageSiteCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *keyUsageSiteCache) GetOrLoad(key string, load func() (*keyUsageSiteSnapshot, error)) (*keyUsageSiteSnapshot, error) {
	if c == nil || load == nil {
		return nil, nil
	}
	if snapshot, ok := c.get(key); ok {
		return snapshot, nil
	}
	value, err, _ := c.sf.Do(key, func() (any, error) {
		if snapshot, ok := c.get(key); ok {
			return snapshot, nil
		}
		snapshot, loadErr := load()
		if loadErr != nil {
			return nil, loadErr
		}
		c.set(key, snapshot)
		return snapshot, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot, _ := value.(*keyUsageSiteSnapshot)
	return snapshot, nil
}
