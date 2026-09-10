package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const accountDiagnosticsPath = "/api/v1/admin/accounts/diagnostics/batch"

// diagnosticsAdminServiceStub returns only the configured accounts, so
// unknown IDs behave like deleted accounts.
type diagnosticsAdminServiceStub struct {
	*stubAdminService
	accounts  map[int64]*service.Account
	requested []int64
	calls     int
}

func (s *diagnosticsAdminServiceStub) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	s.calls++
	s.requested = append([]int64(nil), ids...)
	out := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		if account, ok := s.accounts[id]; ok {
			out = append(out, account)
		}
	}
	return out, nil
}

// diagnosticsOpsRepoStub implements the optional error-summary capability; any
// other OpsRepository method panics via the nil embedded interface.
type diagnosticsOpsRepoStub struct {
	service.OpsRepository
	summaries map[int64]*service.OpsAccountRecentErrorSummary
	err       error
	calls     int
	gotIDs    []int64
	gotSince  time.Time
}

func (r *diagnosticsOpsRepoStub) GetAccountRecentErrorSummaries(_ context.Context, ids []int64, since time.Time) (map[int64]*service.OpsAccountRecentErrorSummary, error) {
	r.calls++
	r.gotIDs = append([]int64(nil), ids...)
	r.gotSince = since
	if r.err != nil {
		return nil, r.err
	}
	return r.summaries, nil
}

// diagnosticsOpsRepoWithoutSummaries lacks the optional capability.
type diagnosticsOpsRepoWithoutSummaries struct {
	service.OpsRepository
}

func newDiagnosticsAccount(id int64) *service.Account {
	return &service.Account{
		ID:          id,
		Name:        "acc",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
	}
}

func setupAccountDiagnosticsRouter(accounts []*service.Account, gateway *service.OpenAIGatewayService, ops *service.OpsService) (*gin.Engine, *diagnosticsAdminServiceStub) {
	gin.SetMode(gin.TestMode)
	adminSvc := &diagnosticsAdminServiceStub{
		stubAdminService: newStubAdminService(),
		accounts:         make(map[int64]*service.Account, len(accounts)),
	}
	for _, account := range accounts {
		adminSvc.accounts[account.ID] = account
	}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if gateway != nil {
		handler.SetOpenAIGatewayService(gateway)
	}
	if ops != nil {
		handler.SetOpsService(ops)
	}
	router := gin.New()
	router.POST(accountDiagnosticsPath, handler.GetBatchDiagnostics)
	return router, adminSvc
}

