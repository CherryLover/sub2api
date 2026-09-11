//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// opsRecentErrorRepoStub 同时实现「汇总」与「拆分」两个可选能力。
type opsRecentErrorRepoStub struct {
	OpsRepository
	summaries    map[int64]*OpsAccountRecentErrorSummary
	outcomes     map[int64]*OpsAccountRecentErrorOutcome
	summaryErr   error
	outcomeErr   error
	summaryCalls int
	outcomeCalls int
}

func (s *opsRecentErrorRepoStub) GetAccountRecentErrorSummaries(_ context.Context, _ []int64, _ time.Time) (map[int64]*OpsAccountRecentErrorSummary, error) {
	s.summaryCalls++
	if s.summaryErr != nil {
		return nil, s.summaryErr
	}
	return s.summaries, nil
}

func (s *opsRecentErrorRepoStub) GetAccountRecentErrorOutcomes(_ context.Context, _ []int64, _ time.Time) (map[int64]*OpsAccountRecentErrorOutcome, error) {
	s.outcomeCalls++
	if s.outcomeErr != nil {
		return nil, s.outcomeErr
	}
	return s.outcomes, nil
}

// opsRecentErrorSummaryOnlyRepoStub 只有老的汇总能力，没有拆分能力。
type opsRecentErrorSummaryOnlyRepoStub struct {
	OpsRepository
	summaries map[int64]*OpsAccountRecentErrorSummary
}

func (s *opsRecentErrorSummaryOnlyRepoStub) GetAccountRecentErrorSummaries(_ context.Context, _ []int64, _ time.Time) (map[int64]*OpsAccountRecentErrorSummary, error) {
	return s.summaries, nil
}

// 「已恢复」的口径就是客户端最终拿到 2xx：上游报 502 但客户端拿到 200 的行，
// 说明重试/换号已经把事故兜住了，不该再被站长当成一次故障。
func TestIsOpsRecoveredClientStatus(t *testing.T) {
	for status, want := range map[int]bool{
		0:   false,
		199: false,
		200: true,
		204: true,
		299: true,
		300: false,
		429: false,
		502: false,
	} {
		require.Equal(t, want, isOpsRecoveredClientStatus(status), "status=%d", status)
	}
}

func TestGetAccountRecentErrorSummaries_SplitsRecoveredAndFailed(t *testing.T) {
	lastAt := time.Date(2026, 9, 11, 6, 59, 30, 0, time.UTC)
	upstream502 := 502

	stub := &opsRecentErrorRepoStub{
		summaries: map[int64]*OpsAccountRecentErrorSummary{
			6: {
				Total: 18,
				ByStatus: []OpsAccountRecentErrorStatusCount{
					{StatusCode: 429, Count: 4},
					{StatusCode: 502, Count: 14},
				},
				Last: &OpsAccountRecentError{
					At:                 lastAt,
					StatusCode:         200,
					UpstreamStatusCode: &upstream502,
					Message:            "Our servers are currently overloaded.",
					Model:              "gpt-5.6-sol",
				},
			},
		},
		outcomes: map[int64]*OpsAccountRecentErrorOutcome{
			6: {
				Recovered: 15,
				Failed:    3,
				ByStatus: []OpsAccountRecentErrorStatusOutcome{
					{
						StatusCode:          502,
						Recovered:           13,
						Failed:              1,
						LastCreatedAt:       lastAt,
						LastClientRequestID: "req-502-last",
					},
					{
						StatusCode:          429,
						Recovered:           2,
						Failed:              2,
						LastCreatedAt:       lastAt.Add(-time.Minute),
						LastClientRequestID: "req-429-last",
					},
				},
			},
		},
	}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{6, 5}, lastAt.Add(-15*time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, stub.summaryCalls)
	require.Equal(t, 1, stub.outcomeCalls)

	summary := got[6]
	require.NotNil(t, summary)
	require.EqualValues(t, 18, summary.Total, "既有字段不动")
	require.EqualValues(t, 15, summary.Recovered)
	require.EqualValues(t, 3, summary.Failed)

	require.Len(t, summary.ByStatus, 2)
	require.Equal(t, 502, summary.ByStatus[0].StatusCode, "仍按 count 倒序")
	require.EqualValues(t, 14, summary.ByStatus[0].Count)
	require.EqualValues(t, 13, summary.ByStatus[0].Recovered)
	require.EqualValues(t, 1, summary.ByStatus[0].Failed)
	require.Equal(t, "req-502-last", summary.ByStatus[0].LastClientRequestID)
	require.Equal(t, 429, summary.ByStatus[1].StatusCode)
	require.EqualValues(t, 2, summary.ByStatus[1].Recovered)
	require.EqualValues(t, 2, summary.ByStatus[1].Failed)
	require.Equal(t, "req-429-last", summary.ByStatus[1].LastClientRequestID)

	require.NotNil(t, summary.Last)
	require.Equal(t, "req-502-last", summary.Last.ClientRequestID, "整格入口取最近那一行所在档位的 ID")

	// 窗口内没有错误的账号仍是零值摘要，不会被拆分字段污染。
	empty := got[5]
	require.NotNil(t, empty)
	require.EqualValues(t, 0, empty.Total)
	require.EqualValues(t, 0, empty.Recovered)
	require.EqualValues(t, 0, empty.Failed)
	require.NotNil(t, empty.ByStatus)
	require.Empty(t, empty.ByStatus)
	require.Nil(t, empty.Last)
}

