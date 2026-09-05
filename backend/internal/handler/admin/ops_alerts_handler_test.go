package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// opsAlertsEnvelope 带 reason 的响应外壳（错误码断言用）。
type opsAlertsEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Reason  string          `json:"reason"`
	Data    json.RawMessage `json:"data"`
}

// opsAlertRulesCaptureRepo 只捕获 Create / Update 拿到的规则（filters 已被 handler 规范化）。
type opsAlertRulesCaptureRepo struct {
	service.OpsRepository
	created *service.OpsAlertRule
	updated *service.OpsAlertRule
}

func (r *opsAlertRulesCaptureRepo) CreateAlertRule(_ context.Context, rule *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	r.created = rule
	rule.ID = 1
	return rule, nil
}

func (r *opsAlertRulesCaptureRepo) UpdateAlertRule(_ context.Context, rule *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	r.updated = rule
	return rule, nil
}

type stubAlertRuleEvaluator struct {
	result  *service.OpsAlertRuleEvaluation
	err     error
	gotID   int64
	gotSend bool
	calls   int
}

func (s *stubAlertRuleEvaluator) EvaluateRuleNow(_ context.Context, ruleID int64, send bool) (*service.OpsAlertRuleEvaluation, error) {
	s.calls++
	s.gotID = ruleID
	s.gotSend = send
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func newOpsAlertsTestRouter(svc *service.OpsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewOpsHandler(svc)
	r := gin.New()
	r.POST("/alert-rules", h.CreateAlertRule)
	r.PUT("/alert-rules/:id", h.UpdateAlertRule)
	r.POST("/alert-rules/:id/evaluate", h.EvaluateAlertRule)
	return r
}

func doOpsAlertsJSON(t *testing.T, r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, opsAlertsEnvelope) {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	var env opsAlertsEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	return w, env
}

func newOpsAlertsService(repo service.OpsRepository) *service.OpsService {
	return service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestOpsAlertsHandler_CreateRejectsUnknownMetric(t *testing.T) {
	repo := &opsAlertRulesCaptureRepo{}
	r := newOpsAlertsTestRouter(newOpsAlertsService(repo))

	w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", `{"name":"x","metric_type":"account_moon_phase","operator":">","threshold":1}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, env.Message, "metric_type must be one of")
	require.Contains(t, env.Message, "account_window_used_percent")
	require.Nil(t, repo.created)
}

func TestOpsAlertsHandler_CreateAccountRuleNormalizesFilters(t *testing.T) {
	repo := &opsAlertRulesCaptureRepo{}
	r := newOpsAlertsTestRouter(newOpsAlertsService(repo))

	body := `{"name":"Codex 5h","metric_type":"account_window_used_percent","operator":">=","threshold":80,
		"filters":{"window":" 5H ","platform":"OpenAI","group_id":3,"account_ids":[2,1,2]}}`
	w, _ := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, repo.created)
	require.Equal(t, "account_window_used_percent", repo.created.MetricType)
	require.Equal(t, "5h", repo.created.Filters["window"])
	require.Equal(t, "openai", repo.created.Filters["platform"])
	require.Equal(t, int64(3), repo.created.Filters["group_id"])
	require.Equal(t, []int64{2, 1}, repo.created.Filters["account_ids"])
}

func TestOpsAlertsHandler_AccountFilterValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "window 缺失",
			body:    `{"name":"a","metric_type":"account_window_used_percent","operator":">=","threshold":80,"filters":{"platform":"openai"}}`,
			wantMsg: "filters.window must be one of: 5h, 7d",
		},
		{
			name:    "filters 整个缺失",
			body:    `{"name":"a","metric_type":"account_window_used_percent","operator":">=","threshold":80}`,
			wantMsg: "filters is required",
		},
		{
			name:    "window 非法",
			body:    `{"name":"a","metric_type":"account_window_used_percent","operator":">=","threshold":80,"filters":{"window":"1h"}}`,
			wantMsg: "filters.window must be one of",
		},
		{
			name:    "dimension 非法",
			body:    `{"name":"a","metric_type":"account_quota_used_percent","operator":">=","threshold":80,"filters":{"dimension":"monthly"}}`,
			wantMsg: "filters.dimension must be one of: daily, weekly, total",
		},
		{
			name:    "provider 非法",
			body:    `{"name":"a","metric_type":"account_balance","operator":"<","threshold":5,"filters":{"provider":"zhipu"}}`,
			wantMsg: "filters.provider must be one of: kimi, deepseek",
		},
		{
			name:    "provider 与 platform 冲突",
			body:    `{"name":"a","metric_type":"account_balance","operator":"<","threshold":5,"filters":{"provider":"kimi","platform":"deepseek"}}`,
			wantMsg: "conflicts with filters.platform",
		},
		{
			name:    "account_ids 含非整数",
			body:    `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"account_ids":[1,"x"]}}`,
			wantMsg: "filters.account_ids must be an array of positive integers",
		},
		{
			name:    "account_ids 含非正数",
			body:    `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"account_ids":[0]}}`,
			wantMsg: "filters.account_ids must be an array of positive integers",
		},
		{
			name:    "account_ids 不是数组",
			body:    `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"account_ids":"1,2"}}`,
			wantMsg: "filters.account_ids must be an array of positive integers",
		},
		{
			name:    "group_id 非法",
			body:    `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"group_id":"abc"}}`,
			wantMsg: "filters.group_id must be a positive integer",
		},
		{
			name:    "百分比指标阈值超过 100",
			body:    `{"name":"a","metric_type":"account_quota_used_percent","operator":">=","threshold":120,"filters":{"dimension":"daily"}}`,
			wantMsg: "threshold must be between 0 and 100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &opsAlertRulesCaptureRepo{}
			r := newOpsAlertsTestRouter(newOpsAlertsService(repo))
			w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", tt.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, env.Message, tt.wantMsg)
			require.Nil(t, repo.created)
		})
	}
}

