//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// opsRequestChainRepoStub 只实现链路查询这一个可选能力，其余方法靠嵌入的 nil 接口占位
// （被调到就 panic，等于断言服务层不该碰别的方法）。
type opsRequestChainRepoStub struct {
	OpsRepository
	source *OpsRequestChainSource
	err    error
	calls  int
	gotID  string
}

func (s *opsRequestChainRepoStub) GetRequestChainSource(_ context.Context, clientRequestID string) (*OpsRequestChainSource, error) {
	s.calls++
	s.gotID = clientRequestID
	if s.err != nil {
		return nil, s.err
	}
	return s.source, nil
}

// opsRepoWithoutRequestChain 缺少链路查询能力。
type opsRepoWithoutRequestChain struct {
	OpsRepository
}

var opsChainT0 = time.Date(2026, 9, 11, 6, 59, 39, 0, time.UTC)

func opsChainAttempt(offset time.Duration, accountID int64, name string, status int, message string) *OpsRequestChainAttemptRecord {
	return &OpsRequestChainAttemptRecord{
		AtUnixMs:           opsChainT0.Add(offset).UnixMilli(),
		AccountID:          accountID,
		AccountName:        name,
		Platform:           "openai",
		UpstreamStatusCode: status,
		Message:            message,
		Kind:               "failover",
	}
}

// 实测样例：某条记录 3 次尝试全在账号 7 上、全是 502，最终仍由账号 7 成功。
// 网关中途落过一条失败记录、收尾又落一条，两条的 upstream_errors 互相包含，
// 合并后必须去重、按时间升序、seq 从 1 连续编号。
func TestBuildOpsRequestChain_MergesRecordsSortsAndDedupes(t *testing.T) {
	first := opsChainAttempt(100*time.Millisecond, 7, "anviz-7", 502, "Our servers are currently overloaded.")
	second := opsChainAttempt(2*time.Second, 7, "anviz-7", 502, "Our servers are currently overloaded.")
	third := opsChainAttempt(5*time.Second, 7, "anviz-7", 502, "Our servers are currently overloaded.")

	dupFirst := *first
	dupSecond := *second

	ttft := int64(18501)
	source := &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{
			// 故意乱序喂进来，装配函数必须自己排好。
			{
				ID:             2,
				CreatedAt:      opsChainT0.Add(19 * time.Second),
				StatusCode:     200,
				Model:          "gpt-5.6-sol",
				RequestedModel: "gpt-5.6-sol",
				Stream:         true,
				// 收尾这条把三次尝试全抄了一遍（upstream_errors 挂在 gin 上下文上累加）。
				Attempts:           []*OpsRequestChainAttemptRecord{&dupFirst, &dupSecond, third},
				TimeToFirstTokenMs: &ttft,
			},
			{
				ID:             1,
				CreatedAt:      opsChainT0,
				StatusCode:     502,
				Model:          "gpt-5.6-sol",
				RequestedModel: "gpt-5.6-sol",
				Stream:         true,
				Attempts:       []*OpsRequestChainAttemptRecord{first, second},
			},
		},
		Usage: &OpsRequestChainUsageRecord{
			AccountID:   7,
			AccountName: "anviz-7",
			TotalTokens: 1883,
			Cost:        0.0212,
		},
	}

	chain := buildOpsRequestChain("req-abc", source)
	require.NotNil(t, chain)

	require.Equal(t, "req-abc", chain.ClientRequestID)
	require.True(t, opsChainT0.Equal(chain.CreatedAt), "链路起点取最早那条错误记录")
	require.Equal(t, "gpt-5.6-sol", chain.Model)
	require.Equal(t, "gpt-5.6-sol", chain.RequestedModel)
	require.True(t, chain.Stream)
	require.Equal(t, OpsRequestChainOutcomeRecovered, chain.Outcome)
	require.Equal(t, 200, chain.ClientStatusCode, "客户端最终看到的是最后那条记录的状态码")

	require.Len(t, chain.Attempts, 3, "三次尝试被抄了两遍，合并后只剩三条")
	for i, attempt := range chain.Attempts {
		require.Equal(t, i+1, attempt.Seq)
		require.EqualValues(t, 7, attempt.AccountID)
		require.Equal(t, "anviz-7", attempt.AccountName)
		require.Equal(t, "openai", attempt.Platform)
		require.NotNil(t, attempt.UpstreamStatusCode)
		require.Equal(t, 502, *attempt.UpstreamStatusCode)
		require.Equal(t, "failover", attempt.Kind)
		if i > 0 {
			require.True(t, !attempt.At.Before(chain.Attempts[i-1].At), "按时间升序")
		}
	}
	require.True(t, opsChainT0.Add(100*time.Millisecond).Equal(chain.Attempts[0].At))
	require.True(t, opsChainT0.Add(5*time.Second).Equal(chain.Attempts[2].At))

	require.NotNil(t, chain.Final)
	require.EqualValues(t, 7, chain.Final.AccountID)
	require.Equal(t, "anviz-7", chain.Final.AccountName)
	require.True(t, chain.Final.Succeeded)
	require.NotNil(t, chain.Final.TimeToFirstTokenMs)
	require.EqualValues(t, 18501, *chain.Final.TimeToFirstTokenMs)
	require.EqualValues(t, 1883, chain.Final.TotalTokens)
	require.InDelta(t, 0.0212, chain.Final.Cost, 1e-9)
}

