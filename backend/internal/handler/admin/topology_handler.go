package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const topologyHeartbeatInterval = 15 * time.Second

// TopologyHandler 调度拓扑页：快照 + 实时路由事件流。
type TopologyHandler struct {
	service   *service.TopologyService
	heartbeat time.Duration
}

// NewTopologyHandler creates the topology handler.
func NewTopologyHandler(topologyService *service.TopologyService) *TopologyHandler {
	return &TopologyHandler{service: topologyService, heartbeat: topologyHeartbeatInterval}
}

// GetSnapshot 返回当前拓扑快照。
// GET /api/v1/admin/topology/snapshot
func (h *TopologyHandler) GetSnapshot(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Topology service not available")
		return
	}
	snap, err := h.service.Snapshot(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snap)
}

// StreamEvents 以 Server-Sent Events 推送实时路由事件。
// GET /api/v1/admin/topology/events
//
// 每条事件：`event: routing\ndata: <RoutingEvent JSON>\n\n`；每 15 秒一行 `: ping` 心跳。
// 前端用 fetch + Authorization 头读取，客户端断开即结束。
func (h *TopologyHandler) StreamEvents(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Topology service not available")
		return
	}
	sub, err := h.service.Subscribe()
	if err != nil {
		if errors.Is(err, service.ErrTopologyTooManySubscribers) {
			response.Error(c, http.StatusTooManyRequests, "Too many topology event subscribers")
			return
		}
		response.Error(c, http.StatusServiceUnavailable, "Topology service not available")
		return
	}
	defer sub.Close()

	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if !writeTopologySSE(c, []byte(": connected\n\n")) {
		return
	}

	interval := h.heartbeat
	if interval <= 0 {
		interval = topologyHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			buf := make([]byte, 0, len(data)+32)
			buf = append(buf, "event: routing\ndata: "...)
			buf = append(buf, data...)
			buf = append(buf, "\n\n"...)
			if !writeTopologySSE(c, buf) {
				return
			}
		case <-ticker.C:
			if !writeTopologySSE(c, []byte(": ping\n\n")) {
				return
			}
		}
	}
}

func writeTopologySSE(c *gin.Context, b []byte) bool {
	if _, err := c.Writer.Write(b); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}
