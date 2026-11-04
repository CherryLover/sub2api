# 功能清单

记录项目已实现的功能、入口、流程和关键文件。

## 使用记录思考档位与独立上游错误面板
- **标签**: usage, ops, feature
- **描述**: 保真展示已记录reasoning_effort，并复用Ops采集展示502/503请求级错误与恢复链路。
- **入口**: 使用记录思考模式列；管理员侧栏上游错误 /admin/upstream-errors；GET /api/v1/admin/ops/upstream-error-requests
- **核心流程**:
  1. 共享formatter保留none/minimal/low/medium/high/xhigh/max和实际已记录值，管理端新默认显示列，保留历史隐藏偏好。
  2. 按任意同一事件的状态码与失败账号匹配，按时间倒序分页后关联同client_request_id与api_key_id的usage查最终成功账号。
  3. 详情展示已留存事件、错误消息、正文与最终结果，返回前补充凭据与URL脱敏。
- **关键文件**:
  - `frontend/src/views/admin/UpstreamErrorsView.vue` - 独立错误列表、筛选、详情与响应式页面
  - `backend/internal/repository/ops_repo_upstream_error_requests.go` - 请求级事件筛选、分页与最终账号关联
  - `backend/internal/service/ops_upstream_error_requests.go` - 参数验证与历史事件脱敏
  - `frontend/src/utils/format.ts` - 思考档位原值展示
- **备注**: 无数据库迁移；不改变转发计费归一化；仅显示已有采集记录，最多末16事件，监控开关/异步队列/保留策略影响完整性。事件数不等于重试次数。缺失usage时最终账号未记录，不从失败账号猜测。未浏览器实测；前端覆盖档位保真、默认列、CSV、筛选分页、详情转义、竞态和API契约，后端覆盖参数验证、同事件筛选、Key关联、历史回退与脱敏。
