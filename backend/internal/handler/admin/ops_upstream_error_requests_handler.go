package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ListUpstreamErrorRequests lists requests with at least one upstream failure,
// including recovered retries.
func (h *OpsHandler) ListUpstreamErrorRequests(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	f, err := parseOpsUpstreamErrorRequestFilter(c, true)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.opsService.ListUpstreamErrorRequests(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetUpstreamErrorRequestsSummary returns request and failed-attempt aggregates
// for the same filters as the list endpoint.
func (h *OpsHandler) GetUpstreamErrorRequestsSummary(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	f, err := parseOpsUpstreamErrorRequestFilter(c, true)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.opsService.SummarizeUpstreamErrorRequests(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetUpstreamErrorRequestsTrend returns Shanghai-calendar daily buckets. The
// page's start_time/end_time are intentionally ignored in favor of days.
func (h *OpsHandler) GetUpstreamErrorRequestsTrend(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(c.Query("days")))
	if err != nil || (days != 7 && days != 14 && days != 30) {
		response.BadRequest(c, "Invalid days")
		return
	}
	f, err := parseOpsUpstreamErrorRequestFilter(c, false)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.opsService.GetUpstreamErrorRequestTrend(c.Request.Context(), f, days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseOpsUpstreamErrorRequestFilter(c *gin.Context, includeTime bool) (*service.OpsUpstreamErrorRequestFilter, error) {
	f := &service.OpsUpstreamErrorRequestFilter{Outcome: c.Query("outcome")}
	for _, p := range []struct {
		name   string
		target *int
	}{{"page", &f.Page}, {"page_size", &f.PageSize}} {
		if raw := strings.TrimSpace(c.Query(p.name)); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				return nil, opsFilterParamError("Invalid " + p.name)
			}
			*p.target = n
		}
	}
	for _, p := range []struct {
		name   string
		target *int
	}{{"status_code", &f.StatusCode}, {"final_status_code", &f.FinalStatusCode}} {
		if raw := strings.TrimSpace(c.Query(p.name)); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 100 || n > 599 {
				return nil, opsFilterParamError("Invalid " + p.name)
			}
			*p.target = n
		}
	}
	for _, p := range []struct {
		name   string
		target *int64
	}{{"api_key_id", &f.APIKeyID}, {"account_id", &f.AccountID}} {
		if raw := strings.TrimSpace(c.Query(p.name)); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				return nil, opsFilterParamError("Invalid " + p.name)
			}
			*p.target = n
		}
	}
	if includeTime {
		for _, p := range []struct {
			name   string
			target *time.Time
		}{{"start_time", &f.StartTime}, {"end_time", &f.EndTime}} {
			if raw := strings.TrimSpace(c.Query(p.name)); raw != "" {
				v, err := time.Parse(time.RFC3339, raw)
				if err != nil {
					return nil, opsFilterParamError("Invalid " + p.name)
				}
				*p.target = v
			}
		}
	}
	return f, nil
}
