package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var validOpsAlertMetricTypes = []string{
	"success_rate",
	"error_rate",
	"upstream_error_rate",
	"cpu_usage_percent",
	"memory_usage_percent",
	"concurrency_queue_depth",
	"group_available_accounts",
	"group_available_ratio",
	"group_rate_limit_ratio",
	"account_rate_limited_count",
	"account_error_count",
	"account_error_ratio",
	"account_temp_unscheduled_count",
	"overload_account_count",
	"proxy_expired_count",
	"proxy_expiring_soon_count",
	// 账号用量阈值提醒（批次 6 / A6-2 第二步）：按账号拆分评估。
	service.OpsAlertMetricAccountWindowUsedPercent,
	service.OpsAlertMetricAccountQuotaUsedPercent,
	service.OpsAlertMetricAccountBalance,
	service.OpsAlertMetricAccountTodayCost,
}

var validOpsAlertAccountWindows = []string{"5h", "7d"}
var validOpsAlertQuotaDimensions = []string{"daily", "weekly", "total"}
var validOpsAlertBalanceProviders = []string{"kimi", "deepseek"}

var validOpsAlertMetricTypeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertMetricTypes))
	for _, v := range validOpsAlertMetricTypes {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertOperators = []string{">", "<", ">=", "<=", "==", "!="}

var validOpsAlertOperatorSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertOperators))
	for _, v := range validOpsAlertOperators {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertSeverities = []string{"P0", "P1", "P2", "P3"}

var validOpsAlertSeveritySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertSeverities))
	for _, v := range validOpsAlertSeverities {
		set[v] = struct{}{}
	}
	return set
}()

type opsAlertRuleValidatedInput struct {
	Name       string
	MetricType string
	Operator   string
	Threshold  float64

	Severity string

	WindowMinutes    int
	SustainedMinutes int
	CooldownMinutes  int

	Enabled     bool
	NotifyEmail bool

	WindowProvided    bool
	SustainedProvided bool
	CooldownProvided  bool
	SeverityProvided  bool
	EnabledProvided   bool
	NotifyProvided    bool
}

func isPercentOrRateMetric(metricType string) bool {
	switch metricType {
	case "success_rate",
		"error_rate",
		"upstream_error_rate",
		"cpu_usage_percent",
		"memory_usage_percent",
		"group_available_ratio",
		"group_rate_limit_ratio",
		"account_error_ratio",
		service.OpsAlertMetricAccountWindowUsedPercent,
		service.OpsAlertMetricAccountQuotaUsedPercent:
		return true
	default:
		return false
	}
}

