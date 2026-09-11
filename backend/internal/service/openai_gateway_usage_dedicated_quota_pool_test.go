//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 这组测试锁住 2026-09-10 线上事故在「成功响应」这条写入路径上的修复：
// 走独立额度池的模型（gpt-5.3-codex-spark，池名 codex_bengalfox）返回的 x-codex-*
// 额度头，不得被写进账号的 global codex_5h_* / codex_7d_* 规范字段——否则 spark 池的
// 100% 会被当成账号周额度用尽，把整号限流掉、连累其它模型全部 503。
//
// 名单本身（openAIDedicatedQuotaPoolModels）与它的归一化判定在 model_rate_limit.go，
// 由 ratelimit_service_dedicated_quota_pool_test.go 覆盖；这里只验证「用量快照写入」
// 这条路有没有正确接上那个判定。
//
// 反向同样重要：主池模型（rate_limit：gpt-5.5 / gpt-6-astra / gpt-5.6-sol 等）必须
// 照常写 global，否则调度侧读到的余量会停更。

// newDedicatedQuotaPoolSnapshotService 造一个只关心 Extra 写入的网关服务。
// 显式给 throttle 传 0 间隔：默认 throttle 是包级共享的（30s/账号），用例之间会互相干扰。
func newDedicatedQuotaPoolSnapshotService() (*OpenAIGatewayService, *snapshotUpdateAccountRepo) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}
	svc := &OpenAIGatewayService{
		accountRepo:           repo,
		codexSnapshotThrottle: newAccountWriteThrottle(0),
	}
	return svc, repo
}

// codexQuotaPoolHeadersFixture 模拟事故现场的响应头：secondary(7d) 已用 100%。
// 这正是被误写进 codex_7d_used_percent、进而判定「整号周额度用尽」的那个数字。
func codexQuotaPoolHeadersFixture() http.Header {
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-reset-after-seconds", "86400")
	return headers
}

func requireGlobalCodexExtraWritten(t *testing.T, repo *snapshotUpdateAccountRepo) map[string]any {
	t.Helper()
	select {
	case updates := <-repo.updateExtraCalls:
		return updates
	case <-time.After(2 * time.Second):
		t.Fatal("expected UpdateExtra to be called with global codex usage")
		return nil
	}
}

// requireNoCodexExtraWritten 断言完全没有发生 Extra 写入。写入是在
// updateCodexUsageSnapshotForModel 里 go 出去的，命中名单时连 goroutine 都不会起，
// 所以这里等一小段时间足以证伪。
func requireNoCodexExtraWritten(t *testing.T, repo *snapshotUpdateAccountRepo) {
	t.Helper()
	select {
	case updates := <-repo.updateExtraCalls:
		t.Fatalf("expected no Extra write for a dedicated quota pool model, got %v", updates)
	case <-time.After(300 * time.Millisecond):
		// 没有写入 = 期望结果。
	}
}

func TestUpdateCodexUsageSnapshotFromHeadersForModelWritesGlobalForMainPoolModel(t *testing.T) {
	// gpt-5.6-sol 一度被怀疑有独立池，2026-09-11 查 /wham/usage 证伪（上游只有
	// codex_bengalfox 一个附加池），它必须继续走 global。
	for _, model := range []string{"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol", "gpt-5.3-codex"} {
		t.Run(model, func(t *testing.T) {
			svc, repo := newDedicatedQuotaPoolSnapshotService()

			svc.UpdateCodexUsageSnapshotFromHeadersForModel(context.Background(), 90101, codexQuotaPoolHeadersFixture(), model)

			updates := requireGlobalCodexExtraWritten(t, repo)
			require.Equal(t, 12.0, updates["codex_5h_used_percent"])
			require.Equal(t, 100.0, updates["codex_7d_used_percent"])
			require.Equal(t, 600, updates["codex_5h_reset_after_seconds"])
			require.Equal(t, 86400, updates["codex_7d_reset_after_seconds"])
			// 原始 primary/secondary 字段同样是调度侧输入（openAIQuotaHeadroomFactor
			// 读 codex_primary_used_percent），主池模型下必须照常落盘。
			require.Equal(t, 12.0, updates["codex_primary_used_percent"])
			require.Equal(t, 100.0, updates["codex_secondary_used_percent"])
		})
	}
}

