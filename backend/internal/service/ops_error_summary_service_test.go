package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stubOpsRepoForSummary struct {
	OpsRepository
	gotFilter *OpsErrorLogFilter
	calls     int
	result    *OpsErrorLogSummary
	err       error
}

func (s *stubOpsRepoForSummary) SummarizeErrorLogs(_ context.Context, f *OpsErrorLogFilter) (*OpsErrorLogSummary, error) {
	s.calls++
	s.gotFilter = f
	return s.result, s.err
}

func TestGetErrorLogsSummary_DropsListOnlyFieldsAndKeepsFilters(t *testing.T) {
	stub := &stubOpsRepoForSummary{result: &OpsErrorLogSummary{Items: []*OpsErrorKindSummary{}}}
	svc := &OpsService{opsRepo: stub}
	gid := int64(5)
	in := &OpsErrorLogFilter{
		Page: 3, PageSize: 50, SortBy: "status_code", SortOrder: "asc",
		ErrorKind:           "routing|503|model_rate_limited",
		GroupID:             &gid,
		Model:               "gpt-5.5",
		MinRequestBodyBytes: 4 << 20,
	}

	_, err := svc.GetErrorLogsSummary(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 1, stub.calls)

	got := stub.gotFilter
	require.Empty(t, got.ErrorKind, "selected kind must not hide the other cards")
	require.Zero(t, got.Page)
	require.Zero(t, got.PageSize)
	require.Empty(t, got.SortBy)
	require.Empty(t, got.SortOrder)
	require.Equal(t, &gid, got.GroupID)
	require.Equal(t, "gpt-5.5", got.Model)
	require.EqualValues(t, 4<<20, got.MinRequestBodyBytes)

	// The caller's filter object is not mutated (the list request reuses it).
	require.Equal(t, "routing|503|model_rate_limited", in.ErrorKind)
	require.Equal(t, 3, in.Page)
}

func TestGetErrorLogsSummary_RedactsSampleMessages(t *testing.T) {
	stub := &stubOpsRepoForSummary{result: &OpsErrorLogSummary{
		Total: 2,
		Items: []*OpsErrorKindSummary{
			{Kind: "upstream|400|x", SampleMessage: "  bad request https://g.example/v1?key=SECRET123&alt=sse  ", Count: 2},
			nil,
		},
	}}
	svc := &OpsService{opsRepo: stub}

	out, err := svc.GetErrorLogsSummary(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, out.Items, 2)
	require.NotContains(t, out.Items[0].SampleMessage, "SECRET123")
	require.Contains(t, out.Items[0].SampleMessage, "key=***")
	require.Equal(t, out.Items[0].SampleMessage, "bad request https://g.example/v1?key=***&alt=sse")
}

func TestGetErrorLogsSummary_NormalizesEmptyResults(t *testing.T) {
	for name, result := range map[string]*OpsErrorLogSummary{
		"nil result": nil,
		"nil items":  {Total: 0},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &OpsService{opsRepo: &stubOpsRepoForSummary{result: result}}
			out, err := svc.GetErrorLogsSummary(context.Background(), &OpsErrorLogFilter{})
			require.NoError(t, err)
			require.NotNil(t, out)
			require.NotNil(t, out.Items, "frontend expects an array, never null")
			require.Empty(t, out.Items)
		})
	}

	out, err := (&OpsService{}).GetErrorLogsSummary(context.Background(), &OpsErrorLogFilter{})
	require.NoError(t, err)
	require.NotNil(t, out.Items)
}

func TestGetErrorLogsSummary_PropagatesRepoError(t *testing.T) {
	boom := errors.New("db down")
	svc := &OpsService{opsRepo: &stubOpsRepoForSummary{err: boom}}
	_, err := svc.GetErrorLogsSummary(context.Background(), &OpsErrorLogFilter{})
	require.ErrorIs(t, err, boom)
}

func TestGetErrorLogsSummary_RequiresMonitoring(t *testing.T) {
	stub := &stubOpsRepoForSummary{}
	cfg := &config.Config{}
	cfg.Ops.Enabled = false
	svc := &OpsService{opsRepo: stub, cfg: cfg}

	_, err := svc.GetErrorLogsSummary(context.Background(), &OpsErrorLogFilter{})
	require.ErrorIs(t, err, ErrOpsDisabled)
	require.Zero(t, stub.calls)
}