// validateAccountAlertFilters 校验并规范化账号用量类规则的 filters（非账号指标原样放过）：
// window / dimension 必填枚举，provider 可选且只允许 kimi / deepseek，platform 可选字符串，
// group_id 可选正整数，account_ids 可选正整数数组（去重后 ≤ 200）。通过后把这些键写回规范值。
func validateAccountAlertFilters(metricType string, filters map[string]any) error {
	if !service.IsOpsAlertAccountMetric(metricType) {
		return nil
	}
	if filters == nil {
		return fmt.Errorf("filters is required for metric_type %s", metricType)
	}

	stringFilter := func(key string) (string, error) {
		raw, ok := filters[key]
		if !ok || raw == nil {
			return "", nil
		}
		s, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("filters.%s must be a string", key)
		}
		return strings.ToLower(strings.TrimSpace(s)), nil
	}
	inList := func(value string, allowed []string) bool {
		for _, v := range allowed {
			if v == value {
				return true
			}
		}
		return false
	}

	switch metricType {
	case service.OpsAlertMetricAccountWindowUsedPercent:
		window, err := stringFilter("window")
		if err != nil {
			return err
		}
		if !inList(window, validOpsAlertAccountWindows) {
			return fmt.Errorf("filters.window must be one of: %s", strings.Join(validOpsAlertAccountWindows, ", "))
		}
		filters["window"] = window
	case service.OpsAlertMetricAccountQuotaUsedPercent:
		dimension, err := stringFilter("dimension")
		if err != nil {
			return err
		}
		if !inList(dimension, validOpsAlertQuotaDimensions) {
			return fmt.Errorf("filters.dimension must be one of: %s", strings.Join(validOpsAlertQuotaDimensions, ", "))
		}
		filters["dimension"] = dimension
	case service.OpsAlertMetricAccountBalance:
		provider, err := stringFilter("provider")
		if err != nil {
			return err
		}
		if provider != "" {
			if !inList(provider, validOpsAlertBalanceProviders) {
				return fmt.Errorf("filters.provider must be one of: %s", strings.Join(validOpsAlertBalanceProviders, ", "))
			}
			filters["provider"] = provider
		} else {
			delete(filters, "provider")
		}
	}

	platform, err := stringFilter("platform")
	if err != nil {
		return err
	}
	if platform != "" {
		filters["platform"] = platform
	} else {
		delete(filters, "platform")
	}
	if provider, _ := filters["provider"].(string); provider != "" && platform != "" && provider != platform {
		return fmt.Errorf("filters.provider %s conflicts with filters.platform %s", provider, platform)
	}

	if raw, ok := filters["group_id"]; ok && raw != nil {
		n, ok := raw.(float64)
		if !ok || n <= 0 || n != math.Trunc(n) {
			return fmt.Errorf("filters.group_id must be a positive integer")
		}
		filters["group_id"] = int64(n)
	} else {
		delete(filters, "group_id")
	}

	if raw, ok := filters["account_ids"]; ok && raw != nil {
		items, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("filters.account_ids must be an array of positive integers")
		}
		ids := make([]int64, 0, len(items))
		for _, item := range items {
			n, ok := item.(float64)
			if !ok || n <= 0 || n != math.Trunc(n) {
				return fmt.Errorf("filters.account_ids must be an array of positive integers")
			}
			ids = append(ids, int64(n))
		}
		ids = service.ParseOpsAlertAccountIDs(ids)
		if len(ids) > service.OpsAlertAccountIDsMax {
			return fmt.Errorf("filters.account_ids must contain at most %d accounts", service.OpsAlertAccountIDsMax)
		}
		if len(ids) > 0 {
			filters["account_ids"] = ids
		} else {
			delete(filters, "account_ids")
		}
	}
	return nil
}

