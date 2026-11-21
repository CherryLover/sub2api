# Sub2API 上游 v0.2.23 增量回移审查与开发续记

> **2026-11-21 后续真实验证更新**：调度回源、请求内存、Responses 历史修复已继续实施；真实 PG/Redis 仓储 1,384 项通过（1 原有跳过），审计 207 项通过。最新状态见[续作记录](2026-11-21-scheduler-memory-integration.md)，下方“数据库未运行”描述属于早期阶段。统一交付范围仍未全部完成。

> **2026-11-21 隔离统一开发续记**：见[开发与验收记录](2026-11-21-unified-validation.md)。统一代码最终收尾 `660108582d14d71812441660515f328b51509b11`；全仓 unit、关键 race、前端、构建、Lint 和本地安全门禁已有证据。真实 PostgreSQL/Redis 未执行，GitHub CI/Scan dispatch 返回 Actions disabled (HTTP 422)；剩余适用范围及统一 PR 仍未完成。原始工作区只读。

> 2026-11-18：上游 `upstream/main=2af3f1e15c98c893a89255a037f551046883b8c0`，已发布 `v0.2.23`；精简版 `lite/main=8fe8e095ed8abfef9a7e9fda6de4ae45de425296`，仍为 `0.1.224`。沿用[原回移清单](2026-11-17-v022-backport-development-checklist.md)和“全部适用项开发/综合验收后集中交付 PR”策略。**本文件记录新增范围，不宣称所有回移已结束。**

## 新版合并 PR 的逐项分类

