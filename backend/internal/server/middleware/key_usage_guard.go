package middleware

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 免登录用量页的全局滥用预算。
//
// 背景（为什么 per-IP 限流不够）：限流桶的 key 来自 GetSecurityClientIP，而
// security.trust_forwarded_ip_for_api_key_acl 默认为 true，全局中间件 SessionBindingContext
// 每个请求都会把这个开关快照进 context，于是限流直接读原始 CF-Connecting-IP / X-Real-IP /
// X-Forwarded-For，且不做可信代理链校验。攻击者单机单连接轮换 XFF 就能拿到任意多个
// 独立的桶（发私网地址还能让面板层的 PublicIP 限流直接跳过计数），per-IP 阈值形同虚设。
// 同一套 IP 解析也是 rejectInvalidAuthAbuse 的 key，所以那层同样可被绕开。
//
// 因此这里再叠一层与客户端 IP 完全无关的**端点级**预算：计数器只有一个，不带任何
// 请求可控的维度，轮换多少个 XFF 都撞同一个桶。故意做成进程内令牌桶而不是 Redis 计数：
// 这是最后一道防线，不能因为 Redis 抖动就整体失效（fail-open）或整体拒绝（fail-close）。
// 多实例部署时有效预算是 实例数 × 阈值，依然有界。
const (
	// keyUsageProbeRequestsPerMinute 凭证探测路径（POST /session、GET /report + Bearer）的
	// 全站请求预算。正常站点上"有人来查自己的 Key 用量"远达不到这个量级。
	keyUsageProbeRequestsPerMinute = 60
	// keyUsageProbeRequestsBurst 允许的瞬时突发（页面首屏会连着打 session + report）。
	keyUsageProbeRequestsBurst = 120

	// keyUsageProbeFailuresPerMinute 凭证探测路径的全站**失败**预算。
	// 这是真正挡爆破的那一层：暴力枚举几乎 100% 失败，而正常用户几乎 100% 成功，
	// 所以攻击者会先烧光失败预算被挡住，持有有效 Key 的正常用户完全不受影响
	// （成功的请求不消耗失败预算，且失败预算耗尽时的拒绝发生在扣请求预算之前，
	// 攻击流量不会把正常用户的请求预算也带走）。
	keyUsageProbeFailuresPerMinute = 20
	// keyUsageProbeFailuresBurst 失败预算的突发额度（用户手滑贴错几次 key 属于正常）。
	keyUsageProbeFailuresBurst = 40

	// keyUsageTokenRequestsPerMinute 令牌路径（GET /report?token=）的全站请求预算。
	// 令牌是服务端签名的，猜不出来，不构成"某个 key 是否存在"的探测面，
	// 所以只需要一个防打爆聚合查询的粗预算，阈值明显放宽。
	keyUsageTokenRequestsPerMinute = 600
	// keyUsageTokenRequestsBurst 令牌路径的突发额度。
	keyUsageTokenRequestsBurst = 1200
)

// keyUsageAbuseTracker 是本中间件需要的最小能力（由 *service.APIKeyService 实现）。
// 复用网关那套无效鉴权滥用计数：免登录页的探测必须留下记录并参与封禁，
// 否则攻击者可以在这个端点上无限试错而网关侧毫无感知。
type keyUsageAbuseTracker interface {
	CheckInvalidAuthAbuse(string) (time.Duration, bool)
	RecordInvalidAuthFailure(string)
}

// keyUsageTokenBucket 进程内令牌桶。
type keyUsageTokenBucket struct {
	mu       sync.Mutex
	capacity float64
	perSec   float64
	tokens   float64
	last     time.Time
	now      func() time.Time
}

func newKeyUsageTokenBucket(ratePerMinute, burst int) *keyUsageTokenBucket {
	if ratePerMinute <= 0 {
		ratePerMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &keyUsageTokenBucket{
		capacity: float64(burst),
		perSec:   float64(ratePerMinute) / 60,
		tokens:   float64(burst),
		now:      time.Now,
	}
}

// refillLocked 按经过时间补充令牌。调用方必须持有锁。
func (b *keyUsageTokenBucket) refillLocked(now time.Time) {
	if b.last.IsZero() {
		b.last = now
		return
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(b.capacity, b.tokens+elapsed*b.perSec)
		b.last = now
	}
}

// retryAfterLocked 返回距离下一个令牌可用的时间。调用方必须持有锁。
func (b *keyUsageTokenBucket) retryAfterLocked() time.Duration {
	if b.perSec <= 0 {
		return time.Minute
	}
	missing := 1 - b.tokens
	if missing <= 0 {
		return 0
	}
	return time.Duration(missing/b.perSec*float64(time.Second)) + time.Second
}

// Take 消耗一个令牌；预算耗尽时返回 false 与建议的重试间隔。
func (b *keyUsageTokenBucket) Take() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(b.now())
	if b.tokens < 1 {
		return false, b.retryAfterLocked()
	}
	b.tokens--
	return true, 0
}

