# 2026-11-21 续作：调度回源、请求内存与真实数据库验收

> 本轮是统一分支的继续开发，不表示整个选择性回移清单已经完成。原始冲突 checkout 保持只读；没有新 PR、合并、发布、部署或权限变更。

## 基线和范围

- 隔离 worktree：`/Users/leobot/Projects/.webcodex-worktrees/sub2api-edb4965d`。
- 分支：`backport/unified-integration-20261120`；本轮起始已提交 HEAD `29509a944ed6e1c7c7845bcbcbe2d9f4d0a838d3`。
- 本轮 fetch 后 upstream/main 仍为 `328db26f450edbdf9f8d4344a3ea903ac6396842`，Lite main 仍为 `8fe8e095ed8abfef9a7e9fda6de4ae45de425296`；最新正式版 v0.2.24。
- 使用分页 REST 完整取得 1,106 个 Open PR（其中 23 Draft）的元数据，避免 `gh pr list --limit 1000` 的截断。**取得名单不等于全部逐行审查完成**。本轮固定审查来源列在下方；#8971 的收费扩展未自动采用。
- 保留上一轮全部未提交业务改动，仅在隔离工作区继续最小适配。前端本轮没有新增改动。

## 本轮实际完成

### 同桶数据库回源合并

按 bucket **及调用前取得的生命周期 epoch** 合并重叠回源；QPS 预算按真正数据库查询计数，等待者各自获得独立顶层账号切片。首位等待者取消不取消其他等待者，共享查询有配置期限或 30 秒兜底，并响应服务停止。无法取得有效生命周期 token 时不共享、不脱离调用方上下文；Retire/Reopen 不加入旧代查询，回写继续受原 token fence 约束。

合成并发基准固定 32 个 worker、每轮 100 次请求、模拟 1ms DB 查询，前后三组样本均从 `1.000` 降到 `0.04000 db_queries/op`，即 100 次模拟回源降为 4 次。墙钟时间约 50–52μs/op，未观察到显著延迟改善；该结果仅证明合并降低查询次数，不外推生产吞吐量。

没有引入上游 20 秒过期快照宽限、没有改 Redis 快照新鲜度和默认选号策略。错误不形成第二层结果缓存，嵌套 map/指针仍遵循已有只读/COW 契约。44 项扩大回归通过；新增测试覆盖并发预算、取消、epoch、切片隔离、停机、默认期限、panic、未知 epoch 和关闭回源。

### 请求与审计的内存所有权

OpenAI 的短模型名、缓存键、前序响应 ID、tier 和 effort 单独持有字符串；usage 日志不再持有整个输入对象字段的指针；Chat/Messages 转换完成后不继续保留第二份完整 input。审计在配置和分组过滤后才复制需异步保存的数据；仍在返回调用方之前复制，避免 handler 随后修改原 body/group 影响审计。UTF-8 截断/分块及脱敏保留旧行为和替换顺序，不再无条件分配整段 rune 数组或未变化字符串副本。

没有恢复 Lite 已移除的 Legacy 审计模块，没有更改内存预算默认值、请求计价或路由。先取得失败复现，再通过 447 项内存/协议/探测/usage 扩大回归。

### Responses 历史尾部合法化

Responses→Anthropic 在工具配对清理后剔除 assistant 尾部孤立 thinking/redacted_thinking；只有 thinking 的残留轮次丢弃。Responses 历史以 assistant 结束时添加最小 user `Continue.`，不改变原生 Anthropic prefill 路径。完整的 thinking→tool_use→tool_result 签名及配对保留。

真实 ForwardAsResponses 覆盖公开模型别名映射到 Opus 5.5/Fable 5.2、流式/缓冲、commentary/未答复工具/仅 thinking/已答复工具，共 34 项测试事件（重复两轮）通过；同时验证客户端 usage 不被破坏。

### 数据库验证与既有夹具修正

为 OpenAI 父/影子探测 CAS 增加实际锁竞争用例：并发事务持有父账号行锁并重新授权，探测确实阻塞于 owner lock，提交后必须重新检查凭据并拒绝旧恢复证据。原影子 refresh 用例创建 parent 关系时遗漏 `quota_dimension=spark`，已补齐合法夹具，不修改数据库约束或生产 SQL 以迎合测试。

旧普通 401 测试不再要求永久停用账号；按已实现的可恢复 401 契约验证不写永久错误。Lint 还发现 unit 专用测试标签、类型断言和废弃计数 wrapper，已按实际运行路径清理，不关闭检查器或删除业务断言。

## 本地验证证据（持续更新，非远端 CI）

| 范围 | 已取得结果 | 证据说明 |
|---|---|---|
| 全量 Go unit 最终树 | **20,215 通过，14 跳过，0 失败；52 包通过** | `full-unit-final`，Lint 清理后重新全跑 |
| 关键并发 race 最终树 | **190 通过，0 失败，4 包通过** | `critical-race-final`；两轮；调度、探测、WS 搜索、AG 首字节及审计入队 |
| 真实 PostgreSQL/Redis 仓储最终树 | **1,384 通过，1 原有跳过，0 失败** | `native-repository-current`；含真实 owner-lock 竞争、root/shadow CAS、outbox 原子性、窗口和 TTL 落库 |
| 真实 PostgreSQL/Redis 审计最终树 | **207 通过，0 跳过，0 失败** | `native-audit-current`；真实配置 CAS、任务/事件、Redis payload TTL、脱敏及确认删除 |
| 嵌入式构建最终树 | **通过** | `go build -tags=embed -trimpath ./cmd/server`；当前宿主目标，未部署 |
| 后端 Lint 最终树 | **0 issues** | 未关闭检查器或降低门槛 |
| 前端 | 上一轮 228 文件、1,846 测试，typecheck/lint/build 通过 | 本轮无前端修改；不声称重新跑过 |
| GitHub CI/Scan | 本轮未重新触发 | 历史 dispatch HTTP 422 禁用；按用户要求代码收尾后再处理 |

