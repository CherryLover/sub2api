package service

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpsUpstreamErrorRequestFilter selects requests with matching upstream attempts.
type OpsUpstreamErrorRequestFilter struct {
	Page            int
	PageSize        int
	StatusCode      int
	FinalStatusCode int
	APIKeyID        int64
	AccountID       int64
	StartTime       time.Time
	EndTime         time.Time
	Outcome         string
}
type OpsUpstreamErrorRequestFinal struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
}
type OpsUpstreamErrorRequest struct {
	ID               int64                         `json:"id"`
	CreatedAt        time.Time                     `json:"created_at"`
	ClientRequestID  string                        `json:"client_request_id"`
	RequestID        string                        `json:"request_id"`
	APIKeyID         *int64                        `json:"api_key_id"`
	APIKeyName       string                        `json:"api_key_name"`
	AccountID        *int64                        `json:"account_id"`
	AccountName      string                        `json:"account_name"`
	Model            string                        `json:"model"`
	ReasoningEffort  string                        `json:"reasoning_effort"`
	RequestBodyBytes *int64                        `json:"request_body_bytes"`
	ClientStatusCode int                           `json:"client_status_code"`
	Outcome          string                        `json:"outcome"`
	Attempts         []*OpsUpstreamErrorEvent      `json:"attempts"`
	Final            *OpsUpstreamErrorRequestFinal `json:"final"`
}
type OpsUpstreamErrorRequestList struct {
	Items    []*OpsUpstreamErrorRequest `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}
type OpsUpstreamErrorStatusCount struct {
	StatusCode int   `json:"status_code"`
	Count      int64 `json:"count"`
}
type OpsUpstreamErrorRequestSummary struct {
	Total          int64                         `json:"total"`
	Recovered      int64                         `json:"recovered"`
	Failed         int64                         `json:"failed"`
	Attempts       int64                         `json:"attempts"`
	FailedByStatus []OpsUpstreamErrorStatusCount `json:"failed_by_status"`
}
type OpsUpstreamErrorTrendDay struct {
	Date      string `json:"date"`
	Total     int64  `json:"total"`
	Recovered int64  `json:"recovered"`
	Failed    int64  `json:"failed"`
}
type OpsUpstreamErrorTrend struct {
	Days []*OpsUpstreamErrorTrendDay `json:"days"`
}
type OpsUpstreamErrorRequestReader interface {
	ListUpstreamErrorRequests(context.Context, *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestList, error)
}
type OpsUpstreamErrorRequestSummaryReader interface {
	SummarizeUpstreamErrorRequests(context.Context, *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestSummary, error)
}
type OpsUpstreamErrorRequestTrendReader interface {
	GetUpstreamErrorRequestTrend(context.Context, *OpsUpstreamErrorRequestFilter) ([]*OpsUpstreamErrorTrendDay, error)
}

func (s *OpsService) ListUpstreamErrorRequests(ctx context.Context, f *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if f == nil {
		f = &OpsUpstreamErrorRequestFilter{}
	}
	if err := normalizeOpsUpstreamErrorRequestFilter(f); err != nil {
		return nil, err
	}
	reader, ok := s.opsRepo.(OpsUpstreamErrorRequestReader)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Upstream error lookup unavailable")
	}
	result, err := reader.ListUpstreamErrorRequests(ctx, f)
	if err != nil {
		return nil, err
	}
	// Reuse storage sanitization on read as well, including historical rows.
	for _, item := range result.Items {
		item.Attempts, err = sanitizeUpstreamRequestAttempts(item.Attempts)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func normalizeOpsUpstreamErrorRequestFilter(f *OpsUpstreamErrorRequestFilter) error {
	if f == nil {
		return nil
	}
	if f.Page > 10000000 {
		return infraerrors.BadRequest("OPS_PAGE_INVALID", "page exceeds maximum")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.StatusCode != 0 && (f.StatusCode < 100 || f.StatusCode > 599) {
		return infraerrors.BadRequest("OPS_STATUS_INVALID", "status_code must be between 100 and 599")
	}
	if f.FinalStatusCode != 0 && (f.FinalStatusCode < 100 || f.FinalStatusCode > 599) {
		return infraerrors.BadRequest("OPS_FINAL_STATUS_INVALID", "final_status_code must be between 100 and 599")
	}
	if f.Outcome != "" && f.Outcome != "recovered" && f.Outcome != "failed" {
		return infraerrors.BadRequest("OPS_OUTCOME_INVALID", "invalid outcome")
	}
	if f.EndTime.IsZero() {
		f.EndTime = time.Now()
	}
	if f.StartTime.IsZero() {
		f.StartTime = f.EndTime.Add(-24 * time.Hour)
	}
	if !f.StartTime.Before(f.EndTime) {
		return infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be before end_time")
	}
	if f.EndTime.Sub(f.StartTime) > 31*24*time.Hour {
		return infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "time range must not exceed 31 days")
	}
	return nil
}

func (s *OpsService) SummarizeUpstreamErrorRequests(ctx context.Context, f *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestSummary, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if f == nil {
		f = &OpsUpstreamErrorRequestFilter{}
	}
	if err := normalizeOpsUpstreamErrorRequestFilter(f); err != nil {
		return nil, err
	}
	reader, ok := s.opsRepo.(OpsUpstreamErrorRequestSummaryReader)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Upstream error lookup unavailable")
	}
	result, err := reader.SummarizeUpstreamErrorRequests(ctx, f)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &OpsUpstreamErrorRequestSummary{}
	}
	if result.FailedByStatus == nil {
		result.FailedByStatus = []OpsUpstreamErrorStatusCount{}
	}
	return result, nil
}

var opsUpstreamErrorTrendLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func opsUpstreamErrorTrendWindow(days int, now time.Time) (today, start, end time.Time) {
	now = now.In(opsUpstreamErrorTrendLocation)
	today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, opsUpstreamErrorTrendLocation)
	return today, today.AddDate(0, 0, -(days - 1)), today.AddDate(0, 0, 1)
}

func (s *OpsService) GetUpstreamErrorRequestTrend(ctx context.Context, f *OpsUpstreamErrorRequestFilter, days int) (*OpsUpstreamErrorTrend, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if days != 7 && days != 14 && days != 30 {
		return nil, infraerrors.BadRequest("OPS_TREND_DAYS_INVALID", "days must be 7, 14, or 30")
	}
	if f == nil {
		f = &OpsUpstreamErrorRequestFilter{}
	}
	today, start, end := opsUpstreamErrorTrendWindow(days, time.Now())
	f.StartTime = start
	f.EndTime = end
	if err := normalizeOpsUpstreamErrorRequestFilter(f); err != nil {
		return nil, err
	}
	reader, ok := s.opsRepo.(OpsUpstreamErrorRequestTrendReader)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Upstream error lookup unavailable")
	}
	rows, err := reader.GetUpstreamErrorRequestTrend(ctx, f)
	if err != nil {
		return nil, err
	}
	return &OpsUpstreamErrorTrend{Days: fillOpsUpstreamErrorTrend(days, today, rows)}, nil
}

func fillOpsUpstreamErrorTrend(days int, today time.Time, rows []*OpsUpstreamErrorTrendDay) []*OpsUpstreamErrorTrendDay {
	byDate := make(map[string]*OpsUpstreamErrorTrendDay, len(rows))
	for _, row := range rows {
		if row != nil {
			byDate[row.Date] = row
		}
	}
	out := make([]*OpsUpstreamErrorTrendDay, 0, days)
	for i := days - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		point := byDate[date]
		if point == nil {
			point = &OpsUpstreamErrorTrendDay{Date: date}
		}
		out = append(out, point)
	}
	return out
}

var opsAttemptBearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
var opsAttemptCredentialPattern = regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|password|secret|cookie)["']?\s*[:=]\s*["']?)[^\s"'&,;\x7d]+`)