func TestOpsAlertsHandler_AccountIDsCappedAt200(t *testing.T) {
	ids := make([]string, 0, 201)
	for i := 1; i <= 201; i++ {
		ids = append(ids, strconv.Itoa(i))
	}
	body := `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"account_ids":[` + strings.Join(ids, ",") + `]}}`

	repo := &opsAlertRulesCaptureRepo{}
	r := newOpsAlertsTestRouter(newOpsAlertsService(repo))
	w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", body)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, env.Message, "at most 200 accounts")

	// 恰好 200 个可以通过。
	body = `{"name":"a","metric_type":"account_today_cost","operator":">","threshold":10,"filters":{"account_ids":[` + strings.Join(ids[:200], ",") + `]}}`
	w, _ = doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	accountIDs, ok := repo.created.Filters["account_ids"].([]int64)
	require.True(t, ok)
	require.Len(t, accountIDs, 200)
}

func TestOpsAlertsHandler_UpdateAlsoValidatesAccountFilters(t *testing.T) {
	repo := &opsAlertRulesCaptureRepo{}
	r := newOpsAlertsTestRouter(newOpsAlertsService(repo))

	w, env := doOpsAlertsJSON(t, r, http.MethodPut, "/alert-rules/5", `{"name":"a","metric_type":"account_balance","operator":"<","threshold":5,"filters":{"provider":"openai"}}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, env.Message, "filters.provider must be one of")
	require.Nil(t, repo.updated)

	w, _ = doOpsAlertsJSON(t, r, http.MethodPut, "/alert-rules/5", `{"name":"a","metric_type":"account_balance","operator":"<","threshold":5,"filters":{"provider":"Kimi"}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, repo.updated)
	require.Equal(t, int64(5), repo.updated.ID)
	require.Equal(t, "kimi", repo.updated.Filters["provider"])
}

func TestOpsAlertsHandler_HealthMetricFiltersUntouched(t *testing.T) {
	repo := &opsAlertRulesCaptureRepo{}
	r := newOpsAlertsTestRouter(newOpsAlertsService(repo))

	w, _ := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules", `{"name":"cpu","metric_type":"cpu_usage_percent","operator":">","threshold":90,"filters":{"platform":"OpenAI","group_id":3}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "OpenAI", repo.created.Filters["platform"], "健康度指标的 filters 原样透传")
	require.Equal(t, float64(3), repo.created.Filters["group_id"])
}

func TestOpsAlertsHandler_Evaluate(t *testing.T) {
	value := 85.0
	okResult := &service.OpsAlertRuleEvaluation{
		RuleID: 7, RuleName: "Codex 5h", MetricType: "account_window_used_percent", Operator: ">=", Threshold: 80,
		HasData: true, Value: &value, Breached: true,
		Accounts: []service.OpsAlertAccountSample{{AccountID: 1, AccountName: "a", Platform: "openai", Value: 85, Breached: true}},
		Sent:     true,
	}

	t.Run("200：透传 send=true", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{result: okResult}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{"send":true}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, int64(7), ev.gotID)
		require.True(t, ev.gotSend)

		var data map[string]any
		require.NoError(t, json.Unmarshal(env.Data, &data))
		require.Equal(t, float64(7), data["rule_id"])
		require.Equal(t, "Codex 5h", data["rule_name"])
		require.Equal(t, true, data["has_data"])
		require.Equal(t, 85.0, data["value"])
		require.Equal(t, true, data["breached"])
		require.Equal(t, true, data["sent"])
		require.NotContains(t, data, "send_error")
		accounts, ok := data["accounts"].([]any)
		require.True(t, ok)
		require.Len(t, accounts, 1)
	})

	t.Run("200：body 省略等于 send=false；无数据时 value 为 null、accounts 为空数组", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{result: &service.OpsAlertRuleEvaluation{RuleID: 7, Accounts: []service.OpsAlertAccountSample{}}}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.False(t, ev.gotSend)
		raw := string(env.Data)
		require.Contains(t, raw, `"value":null`)
		require.Contains(t, raw, `"accounts":[]`)
		require.Contains(t, raw, `"has_data":false`)
	})

	t.Run("200：推送失败带 send_error", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{result: &service.OpsAlertRuleEvaluation{RuleID: 7, HasData: true, Value: &value, Accounts: []service.OpsAlertAccountSample{}, Sent: false, SendError: "bark unreachable"}}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{"send":true}`)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, string(env.Data), `"sent":false`)
		require.Contains(t, string(env.Data), `"send_error":"bark unreachable"`)
	})

	t.Run("404：规则不存在", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{err: infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/99/evaluate", `{}`)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, "OPS_ALERT_RULE_NOT_FOUND", env.Reason)
	})

	t.Run("400：Bark 未启用", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{err: service.ErrBarkNotEnabled}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{"send":true}`)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "BARK_NOT_ENABLED", env.Reason)
	})

	t.Run("400：规则 ID 或请求体非法", func(t *testing.T) {
		ev := &stubAlertRuleEvaluator{result: okResult}
		svc := newOpsAlertsService(&opsAlertRulesCaptureRepo{})
		svc.SetAlertRuleEvaluator(ev)
		r := newOpsAlertsTestRouter(svc)

		w, _ := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/abc/evaluate", `{"send":true}`)
		require.Equal(t, http.StatusBadRequest, w.Code)
		w, _ = doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{"send":`)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, 0, ev.calls)
	})

	t.Run("503：评估器未接线 / 服务不可用", func(t *testing.T) {
		r := newOpsAlertsTestRouter(newOpsAlertsService(&opsAlertRulesCaptureRepo{}))
		w, env := doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{}`)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		require.Equal(t, "OPS_ALERT_EVALUATOR_UNAVAILABLE", env.Reason)

		r = newOpsAlertsTestRouter(nil)
		w, _ = doOpsAlertsJSON(t, r, http.MethodPost, "/alert-rules/7/evaluate", `{}`)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