// Available 只查询是否还有预算，不消耗。
func (b *keyUsageTokenBucket) Available() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(b.now())
	if b.tokens < 1 {
		return false, b.retryAfterLocked()
	}
	return true, 0
}

// KeyUsageAbuseGuard 是免登录用量页两个端点共用的滥用防护。
type KeyUsageAbuseGuard struct {
	abuse         keyUsageAbuseTracker
	probeRequests *keyUsageTokenBucket
	probeFailures *keyUsageTokenBucket
	tokenRequests *keyUsageTokenBucket
}

// NewKeyUsageAbuseGuard 创建守卫。两个端点共用同一个实例，共享同一份全局预算：
// 攻击者在 /session 和 /report 之间来回切换并不能拿到双倍额度。
func NewKeyUsageAbuseGuard(apiKeyService keyUsageAbuseTracker) *KeyUsageAbuseGuard {
	return &KeyUsageAbuseGuard{
		abuse:         apiKeyService,
		probeRequests: newKeyUsageTokenBucket(keyUsageProbeRequestsPerMinute, keyUsageProbeRequestsBurst),
		probeFailures: newKeyUsageTokenBucket(keyUsageProbeFailuresPerMinute, keyUsageProbeFailuresBurst),
		tokenRequests: newKeyUsageTokenBucket(keyUsageTokenRequestsPerMinute, keyUsageTokenRequestsBurst),
	}
}

// IsKeyUsageCredentialProbe 判断本次请求是否走"提交原始凭证"的路径。
//
// 带 token query param 的请求走签名令牌，猜不出来，不是 oracle；
// 其余（POST /session 的 body key、GET /report 的 Authorization: Bearer，以及
// 什么都不带的请求）都是"提交一个候选 key 看服务端认不认"，必须按同一档严格对待。
func IsKeyUsageCredentialProbe(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return true
	}
	return strings.TrimSpace(c.Query("token")) == ""
}

// Handler 返回守卫中间件。
func (g *KeyUsageAbuseGuard) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil {
			c.Next()
			return
		}
		probe := IsKeyUsageCredentialProbe(c)

		// 1) 网关既有的无效鉴权封禁（per-IP，可被 XFF 轮换绕过，所以只是第一层）。
		if rejectInvalidAuthAbuse(c, g.abuse) {
			abortKeyUsageRateLimited(c, 0)
			return
		}

		// 2) 全局预算（与客户端 IP 无关，XFF 轮换无效）。
		if probe {
			// 失败预算先查后扣：耗尽时在扣请求预算之前就拒绝，
			// 这样攻击流量不会顺带把正常用户的请求预算也吃掉。
			if ok, retry := g.probeFailures.Available(); !ok {
				abortKeyUsageRateLimited(c, retry)
				return
			}
			if ok, retry := g.probeRequests.Take(); !ok {
				abortKeyUsageRateLimited(c, retry)
				return
			}
		} else if ok, retry := g.tokenRequests.Take(); !ok {
			abortKeyUsageRateLimited(c, retry)
			return
		}

		c.Next()

		// 3) 事后记账：只有凭证探测路径上的 401 才算"无效鉴权尝试"。
		// 令牌过期同样是 401，但那是正常用户刷新一个旧链接，绝不能把他的 IP
		// 送进网关的封禁计数里。
		if probe && c.Writer.Status() == http.StatusUnauthorized {
			g.probeFailures.Take()
			recordInvalidAuthFailure(c, g.abuse)
		}
	}
}

// abortKeyUsageRateLimited 以免登录用量页的错误契约返回 429。
// 文案与 key 是否有效无关，不泄露任何存在性信息。
func abortKeyUsageRateLimited(c *gin.Context, retryAfter time.Duration) {
	if retryAfter > 0 {
		seconds := int64(retryAfter / time.Second)
		if retryAfter%time.Second > 0 {
			seconds++
		}
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": gin.H{"message": "Too many requests, please try again later"},
	})
}