所有测试计数包含 Go 的子测试事件，不相加计算独立用例数。仓储唯一跳过为既有 `TestConcurrencyCacheSuite/TestGetAccountsLoadBatch`；没有将跳过、零测试或仅编译算作数据库通过。

### 隔离执行方式

PostgreSQL 18.1、Redis 8.4.0 使用已校验的临时编译工具，在独立 `/tmp/sub2api-it-*` 私有目录运行，禁用 TCP 监听，只使用权限 0700 的 Unix socket。每次运行创建全新数据库，结束仅停止该次自己的服务。不启动 OrbStack，不连接生产，未改项目集成测试的 Docker 默认入口。

测试通过 Go overlay 替换测试启动器或 Redis 测试 transport，并按实际 `/private/var/...` canonical 路径绑定；源生产代码不替换。第一次路径未命中导致 Docker 跳过，零执行检查将其判为失败；修正后重新执行真实数据库测试并记录结果。测试快照清单及命令可追溯，不能拿前一次编译失败当作后一次运行成功。

最终后端/前端源清单共 **3,508 文件**，摘要 `56fbd8fc48f1c20a4f8ec77c78b2e6cb5e705cd8c71302fe3d56bbb57b55323e`，最终检查前后无新增/缺失/内容漂移。可审计索引：[验证 JSON](2026-11-21-followup-validation.json)。

本轮证据目录：`/var/folders/l8/w4tw942n20b9y31t75vw179m0000gn/T/sub2api-continue-20261121b-q9zhv4pk`。原始日志索引在同目录 `followup-closeout.json`；列明源摘要、各运行状态、跳过项和日志哈希。所有独立数据库服务均已停止。

## 仍未收尾或未批准

- #8809 的 Chat signed reasoning_details 往返、#8770 的 WS 当前轮重建及 #8739 effort 策略仍需 Lite 最小适配与真实路径测试；本轮没有将审查等同于完成。
- #8705 的部分 requestmodel/content_moderation 路径在 Lite 已不存在，不恢复旧模块；其余大请求路径和 #8876 JSON 预算、#8537 停机期限仍须独立核验。#8833 额外 stale grace 没有采用。
- #8730 Gemini 模糊 403 的跨账号处理仍待核验，不把明确内容/政策拒绝改为绕过式重放。
- #8693 新迁移、批量结算、缓存新计价桶、Ultra Fast/Carbon 日期倍率、Vertex 默认路由及 #8971 新搜索收费仍需批准。
- Decisions、Rerank、DeepSeek Files/新平台仍是产品范围待定项；真实套餐资格/外部服务能力不由本地 mock 结果推断。
- 全部适用项完成、最终 CI/Security 证据明确后才提交一个统一 PR。本记录不是全部清单结案。

## 本轮固定来源

| 上游 PR | 读取时状态 | 审查 head SHA |
|---|---|---|
| [#8832](https://github.com/Wei-Shaw/sub2api/pull/8832) | OPEN | `0fab9304e121100cb00e7dc120dab00deebe6eaf` |
| [#8875](https://github.com/Wei-Shaw/sub2api/pull/8875) | MERGED | `bcb22d4de855635ffb2827a610ecde0c846461f0` |
| [#8809](https://github.com/Wei-Shaw/sub2api/pull/8809) | OPEN | `f2a6f250abde628a4dce587b83c41964403c8d61` |
| [#8770](https://github.com/Wei-Shaw/sub2api/pull/8770) | OPEN | `73c1059ac255249ce3bf6040579555e2fe5a2a52` |
| [#8705](https://github.com/Wei-Shaw/sub2api/pull/8705) | OPEN | `87c5cc0719265bc5ba2e09f8fde60b641f2f65a6` |
| [#8876](https://github.com/Wei-Shaw/sub2api/pull/8876) | OPEN | `5cb893a6a3c1410c2abdeff8f2d1ec1f39888d88` |
| [#8537](https://github.com/Wei-Shaw/sub2api/pull/8537) | OPEN | `5914793323b0cd96e5107efd4dea673b9fc6c167` |
| [#8739](https://github.com/Wei-Shaw/sub2api/pull/8739) | MERGED | `d850b8e3a25ce1e19d9e6c23965344000927a4bd` |
| [#8971](https://github.com/Wei-Shaw/sub2api/pull/8971) | OPEN | `bb032bb9cda1fa1c0351f922aa3939b7f1a50733` |

## 下次续作入口和交付状态

本轮源码仍在上述统一工作区，未提交或推送，HEAD 保持本轮起始值；不能把验证结果标到一个未包含这些改动的远端 SHA 上。下一轮从本文件和 `cairn/LOG.md` 读取真实状态，优先完成仍列明的协议缺口；一旦修改源码，需刷新相应验证。

新 review Markdown/JSON 受 `docs/*` 忽略规则影响，最终集中提交时应显式纳入；Cairn 保留为本地续作记录，不能把整个 untracked `cairn/` 目录顺手加入业务提交。原始 `/Users/leobot/Projects/sub2api` 冲突工作区未修改。