func validateOpsAlertRulePayload(raw map[string]json.RawMessage) (*opsAlertRuleValidatedInput, error) {
	if raw == nil {
		return nil, fmt.Errorf("invalid request body")
	}

	requiredFields := []string{"name", "metric_type", "operator", "threshold"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			return nil, fmt.Errorf("%s is required", field)
		}
	}

	var name string
	if err := json.Unmarshal(raw["name"], &name); err != nil || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	name = strings.TrimSpace(name)

	var metricType string
	if err := json.Unmarshal(raw["metric_type"], &metricType); err != nil || strings.TrimSpace(metricType) == "" {
		return nil, fmt.Errorf("metric_type is required")
	}
	metricType = strings.TrimSpace(metricType)
	if _, ok := validOpsAlertMetricTypeSet[metricType]; !ok {
		return nil, fmt.Errorf("metric_type must be one of: %s", strings.Join(validOpsAlertMetricTypes, ", "))
	}

	var operator string
	if err := json.Unmarshal(raw["operator"], &operator); err != nil || strings.TrimSpace(operator) == "" {
		return nil, fmt.Errorf("operator is required")
	}
	operator = strings.TrimSpace(operator)
	if _, ok := validOpsAlertOperatorSet[operator]; !ok {
		return nil, fmt.Errorf("operator must be one of: %s", strings.Join(validOpsAlertOperators, ", "))
	}

	var threshold float64
	if err := json.Unmarshal(raw["threshold"], &threshold); err != nil {
		return nil, fmt.Errorf("threshold must be a number")
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil, fmt.Errorf("threshold must be a finite number")
	}
	if isPercentOrRateMetric(metricType) {
		if threshold < 0 || threshold > 100 {
			return nil, fmt.Errorf("threshold must be between 0 and 100 for metric_type %s", metricType)
		}
	} else if threshold < 0 {
		return nil, fmt.Errorf("threshold must be >= 0")
	}

	validated := &opsAlertRuleValidatedInput{
		Name:       name,
		MetricType: metricType,
		Operator:   operator,
		Threshold:  threshold,
	}

	if v, ok := raw["severity"]; ok {
		validated.SeverityProvided = true
		var sev string
		if err := json.Unmarshal(v, &sev); err != nil {
			return nil, fmt.Errorf("severity must be a string")
		}
		sev = strings.ToUpper(strings.TrimSpace(sev))
		if sev != "" {
			if _, ok := validOpsAlertSeveritySet[sev]; !ok {
				return nil, fmt.Errorf("severity must be one of: %s", strings.Join(validOpsAlertSeverities, ", "))
			}
			validated.Severity = sev
		}
	}
	if validated.Severity == "" {
		validated.Severity = "P2"
	}

	if v, ok := raw["enabled"]; ok {
		validated.EnabledProvided = true
		if err := json.Unmarshal(v, &validated.Enabled); err != nil {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
	} else {
		validated.Enabled = true
	}

	if v, ok := raw["notify_email"]; ok {
		validated.NotifyProvided = true
		if err := json.Unmarshal(v, &validated.NotifyEmail); err != nil {
			return nil, fmt.Errorf("notify_email must be a boolean")
		}
	} else {
		validated.NotifyEmail = true
	}

	if v, ok := raw["window_minutes"]; ok {
		validated.WindowProvided = true
		if err := json.Unmarshal(v, &validated.WindowMinutes); err != nil {
			return nil, fmt.Errorf("window_minutes must be an integer")
		}
		switch validated.WindowMinutes {
		case 1, 5, 60:
		default:
			return nil, fmt.Errorf("window_minutes must be one of: 1, 5, 60")
		}
	} else {
		validated.WindowMinutes = 1
	}

	if v, ok := raw["sustained_minutes"]; ok {
		validated.SustainedProvided = true
		if err := json.Unmarshal(v, &validated.SustainedMinutes); err != nil {
			return nil, fmt.Errorf("sustained_minutes must be an integer")
		}
		if validated.SustainedMinutes < 1 || validated.SustainedMinutes > 1440 {
			return nil, fmt.Errorf("sustained_minutes must be between 1 and 1440")
		}
	} else {
		validated.SustainedMinutes = 1
	}

	if v, ok := raw["cooldown_minutes"]; ok {
		validated.CooldownProvided = true
		if err := json.Unmarshal(v, &validated.CooldownMinutes); err != nil {
			return nil, fmt.Errorf("cooldown_minutes must be an integer")
		}
		if validated.CooldownMinutes < 0 || validated.CooldownMinutes > 1440 {
			return nil, fmt.Errorf("cooldown_minutes must be between 0 and 1440")
		}
	} else {
		validated.CooldownMinutes = 0
	}

	return validated, nil
}

