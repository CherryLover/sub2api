package admin

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// clearAccountRuntimeBlocksMaxBodyBytes 给可选请求体一个上限，免得逃生口本身成了
// 一个能被撑爆内存的入口。正常请求体只有几十字节。
const clearAccountRuntimeBlocksMaxBodyBytes = 1 << 20

// clearAccountRuntimeBlocksRequest 是可选请求体。
// account_ids 缺省或为空数组都表示「全部账号」。
type clearAccountRuntimeBlocksRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

// ClearRuntimeBlocks 无条件清空网关进程内存里的调度封锁状态。
// POST /api/v1/admin/accounts/runtime-blocks/clear
//
// 为什么要有这么一个接口：进程内的封锁只活在正在跑的服务进程里，外部再起一个进程也碰不到，
// 后台账号页读的只是数据库，于是内存封锁在页面上完全不可见。2026-09-11 公司实例上，账号
// 5、6 的数据库记录干干净净（active、schedulable=true，无限流、无过载、无临时停调、未过期），
// 却一个请求都不接——它们被封在内存里，页面上却显示一切正常。
//
// 而现成的「测试」「恢复状态」按钮走的 RateLimitService.RecoverAccountState 恰恰在这种
// 场景下失灵：它解除内存封锁的那一行被包在 if result.ClearedError &&
// !result.ClearedRateLimit 里，库里干净时这两个标志都是 false，两个分支都进不去。
// 当时只能重启容器——而重启会掐断所有在途请求。
//
// 所以这个接口是**无条件**的：不读数据库、不看账号状态，调用即清。任何"先检查再清"的改动
// 都会让它退化成又一个 RecoverAccountState，失去存在的意义。
//
// 请求体可选：不带 body、带 {}、带 {"account_ids": []} 都等价于全量清除，方便运维在终端里
// 直接 curl -X POST -H "x-api-key: ..." .../runtime-blocks/clear 就能救场。
func (h *AccountHandler) ClearRuntimeBlocks(c *gin.Context) {
	req, ok := bindClearAccountRuntimeBlocksRequest(c)
	if !ok {
		return
	}

	var gateway *service.OpenAIGatewayService
	if h != nil {
		gateway = h.openAIGatewayService
	}
	// gateway 为 nil 也照样往下走：Grok 的几张表是进程级全局变量，没有网关实例一样能清，
	// ClearAccountRuntimeBlocks 的接收者是 nil-safe 的。逃生口不该因为某个依赖没装上就罢工。
	result := gateway.ClearAccountRuntimeBlocks(c.Request.Context(), normalizeInt64IDList(req.AccountIDs))
	response.Success(c, result)
}

// bindClearAccountRuntimeBlocksRequest 解析可选请求体。
// 空 body 不算错误（等价于全量），只有 body 非空且不是合法 JSON 才返回 400。
// 这里没用 ShouldBindJSON：它对空 body 会返回 EOF 错误，而 curl 不带 -d 时正是空 body。
func bindClearAccountRuntimeBlocksRequest(c *gin.Context) (clearAccountRuntimeBlocksRequest, bool) {
	var req clearAccountRuntimeBlocksRequest
	if c.Request.Body == nil {
		return req, true
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, clearAccountRuntimeBlocksMaxBodyBytes))
	if err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return req, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return req, true
	}
	if err := json.Unmarshal(body, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return req, false
	}
	return req, true
}