| 上游 PR | 作用 | 本轮结论 | 精简版开发/验证证据 |
|---|---|---|---|
| [#8906](https://github.com/Wei-Shaw/sub2api/pull/8906) | GPT-6.2 Sol/Astra 模型、协议、官方 Codex 元数据与价格 | **主线模型，继续实施** | 本地 `9a91f0db6`：目录、合法别名、Responses 工具要求、基础 Thinking/采样、官方离线描述、标准价格、前端列表完成；Go 单测及构建、Vue typecheck 与 16 项 whitelist 通过。**Pro 套餐 eligibility、日期 Ultra Fast 3×→6×、WS/Composite 及实时账号能力仍需审查，不得称完整支持。** |
| [#8577](https://github.com/Wei-Shaw/sub2api/pull/8577)、[#8920](https://github.com/Wei-Shaw/sub2api/pull/8920) | OpenAI SSE 空闲 deadline、首内容前 failover 的单次切换预算 | **适用，已本地实施** | `6814647ff`：先复现 idle 按整周期轮询延迟及不返回 failover，再改最后 read 时间计算、首内容前限预算换号。聚焦及扩展 Go 回归、Service/Handler 编译通过；待统一 CI。 |
| [#8903](https://github.com/Wei-Shaw/sub2api/pull/8903) | Raw Chat SSE 逐行 flush 的批处理优化 | **适用，已本地实施** | `76383eac0`：在下一次向上游阻塞读取前 flush，保证已收到事件及时下发，带 `unit` 测试验证批次合并与等待前下发；待统一 CI。 |
| [#8174](https://github.com/Wei-Shaw/sub2api/pull/8174) | Anthropic 缓存创建 5m/1h TTL token 保真 | **部分已实施，真实数据库与全部协议仍待验收** | `02fe90cb9` 原生、Chat/Responses、SSE TTL 和计费桶；`731d55198` 使用日志缓存聚合缺失修复；`fa74ef070` Antigravity 兼容流输出-only 事件保留先前输入/缓存以及 PostgreSQL TTL 往返集成测试（本机 Docker 缺失，实际跳过）；`7b24c3d54` 自定义统计模式 detail-only 定向回归。尚未在 CI/真实 DB 执行完整集成，不改价格和 schema。 |
| [#8914](https://github.com/Wei-Shaw/sub2api/pull/8914) | 可配置 Ultra Fast 收费与档位 | **需用户批准定价/运营策略** | 与 #8906 日期倍率及 #8812 旧规则有关。未批准前不改变已有价格政策/默认强制档位。 |
| [#8904](https://github.com/Wei-Shaw/sub2api/pull/8904) | DeepSeek/Ollama Chat reasoning 单遍对象改写 | **Lite Ollama 路径已回移；DeepSeek 原函数不存在，不机械扩面** | 本地 `fc994ef2a`：为 Ollama Cloud reasoning 使用单遍 JSON 字符串字段编辑（失败回退 sjson），随机 payload 字节等价回归重复 3 次、Service/Handler build 通过；未恢复 Lite 没有的 DeepSeek 占位路径。 |
| [#8598](https://github.com/Wei-Shaw/sub2api/pull/8598) | 计费 heap 测试修复 | **测试性维护候选** | 暂无独立生产行为缺口；主线 billing test 变更随账单专题复核，不单独回移。 |

## 2026-11-19 PostgreSQL 测试准备与 Antigravity 用量修复

- Mac mini 独立分支 `backport/consolidated-integration-20261119`，本地提交 `fa74ef070`：新增 `backend/internal/repository/usage_log_cache_ttl_backport_integration_test.go`，验证 5m/1h 分桶、聚合优先级及费用可从 PostgreSQL `usage_logs` 读取；本机没有 Docker，integration harness 输出 `ok` 仅表示**实际跳过**，严禁宣称 PostgreSQL 测试通过。须在最终目标仓库 GitHub CI 的真实 PostgreSQL/Redis 环境执行。
- 在 Antigravity 兼容流 `mergeAntigravityCompatUsage` 中复现 `message_start` 携带输入/缓存，而后 `message_delta` 仅携带 output 时把先前用量覆盖为零的错误。先失败复现后修复，聚焦测试重复 3 次和扩展服务用例通过，`fa74ef070`；原生 Antigravity SSE 的零值/Null 稀疏 TTL 已经能够保持总量，作为保护回归加入。
- 本地 `7b24c3d54` 为现有 Anthropic 渠道账号统计/分组费率测试增加“只提供 TTL 明细、aggregate=0”的用例，验证使用日志总量和 5m/1h 分桶计费相符，重复运行 2 次通过；不修改默认/自定义价格。
- [上游 #8778](https://github.com/Wei-Shaw/sub2api/pull/8778) 当前状态为 **CLOSED、未合并**（2026-11-17 关闭），不能当作主线已发布 Hydrogen 功能。此前上游已合并的 #8746 是 Hydrogen 定价补充，并不代表全协议可用；仍需独立验证动态努力档位与端点，不机械引入 41 文件实验性路由更改。
- **仍未完成**真实 PostgreSQL/Redis 集成、实网 WebSocket、Hydrogen、审批范围及最终统一 CI/Security Scan。无新远程分支、无新 PR、不自动合并部署。

## 2026-11-18 计费日志和 WebSocket/Composite 回归补强

- 分支 `backport/consolidated-billing-validation-20261118`：本地提交 `731d55198`，回归实测精简版在仅有 5m/1h 缓存创建 TTL 明细、aggregate 缺省时虽然有计费桶，`usage_logs.cache_creation_tokens` 却写 0。现保持正数聚合优先，缺失/零则取 TTL 明细和，**客户计费、账号统计和使用日志统一使用这一有效总量**。`TestAnthropicCacheTTLDetailOnlyPersistsConsistentUsageLog` 先失败后通过，显式聚合优先级用例也通过；未改数据库 schema/定价或历史账单。
- 本地提交 `449cf40b6`：为 GPT-6.2 Sol/Astra 增加 WebSocket 账号级模型限流 canonical key 和 Composite 显式 Responses 路由映射回归。相关测试重复 3 次、Service/Handler 编译通过。**这是模拟的入口级安全/模型判断测试，不代表实网 WS 握手、长连接数据或套餐 eligibility 完整验收。**
- 尚未开展 PostgreSQL usage_logs 实际落库集成、跨渠道自定义价格数据库复验、Antigravity 流式或 GPT-6.2 WebSocket 实网验证；最终 CI/Scan 仍未执行。分支未推送、未创建新 PR。

## 2026-11-18 进一步实现：Anthropic TTL 与 Ollama JSON

- 新增隔离工作分支 `backport/consolidated-cache-ttl-20261118`（基于 `76383eac0`），本地 `02fe90cb9`：Anthropic 5m/1h TTL 的 usage 聚合、Chat/Responses 协议转换和 native 计费桶；`fc994ef2a`：Ollama Cloud Chat JSON 单遍改写。原 checkout 的冲突及业务修改未动。
- **尚不能认定全量 #8174 已回移**：usage_logs PostgreSQL/Redis、渠道特殊计费、部分 Antigravity 路径等仍缺完整端到端验收，继续按分组价格语义审查。没有新远程分支或 PR。

## 当前统一工作与交付边界

- 隔离 Mac mini worktree：`backport/consolidated-v023-20261118`，最新本地 commit `76383eac0`，上游已有 3 个新提交加入原统一开发基线。**未推送新分支、未创建新的 PR**，旧 #73–#81 保持原状且与最终集中 PR 去重。
- **验证已执行**：OpenAI idle 与 Raw Chat SSE 聚焦、扩大 Go 测试和 Service/Handler 编译；GPT-6.2 Sol/Astra 核心目录/标准价卡/别名/采样单测，前端 Vue typecheck 和 16 个白名单测试。测试结果不得外推成整仓 CI、生产套餐资格或实网 WebSocket 行为。
- **后续优先级**：核实 GPT-6.2 HTTP/WS/Composite/订阅 eligibility 与成本生效时间，再拆 Anthropic TTL 保真；其后审查剩余 v0.2.22 主线（Hydrogen、Vertex、多轮协议与账单），保留精简版的授权、默认调度和 schema 定制。
- **争议范围**：Ultra Fast 促销期与订阅资格、Carbon/Hydrogen 切换价格、缓存 TTL 计费迁移和多实例租约应按上游证据及精简版行为逐项提出审批，不默认采用。最终 PR 标题/正文写功能变化和上游链接，不写开发编号；最终 SHA GitHub CI/Security Scan 全绿前不报验收完成，不自动合并/部署。


## 2026-11-21 续作进度（未统一验收）

后续协议、额度、账号探测及费用口径开发见 [统一续作记录](2026-11-21-unified-followup.md)。本轮尚未提交，定向结果不能替代最终全仓/真实数据库/CI；原审批边界与一个最终 PR 的要求保持不变。