// 两条语句之间落进新行时，last 与拆分里最近那档对不上号：宁可不给链路入口，
// 也不能让前端拿着别的请求的 ID 去开面板。
func TestGetAccountRecentErrorSummaries_LastClientRequestIDOnlyWhenRowsMatch(t *testing.T) {
	lastAt := time.Date(2026, 9, 11, 7, 0, 0, 0, time.UTC)
	stub := &opsRecentErrorRepoStub{
		summaries: map[int64]*OpsAccountRecentErrorSummary{
			6: {
				Total:    2,
				ByStatus: []OpsAccountRecentErrorStatusCount{{StatusCode: 502, Count: 2}},
				Last:     &OpsAccountRecentError{At: lastAt, StatusCode: 200},
			},
		},
		outcomes: map[int64]*OpsAccountRecentErrorOutcome{
			6: {
				Recovered: 2,
				ByStatus: []OpsAccountRecentErrorStatusOutcome{{
					StatusCode:          502,
					Recovered:           2,
					LastCreatedAt:       lastAt.Add(time.Second),
					LastClientRequestID: "req-newer",
				}},
			},
		},
	}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{6}, lastAt.Add(-time.Hour))
	require.NoError(t, err)
	require.NotNil(t, got[6].Last)
	require.Empty(t, got[6].Last.ClientRequestID)
	require.Equal(t, "req-newer", got[6].ByStatus[0].LastClientRequestID, "档位上的 ID 不受影响")
}

// 老的测试替身只实现汇总能力：不能因此报错，只是拿不到拆分。
func TestGetAccountRecentErrorSummaries_WithoutOutcomeCapability(t *testing.T) {
	svc := &OpsService{opsRepo: &opsRecentErrorSummaryOnlyRepoStub{
		summaries: map[int64]*OpsAccountRecentErrorSummary{
			6: {Total: 5, ByStatus: []OpsAccountRecentErrorStatusCount{{StatusCode: 502, Count: 5}}},
		},
	}}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{6}, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 5, got[6].Total)
	require.EqualValues(t, 0, got[6].Recovered)
	require.EqualValues(t, 0, got[6].Failed)
	require.EqualValues(t, 0, got[6].ByStatus[0].Recovered)
}

// 拆分查询失败时整块判为不可用（handler 会把 recent_errors 置 nil = 未知），
// 而不是回一个 total=18 / recovered=0 / failed=0 的假象。
func TestGetAccountRecentErrorSummaries_OutcomeErrorFailsWholeCall(t *testing.T) {
	dbErr := errors.New("db down")
	stub := &opsRecentErrorRepoStub{
		summaries:  map[int64]*OpsAccountRecentErrorSummary{6: {Total: 18}},
		outcomeErr: dbErr,
	}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{6}, time.Now().Add(-time.Hour))
	require.Nil(t, got)
	require.ErrorIs(t, err, dbErr)
	require.Equal(t, 1, stub.outcomeCalls)
}

// Total 为 0 的格子在前端就是「没有错误」，不该再挂上非零拆分。
func TestGetAccountRecentErrorSummaries_ZeroTotalDropsOutcome(t *testing.T) {
	stub := &opsRecentErrorRepoStub{
		// 账号 5 有错误，所以拆分查询会照常发出；账号 6 在窗口内是干净的。
		summaries: map[int64]*OpsAccountRecentErrorSummary{
			5: {Total: 1, ByStatus: []OpsAccountRecentErrorStatusCount{{StatusCode: 502, Count: 1}}},
			6: {Total: 0},
		},
		outcomes: map[int64]*OpsAccountRecentErrorOutcome{
			6: {Recovered: 3, Failed: 1},
		},
	}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{5, 6}, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, stub.outcomeCalls)
	require.EqualValues(t, 0, got[6].Total)
	require.EqualValues(t, 0, got[6].Recovered)
	require.EqualValues(t, 0, got[6].Failed)
}

// 窗口内一条错误都没有时不该为一堆零值多打一趟库（这个接口会被账号列表轮询）。
func TestGetAccountRecentErrorSummaries_NoErrorsSkipsOutcomeQuery(t *testing.T) {
	stub := &opsRecentErrorRepoStub{summaries: map[int64]*OpsAccountRecentErrorSummary{}}
	svc := &OpsService{opsRepo: stub}

	got, err := svc.GetAccountRecentErrorSummaries(context.Background(), []int64{6}, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, stub.summaryCalls)
	require.Zero(t, stub.outcomeCalls)
	require.NotNil(t, got[6])
	require.EqualValues(t, 0, got[6].Total)
}
