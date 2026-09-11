package service

import (
	"context"
	"sort"
	"time"
)

// 「已恢复 / 最终失败」判定口径（整个文件、以及请求链路接口共用同一口径）：
//
// ops_error_logs 里一行 = 网关处理一次客户端请求时记下的一次失败上下文。网关遇到上游
// 报错会重试、必要时换账号，所以这一行的 status_code（客户端最终看到的 HTTP 码）才是
// 「这次请求到底成没成」的唯一权威来源：
//
//   - status_code 属于 2xx：客户端最终拿到了成功响应，上游那几次报错已经被重试/换号
//     自动兜住了，记为「已恢复」（recovered）。
//   - 其余（含 status_code 为 NULL，被 COALESCE 成 0）：客户端最终拿到的是失败响应，
//     记为「最终失败」（failed）。NULL 不敢乐观地算作恢复——没看到 2xx 就不声称成功。
//
// 不这么拆的话，后台只会显示「502 × 5」，站长看到就以为出了五次事故，实际上五次可能
// 全部自动恢复了。
const (
	opsRecoveredStatusMin = 200
	opsRecoveredStatusMax = 299
)

// isOpsRecoveredClientStatus 判定客户端最终状态码是否算「已恢复」。
func isOpsRecoveredClientStatus(statusCode int) bool {
	return statusCode >= opsRecoveredStatusMin && statusCode <= opsRecoveredStatusMax
}

// OpsAccountRecentErrorStatusCount is the number of error rows for one status
// code (upstream status preferred over the client-facing status).
//
// Recovered/Failed 把 Count 按上面的口径拆成「已恢复 / 最终失败」两档；两者之和正常
// 等于 Count（同一条 SQL 里 FILTER 出来的互补计数），只有在计数与拆分两条语句之间
// 恰好落进新行时才会有一行之差。Count 是既有字段，前端有用例依赖，不动。
type OpsAccountRecentErrorStatusCount struct {
	StatusCode int   `json:"status_code"`
	Count      int64 `json:"count"`

	Recovered int64 `json:"recovered"`
	Failed    int64 `json:"failed"`

	// LastClientRequestID 是这一档里最近一行错误的 client_request_id，
	// 供前端把每个状态码 chip 直接链到「请求链路」接口
	// （GET /api/v1/admin/ops/requests/:clientRequestId/chain）。
	// 该行没记 client_request_id 时为空串，前端据此隐藏入口。
	LastClientRequestID string `json:"last_client_request_id,omitempty"`
}

// OpsAccountRecentError is the most recent error row of an account.
type OpsAccountRecentError struct {
	At                 time.Time `json:"at"`
	StatusCode         int       `json:"status_code"`
	UpstreamStatusCode *int      `json:"upstream_status_code,omitempty"`
	Message            string    `json:"message,omitempty"`
	Model              string    `json:"model,omitempty"`

	// ClientRequestID 是这一行的 client_request_id，用来打开「请求链路」面板。
	// 空串表示这行没记（老数据或非网关路径），前端据此隐藏入口。
	ClientRequestID string `json:"client_request_id,omitempty"`
}

// OpsAccountRecentErrorSummary summarizes an account's recent upstream errors
// from ops_error_logs. Last is nil when Total is 0.
//
// Recovered/Failed 是 Total 的「已恢复 / 最终失败」拆分（口径见文件顶部）。仓储不具备
// 拆分能力时两者都是 0：生产的 *opsRepository 一定实现该能力（编译期断言在
// repository/ops_repo_account_error_outcomes.go），只有测试替身会走到这条退化路径。
type OpsAccountRecentErrorSummary struct {
	Total    int64                              `json:"total"`
	ByStatus []OpsAccountRecentErrorStatusCount `json:"by_status"`
	Last     *OpsAccountRecentError             `json:"last"`

	Recovered int64 `json:"recovered"`
	Failed    int64 `json:"failed"`
}

// OpsAccountErrorSummaryReader is an optional OpsRepository capability. It is
// kept off the OpsRepository interface so existing test doubles keep
// compiling; OpsService reaches it through a type assertion.
type OpsAccountErrorSummaryReader interface {
	// GetAccountRecentErrorSummaries returns summaries keyed by account ID for
	// error rows created at or after since. Accounts without errors may be
	// absent from the result.
	GetAccountRecentErrorSummaries(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]*OpsAccountRecentErrorSummary, error)
}

// OpsAccountRecentErrorStatusOutcome 是一个状态码档位的「已恢复 / 最终失败」拆分。
type OpsAccountRecentErrorStatusOutcome struct {
	// StatusCode 必须与 GetAccountRecentErrorSummaries 用的状态码分桶表达式完全一致，
	// 否则两边对不上号、拆分会挂在错误的档位上。
	StatusCode int

	Recovered int64
	Failed    int64

	// LastCreatedAt 是这一档最近一行错误的时间，服务层据此在所有档位里挑出
	// 「整个账号最近那一行」属于哪一档，从而给 Summary.Last 补 client_request_id。
	LastCreatedAt time.Time

	// LastClientRequestID 是这一档最近一行错误的 client_request_id（可能为空串）。
	LastClientRequestID string
}

// OpsAccountRecentErrorOutcome 是一个账号在窗口内所有错误行的拆分结果。
type OpsAccountRecentErrorOutcome struct {
	Recovered int64
	Failed    int64
	ByStatus  []OpsAccountRecentErrorStatusOutcome
}