// ListAlertRules returns all ops alert rules.
// GET /api/v1/admin/ops/alert-rules
func (h *OpsHandler) ListAlertRules(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rules, err := h.opsService.ListAlertRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

// CreateAlertRule creates an ops alert rule.
// POST /api/v1/admin/ops/alert-rules
func (h *OpsHandler) CreateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail
	if err := validateAccountAlertFilters(rule.MetricType, rule.Filters); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	created, err := h.opsService.CreateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

// UpdateAlertRule updates an existing ops alert rule.
// PUT /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) UpdateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	rule.ID = id
	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail
	if err := validateAccountAlertFilters(rule.MetricType, rule.Filters); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	updated, err := h.opsService.UpdateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// DeleteAlertRule deletes an ops alert rule.
// DELETE /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) DeleteAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}

	if err := h.opsService.DeleteAlertRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// EvaluateAlertRule 立即试算一条规则（不落事件、不动持续计数），body `{"send": bool}` 可省略；
// send=true 时把结果以「手动试发」推到 Bark：未启用回 400 BARK_NOT_ENABLED，推送失败仍回 200 且 sent=false。
// POST /api/v1/admin/ops/alert-rules/:id/evaluate
func (h *OpsHandler) EvaluateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}

	var payload struct {
		Send bool `json:"send"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request body")
		return
	}

	result, err := h.opsService.EvaluateAlertRuleNow(c.Request.Context(), id, payload.Send)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetAlertEvent returns a single ops alert event.
// GET /api/v1/admin/ops/alert-events/:id
func (h *OpsHandler) GetAlertEvent(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
	}

	ev, err := h.opsService.GetAlertEventByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, ev)
}

// UpdateAlertEventStatus updates an ops alert event status.
// PUT /api/v1/admin/ops/alert-events/:id/status
func (h *OpsHandler) UpdateAlertEventStatus(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	payload.Status = strings.TrimSpace(payload.Status)
	if payload.Status == "" {
		response.BadRequest(c, "Invalid status")
		return
	}
	if payload.Status != service.OpsAlertStatusResolved && payload.Status != service.OpsAlertStatusManualResolved {
		response.BadRequest(c, "Invalid status")
		return
	}

	var resolvedAt *time.Time
	if payload.Status == service.OpsAlertStatusResolved || payload.Status == service.OpsAlertStatusManualResolved {
		now := time.Now().UTC()
		resolvedAt = &now
	}
	if err := h.opsService.UpdateAlertEventStatus(c.Request.Context(), id, payload.Status, resolvedAt); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// ListAlertEvents lists recent ops alert events.
// GET /api/v1/admin/ops/alert-events
// CreateAlertSilence creates a scoped silence for ops alerts.
// POST /api/v1/admin/ops/alert-silences
func (h *OpsHandler) CreateAlertSilence(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var payload struct {
		RuleID   int64   `json:"rule_id"`
		Platform string  `json:"platform"`
		GroupID  *int64  `json:"group_id"`
		Region   *string `json:"region"`
		Until    string  `json:"until"`
		Reason   string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.Until))
	if err != nil {
		response.BadRequest(c, "Invalid until")
		return
	}

	createdBy := (*int64)(nil)
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		uid := subject.UserID
		createdBy = &uid
	}

	silence := &service.OpsAlertSilence{
		RuleID:    payload.RuleID,
		Platform:  strings.TrimSpace(payload.Platform),
		GroupID:   payload.GroupID,
		Region:    payload.Region,
		Until:     until,
		Reason:    strings.TrimSpace(payload.Reason),
		CreatedBy: createdBy,
	}

	created, err := h.opsService.CreateAlertSilence(c.Request.Context(), silence)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

func (h *OpsHandler) ListAlertEvents(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
		}
		limit = n
	}

	filter := &service.OpsAlertEventFilter{
		Limit:    limit,
		Status:   strings.TrimSpace(c.Query("status")),
		Severity: strings.TrimSpace(c.Query("severity")),
	}

	if v := strings.TrimSpace(c.Query("email_sent")); v != "" {
		vv := strings.ToLower(v)
		switch vv {
		case "true", "1":
			b := true
			filter.EmailSent = &b
		case "false", "0":
			b := false
			filter.EmailSent = &b
		default:
			response.BadRequest(c, "Invalid email_sent")
			return
		}
	}

	// Cursor pagination: both params must be provided together.
	rawTS := strings.TrimSpace(c.Query("before_fired_at"))
	rawID := strings.TrimSpace(c.Query("before_id"))
	if (rawTS == "") != (rawID == "") {
		response.BadRequest(c, "before_fired_at and before_id must be provided together")
		return
	}
	if rawTS != "" {
		ts, err := time.Parse(time.RFC3339Nano, rawTS)
		if err != nil {
			if t2, err2 := time.Parse(time.RFC3339, rawTS); err2 == nil {
				ts = t2
			} else {
				response.BadRequest(c, "Invalid before_fired_at")
				return
			}
		}
		filter.BeforeFiredAt = &ts
	}
	if rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid before_id")
			return
		}
		filter.BeforeID = &id
	}

	// Optional global filter support (platform/group/time range).
	if platform := strings.TrimSpace(c.Query("platform")); platform != "" {
		filter.Platform = platform
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	if startTime, endTime, err := parseOpsTimeRange(c, "24h"); err == nil {
		// Only apply when explicitly provided to avoid surprising default narrowing.
		if strings.TrimSpace(c.Query("start_time")) != "" || strings.TrimSpace(c.Query("end_time")) != "" || strings.TrimSpace(c.Query("time_range")) != "" {
			filter.StartTime = &startTime
			filter.EndTime = &endTime
		}
	} else {
		response.BadRequest(c, err.Error())
		return
	}

	events, err := h.opsService.ListAlertEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, events)
}
