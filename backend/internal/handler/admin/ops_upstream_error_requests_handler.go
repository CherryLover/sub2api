package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ListUpstreamErrorRequests lists upstream 502/503 requests, including recovered retries.
func (h *OpsHandler) ListUpstreamErrorRequests(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	f := &service.OpsUpstreamErrorRequestFilter{Outcome: c.Query("outcome")}
	for _, p := range []struct {
		name   string
		target *int
	}{{"page", &f.Page}, {"page_size", &f.PageSize}, {"status_code", &f.StatusCode}} {
		if raw := c.Query(p.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				response.BadRequest(c, "Invalid "+p.name)
				return
			}
			*p.target = n
		}
	}
	for _, p := range []struct {
		name   string
		target *int64
	}{{"api_key_id", &f.APIKeyID}, {"account_id", &f.AccountID}} {
		if raw := c.Query(p.name); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				response.BadRequest(c, "Invalid "+p.name)
				return
			}
			*p.target = n
		}
	}
	for _, p := range []struct {
		name   string
		target *time.Time
	}{{"start_time", &f.StartTime}, {"end_time", &f.EndTime}} {
		if raw := c.Query(p.name); raw != "" {
			v, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				response.BadRequest(c, "Invalid "+p.name)
				return
			}
			*p.target = v
		}
	}
	result, err := h.opsService.ListUpstreamErrorRequests(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