func TestUpdateCodexUsageSnapshotFromHeadersForModelSkipsDedicatedQuotaPoolModel(t *testing.T) {
	svc, repo := newDedicatedQuotaPoolSnapshotService()

	svc.UpdateCodexUsageSnapshotFromHeadersForModel(context.Background(), 90102, codexQuotaPoolHeadersFixture(), "gpt-5.3-codex-spark")

	requireNoCodexExtraWritten(t, repo)
}

// 模型名的大小写 / 空白 / 下划线 / 供应商前缀 / 后缀变体都不能让守卫失效——
// 上游和客户端的写法五花八门，只要落到 spark 这个池上就必须跳过。
func TestUpdateCodexUsageSnapshotFromHeadersForModelIsModelSpellingInsensitive(t *testing.T) {
	variants := []string{
		"  GPT-5.3-Codex-Spark  ",
		"GPT-5.3-CODEX-SPARK",
		"gpt-5.3_codex_spark",
		"openai/gpt-5.3-codex-spark",
		"gpt-5.3-codexspark",
		"gpt-5.3-codex-spark-2026-01-01",
		"gpt-5.3-codex-spark-openai-compact",
	}
	for _, model := range variants {
		t.Run(model, func(t *testing.T) {
			svc, repo := newDedicatedQuotaPoolSnapshotService()

			svc.UpdateCodexUsageSnapshotFromHeadersForModel(context.Background(), 90103, codexQuotaPoolHeadersFixture(), model)

			requireNoCodexExtraWritten(t, repo)
		})
	}
}

// 模型名为空 = 「本次请求没有模型上下文」。约定行为：按普通模型处理、照常写 global。
// 这是刻意选的保守方向（少写 global 会让调度侧读到过期快照，危害大于多写一次），
// 也保证没有模型上下文的旧调用点行为不变。若将来出现「有独立池但拿不到模型名」的
// 路径，正确做法是把模型名补上，而不是把这里改成跳过。
func TestUpdateCodexUsageSnapshotFromHeadersForModelEmptyModelWritesGlobal(t *testing.T) {
	for name, model := range map[string]string{"empty": "", "whitespace": "   "} {
		t.Run(name, func(t *testing.T) {
			svc, repo := newDedicatedQuotaPoolSnapshotService()

			svc.UpdateCodexUsageSnapshotFromHeadersForModel(context.Background(), 90104, codexQuotaPoolHeadersFixture(), model)

			updates := requireGlobalCodexExtraWritten(t, repo)
			require.Equal(t, 12.0, updates["codex_5h_used_percent"])
			require.Equal(t, 100.0, updates["codex_7d_used_percent"])
		})
	}
}

// 旧的无模型重载必须与 model="" 完全等价，避免遗留调用点行为漂移。
func TestUpdateCodexUsageSnapshotFromHeadersLegacyOverloadStillWritesGlobal(t *testing.T) {
	svc, repo := newDedicatedQuotaPoolSnapshotService()

	svc.UpdateCodexUsageSnapshotFromHeaders(context.Background(), 90105, codexQuotaPoolHeadersFixture())

	updates := requireGlobalCodexExtraWritten(t, repo)
	require.Equal(t, 12.0, updates["codex_5h_used_percent"])
	require.Equal(t, 100.0, updates["codex_7d_used_percent"])
}

// 内部快照入口（forward / passthrough / chat_completions / messages 走这条）同样受
// 名单保护，防止有人绕过 FromHeaders 那层直接写。
func TestUpdateCodexUsageSnapshotForModelSkipsDedicatedQuotaPoolModel(t *testing.T) {
	snapshot := ParseCodexRateLimitHeaders(codexQuotaPoolHeadersFixture())
	require.NotNil(t, snapshot)

	svc, repo := newDedicatedQuotaPoolSnapshotService()
	svc.updateCodexUsageSnapshotForModel(context.Background(), 90106, snapshot, "gpt-5.3-codex-spark")
	requireNoCodexExtraWritten(t, repo)

	svc2, repo2 := newDedicatedQuotaPoolSnapshotService()
	svc2.updateCodexUsageSnapshotForModel(context.Background(), 90107, snapshot, "gpt-6-astra")
	updates := requireGlobalCodexExtraWritten(t, repo2)
	require.Equal(t, 100.0, updates["codex_7d_used_percent"])
}
