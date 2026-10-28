# 第五批剩余事项：独立兼容修复与冲突核验

## 约束和基线

- 精简版目标：`CherryLover/sub2api:main@6af365be55481a5a14d86222eefc8e742b70d738`。本分支直接从此已合并 main 创建，**没有包含 PR #32–#36 未合并提交**。
- 上游参考：O03 PR #8251（`38b72d734` / `53002f677`）、O06 PR #7173（`da74bf13a`）、O07 PR #7793（`1888de135`）、O08 PR #7057（`195a33132`）、O11 PR #8094（`2a8cb088a`）、O15 PR #7064（`956f4672e`）、O16 PR #6434（`a10ff1255`）。
- 已阅读原回移规范及第五批 #36 的范围限制：仅保留精简版已存在的代理核心能力、调度、模型目录、WebSocket；不恢复余额、支付、营销、站点 OAuth、订阅及裁剪过的元数据模块。
- 本分支独立携带 CI 安全基线 Go 1.26.10 / DOMPurify 3.4.17 / follow-redirects 1.16.1，避免与未合并的批次形成编译依赖。

## 用户确认逐项处理

| 项目 | 结论 | 最小适配与验收 |
|---|---|---|
| O03 调度快照 | **开发** | 最小回移 `38b72d734`（模型限流 map 写时复制）、`53002f677`（调度进程内快照 memo）；保留 lite 精简快照序列化和用量冷却机制，不重新引入已裁剪字段或服务。补齐 Redis 修订号、缓存删除、并发读写与模型映射共享测试。**数据库持久化失败也必须保留内存阻断**，新增 `TestTransportCooldownRemainsInMemoryWhenDatabaseWriteFails`；不能以快照读出旧字段解除阻断。 |
| O06 Gemini/Antigravity 目录 | **安全增量已实现；历史 Vertex 冲突保持隔离** | 补上 `AntigravityGeminiModelIDs`：仅列出组内可调度账号有效映射 Gemini 模型；混合 Gemini 只取 `mixed_scheduling` 启用账号；在 native 模型信封中保留上游元数据、分页字段，追加未重复的 AG 模型；保持 lite `CustomModelsList` 优先及强制 AG 例外。原主工作树 `backport/2026-10-12-core-updates` 的 `gemini_models_endpoints.go`、Vertex 分页实现与 `upstream_models.go` 尚未提交且文本/行为冲突，**未覆盖、未复制、更没有将原工作树提交当作依赖**。未来合并 Vertex 分支前需对齐 `GeminiV1BetaListModels` 的选账号与添加 AG 模型的执行顺序，保留两者能力后再做联合回归。 |
| O07 Composite 精确别名 | **开发** | 从启用的 exact 路由列出 public alias，按 groupID 和 endpoint 限定，跳过 prefix 与 disabled、去重排序。追加到 lite 账号来源目录后，继续使用原 `CustomModelsList` 过滤，不引进上游不同的 `ModelAllowlist`、模型归属解析器或新 Codex 入口。 |
| O08 Codex 最大上下文 | **精简版已有核心等价处理；未恢复裁剪元数据模块** | `openai_codex_models_catalog.go` 的 `configuredCodexModelDescriptor` 已区分 `context_window` / `max_context_window`，对 GPT-5.6、GPT-6 Sol/Astra 和 Claude/Grok 有对应分档，现有 `openai_codex_models_catalog_test.go` 已验证二者。上游同步器 `UpstreamModelMetadata` / `openai_codex_model_metadata.go` 在 lite 未保留，故不迁入其整套模块；未来若确认需对第三方动态模型上游 `max_context_window` 传递，可另做限界回移。 |
| O11 输出 Token 最小值 | **按用户指示暂缓** | 未修改配置或协议转换器；先依现有协议兼容路径核查，只有出现真实低输出上限故障才评估最小补丁。 |
| O15 ctx_pool 容量 | **验证有问题并修复** | lite `mode_router_v2` 原逻辑直接 `min(account.Concurrency,hardCap)`，无视 `DynamicMaxConnsByAccountConcurrencyEnabled` 和 OAuth/API Key 倍率，导致不同 WS 模式容量不一致。修复为 v2 并发 ≤0 仍禁止调度，其余与普通 ctx_pool 共用动态倍率/硬上限逻辑；增加扩容、缩容、禁用和零并发测试。 |
| O16 WS 断流 Usage | **验重后补缺** | lite `openai_ws_forwarder_v2` 尚缺取消后有界排水：最小回移 upstream `a10ff1255` 的请求取消/下游写失败识别及有限上游继续读取；保留 lite 原有 `ImageCount` 和 `ImageOutputSizes` 计费独立修复，不能重复提交 usage。回归断流后最终 Usage 与无虚假 failure。与第四批 #35 中的 Anthropic/Gemini 及 ws 图片适配互不继承，若后续同时合并要重复验收一次。 |

## 冲突保留和回滚

- O06 历史未提交 Vertex 目录改动所在原工作区及其 `upstream_models.go` 的 merge 冲突均保持原样。该冲突不经擅自 `checkout`/reset 处理。本次只实装上游 AG mapping 在已合并 lite 目录入口的正交部分；Vertex 账号列表/详情合并需后续单独决策和完整回归。
- O03 上游测试使用 lite 不持久化的 `session_window_utilization`；仅将用例验证字段切换成 lite 实际保留的额度字段，**未扩大生产序列化白名单**。模型限流 reason 测试直接检查 lite 保留的 `Extra` JSON，不恢复不存在的 `modelRateLimitReason`。
- O16 上游 `UpstreamResponseServiceTier` 不存在于 lite 结构，保留 lite 原始 `extractOpenAIServiceTier` 语义与图片计数。与既有计费最终化需要单独断流回归。
- 所有代码可按独立 PR revert；不得自动合并、部署、发布镜像。任何安全/协议行为冲突优先保护精简版现有独立修复。

## 验证

- O03/O06/O07/O15/O16 定向 Go 回归通过：Redis 快照修订号、共享模型映射 COW、模型目录有效映射、Composite 路由别名白名单、ctx_pool 动态倍率和 WebSocket 取消后 Usage；数据库持久化失败后内存冷却保持阻断的故障注入测试通过。
- 针对 O03 的 `-race` 共享快照/读写并发测试通过；Go `vet ./...` 与 embed 编译通过。
- 前端 pnpm9 frozen-lockfile 安装、`lint:check`、`typecheck`、全量 Vitest 和生产构建通过。
- 最终 Go unit、race、前端、GitHub CI/Security Scan 具体结果须以最新提交 SHA 与真实运行记录为准，**未完成不写作通过**。
