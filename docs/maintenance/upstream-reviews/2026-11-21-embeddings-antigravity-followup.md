# 2026-11-21 统一回移续作：Embeddings 路由与 Antigravity 客户端工具

> 本记录是同一隔离分支上的增量，不表示全部适用回移完工。原始冲突 checkout 只读，未清理、覆盖、建 PR、合并或部署。

## 基线

- 隔离 worktree：/Users/leobot/Projects/.webcodex-worktrees/sub2api-edb4965d，分支 backport/unified-integration-20261120，起始已提交 HEAD 29509a944；本轮主要代码提交 aa2f369bcff32d4523f2d5e843e5bfb224dc5da5（统一 PR 暂未创建）。
- 刷新后上游 328db26f4、精简版 8fe8e095e、正式版 v0.2.24。
- 保留此前会话未提交修改及备份，没有覆盖或清理原始工作区。

## 修复

1. internal/handler/openai_embeddings.go：按已解析的实际目标平台（包括国产供应商）选号；模型无匹配时以同一平台归因。新增 handler＋模拟上游 Zhipu 分组选中正确账号及 URL 的回归。
2. internal/service/antigravity_gateway_compat.go 与 _stream.go：Antigravity Responses custom 工具先使用既有 ResponsesClientToolMapping 降为 function，再在流式与非流式回包恢复 custom_tool_call，保留工具结果、参数和生命周期。无工具请求仍走直接解析、未适配 SSE 沿用旧序列；原始请求必须完整合法 JSON，冲突工具名本地拒绝，不能错误发往上游。
3. 修正旧代码新增测试的错误返回值检查、静态检查诊断及格式，未改数据库 schema、收费与默认内存配置。

## 验证边界

- service、handler 两包定向回归通过，新增畸形客户端工具测试也在扩大定向回归中通过。较早全仓 unit 通过发生在最后一轮源码修改之前，不能充当最新源树最终证明。
- 本轮真实 PostgreSQL 18.1、Redis 8.4.0 仓储集成 1,384 pass、1 条旧 skip，0 fail；临时 Unix socket 环境退出已停止服务。
- 最新本轮 Go unit 全仓 52 个有测试的包通过（service 216.641 秒）；go test -tags=unit ./... -count=1 -timeout=10m 退出 0，未出现包级超时。
- 后端 golangci-lint 复验严格原项目规则返回 0 issues；真实 PostgreSQL/Redis 审计 210 项全通过、0 跳过。
- 最新前端：typecheck、ESLint、生产 build 退出码 0；228 个 Vitest 文件、1,858 项测试通过；build 仅保留大 chunk 提示。
- 最新关键 race：go test -race -tags=unit 针对 service、handler、repository、securityaudit 四个包成功；覆盖调度/OAuth/Antigravity/custom tools/Embeddings/审计关键并发。
- 最终 embed 构建：go build -tags=embed -trimpath ./cmd/server 成功（输出仅在独立临时验证目录）。
- 最新代码 manifest：3,218 个 backend Go / frontend src 文件，SHA256 40911d6176377385abaacfaa0f44634dd014c02e219746cfadb6316fca236df7；git diff --check 0。
- 本轮主要代码提交 aa2f369bc；最终 PR HEAD 对应的 GitHub CI/Security 仍待，不能用本地结果替代远端运行。

## 仍需收尾

- 继续核对 WS、signed reasoning、模型资格和剩余高风险缺口；数据库迁移、新价格、默认路由、决策/Rerank/Files 新端点的产品边界须先审批。
- 遵从先开发、后 GitHub 权限，再统一 PR；未经全部验证不创建 PR，也不自动合并发布部署。

## 关联上游候选

- 国产 Embeddings 兼容路由参考 #8773 与 #8309 的路径核对，只回移在 Lite 既有端点可复现的选号问题，不恢复新增平台。
- Antigravity Responses 兼容工具参考 #8164、#8804，使用 Lite 现有 shared client-tool mapping 重写，不将未合并上游补丁整包复制。

## 2026-11-21 统一 PR 交付门槛复核

- 统一分支 backport/unified-integration-20261120 已推送个人 fork xiaoguai1818/sub2api。推送时 SHA 660588592c304d5365211f3a6cdfa7e9495004ca，目标 CherryLover/sub2api:main，未创建 PR。
- lite/main 基线 8fe8e095e，上游 main 328db26f4，最新标签 v0.2.24。原始冲突工作区只读。
- 本地 52 包 Go unit、真实 PostgreSQL/Redis 仓储/审计、关键四包 race、前端 typecheck/lint/1,858 Vitest/build、golangci-lint 0 issues、embed build 已通过；不等于最终 SHA 远端 CI。
- Fork 仓库 Actions 权限 API 显示 enabled=true,allowed_actions=all、workflow state=active。但 push 未创建最终 SHA 运行，手动 dispatch backend-ci.yml 和 security-scan.yml 都返回 **HTTP 422 Actions has been disabled for this repository**。使用管理员权限幂等 PUT 开启 Actions 后复测仍返回 422；没有 run ID 或结果。
- CherryLover/sub2api 上 #73–#81 九项旧 PR 仍 Open，其变更也包含在统一分支；不得隐瞒重叠或擅自关闭旧 PR。
- **结论：暂不创建正式 PR。** 先在 fork Actions 页核对开关状态；若仍 422，需要 GitHub Support 排查执行限制，或由目标仓库维护者明确允许在 PR 内运行 CI。只有最终 SHA CI/Security Scan 实际执行且通过后再创建一个功能标题清晰的 PR，不合并或部署；新计价、迁移和新 API 仍待范围审批。
