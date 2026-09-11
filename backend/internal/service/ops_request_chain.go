package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 请求链路（request chain）把「一次客户端请求里网关到底试了几次、每次撞在哪个账号上、
// 最后成没成」摊开给站长看。数据早就全在 ops_error_logs 里了：
//
//   - upstream_errors 是 JSONB 数组，每次上游尝试一条（失败重试、换账号各记一条）。
//   - status_code 是客户端最终看到的 HTTP 码，决定这条链路算「已恢复」还是「最终失败」。
//
// 不摊开的话，后台只显示「502 × 5」，站长会当成五次事故，而实际上多数都被自动兜住了。
const (
	// OpsRequestChainOutcomeRecovered 客户端最终拿到 2xx：上游报错被重试/换号兜住了。
	OpsRequestChainOutcomeRecovered = "recovered"
	// OpsRequestChainOutcomeFailed 客户端最终拿到的是失败响应。
	OpsRequestChainOutcomeFailed = "failed"
)

// opsUsageLogClientRequestIDPrefix 是 usage_logs.request_id 上的前缀。
//
// 这是个必须记住的坑：网关写 usage_logs 时走 resolveUsageBillingRequestID，
// 优先用 "client:" + client_request_id 作为幂等键（见 gateway_usage_billing.go），
// 而 ops_error_logs.client_request_id 存的是不带前缀的裸 ID。
// 直接 ops_error_logs.client_request_id = usage_logs.request_id 关联会 100% 落空，
// 必须补上这个前缀。
const opsUsageLogClientRequestIDPrefix = "client:"

// opsRequestChainMaxClientRequestIDLen 与 ops_error_logs.client_request_id 的
// VARCHAR(64) 对齐后再留一倍余量，超长直接判定为非法入参，不进 SQL。
const opsRequestChainMaxClientRequestIDLen = 128

var (
	// ErrOpsRequestChainNotFound 该 client_request_id 在 ops_error_logs 里没有任何记录。
	// 一次干净成功（从未报错）的请求也会走到这里：本接口只讲「出过错的请求后来怎么样了」。
	ErrOpsRequestChainNotFound = infraerrors.NotFound("OPS_REQUEST_CHAIN_NOT_FOUND", "Request chain not found")
	// ErrOpsRequestChainUnavailable 仓储不具备链路查询能力（只可能出现在测试替身上）。
	ErrOpsRequestChainUnavailable = infraerrors.ServiceUnavailable("OPS_REQUEST_CHAIN_UNAVAILABLE", "Request chain lookup is not available")
	// ErrOpsRequestChainInvalidID 入参为空或超长。
	ErrOpsRequestChainInvalidID = infraerrors.BadRequest("OPS_REQUEST_CHAIN_INVALID_ID", "Invalid client request id")
)

// OpsRequestChainAttempt 是链路里的一次上游尝试，按时间升序编号。
//
// 字段一律不加 omitempty：前端按「字段总是在」写类型，缺字段和空值是两种不同的 bug，
// 排障接口不该让人去分辨。
type OpsRequestChainAttempt struct {
	// Seq 从 1 开始，按 At 升序编号，纯展示用。
	Seq int       `json:"seq"`
	At  time.Time `json:"at"`

	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Platform    string `json:"platform"`

	// UpstreamStatusCode 为 null 表示这次尝试压根没拿到上游响应（连接错误、凭证获取
	// 失败等）。上游事件里这个字段本身就是 omitempty 的，0 与缺失无法区分，所以统一
	// 归一成 null —— 免得前端把 0 当成一个真实的 HTTP 状态码去展示。
	UpstreamStatusCode *int   `json:"upstream_status_code"`
	Message            string `json:"message"`
	Kind               string `json:"kind"`
}

// OpsRequestChainFinal 是这次请求最终落到哪个账号、花了多少。
// 只有关联到 usage_logs 才会有值，关联不到就是 null —— 不编造。
type OpsRequestChainFinal struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	// Succeeded 与顶层 Outcome 同源（口径见 ops_account_recent_errors.go 顶部）。
	Succeeded bool `json:"succeeded"`

	// TimeToFirstTokenMs 优先取 ops_error_logs.time_to_first_token_ms：那是客户端
	// 真正等到首字节的时间（含前面那几次失败重试），比 usage_logs.first_token_ms
	// 只算成功那一次更贴近用户体感。ops 没记才回退到 usage_logs；两边都没有就是 null。
	TimeToFirstTokenMs *int64 `json:"time_to_first_token_ms"`

	TotalTokens int64 `json:"total_tokens"`
	// Cost 取 usage_logs.total_cost（标准计费），与仓库里其它地方 cost 字段同口径；
	// 实际扣款额是 actual_cost，不在本接口范围内。
	Cost float64 `json:"cost"`
}