// 实测样例：账号 5→6 切换后成功。
func TestBuildOpsRequestChain_AccountSwitchRecovered(t *testing.T) {
	source := &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{{
			ID:         11,
			CreatedAt:  opsChainT0,
			StatusCode: 200,
			Model:      "gpt-5.6-sol",
			Stream:     false,
			Attempts: []*OpsRequestChainAttemptRecord{
				opsChainAttempt(0, 5, "acct-5", 502, "overloaded"),
				opsChainAttempt(time.Second, 6, "acct-6", 529, "overloaded"),
			},
		}},
		Usage: &OpsRequestChainUsageRecord{AccountID: 6, AccountName: "acct-6", TotalTokens: 42, Cost: 0.001},
	}

	chain := buildOpsRequestChain("req-switch", source)
	require.NotNil(t, chain)
	require.Equal(t, OpsRequestChainOutcomeRecovered, chain.Outcome)
	require.Len(t, chain.Attempts, 2)
	require.EqualValues(t, 5, chain.Attempts[0].AccountID)
	require.EqualValues(t, 6, chain.Attempts[1].AccountID)
	require.NotNil(t, chain.Attempts[1].UpstreamStatusCode)
	require.Equal(t, 529, *chain.Attempts[1].UpstreamStatusCode)
	require.NotNil(t, chain.Final)
	require.EqualValues(t, 6, chain.Final.AccountID, "最终落在切换后的账号上")
	require.True(t, chain.Final.Succeeded)
}

// 实测样例：5/6/7/8 四个账号全试一遍最后还是失败 —— 这种才是真事故。
func TestBuildOpsRequestChain_AllAccountsExhaustedFails(t *testing.T) {
	source := &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{{
			ID:         21,
			CreatedAt:  opsChainT0,
			StatusCode: 502,
			Attempts: []*OpsRequestChainAttemptRecord{
				opsChainAttempt(0, 5, "acct-5", 502, "overloaded"),
				opsChainAttempt(time.Second, 6, "acct-6", 502, "overloaded"),
				opsChainAttempt(2*time.Second, 7, "acct-7", 502, "overloaded"),
				opsChainAttempt(3*time.Second, 8, "acct-8", 502, "overloaded"),
			},
		}},
	}

	chain := buildOpsRequestChain("req-failed", source)
	require.NotNil(t, chain)
	require.Equal(t, OpsRequestChainOutcomeFailed, chain.Outcome)
	require.Equal(t, 502, chain.ClientStatusCode)
	require.Len(t, chain.Attempts, 4)
	require.Nil(t, chain.Final, "没有 usage_logs 就不编造最终结果")
}

