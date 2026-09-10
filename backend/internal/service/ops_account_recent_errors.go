package service

import (
	"context"
	"sort"
	"time"
)

// OpsAccountRecentErrorStatusCount is the number of error rows for one status
// code (upstream status preferred over the client-facing status).
type OpsAccountRecentErrorStatusCount struct {
	StatusCode int   `json:"status_code"`
	Count      int64 `json:"count"`
}

// OpsAccountRecentError is the most recent error row of an account.
type OpsAccountRecentError struct {
	At                 time.Time `json:"at"`
	StatusCode         int       `json:"status_code"`
	UpstreamStatusCode *int      `json:"upstream_status_code,omitempty"`
	Message            string    `json:"message,omitempty"`
	Model              string    `json:"model,omitempty"`
}

// OpsAccountRecentErrorSummary summarizes an account's recent upstream errors
// from ops_error_logs. Last is nil when Total is 0.
type OpsAccountRecentErrorSummary struct {
	Total    int64                              `json:"total"`
	ByStatus []OpsAccountRecentErrorStatusCount `json:"by_status"`
	Last     *OpsAccountRecentError             `json:"last"`
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
	for _, id := range ids {
		out[id] = normalizeOpsAccountRecentErrorSummary(summaries[id])
	}
	return out, nil
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
	}
	return summary
}