func postAccountDiagnostics(router *gin.Engine, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, accountDiagnosticsPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// decodeAccountDiagnostics decodes into generic maps so the test pins the
// exact JSON field names rather than the Go struct shape.
func decodeAccountDiagnostics(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	require.Equal(t, 0, env.Code)
	require.NotNil(t, env.Data)
	return env.Data
}

func diagnosticsEntry(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()
	diagnostics, ok := data["diagnostics"].(map[string]any)
	require.True(t, ok, "diagnostics must be an object: %#v", data["diagnostics"])
	entry, ok := diagnostics[id].(map[string]any)
	require.True(t, ok, "missing diagnostics entry %s: %#v", id, diagnostics)
	return entry
}

func TestAccountBatchDiagnostics_ReturnsDocumentedShape(t *testing.T) {
	healthy := newDiagnosticsAccount(5)
	blocked := newDiagnosticsAccount(6)

	gateway := &service.OpenAIGatewayService{}
	blockUntil := time.Now().Add(5 * 24 * time.Hour).UTC().Truncate(time.Second)
	gateway.BlockAccountScheduling(blocked, blockUntil, "429")

	lastAt := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	upstream502 := 502
	repo := &diagnosticsOpsRepoStub{summaries: map[int64]*service.OpsAccountRecentErrorSummary{
		6: {
			Total: 18,
			// Deliberately unsorted: the service orders by count desc.
			ByStatus: []service.OpsAccountRecentErrorStatusCount{
				{StatusCode: 429, Count: 4},
				{StatusCode: 502, Count: 14},
			},
			Last: &service.OpsAccountRecentError{
				At:                 lastAt,
				StatusCode:         200,
				UpstreamStatusCode: &upstream502,
				Message:            "Our servers are currently overloaded. Please try again later.",
				Model:              "gpt-5.6-sol",
			},
		},
	}}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router, adminSvc := setupAccountDiagnosticsRouter([]*service.Account{healthy, blocked}, gateway, ops)

	before := time.Now()
	rec := postAccountDiagnostics(router, `{"account_ids":[6,5,6,0,999],"window_minutes":15}`)
	after := time.Now()
	data := decodeAccountDiagnostics(t, rec)

	require.EqualValues(t, 15, data["window_minutes"])
	generatedAt, err := time.Parse(time.RFC3339Nano, data["generated_at"].(string))
	require.NoError(t, err)
	require.False(t, generatedAt.Before(before.Add(-time.Second)))
	require.Equal(t, []int64{5, 6, 999}, adminSvc.requested, "ids are deduplicated, positive and sorted")

	diagnostics := data["diagnostics"].(map[string]any)
	require.Len(t, diagnostics, 2, "unknown account 999 gets no entry")

	// Account 6: runtime-blocked in process, with recent errors.
	entry6 := diagnosticsEntry(t, data, "6")
	scheduling6 := entry6["scheduling"].(map[string]any)
	require.Equal(t, false, scheduling6["schedulable"])
	blocks6 := scheduling6["blocks"].([]any)
	require.Len(t, blocks6, 1)
	block := blocks6[0].(map[string]any)
	require.Equal(t, "runtime_block", block["source"])
	until, err := time.Parse(time.RFC3339Nano, block["until"].(string))
	require.NoError(t, err)
	require.True(t, blockUntil.Equal(until))
	for _, omitted := range []string{"model", "proxy_id", "window", "threshold", "utilization", "status", "reason"} {
		require.NotContains(t, block, omitted, "empty optional field %q must be omitted", omitted)
	}

	errors6 := entry6["recent_errors"].(map[string]any)
	require.EqualValues(t, 18, errors6["total"])
	byStatus := errors6["by_status"].([]any)
	require.Len(t, byStatus, 2)
	require.EqualValues(t, 502, byStatus[0].(map[string]any)["status_code"])
	require.EqualValues(t, 14, byStatus[0].(map[string]any)["count"])
	require.EqualValues(t, 429, byStatus[1].(map[string]any)["status_code"])
	require.EqualValues(t, 4, byStatus[1].(map[string]any)["count"])
	last := errors6["last"].(map[string]any)
	lastAtGot, err := time.Parse(time.RFC3339Nano, last["at"].(string))
	require.NoError(t, err)
	require.True(t, lastAt.Equal(lastAtGot))
	require.EqualValues(t, 200, last["status_code"])
	require.EqualValues(t, 502, last["upstream_status_code"])
	require.Equal(t, "Our servers are currently overloaded. Please try again later.", last["message"])
	require.Equal(t, "gpt-5.6-sol", last["model"])

	// Account 5: healthy, no errors in the window.
	entry5 := diagnosticsEntry(t, data, "5")
	scheduling5 := entry5["scheduling"].(map[string]any)
	require.Equal(t, true, scheduling5["schedulable"])
	blocks5, ok := scheduling5["blocks"].([]any)
	require.True(t, ok, "blocks must be an empty array, not null")
	require.Empty(t, blocks5)
	errors5 := entry5["recent_errors"].(map[string]any)
	require.EqualValues(t, 0, errors5["total"])
	byStatus5, ok := errors5["by_status"].([]any)
	require.True(t, ok, "by_status must be an empty array, not null")
	require.Empty(t, byStatus5)
	require.Contains(t, errors5, "last")
	require.Nil(t, errors5["last"])

	// Only existing accounts are queried, over the requested window.
	require.Equal(t, 1, repo.calls)
	require.Equal(t, []int64{5, 6}, repo.gotIDs)
	require.False(t, repo.gotSince.Before(before.Add(-15*time.Minute-time.Second)))
	require.False(t, repo.gotSince.After(after.Add(-15*time.Minute)))
}

func TestAccountBatchDiagnostics_RecentErrorsNullWhenOpsUnavailable(t *testing.T) {
	cases := []struct {
		name      string
		ops       func(repo *diagnosticsOpsRepoStub) *service.OpsService
		wantCalls int
	}{
		{
			name: "ops service not wired",
			ops:  func(*diagnosticsOpsRepoStub) *service.OpsService { return nil },
		},
		{
			name: "ops disabled by config",
			ops: func(repo *diagnosticsOpsRepoStub) *service.OpsService {
				return service.NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: false}}, nil, nil, nil, nil, nil, nil, nil, nil)
			},
		},
		{
			name: "repository lacks error summaries",
			ops: func(*diagnosticsOpsRepoStub) *service.OpsService {
				return service.NewOpsService(&diagnosticsOpsRepoWithoutSummaries{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			},
		},
		{
			name: "query fails",
			ops: func(repo *diagnosticsOpsRepoStub) *service.OpsService {
				repo.err = errors.New("db down")
				return service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			},
			wantCalls: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &diagnosticsOpsRepoStub{}
			account := newDiagnosticsAccount(7)
			until := time.Now().Add(time.Hour)
			account.RateLimitResetAt = &until
			router, _ := setupAccountDiagnosticsRouter([]*service.Account{account}, nil, tc.ops(repo))

			data := decodeAccountDiagnostics(t, postAccountDiagnostics(router, `{"account_ids":[7]}`))
			entry := diagnosticsEntry(t, data, "7")
			require.Contains(t, entry, "recent_errors")
			require.Nil(t, entry["recent_errors"])
			require.Equal(t, tc.wantCalls, repo.calls)

			// Scheduling is still reported (persisted blocks only without a gateway).
			scheduling := entry["scheduling"].(map[string]any)
			require.Equal(t, false, scheduling["schedulable"])
			blocks := scheduling["blocks"].([]any)
			require.Len(t, blocks, 1)
			require.Equal(t, "rate_limited", blocks[0].(map[string]any)["source"])
		})
	}
}