// 关联不到 usage_logs 时 final 必须是 null，即使这条链路其实已经恢复了。
func TestBuildOpsRequestChain_RecoveredWithoutUsageLogKeepsFinalNil(t *testing.T) {
	source := &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{{
			ID:         31,
			CreatedAt:  opsChainT0,
			StatusCode: 204,
			Attempts:   []*OpsRequestChainAttemptRecord{opsChainAttempt(0, 5, "acct-5", 502, "overloaded")},
		}},
	}

	chain := buildOpsRequestChain("req-no-usage", source)
	require.NotNil(t, chain)
	require.Equal(t, OpsRequestChainOutcomeRecovered, chain.Outcome, "2xx 一律算已恢复，不止 200")
	require.Nil(t, chain.Final)

	raw, err := json.Marshal(chain)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "final")
	require.Nil(t, decoded["final"], "final 必须序列化成 null 而不是被省略")
	attempts, ok := decoded["attempts"].([]any)
	require.True(t, ok, "attempts 必须是数组：%#v", decoded["attempts"])
	require.Len(t, attempts, 1)
}

// upstream_errors 为 NULL / 空数组的记录只是没有尝试可展开，不能让整条链路炸掉。
func TestBuildOpsRequestChain_EmptyUpstreamErrorsDoesNotPanic(t *testing.T) {
	cases := []struct {
		name     string
		attempts []*OpsRequestChainAttemptRecord
	}{
		{name: "nil 切片", attempts: nil},
		{name: "空切片", attempts: []*OpsRequestChainAttemptRecord{}},
		{name: "切片里混了 nil", attempts: []*OpsRequestChainAttemptRecord{nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain := buildOpsRequestChain("req-empty", &OpsRequestChainSource{
				Errors: []*OpsRequestChainErrorRecord{{
					ID:         41,
					CreatedAt:  opsChainT0,
					StatusCode: 500,
					Attempts:   tc.attempts,
				}},
			})
			require.NotNil(t, chain)
			require.Equal(t, OpsRequestChainOutcomeFailed, chain.Outcome)
			require.NotNil(t, chain.Attempts, "attempts 必须是空数组而不是 nil")
			require.Empty(t, chain.Attempts)

			raw, err := json.Marshal(chain)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"attempts":[]`)
		})
	}
}

// at_unix_ms 缺失（老数据）时退回所属记录的 created_at，并且不参与跨记录去重 ——
// 同一条记录里内容完全相同的多次尝试不能被误合成一条。
func TestBuildOpsRequestChain_MissingTimestampFallsBackToRecordTime(t *testing.T) {
	sameContent := func() *OpsRequestChainAttemptRecord {
		return &OpsRequestChainAttemptRecord{
			AccountID:          5,
			AccountName:        "acct-5",
			UpstreamStatusCode: 502,
			Message:            "overloaded",
			Kind:               "failover",
		}
	}
	timed := opsChainAttempt(30*time.Second, 6, "acct-6", 529, "overloaded")

	chain := buildOpsRequestChain("req-legacy", &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{{
			ID:         51,
			CreatedAt:  opsChainT0,
			StatusCode: 200,
			Attempts:   []*OpsRequestChainAttemptRecord{sameContent(), sameContent(), timed},
		}},
	})
	require.NotNil(t, chain)
	require.Len(t, chain.Attempts, 3, "没有时间戳的两次尝试各算一条，不按内容去重")
	require.True(t, opsChainT0.Equal(chain.Attempts[0].At))
	require.True(t, opsChainT0.Equal(chain.Attempts[1].At))
	require.True(t, opsChainT0.Add(30*time.Second).Equal(chain.Attempts[2].At))
	require.Equal(t, []int{1, 2, 3}, []int{chain.Attempts[0].Seq, chain.Attempts[1].Seq, chain.Attempts[2].Seq})
}

// 「压根没拿到上游响应」的尝试（连接失败、凭证获取失败）必须回 null，
// 不能回 0 —— 0 不是一个 HTTP 状态码，前端照着渲染就是在编造事实。
func TestBuildOpsRequestChain_MissingUpstreamStatusSerializesAsNull(t *testing.T) {
	chain := buildOpsRequestChain("req-no-status", &OpsRequestChainSource{
		Errors: []*OpsRequestChainErrorRecord{{
			ID:         71,
			CreatedAt:  opsChainT0,
			StatusCode: 502,
			Attempts: []*OpsRequestChainAttemptRecord{{
				AtUnixMs:    opsChainT0.UnixMilli(),
				AccountID:   5,
				AccountName: "acct-5",
				Kind:        "request_error",
				Message:     "dial tcp: connection refused",
			}},
		}},
	})
	require.NotNil(t, chain)
	require.Len(t, chain.Attempts, 1)
	require.Nil(t, chain.Attempts[0].UpstreamStatusCode)

	raw, err := json.Marshal(chain.Attempts[0])
	require.NoError(t, err)
	require.Contains(t, string(raw), `"upstream_status_code":null`)
	// 其余字段一律下发，前端按「字段总是在」写的类型才成立。
	for _, field := range []string{`"account_id"`, `"account_name"`, `"platform"`, `"message"`, `"kind"`, `"seq"`, `"at"`} {
		require.Contains(t, string(raw), field)
	}
}

func TestBuildOpsRequestChain_NoErrorRecordsReturnsNil(t *testing.T) {
	require.Nil(t, buildOpsRequestChain("x", nil))
	require.Nil(t, buildOpsRequestChain("x", &OpsRequestChainSource{}))
	require.Nil(t, buildOpsRequestChain("x", &OpsRequestChainSource{Errors: []*OpsRequestChainErrorRecord{nil}}))
}

func TestOpsServiceGetRequestChain(t *testing.T) {
	ctx := context.Background()

	t.Run("找不到记录返回 404", func(t *testing.T) {
		stub := &opsRequestChainRepoStub{source: &OpsRequestChainSource{}}
		svc := &OpsService{opsRepo: stub}

		chain, err := svc.GetRequestChain(ctx, " req-missing ")
		require.Nil(t, chain)
		require.ErrorIs(t, err, ErrOpsRequestChainNotFound)
		require.Equal(t, 404, infraerrors.Code(err))
		require.Equal(t, "req-missing", stub.gotID, "入参先 trim 再查")
	})

	t.Run("空 ID 与超长 ID 返回 400 且不查库", func(t *testing.T) {
		stub := &opsRequestChainRepoStub{}
		svc := &OpsService{opsRepo: stub}

		for _, id := range []string{"", "   ", string(make([]byte, opsRequestChainMaxClientRequestIDLen+1))} {
			chain, err := svc.GetRequestChain(ctx, id)
			require.Nil(t, chain)
			require.ErrorIs(t, err, ErrOpsRequestChainInvalidID)
			require.Equal(t, 400, infraerrors.Code(err))
		}
		require.Zero(t, stub.calls)
	})

	t.Run("仓储不支持链路查询返回 503", func(t *testing.T) {
		svc := &OpsService{opsRepo: &opsRepoWithoutRequestChain{}}

		chain, err := svc.GetRequestChain(ctx, "req-1")
		require.Nil(t, chain)
		require.ErrorIs(t, err, ErrOpsRequestChainUnavailable)
		require.Equal(t, 503, infraerrors.Code(err))
	})

	t.Run("查询报错原样上抛", func(t *testing.T) {
		dbErr := errors.New("db down")
		svc := &OpsService{opsRepo: &opsRequestChainRepoStub{err: dbErr}}

		chain, err := svc.GetRequestChain(ctx, "req-1")
		require.Nil(t, chain)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("正常返回装配后的链路", func(t *testing.T) {
		stub := &opsRequestChainRepoStub{source: &OpsRequestChainSource{
			Errors: []*OpsRequestChainErrorRecord{{
				ID:         61,
				CreatedAt:  opsChainT0,
				StatusCode: 200,
				Attempts:   []*OpsRequestChainAttemptRecord{opsChainAttempt(0, 7, "acct-7", 502, "overloaded")},
			}},
		}}
		svc := &OpsService{opsRepo: stub}

		chain, err := svc.GetRequestChain(ctx, "req-ok")
		require.NoError(t, err)
		require.NotNil(t, chain)
		require.Equal(t, "req-ok", chain.ClientRequestID)
		require.Equal(t, OpsRequestChainOutcomeRecovered, chain.Outcome)
		require.Equal(t, 1, stub.calls)
	})
}

// usage_logs 的 request_id 带 "client:" 前缀，直接拿裸 client_request_id 关联会全部落空。
func TestOpsUsageLogRequestIDForClientRequestID(t *testing.T) {
	require.Equal(t, "client:abc", OpsUsageLogRequestIDForClientRequestID("abc"))
	require.Equal(t, "client:abc", OpsUsageLogRequestIDForClientRequestID("  abc  "))
}