func redactOpsAttemptText(raw string) string {
	return opsAttemptCredentialPattern.ReplaceAllString(opsAttemptBearerPattern.ReplaceAllString(raw, "Bearer [REDACTED]"), "${1}[REDACTED]")
}

func sanitizeUpstreamRequestAttempts(events []*OpsUpstreamErrorEvent) ([]*OpsUpstreamErrorEvent, error) {
	clean := make([]*OpsUpstreamErrorEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		ev := *event
		ev.Message = redactOpsAttemptText(ev.Message)
		ev.Detail = redactOpsAttemptText(ev.Detail)
		ev.UpstreamResponseBody = redactOpsAttemptText(ev.UpstreamResponseBody)
		parsed, err := url.Parse(strings.TrimSpace(ev.UpstreamURL))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			ev.UpstreamURL = ""
		} else {
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.ForceQuery = false
			parsed.Fragment = ""
			parsed.RawFragment = ""
			ev.UpstreamURL = parsed.String()
		}
		clean = append(clean, &ev)
	}
	in := &OpsInsertErrorLogInput{UpstreamErrors: clean}
	if err := sanitizeOpsUpstreamErrors(in); err != nil {
		return nil, err
	}
	if in.UpstreamErrorsJSON == nil {
		return []*OpsUpstreamErrorEvent{}, nil
	}
	return ParseOpsUpstreamErrors(*in.UpstreamErrorsJSON)
}