// OpsAccountErrorOutcomeReader 是又一个可选的 OpsRepository 能力：在不改动既有
// GetAccountRecentErrorSummaries 查询的前提下，额外算出「已恢复 / 最终失败」拆分
// 和每档最近一次的 client_request_id。
//
// 之所以拆成独立能力而不是塞进 OpsRepository 接口：接口上加方法会让仓库里所有
// OpsRepository 测试替身一起编译失败。
type OpsAccountErrorOutcomeReader interface {
	// GetAccountRecentErrorOutcomes returns outcome splits keyed by account ID
	// for the same rows GetAccountRecentErrorSummaries counts. Accounts without
	// errors may be absent from the result.
	GetAccountRecentErrorOutcomes(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]*OpsAccountRecentErrorOutcome, error)
}

// GetAccountRecentErrorSummaries returns one summary per requested account for
// errors recorded since the given time.
//
// A nil map with a nil error means the data is unavailable (ops monitoring
// disabled, or the repository lacks the capability); callers should render
// that as "unknown" rather than "no errors".
func (s *OpsService) GetAccountRecentErrorSummaries(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]*OpsAccountRecentErrorSummary, error) {
	if s == nil || s.opsRepo == nil || !s.IsMonitoringEnabled(ctx) {
		return nil, nil
	}
	reader, ok := s.opsRepo.(OpsAccountErrorSummaryReader)
	if !ok {
		return nil, nil
	}

	ids := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	out := make(map[int64]*OpsAccountRecentErrorSummary, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	summaries, err := reader.GetAccountRecentErrorSummaries(ctx, ids, since)
	if err != nil {
		return nil, err
	}

	// 「已恢复 / 最终失败」拆分是一次额外查询：它和上面那条扫的是同一批行、同一个
	// WHERE，失败时说明库本身有问题，宁可整块报不可用（handler 会把 recent_errors
	// 置 nil = "未知"），也不要回一个 total=18 / recovered=0 / failed=0 的假象。
	//
	// 窗口内一条错误都没有时直接跳过：这个接口会被账号列表轮询，不该为一堆零值
	// 多打一趟库（与既有实现跳过 last 查询的理由相同）。
	var outcomes map[int64]*OpsAccountRecentErrorOutcome
	outcomeReader, hasOutcomes := s.opsRepo.(OpsAccountErrorOutcomeReader)
	if hasOutcomes && len(summaries) > 0 {
		outcomes, err = outcomeReader.GetAccountRecentErrorOutcomes(ctx, ids, since)
		if err != nil {
			return nil, err
		}
	}

	for _, id := range ids {
		summary := normalizeOpsAccountRecentErrorSummary(summaries[id])
		applyOpsAccountRecentErrorOutcome(summary, outcomes[id])
		out[id] = summary
	}
	return out, nil
}

// applyOpsAccountRecentErrorOutcome 把拆分结果合并进既有 summary：只追加字段，
// 不改动 Total / Count / Last 等既有取值。
//
// 两条 SQL 之间可能落进新行，因此不强求 Recovered+Failed == Total：宁可让两个数字
// 各自忠实反映自己那条语句看到的快照，也不要为了对齐去伪造。
func applyOpsAccountRecentErrorOutcome(summary *OpsAccountRecentErrorSummary, outcome *OpsAccountRecentErrorOutcome) {
	if summary == nil || outcome == nil {
		return
	}
	// Total==0 时这一格在前端就是「没有错误」，再挂上非零拆分只会自相矛盾
	// （只有两条语句之间落进新行才可能出现）。
	if summary.Total <= 0 {
		return
	}
	summary.Recovered = outcome.Recovered
	summary.Failed = outcome.Failed

	byStatus := make(map[int]OpsAccountRecentErrorStatusOutcome, len(outcome.ByStatus))
	var latest OpsAccountRecentErrorStatusOutcome
	for _, row := range outcome.ByStatus {
		byStatus[row.StatusCode] = row
		if latest.LastCreatedAt.IsZero() || row.LastCreatedAt.After(latest.LastCreatedAt) {
			latest = row
		}
	}
	for i := range summary.ByStatus {
		row, ok := byStatus[summary.ByStatus[i].StatusCode]
		if !ok {
			continue
		}
		summary.ByStatus[i].Recovered = row.Recovered
		summary.ByStatus[i].Failed = row.Failed
		summary.ByStatus[i].LastClientRequestID = row.LastClientRequestID
	}

	// summary.Last 来自另一条 DISTINCT ON 语句，只有当两边指向同一行（时间戳相等）
	// 时才敢把 client_request_id 挂上去；否则宁可不给入口，也不要让前端拿着别的
	// 请求的 ID 去开链路面板。
	if summary.Last != nil && !latest.LastCreatedAt.IsZero() && summary.Last.At.Equal(latest.LastCreatedAt) {
		summary.Last.ClientRequestID = latest.LastClientRequestID
	}
}

func normalizeOpsAccountRecentErrorSummary(summary *OpsAccountRecentErrorSummary) *OpsAccountRecentErrorSummary {
	if summary == nil {
		return &OpsAccountRecentErrorSummary{ByStatus: []OpsAccountRecentErrorStatusCount{}}
	}
	if summary.ByStatus == nil {
		summary.ByStatus = []OpsAccountRecentErrorStatusCount{}
	}
	sort.SliceStable(summary.ByStatus, func(i, j int) bool {
		if summary.ByStatus[i].Count != summary.ByStatus[j].Count {
			return summary.ByStatus[i].Count > summary.ByStatus[j].Count
		}
		return summary.ByStatus[i].StatusCode < summary.ByStatus[j].StatusCode
	})
	if summary.Total <= 0 {
		summary.Total = 0
		summary.Last = nil
		summary.Recovered = 0
		summary.Failed = 0
	}
	return summary
}
