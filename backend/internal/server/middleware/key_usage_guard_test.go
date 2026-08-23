package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubKeyUsageAbuse struct {
	mu       sync.Mutex
	recorded []string
	blocked  bool
	retry    time.Duration
}

func (s *stubKeyUsageAbuse) CheckInvalidAuthAbuse(string) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return s.retry, true
	}
	return 0, false
}

func (s *stubKeyUsageAbuse) RecordInvalidAuthFailure(clientKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, clientKey)
}

func (s *stubKeyUsageAbuse) records() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.recorded...)
}

// newGuardRouter 挂一个可控状态码的假 handler，模拟 /session 与 /report。
func newGuardRouter(guard *KeyUsageAbuseGuard, status int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/key-usage/report", guard.Handler(), func(c *gin.Context) {
		c.JSON(status, gin.H{"error": gin.H{"message": "Invalid or expired key"}})
	})
	router.POST("/api/v1/key-usage/session", guard.Handler(), func(c *gin.Context) {
		c.JSON(status, gin.H{"error": gin.H{"message": "Invalid or expired key"}})
	})
	return router
}

// probeWithForwardedFor 用一个全新的 X-Forwarded-For 打一次"提交原始凭证"的请求。
func probeWithForwardedFor(router *gin.Engine, xff string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report", nil)
	req.Header.Set("X-Forwarded-For", xff)
	req.Header.Set("Authorization", "Bearer sk-guess-"+xff)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder.Code
}

// 攻击者单机单连接轮换 X-Forwarded-For 就能拿到任意多个独立的 per-IP 限流桶
// （限流用的 GetSecurityClientIP 在 trust_forwarded_ip_for_api_key_acl 默认开启时
// 直接读原始转发头，不做可信代理链校验）。端点级全局预算与客户端 IP 无关，
// 无论轮换多少个 IP 都撞同一个桶，必须挡得住。
func TestKeyUsageGuardGlobalBudgetSurvivesXFFRotation(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusUnauthorized)

	statuses := make([]int, 0, 200)
	for i := 0; i < 200; i++ {
		// 每次都是一个全新的公网 IP：per-IP 维度上永远是"第 1 次请求"。
		statuses = append(statuses, probeWithForwardedFor(router, "198.51.100."+strconv.Itoa(i%254+1)))
	}

	blocked := 0
	for _, status := range statuses {
		if status == http.StatusTooManyRequests {
			blocked++
		}
	}
	require.Positive(t, blocked, "轮换 XFF 必须仍然被全局预算拦截")
	// 失败预算的突发额度是 keyUsageProbeFailuresBurst，之后每分钟只补
	// keyUsageProbeFailuresPerMinute 个；这一轮（远不到 1 分钟）之后应该几乎全被拦。
	require.LessOrEqual(t, 200-blocked, keyUsageProbeFailuresBurst+1,
		"放行的探测次数不能超过全局失败预算")
	require.Equal(t, http.StatusTooManyRequests, statuses[len(statuses)-1])
}

// 免登录端点的探测必须进入网关既有的无效鉴权滥用计数，否则爆破不留任何记录、
// 也永远触发不了封禁。
func TestKeyUsageGuardRecordsProbeFailuresIntoAbuseCounter(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusUnauthorized)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/key-usage/session", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Len(t, abuse.records(), 1, "一次失败的凭证探测必须记一次")
}

// 令牌路径不是 oracle：令牌过期只是正常用户刷新了一个旧链接，
// 绝不能把他的 IP 送进网关的封禁计数里。
func TestKeyUsageGuardDoesNotRecordTokenPathFailures(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusUnauthorized)

	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token=expired-token", nil)
		req.Header.Set("X-Forwarded-For", "198.51.100.9")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, "令牌路径不该被失败预算拦下")
	}
	require.Empty(t, abuse.records())
}

// 已被网关封禁的来源在这两个端点上同样进不来。
func TestKeyUsageGuardHonoursExistingAbuseBlock(t *testing.T) {
	abuse := &stubKeyUsageAbuse{blocked: true, retry: 42 * time.Second}
	guard := NewKeyUsageAbuseGuard(abuse)
	handlerCalled := false
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/key-usage/session", guard.Handler(), func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/key-usage/session", nil))

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.False(t, handlerCalled)
	require.Equal(t, "42", recorder.Header().Get("Retry-After"))
}

// 成功的查询不消耗失败预算：持有有效 Key 的正常用户不会被爆破流量连坐。
func TestKeyUsageGuardSuccessfulLookupsKeepFailureBudget(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusOK)

	for i := 0; i < keyUsageProbeFailuresBurst*2; i++ {
		require.Equal(t, http.StatusOK, probeWithForwardedFor(router, "203.0.113.7"))
	}
	require.Empty(t, abuse.records())

	available, _ := guard.probeFailures.Available()
	require.True(t, available, "失败预算不该被成功请求消耗")
}

// 两个端点共用同一份全局预算：在 /session 和 /report 之间来回切换拿不到双倍额度。
func TestKeyUsageGuardBudgetIsSharedAcrossEndpoints(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusUnauthorized)

	allowed := 0
	for i := 0; i < keyUsageProbeFailuresBurst*2; i++ {
		path := "/api/v1/key-usage/report"
		method := http.MethodGet
		if i%2 == 0 {
			path = "/api/v1/key-usage/session"
			method = http.MethodPost
		}
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i%254+1))
		req.Header.Set("Authorization", "Bearer sk-guess")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusTooManyRequests {
			allowed++
		}
	}
	require.LessOrEqual(t, allowed, keyUsageProbeFailuresBurst+1)
}

// 令牌路径有自己独立、更宽松的预算，不会被凭证探测的额度耗尽拖下水。
func TestKeyUsageGuardTokenPathHasSeparateBudget(t *testing.T) {
	abuse := &stubKeyUsageAbuse{}
	guard := NewKeyUsageAbuseGuard(abuse)
	router := newGuardRouter(guard, http.StatusUnauthorized)

	// 先把凭证探测的预算打光
	for i := 0; i < keyUsageProbeRequestsBurst*2; i++ {
		probeWithForwardedFor(router, "198.51.100.1")
	}
	require.Equal(t, http.StatusTooManyRequests, probeWithForwardedFor(router, "198.51.100.2"))

	// 令牌路径照常可用
	req := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token=t", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusUnauthorized, recorder.Code, "令牌路径不应被凭证探测的预算耗尽影响")
}

func TestKeyUsageTokenBucketRefills(t *testing.T) {
	bucket := newKeyUsageTokenBucket(60, 2)
	now := time.Now()
	bucket.now = func() time.Time { return now }

	ok, _ := bucket.Take()
	require.True(t, ok)
	ok, _ = bucket.Take()
	require.True(t, ok)
	ok, retry := bucket.Take()
	require.False(t, ok)
	require.Positive(t, retry)

	now = now.Add(2 * time.Second)
	ok, _ = bucket.Take()
	require.True(t, ok, "按 60/min 的速率，2 秒后应至少补回一个令牌")
}
