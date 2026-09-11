//go:build unit

package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// gin 的路由树在「注册期」就会因通配符冲突 panic，而注册发生在进程启动时——
// 一旦冲突就是整个服务起不来，不是某个接口 404。新增的
// /ops/requests/:clientRequestId/chain 和既有的 /ops/requests、
// /ops/request-errors/:id 共用 "/ops/request" 这段前缀，正是容易踩的位置，
// 所以这里直接把整组 ops 路由注册一遍。
func TestRegisterOpsRoutesHasNoWildcardConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// handler 全是 nil 指针：注册阶段只取方法值，不会解引用。
	require.NotPanics(t, func() {
		registerOpsRoutes(engine.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{}})
	})

	paths := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		if route.Method == "GET" {
			paths[route.Path] = struct{}{}
		}
	}
	require.Contains(t, paths, "/api/v1/admin/ops/requests/:clientRequestId/chain")
	require.Contains(t, paths, "/api/v1/admin/ops/requests", "既有的请求钻取列表不能被顶掉")
	require.Contains(t, paths, "/api/v1/admin/ops/request-errors/:id")
}