func TestAccountBatchDiagnostics_WindowMinutesDefaultAndClamp(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{body: `{"account_ids":[1]}`, want: 15},
		{body: `{"account_ids":[1],"window_minutes":0}`, want: 15},
		{body: `{"account_ids":[1],"window_minutes":1}`, want: 5},
		{body: `{"account_ids":[1],"window_minutes":60}`, want: 60},
		{body: `{"account_ids":[1],"window_minutes":100000}`, want: 1440},
	}
	for _, tc := range cases {
		repo := &diagnosticsOpsRepoStub{}
		ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		router, _ := setupAccountDiagnosticsRouter([]*service.Account{newDiagnosticsAccount(1)}, nil, ops)

		before := time.Now()
		data := decodeAccountDiagnostics(t, postAccountDiagnostics(router, tc.body))
		require.EqualValues(t, tc.want, data["window_minutes"], tc.body)
		require.Equal(t, 1, repo.calls, tc.body)
		expectedSince := before.Add(-time.Duration(tc.want) * time.Minute)
		require.WithinDuration(t, expectedSince, repo.gotSince, 5*time.Second, tc.body)
	}
}

func TestAccountBatchDiagnostics_EmptyIDsReturnsEmptyDiagnostics(t *testing.T) {
	router, adminSvc := setupAccountDiagnosticsRouter(nil, nil, nil)

	data := decodeAccountDiagnostics(t, postAccountDiagnostics(router, `{"account_ids":[0,-3]}`))
	require.EqualValues(t, 15, data["window_minutes"])
	diagnostics, ok := data["diagnostics"].(map[string]any)
	require.True(t, ok)
	require.Empty(t, diagnostics)
	require.Zero(t, adminSvc.calls)
}

func TestAccountBatchDiagnostics_BadBodyReturns400(t *testing.T) {
	router, adminSvc := setupAccountDiagnosticsRouter(nil, nil, nil)
	for _, body := range []string{
		`not-json`,
		`{}`,
		`{"account_ids":"5"}`,
		`{"account_ids":[5],"window_minutes":"ten"}`,
	} {
		rec := postAccountDiagnostics(router, body)
		require.Equal(t, http.StatusBadRequest, rec.Code, body)
	}
	require.Zero(t, adminSvc.calls)
}
