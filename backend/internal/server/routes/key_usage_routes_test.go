package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func newKeyUsageRoutesTestRouter(t *testing.T, enabled bool) *gin.Engine {
	t.Helper()
	cfg := &config.Config{}
	cfg.KeyUsage.Enabled = enabled
	return newKeyUsageRoutesTestRouterWithConfig(t, cfg)
}

func newKeyUsageRoutesTestRouterWithConfig(t *testing.T, cfg *config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// 指向一个必定连不上的地址：用来验证 Redis 故障时各路径的 fail-open/fail-close 策略。
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	router := gin.New()
	v1 := router.Group("/api/v1")
	// keyUsageService 传 nil：本用例只关心路由层的限流与开关，handler 会返回 503。
	RegisterKeyUsageRoutes(v1, &handler.Handlers{KeyUsage: handler.NewKeyUsageHandler(nil, nil, nil)}, rdb, nil, nil, cfg)
	return router
}

func keyUsageProbeRequest(method, target, bearer string) *http.Request {
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{"key":"sk-probe"}`)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// POST /session 与 GET /report + Bearer 是完全等效的两个"这个 key 存不存在"的 oracle，
// 必须同档同策略：Redis 不可用时都 fail-close（宁可拒绝也不放行探测）。
func TestKeyUsageProbePathsFailCloseWhenRedisUnavailable(t *testing.T) {
	router := newKeyUsageRoutesTestRouter(t, true)

	cases := map[string]*http.Request{
		"session":    keyUsageProbeRequest(http.MethodPost, "/api/v1/key-usage/session", ""),
		"report+key": keyUsageProbeRequest(http.MethodGet, "/api/v1/key-usage/report", "sk-probe"),
		"report 裸请求": keyUsageProbeRequest(http.MethodGet, "/api/v1/key-usage/report", ""),
	}
	for name, req := range cases {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusTooManyRequests, w.Code, "path=%s", name)
	}
}

// 令牌路径不是 oracle：Redis 抖动不该把正常访客的页面打挂，保持 fail-open。
func TestKeyUsageTokenPathFailsOpenWhenRedisUnavailable(t *testing.T) {
	router := newKeyUsageRoutesTestRouter(t, true)

	req := keyUsageProbeRequest(http.MethodGet, "/api/v1/key-usage/report?token=some-token", "")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, "应该真的走到了 handler")
}

// 默认配置（key_usage.enabled 默认 true）下整组路由必须注册：
// 锁住"站长升级后不用改配置即可用"这个默认行为，改回默认关闭会让这条用例失败。
func TestKeyUsageRoutesRegisteredWithDefaultConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DATA_DIR", "")
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))

	cfg, err := config.Load()
	require.NoError(t, err)
	require.True(t, cfg.KeyUsage.Enabled, "key_usage.enabled 默认应为 true")

	router := newKeyUsageRoutesTestRouterWithConfig(t, cfg)

	req := keyUsageProbeRequest(http.MethodGet, "/api/v1/key-usage/report?token=t", "")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code, "默认配置下路由应已注册")
}

// kill switch：显式设成 false 后整组路由不注册，出事时不用改代码发版就能关掉这个公开页面。
func TestKeyUsageRoutesNotRegisteredWhenDisabled(t *testing.T) {
	router := newKeyUsageRoutesTestRouter(t, false)

	for _, req := range []*http.Request{
		keyUsageProbeRequest(http.MethodPost, "/api/v1/key-usage/session", ""),
		keyUsageProbeRequest(http.MethodGet, "/api/v1/key-usage/report?token=t", ""),
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	}
}
