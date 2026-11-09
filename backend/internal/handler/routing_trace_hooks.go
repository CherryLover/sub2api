package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// routingTraceGinKey keeps the request's routing trace on the gin context so
// the ops error logger can read it after the handler returns.
const routingTraceGinKey = "routing_trace"

// newRoutingTraceForRequest builds a trace for one gateway request.
func newRoutingTraceForRequest(ctx context.Context, apiKey *service.APIKey, model string) *service.RoutingTrace {
	meta := service.RoutingTraceMeta{Model: strings.TrimSpace(model)}
	if ctx != nil {
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); requestID != "" {
			meta.RequestID = strings.TrimSpace(requestID)
		}
	}
	if apiKey != nil {
		meta.APIKeyID = apiKey.ID
		if apiKey.GroupID != nil {
			meta.GroupID = *apiKey.GroupID
		}
		if apiKey.Group != nil {
			meta.GroupName = apiKey.Group.Name
		}
	}
	return service.NewRoutingTrace(meta)
}

// startRoutingTrace attaches a routing trace to the request and emits
// request_start. Callers defer finishRoutingTrace.
func startRoutingTrace(c *gin.Context, apiKey *service.APIKey, model string) *service.RoutingTrace {
	if c == nil || c.Request == nil {
		return nil
	}
	trace := newRoutingTraceForRequest(c.Request.Context(), apiKey, model)
	c.Request = c.Request.WithContext(service.WithRoutingTrace(c.Request.Context(), trace))
	c.Set(routingTraceGinKey, trace)
	trace.EmitStart()
	return trace
}

// finishRoutingTrace closes the trace with the status written to the client.
func finishRoutingTrace(c *gin.Context, trace *service.RoutingTrace) {
	if trace == nil {
		return
	}
	status := 0
	if c != nil && c.Writer != nil && c.Writer.Written() {
		status = c.Writer.Status()
	}
	trace.Finish(status)
}

// routingTraceFromGin returns the trace stored by startRoutingTrace.
func routingTraceFromGin(c *gin.Context) *service.RoutingTrace {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(routingTraceGinKey); ok {
		if trace, ok := v.(*service.RoutingTrace); ok {
			return trace
		}
	}
	if c.Request != nil {
		return service.RoutingTraceFromContext(c.Request.Context())
	}
	return nil
}

// endRoutingAttempt closes the current upstream attempt from the forward
// result: status comes from the failover error when there is one, success
// uses the status written to the client.
func endRoutingAttempt(c *gin.Context, trace *service.RoutingTrace, err error) {
	if trace == nil {
		return
	}
	if err == nil {
		status := 200
		if c != nil && c.Writer != nil && c.Writer.Written() && c.Writer.Status() > 0 {
			status = c.Writer.Status()
		}
		trace.EndAttempt(status, false, nil)
		return
	}
	status, cooldown := routingAttemptFailure(err)
	trace.EndAttempt(status, true, cooldown)
}

// routingAttemptFailure extracts the upstream status and a 429 cooldown from
// a forward error. Status 0 means no upstream response (network error).
func routingAttemptFailure(err error) (int, *time.Time) {
	var failoverErr *service.UpstreamFailoverError
	if errors.As(err, &failoverErr) && failoverErr != nil {
		return failoverErr.StatusCode, service.RoutingCooldownFromHeaders(failoverErr.StatusCode, failoverErr.ResponseHeaders, time.Now())
	}
	return 0, nil
}

// applyOpsRoutingDiagnosisFromContext attaches the routing diagnosis of a
// failed request to its ops error log entry.
func applyOpsRoutingDiagnosisFromContext(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil {
		return
	}
	trace := routingTraceFromGin(c)
	if trace == nil {
		return
	}
	entry.RoutingDiagnosis = trace.Diagnosis(entry.ErrorReason)
}

// estimateRoutingDiagnosisBytes approximates the JSON size of d for queue
// byte accounting without marshalling it.
func estimateRoutingDiagnosisBytes(d *service.RoutingDiagnosis) int {
	if d == nil {
		return 0
	}
	size := 128 + len(d.GroupName) + len(d.Model) + len(d.Reason) + len(d.Raw)
	for i := range d.Candidates {
		size += 192 + len(d.Candidates[i].AccountName) + len(d.Candidates[i].Reason)
	}
	for i := range d.Attempts {
		size += 112 + len(d.Attempts[i].AccountName)
	}
	return size
}