// OpsRequestChain 是一个 client_request_id 的完整链路。
type OpsRequestChain struct {
	ClientRequestID string `json:"client_request_id"`
	// CreatedAt 取最早那条 ops_error_logs 的落库时间，代表这条链路的起点。
	CreatedAt time.Time `json:"created_at"`

	Model          string `json:"model"`
	RequestedModel string `json:"requested_model"`
	Stream         bool   `json:"stream"`

	// Outcome 取 recovered / failed，口径见 ops_account_recent_errors.go 顶部。
	Outcome string `json:"outcome"`
	// ClientStatusCode 是客户端最终看到的 HTTP 码。
	ClientStatusCode int `json:"client_status_code"`

	// Attempts 永远是数组（可能为空），不会是 null。
	Attempts []*OpsRequestChainAttempt `json:"attempts"`
	// Final 关联不到 usage_logs 时为 null。
	Final *OpsRequestChainFinal `json:"final"`
}

// OpsRequestChainAttemptRecord 是 upstream_errors 数组里的一条原始尝试。
type OpsRequestChainAttemptRecord struct {
	// AtUnixMs 为 0 表示上游事件没带时间戳（老数据）。服务层此时退回所属
	// ops_error_logs 行的 created_at，并且不让它参与跨记录去重。
	AtUnixMs int64

	AccountID          int64
	AccountName        string
	Platform           string
	UpstreamStatusCode int
	Message            string
	Kind               string
}

// OpsRequestChainErrorRecord 是一条 ops_error_logs 记录及其展开后的尝试列表。
type OpsRequestChainErrorRecord struct {
	ID        int64
	CreatedAt time.Time

	// StatusCode 是客户端最终看到的 HTTP 码（NULL 已被 COALESCE 成 0）。
	StatusCode         int
	Model              string
	RequestedModel     string
	Stream             bool
	TimeToFirstTokenMs *int64

	Attempts []*OpsRequestChainAttemptRecord
}

// OpsRequestChainUsageRecord 是关联到的 usage_logs 行（最终成功结果）。
type OpsRequestChainUsageRecord struct {
	AccountID    int64
	AccountName  string
	TotalTokens  int64
	Cost         float64
	FirstTokenMs *int64
}

// OpsRequestChainSource 是仓储层交给服务层的原始素材。
type OpsRequestChainSource struct {
	// Errors 按 (created_at, id) 升序；同一个 client_request_id 可能有多条
	// （多次失败各记一条）。
	Errors []*OpsRequestChainErrorRecord
	// Usage 关联不到时为 nil。
	Usage *OpsRequestChainUsageRecord
}

// OpsRequestChainReader 是可选的 OpsRepository 能力（同 OpsAccountErrorSummaryReader，
// 放在接口外是为了不让既有测试替身编译失败）。
type OpsRequestChainReader interface {
	// GetRequestChainSource 返回该 client_request_id 的全部错误记录与最终用量行。
	// 没有任何错误记录时返回的 Source 里 Errors 为空，由服务层判成 404。
	GetRequestChainSource(ctx context.Context, clientRequestID string) (*OpsRequestChainSource, error)
}

// GetRequestChain 返回一个 client_request_id 的完整请求链路。
//
// 找不到任何 ops_error_logs 记录时返回 ErrOpsRequestChainNotFound（HTTP 404）。
func (s *OpsService) GetRequestChain(ctx context.Context, clientRequestID string) (*OpsRequestChain, error) {
	if s == nil || s.opsRepo == nil {
		return nil, ErrOpsRequestChainUnavailable
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	clientRequestID = strings.TrimSpace(clientRequestID)
	if clientRequestID == "" || len(clientRequestID) > opsRequestChainMaxClientRequestIDLen {
		return nil, ErrOpsRequestChainInvalidID
	}

	reader, ok := s.opsRepo.(OpsRequestChainReader)
	if !ok {
		return nil, ErrOpsRequestChainUnavailable
	}
	source, err := reader.GetRequestChainSource(ctx, clientRequestID)
	if err != nil {
		return nil, err
	}
	chain := buildOpsRequestChain(clientRequestID, source)
	if chain == nil {
		return nil, ErrOpsRequestChainNotFound
	}
	return chain, nil
}

// buildOpsRequestChain 把原始素材装配成链路；没有任何错误记录时返回 nil。
//
// 拆成纯函数是为了能不碰数据库地覆盖「多条记录合并 / 跨账号切换 / 最终失败 /
// upstream_errors 为空」这些分支。
func buildOpsRequestChain(clientRequestID string, source *OpsRequestChainSource) *OpsRequestChain {
	if source == nil {
		return nil
	}
	records := make([]*OpsRequestChainErrorRecord, 0, len(source.Errors))
	for _, record := range source.Errors {
		if record != nil {
			records = append(records, record)
		}
	}
	if len(records) == 0 {
		return nil
	}

	// 仓储已经按 (created_at, id) 升序取回，这里再排一次是为了让纯函数自己成立，
	// 也让测试可以乱序喂数据。
	sort.SliceStable(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.Before(records[j].CreatedAt)
		}
		return records[i].ID < records[j].ID
	})

	first := records[0]
	// 最后一条记录代表客户端最终看到的结果：中间那几条是过程中的失败快照。
	last := records[len(records)-1]

	chain := &OpsRequestChain{
		ClientRequestID:  clientRequestID,
		CreatedAt:        first.CreatedAt,
		Model:            latestNonEmptyFromRecords(records, func(r *OpsRequestChainErrorRecord) string { return r.Model }),
		RequestedModel:   latestNonEmptyFromRecords(records, func(r *OpsRequestChainErrorRecord) string { return r.RequestedModel }),
		Stream:           last.Stream,
		ClientStatusCode: last.StatusCode,
		Attempts:         buildOpsRequestChainAttempts(records),
	}
	if isOpsRecoveredClientStatus(chain.ClientStatusCode) {
		chain.Outcome = OpsRequestChainOutcomeRecovered
	} else {
		chain.Outcome = OpsRequestChainOutcomeFailed
	}

	if source.Usage != nil {
		final := &OpsRequestChainFinal{
			AccountID:   source.Usage.AccountID,
			AccountName: source.Usage.AccountName,
			Succeeded:   chain.Outcome == OpsRequestChainOutcomeRecovered,
			TotalTokens: source.Usage.TotalTokens,
			Cost:        source.Usage.Cost,
		}
		final.TimeToFirstTokenMs = last.TimeToFirstTokenMs
		if final.TimeToFirstTokenMs == nil {
			final.TimeToFirstTokenMs = source.Usage.FirstTokenMs
		}
		chain.Final = final
	}
	return chain
}

