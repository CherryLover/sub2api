package service

import (
	"context"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// OpsUpstreamErrorRequestFilter selects requests with matching upstream attempts.
type OpsUpstreamErrorRequestFilter struct {
	Page       int
	PageSize   int
	StatusCode int
	APIKeyID   int64
	AccountID  int64
	StartTime  time.Time
	EndTime    time.Time
	Outcome    string
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
type OpsUpstreamErrorRequestReader interface {
	ListUpstreamErrorRequests(context.Context, *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestList, error)
}

func (s *OpsService) ListUpstreamErrorRequests(ctx context.Context, f *OpsUpstreamErrorRequestFilter) (*OpsUpstreamErrorRequestList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if f == nil {
		f = &OpsUpstreamErrorRequestFilter{}
	}
	if f.Page > 10000000 {
		return nil, infraerrors.BadRequest("OPS_PAGE_INVALID", "page exceeds maximum")
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
	if f.StatusCode != 0 && f.StatusCode != 502 && f.StatusCode != 503 {
		return nil, infraerrors.BadRequest("OPS_STATUS_INVALID", "status_code must be 502 or 503")
	}
	if f.Outcome != "" && f.Outcome != "recovered" && f.Outcome != "failed" {
		return nil, infraerrors.BadRequest("OPS_OUTCOME_INVALID", "Invalid outcome")
	}
	if f.EndTime.IsZero() {
		f.EndTime = time.Now()
	}
	if f.StartTime.IsZero() {
		f.StartTime = f.EndTime.Add(-24 * time.Hour)
	}
	if !f.StartTime.Before(f.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be before end_time")
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
