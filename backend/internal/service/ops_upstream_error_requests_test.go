//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type upstreamErrorRequestsServiceRepo struct {
	OpsRepository
	filter *OpsUpstreamErrorRequestFilter
	result *OpsUpstreamErrorRequestList
	err    error
	calls  int
}

func (r *upstreamErrorRequestsServiceRepo) ListUpstreamErrorRequests(_ context.Context, f *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestList, error) {
	r.calls++
	copy := *f
	r.filter = &copy
	if r.result == nil {
		r.result = &OpsUpstreamErrorRequestList{Items: []*OpsUpstreamErrorRequest{}}
	}
	return r.result, r.err
}
func TestOpsUpstreamErrorRequestsService_DefaultsAndBoundaries(t *testing.T) {
	end := time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		filter     *OpsUpstreamErrorRequestFilter
		page, size int
	}{
		{"nil", nil, 1, 20},
		{"zero", &OpsUpstreamErrorRequestFilter{}, 1, 20},
		{"negative defaults", &OpsUpstreamErrorRequestFilter{Page: -1, PageSize: -2, EndTime: end}, 1, 20},
		{"maximum page and capped size", &OpsUpstreamErrorRequestFilter{Page: 10000000, PageSize: 101, EndTime: end, StatusCode: 502, Outcome: "failed"}, 10000000, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamErrorRequestsServiceRepo{}
			before := time.Now()
			out, err := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ListUpstreamErrorRequests(context.Background(), tc.filter)
			after := time.Now()
			require.NoError(t, err)
			require.Same(t, repo.result, out)
			require.Equal(t, 1, repo.calls)
			require.Equal(t, tc.page, repo.filter.Page)
			require.Equal(t, tc.size, repo.filter.PageSize)
			require.Equal(t, 24*time.Hour, repo.filter.EndTime.Sub(repo.filter.StartTime))
			if tc.filter == nil || tc.name == "zero" {
				require.False(t, repo.filter.EndTime.Before(before))
				require.False(t, repo.filter.EndTime.After(after))
			} else {
				require.Equal(t, end, repo.filter.EndTime)
			}
		})
	}
}
func TestOpsUpstreamErrorRequestsService_InvalidFiltersDoNotQuery(t *testing.T) {
	at := time.Now()
	for _, tc := range []struct {
		name, reason string
		filter       OpsUpstreamErrorRequestFilter
	}{
		{"page", "OPS_PAGE_INVALID", OpsUpstreamErrorRequestFilter{Page: 10000001}},
		{"status", "OPS_STATUS_INVALID", OpsUpstreamErrorRequestFilter{StatusCode: 500}},
		{"outcome", "OPS_OUTCOME_INVALID", OpsUpstreamErrorRequestFilter{Outcome: "Recovered"}},
		{"equal time", "OPS_TIME_RANGE_INVALID", OpsUpstreamErrorRequestFilter{StartTime: at, EndTime: at}},
		{"reversed time", "OPS_TIME_RANGE_INVALID", OpsUpstreamErrorRequestFilter{StartTime: at, EndTime: at.Add(-time.Second)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamErrorRequestsServiceRepo{}
			out, err := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ListUpstreamErrorRequests(context.Background(), &tc.filter)
			require.Nil(t, out)
			require.Equal(t, tc.reason, infraerrors.Reason(err))
			require.Zero(t, repo.calls)
		})
	}
}
func TestOpsUpstreamErrorRequestsService_UnavailableAndErrors(t *testing.T) {
	repo := &upstreamErrorRequestsServiceRepo{}
	disabled := NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: false}}, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := disabled.ListUpstreamErrorRequests(context.Background(), nil)
	require.Error(t, err)
	require.Zero(t, repo.calls)
	_, err = NewOpsService(&opsRepoWithoutRequestChain{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ListUpstreamErrorRequests(context.Background(), nil)
	require.Equal(t, "OPS_REPO_UNAVAILABLE", infraerrors.Reason(err))
	sentinel := errors.New("query failed")
	repo.err = sentinel
	out, err := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ListUpstreamErrorRequests(context.Background(), nil)
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
}
func TestOpsUpstreamErrorRequestsService_RedactsHistoricalRows(t *testing.T) {
	original := &OpsUpstreamErrorEvent{UpstreamStatusCode: 503, Message: "Bearer message-token", Detail: `password=detail-secret`, UpstreamResponseBody: `{"api_key":"body-secret"}`, UpstreamURL: "https://user:pass@example.com/path?token=query-secret#fragment"}
	repo := &upstreamErrorRequestsServiceRepo{result: &OpsUpstreamErrorRequestList{Items: []*OpsUpstreamErrorRequest{{Attempts: []*OpsUpstreamErrorEvent{original}}}}}
	out, err := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).ListUpstreamErrorRequests(context.Background(), nil)
	require.NoError(t, err)
	clean := out.Items[0].Attempts[0]
	require.NotSame(t, original, clean)
	require.NotContains(t, clean.Message, "message-token")
	require.NotContains(t, clean.Detail, "detail-secret")
	require.NotContains(t, clean.UpstreamResponseBody, "body-secret")
	require.Equal(t, "https://example.com/path", clean.UpstreamURL)
	require.Equal(t, "Bearer message-token", original.Message)
	require.Contains(t, original.UpstreamURL, "query-secret")
}
func TestUpstreamErrorRequestSanitize_EmptyUnsafeURLsAndRetention(t *testing.T) {
	for _, raw := range []string{"", "file:///etc/passwd", "/relative", "javascript:alert(1)", "https://%invalid"} {
		events, err := sanitizeUpstreamRequestAttempts([]*OpsUpstreamErrorEvent{{UpstreamStatusCode: 502, UpstreamURL: raw}})
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Empty(t, events[0].UpstreamURL)
	}
	for _, events := range [][]*OpsUpstreamErrorEvent{nil, {}, {nil}, {{}}} {
		clean, err := sanitizeUpstreamRequestAttempts(events)
		require.NoError(t, err)
		require.NotNil(t, clean)
		require.Empty(t, clean)
	}
	events := make([]*OpsUpstreamErrorEvent, 20)
	for i := range events {
		events[i] = &OpsUpstreamErrorEvent{AtUnixMs: int64(i), UpstreamStatusCode: 502}
	}
	clean, err := sanitizeUpstreamRequestAttempts(events)
	require.NoError(t, err)
	require.Len(t, clean, 16)
	require.EqualValues(t, 4, clean[0].AtUnixMs)
	require.EqualValues(t, 19, clean[15].AtUnixMs)
}
func TestUpstreamErrorRequestSanitize_CredentialVariants(t *testing.T) {
	for _, raw := range []string{
		"Bearer credential-value", "bEaReR credential-value", "api_key=credential-value", "API-KEY: credential-value",
		`{"access_token":"credential-value"}`, `{'refresh-token':'credential-value'}`, "password=credential-value",
		"secret: credential-value", "cookie=session=credential-value", "authorization=credential-value",
	} {
		t.Run(raw, func(t *testing.T) {
			original := &OpsUpstreamErrorEvent{UpstreamStatusCode: 502, Message: raw, Detail: raw, UpstreamResponseBody: raw}
			events, err := sanitizeUpstreamRequestAttempts([]*OpsUpstreamErrorEvent{original})
			require.NoError(t, err)
			require.Len(t, events, 1)
			for _, text := range []string{events[0].Message, events[0].Detail, events[0].UpstreamResponseBody} {
				require.NotContains(t, text, "credential-value")
			}
			require.Equal(t, raw, original.Message)
			require.Equal(t, raw, original.Detail)
			require.Equal(t, raw, original.UpstreamResponseBody)
		})
	}
}
func TestUpstreamErrorRequestSanitize(t *testing.T) {
	events, err := sanitizeUpstreamRequestAttempts([]*OpsUpstreamErrorEvent{nil, {UpstreamStatusCode: 502, UpstreamURL: "https://user:password@example.com/path?api_key=secret#token", Message: strings.Repeat("x", 3000), Detail: `{"api_key":"secret-value","message":"upstream failed"}`, UpstreamResponseBody: `{"authorization":"Bearer private-token"}`}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events: %#v", events)
	}
	ev := events[0]
	if ev.UpstreamURL != "https://example.com/path" {
		t.Fatal(ev.UpstreamURL)
	}
	if len(ev.Message) > 2048 || strings.Contains(ev.Detail, "secret-value") || strings.Contains(ev.UpstreamResponseBody, "private-token") {
		t.Fatalf("unredacted event %#v", ev)
	}
	events, err = sanitizeUpstreamRequestAttempts([]*OpsUpstreamErrorEvent{{UpstreamStatusCode: 503, UpstreamURL: "https://%invalid"}})
	if err != nil || len(events) != 1 || events[0].UpstreamURL != "" {
		t.Fatalf("invalid URL result %#v %v", events, err)
	}
}