// buildOpsRequestChainAttempts 合并所有记录的 attempts，去重后按时间升序编号。
func buildOpsRequestChainAttempts(records []*OpsRequestChainErrorRecord) []*OpsRequestChainAttempt {
	attempts := make([]*OpsRequestChainAttempt, 0, 8)
	seen := make(map[string]struct{}, 8)

	for _, record := range records {
		for index, raw := range record.Attempts {
			if raw == nil {
				continue
			}
			key := opsRequestChainAttemptKey(record, index, raw)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			at := record.CreatedAt
			if raw.AtUnixMs > 0 {
				at = time.UnixMilli(raw.AtUnixMs).UTC()
			}
			attempt := &OpsRequestChainAttempt{
				At:          at,
				AccountID:   raw.AccountID,
				AccountName: strings.TrimSpace(raw.AccountName),
				Platform:    strings.TrimSpace(raw.Platform),
				Message:     strings.TrimSpace(raw.Message),
				Kind:        strings.TrimSpace(raw.Kind),
			}
			if raw.UpstreamStatusCode > 0 {
				code := raw.UpstreamStatusCode
				attempt.UpstreamStatusCode = &code
			}
			attempts = append(attempts, attempt)
		}
	}

	// 稳定排序：时间相同的尝试保持「记录顺序 + 数组内顺序」，不会被打乱。
	sort.SliceStable(attempts, func(i, j int) bool {
		return attempts[i].At.Before(attempts[j].At)
	})
	for i, attempt := range attempts {
		attempt.Seq = i + 1
	}
	return attempts
}

// opsRequestChainAttemptKey 生成去重键。
//
// upstream_errors 是挂在 gin 上下文上累加的，同一次尝试很可能被后写的那条
// ops_error_logs 再抄一遍，所以跨记录合并时必须去重。
//
// 但没有 at_unix_ms 的老数据不参与跨记录去重：同一条记录里若有多次「同账号、同状态、
// 同文案」的尝试，按内容去重会把它们误合成一条，丢数据比多显示一条更糟。
func opsRequestChainAttemptKey(record *OpsRequestChainErrorRecord, index int, raw *OpsRequestChainAttemptRecord) string {
	if raw.AtUnixMs <= 0 {
		return fmt.Sprintf("row:%d:%d", record.ID, index)
	}
	return fmt.Sprintf("at:%d|%d|%d|%s|%s|%s",
		raw.AtUnixMs,
		raw.AccountID,
		raw.UpstreamStatusCode,
		strings.TrimSpace(raw.Kind),
		strings.TrimSpace(raw.Platform),
		strings.TrimSpace(raw.Message),
	)
}

// latestNonEmptyFromRecords 从最后一条记录往前找第一个非空值：最终那条最能代表这次请求，
// 但中间记录可能是唯一记下模型名的那条。
func latestNonEmptyFromRecords(records []*OpsRequestChainErrorRecord, pick func(*OpsRequestChainErrorRecord) string) string {
	for i := len(records) - 1; i >= 0; i-- {
		if value := strings.TrimSpace(pick(records[i])); value != "" {
			return value
		}
	}
	return ""
}

// OpsUsageLogRequestIDForClientRequestID 把裸 client_request_id 转成 usage_logs
// 那边的 request_id。见 opsUsageLogClientRequestIDPrefix 上的说明。
func OpsUsageLogRequestIDForClientRequestID(clientRequestID string) string {
	return opsUsageLogClientRequestIDPrefix + strings.TrimSpace(clientRequestID)
}
