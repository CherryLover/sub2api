# Sub2API v0.2.21–v0.2.22 选择性回移：开发任务清单与跨会话交接

> **2026-11-21 最新开发证据**：统一分支已本地提交 [Antigravity custom tool 与国产 Embeddings、调度/OAuth/用量/内存综合修复](2026-11-21-embeddings-antigravity-followup.md)，代码 SHA aa2f369bc。全仓 Go unit 52 测试包、关键 race 四包、真实 PostgreSQL/Redis 仓储 1,384 pass/1 skip 与审计 210 pass、前端 1,858 项及构建、Lint 0 均通过；**最终 PR HEAD 的 GitHub CI/Security 尚未执行，待审批数据库迁移、计费/默认路由及新 API 不得算完成**。未创建 PR、未合并或部署。


> **2026-11-21 后续真实验证更新**：调度回源、请求内存、Responses 历史修复已继续实施；真实 PG/Redis 仓储 1,384 项通过（1 原有跳过），审计 207 项通过。最新状态见[续作记录](2026-11-21-scheduler-memory-integration.md)，下方“数据库未运行”描述属于早期阶段。统一交付范围仍未全部完成。

> **2026-11-21 隔离统一开发续记**：见[开发与验收记录](2026-11-21-unified-validation.md)。统一代码最终收尾 `660108582d14d71812441660515f328b51509b11`；全仓 unit、关键 race、前端、构建、Lint 和本地安全门禁已有证据。真实 PostgreSQL/Redis 未执行，GitHub CI/Scan dispatch 返回 Actions disabled (HTTP 422)；剩余适用范围及统一 PR 仍未完成。原始工作区只读。

> **任务状态：进行中（集中开发、尚未统一提交最终 PR）。** 本文件是下一会话的工作入口，不表示全部 PR 都值得回移或用户已批准有争议的范围。`[ ]` 表示待办；任何任务都须以最新 `lite/main` 的源码和上游 PR diff 二次确认。
>
> **文件**：`docs/maintenance/upstream-reviews/2026-11-17-v022-backport-development-checklist.md`
> **基线时间**：2026-11-17；**上游**：[v0.2.22](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.22)（`15871f904056f548dd874a1d069ffc6cdf777b78`），前一正式版 [v0.2.21](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.21)；**精简版**：`CherryLover/sub2api:main`，v0.1.223，`71e3c2e4ee38da7e89a8f2ba72da0622af4e8975`。
>
> **PR 范围**：v0.2.20→v0.2.21 已合并 33 条；v0.2.21→v0.2.22 已合并 70 条；2026-11-10 以后仍活跃的 Open/Draft 112 条（2 Draft）。全量逐 PR 附在文末。开放历史 PR 如 #7157、#6359、#7469 同样纳入相关主线专题；不要仅筛选 `fix`。
>
> **已经覆盖的基线**：[精简版 PR #72](https://github.com/CherryLover/sub2api/pull/72) 已合并：Claude Code 已知系统提示词伪 429、裸 429 只冷却失败模型、Pro Argon 限额隔离、Go 1.26.11 相关安全修复。**不要重复引入 #8829/#8836 中相同部分**，其他 404/403/首字节/坏请求误冷却仍需单独判断。
>
> **原工作区保护**：Mac mini `/Users/leobot/Projects/sub2api` 正处于 `backport/2026-10-12-core-updates`，有历史未提交改动和 `backend/internal/service/upstream_models.go` 合并冲突。**不允许 reset/clean/stash/checkout/强行 merge 覆盖这些内容**；仅把原工作区用于只读与维护此任务文档，源码开发通过 WebCodex 在最新 `lite/main` 建立独立 worktree。Laya 是建议来源，不是实施者。

> **2026-11-18 续做**：新增 `02fe90cb9`（Anthropic TTL 5m/1h 用量保真）、`fc994ef2a`（Ollama Cloud reasoning 单遍编辑），详情见[增量审查](2026-11-18-v023-backport-addendum.md)。本地测试/编译通过，但未完成端到端定价、落库和 CI/Scan；仍按选择性回移政策继续。

> **2026-11-18 最新续做**：本地 `731d55198` 修复仅有 Anthropic 缓存 TTL 明细时用户使用日志 aggregate=0 的不一致，`449cf40b6` 补 GPT-6.2 WebSocket 模型限流与 Composite 映射模拟回归。定向 Go 测试重复 3 次及服务构建通过；**数据库集成、实际 WS、订阅资格及最终 CI/Security Scan 未完成**。详情见[增量审查](2026-11-18-v023-backport-addendum.md)。

> **2026-11-19 本地进展**：新增 `fa74ef070` 修复 Antigravity 兼容流 output-only delta 覆盖输入/缓存用量并加入 PostgreSQL TTL 往返集成用例；`7b24c3d54` 验证 Anthropic 明细-only 缓存写入对自定义账号统计与日志的影响。Go 单元/编译通过，但 Mac mini Docker 不可用，真实数据库测试仍需 GitHub CI。#8778 已 CLOSED 未合并，不能视为上游主线 Hydrogen 全量功能，详见[增量审查](2026-11-18-v023-backport-addendum.md)。

## 2026-11-18 上游 v0.2.23 增量范围

上游已从 v0.2.22 前进到 **v0.2.23**，main `2af3f1e15`，新增 GPT-6.2 Sol/Astra、OpenAI SSE deadline/换号预算、raw Chat 流式 flush、Anthropic 缓存 TTL、Ultra Fast 定价政策和若干性能/test 修复。**详见独立[增量回移清单](2026-11-18-v023-backport-addendum.md)**，继续按这份增量与原 54 项去重。当前本地 `backport/consolidated-v023-20261118` 新增提交 `6814647ff`、`9a91f0db6`、`76383eac0`；仅局部验收，**非整体验收，也未新建 PR**。

## 2026-11-17 交付策略更新（用户最新指令）

- **暂停逐项新建 PR**。所有适用于精简版的项目先统一评估、在独立 worktree 逐步实现并完成本地回归；全部需要实施的功能完成且整体验收通过后，才提交集中交付 PR。
- 已存在的 #73–#81 是策略变更前创建的独立 PR，仍待维护者合并；统一开发分支已经从 `lite/main` `8fe8e095e` 整合上述九项提交，最终交付时核实与这些 PR 的重叠，避免重复引入或重复审核，**未经确认不自动关闭或合并旧 PR**。
- 最终 PR 必须以可读的具体功能主题、实际改变、上游来源链接、测试证据及风险作为标题/正文，不使用 A/B、批次、阶段等内部任务代号。需审批的 DB/schema、定价策略、默认行为及大型平台范围仍独立请用户决策，不因集中交付而获得默认批准。
- 本地集中分支 `backport/consolidated-gateway-core-20261117`：已整合 #73–#81；另实现同请求最多冷却一个账号与 Pi 提示词定向兼容（`5e8bdab0e9a6e4a098db92008f2d809c5cde5d94`，上游开放 #8627/#8793），以及 Gemini 并行同名工具调用流式修复（`ecc0d2ec5`，上游已合并 #8828）。以上本地重点回归、编译通过；**未推送新分支、未建新 PR、未进行最终综合 CI/Scan**。

## 1. 下一个会话先做：启动门槛

- [ ] **BOOT-01｜读取规则**：先读 `AGENTS.md`、`CLAUDE.md`、`docs/maintenance/upstream-backport-workflow.md`、本文件、`cairn/ROADMAP.md`、`cairn/LOG.md`；按项目的选择性回移政策执行。
- [ ] **BOOT-02｜最新性复核**：只读 `git status`；更新上游 main、精简版 main、最新 release/tag；核对基线 SHA 是否变化，查全部新合并与 Open/Draft PR，差异追加到清单。变更中的主流新模型、新协议和安全修复都必须单独归档。
- [ ] **BOOT-03｜核对全部来源**：逐项审查附录 A 的 103 条已合并 PR、附录 B 的 112 条活跃 Open/Draft PR，标记“已覆盖、确定回移、待用户确认、暂缓、不适用”的**证据**；同一功能跨多个上游 PR 去重。附录 `[ ]` 的语义是“待对照”，绝不是“103+112 个都要实现”。
- [ ] **BOOT-04｜查精简版已有工作**：查 `CherryLover/sub2api` 的已合并和当前开放 PR，避免重复；核对现有默认行为，尤其 `#72`、Lite 的内存预算禁用默认值、平台裁剪和相关迁移编号。
- [ ] **BOOT-05｜创建隔离工作区**：通过 WebCodex `work_on_project`，`mode=worktree`、`client_id=device-440fcd9bb9a44593`、`path=/Users/leobot/Projects/sub2api`、`base_ref=lite/main`；不要从原 checkout 旧 HEAD 直接开发。
- [ ] **BOOT-06｜定位与复现**：每个缺口先读上游 PR patch + merge SHA，再检查精简版路径并编写失败回归；仅以 GitHub PR 标题/描述判断不足以宣称确定缺失或功能完成。
- [ ] **BOOT-07｜集中交付机制（以 2026-11-17 最新用户指令为准）**：先完成全部确定适用于精简版的回移和测试，再集中提交 PR；不再按小任务单独创建 PR。最终标题、正文必须明确真实修复/新能力和原上游链接，不能写内部代号。最终 SHA 跑完整 GitHub CI 与 Security Scan，失败则修复复验；**不自动合并、发布或部署**。

### 状态与风险定义

**P0** 为实际安全漏洞、全池不可用、明确账单错误、已发布主流模型关键缺口；**P1** 为协议、配额/缓存恢复、主线兼容；**P2** 为 UI 与可测量性能改良。每个任务若涉及数据库迁移、修改用户账单口径/默认路由/权限、新平台或大型组件，则先汇报精简版影响请求用户确认；不把新 PR 存在误认为上游已审核完成。

## 2. 第一阶段 A：安全、调度与账号可靠性（优先启动）

- [x] **A-01｜P0 原子 Refresh Token 轮转**；上游 [#8637](https://github.com/Wei-Shaw/sub2api/pull/8637)，v0.2.21。精简版仍按 `GetRefreshToken`、`DeleteRefreshToken` 两步消费。实现 Redis 原子消费/等价 CAS，16 个并发只成功 1 个，失败时不能发新令牌；保持原会话/错误边界。
  - 2026-11-17 实施：原子 Redis Lua GET+DEL、认证轮转失败关闭；16 并发缓存消费和真实 AuthService 并发轮转、Redis 故障回归通过，受影响 Go 三包聚焦用例连续 5 次与编译通过。提交 `7564c889c165d52a35c4195a713d2db69edc9ce6`；[Draft PR #74](https://github.com/CherryLover/sub2api/pull/74)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44051513737)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44051513761) 正在运行，**最终 CI/Scan 全绿，PR #74 已转 Ready 并留言复审（未合并）**。
- [x] **A-02｜P0 hydration 失败释放并发槽**；[#8631](https://github.com/Wei-Shaw/sub2api/pull/8631)，v0.2.21。精简版 `gateway_scheduling.go:newSelectionResult` 仍缺少失败时 `release()`。验收失败释放恰好 1 次、成功不提前释放，Redis 故障和删除账号不残留槽位/503。
  - 2026-11-17 实施记录：隔离 worktree 基于新 `lite/main` `8fe8e095e`；A-02 修复提交 `66a6ba503ae19cca4e65c1f237d12cfa0ca2cb77`，[精简版 PR #73](https://github.com/CherryLover/sub2api/pull/73)；聚焦回归 10 次、Go service 编译、diff check 通过。完整 service 单测本地 120 秒超时，目标 [CI](https://github.com/CherryLover/sub2api/actions/runs/44034992989) 全部通过；[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44034992885) 前后端均通过。**最终目标 CI/Scan 均通过，PR #73 已转 Ready 并留言请求复审（未合并）**。
- [x] **A-03｜P0 RPM override 仅本组生效**；[#8651](https://github.com/Wei-Shaw/sub2api/pull/8651)，v0.2.22。精简版仍用 `override != nil || groupID == group.ID`。A 组免检不得放行 B 组，同组保持快照优化，用户硬上限不被绕过。
  - 2026-11-17 实施：复现跨组零值 RPM override 导致其他分组限额失效，改为仅匹配 snapshot 的所属组；跨组/同组用例重复 10 次及 Go service 编译通过。提交 `a213889723f8a63056775cb48cb718a66a2f05e5`，[PR #75](https://github.com/CherryLover/sub2api/pull/75)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44052790585) 与 [Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44052790628)，**最终 SHA 目标 CI/Scan 全绿，PR #75 已转 Ready 并通知维护者（未合并）**。
- [x] **A-04｜P0 冷却到期自动恢复**；[#8729](https://github.com/Wei-Shaw/sub2api/pull/8729)。对照 scheduler snapshot 与 DB fallback；冷却到期不依赖重建就能回池，永久禁用/停调不能误恢复。需要 Redis/PostgreSQL 集成回归。
  - 2026-11-17 开发：基于最新 `lite/main` `8fe8e095e` 的独立 worktree，复现 rate-limit/overload/temp-unschedulable 冷却到期后账号仍缺席快照；修复为快照保存配置池，每次读取重新判断 `IsSchedulable`，保留分组/平台隔离。定向回归 3 次、调度相关测试和 Service 编译通过。提交 `f8fefe252c3279a624ca0be0b3b5d4c73afd892d` + `9aa7104281ca526726922e02923a1833a1437158`；[Draft PR #80](https://github.com/CherryLover/sub2api/pull/80)、[目标 CI](https://github.com/CherryLover/sub2api/actions/runs/44056649373)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44056649405)。补充 Redis/PostgreSQL 成员资格集成测试，但本地 Docker 不可用，实际跳过；**等待最终 SHA GitHub CI 中的真实集成测试与安全扫描完成，不提前勾选 A-04**。
  - 后续验证：首次 [CI](https://github.com/CherryLover/sub2api/actions/runs/44056649373) 的 PostgreSQL 集成因共享测试库中其他 fixture 账号导致原测试断言误判（真实业务修复未报错）；已限制断言到本 fixture 自有 ID，仍覆盖分组隔离，修正提交 `06ae9972eecb63a8c769ada92cf063db547a957e`。[新 CI](https://github.com/CherryLover/sub2api/actions/runs/44058642527)、[新安全扫描](https://github.com/CherryLover/sub2api/actions/runs/44058642480) 尚未全部结束，PR #80 保持 Draft。
  - 最终验收：`06ae9972eecb63a8c769ada92cf063db547a957e` 的 [目标 CI](https://github.com/CherryLover/sub2api/actions/runs/44058642527)（Go unit/integration，Frontend，Shell，golangci-lint）及 [Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44058642480)（前后端）全部通过；[PR #80](https://github.com/CherryLover/sub2api/pull/80) 已转 Ready 并[请求复审](https://github.com/CherryLover/sub2api/pull/80#issuecomment-6944710410)，未合并或部署。
- [x] **A-05｜P0 Claude 测试默认模型替换**；[#8757](https://github.com/Wei-Shaw/sub2api/pull/8757)。精简版默认 `claude-sonnet-4-5-20250929`，上游提示 11 月 30 日退役；切 Sonnet 4.6，测 OAuth/API Key/Bedrock/CN 兼容，不删仍有效的显式旧模型。
  - 2026-11-17 实施：上游 #8757 将仅供账号连通性测试的 `claude.DefaultTestModel` 调整为 `claude-sonnet-4-6`；保留显式旧模型目录和映射。失败回归复现并修复，Claude package 测试、Claude/service 编译通过；提交 `f55bb03c299e2ce874e7184e72c9d723cdde5d01`，[PR #77](https://github.com/CherryLover/sub2api/pull/77)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44053764952)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44053764945) 运行中。**最终 SHA CI/Scan 全绿，PR #77 已转 Ready（未合并）；真实上游账号可用性仍需运行时确认**。
- [x] **A-06｜P0 Chat JSON Schema strict 缺省**；[#8761](https://github.com/Wei-Shaw/sub2api/pull/8761)。精简版 `apicompat/response_format.go` 未给缺省 `strict=false`；显式 true/false 原样，避免 Chat→Responses 意外严格化 400。
  - 2026-11-17 实施：上游 #8761；复现缺失 `strict` 原样传出导致 Responses 默认差异，修复缺省 `false`、保留显式 true/false；全 apicompat package 测试和编译通过。提交 `4209a72375fb4925032e65cddab22d81bff8fb2b`，[PR #76](https://github.com/CherryLover/sub2api/pull/76)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44053249170) 和 [Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44053249207) 运行中，**最终 SHA 目标 CI/Scan 全部通过，PR #76 已转 Ready 并请求复审（未合并）**。
- [x] **A-07｜P0 非法 UTF-8 请求本地拒绝**；[#8575](https://github.com/Wei-Shaw/sub2api/pull/8575)。检查是否等价，未覆盖时在 Responses 路由前准确返回 400，不能把客户端错误变成可重试的账号错误。
  - 2026-11-17 实施：上游 #8575，handler 在 JSON 解析及账号路由前拒绝非法 UTF-8，以客户端 400 返回。9 种非法输入变体（流式 true/false/缺省）与合法 Unicode 测试重复 10 次、handler 编译通过。提交 `b4c50999c864efd87a993528f1aaf11de5e789d7`，[PR #78](https://github.com/CherryLover/sub2api/pull/78)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44054361952)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44054361764) 进行中，**最终 SHA CI/Scan 全绿，PR #78 已转 Ready 并请求复审（未合并）**。
- [x] **A-08｜P0 Anthropic 兼容路由执行账号错误策略**；[#8385](https://github.com/Wei-Shaw/sub2api/pull/8385)。精简版如遗漏自定义 410/停调规则则最小修复；保留客户端普通 4xx 终止，避免过宽 failover。
  - 2026-11-17 实施：对照上游 #8385，复现 Chat 与 Responses 的自定义 410/停调规则在默认 failover 列表外未执行；只在账号策略要求时切换，普通 4xx 仍终止。两协议 × 流/缓冲 × 自定义/默认状态回归 5 次及 service 编译通过。提交 `ee72d99ebfa08053f02d0b9bf2e5fd0a501f15e3`，[PR #79](https://github.com/CherryLover/sub2api/pull/79)，[CI](https://github.com/CherryLover/sub2api/actions/runs/44055505102)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44055505030) 已通过，**最终目标 CI 和双端 Security Scan 全部通过，PR #79 已转 Ready 并留言请求复审，未合并**。
- [x] **A-09｜P1 批量同步分组费率/RPM 原子性**；[#8659](https://github.com/Wei-Shaw/sub2api/pull/8659)。全量同步多语句必须在一个事务，外键/重复用户 ID/任意插入失败不留下部分删除，缓存数据一致。
  - 2026-11-17 开发：对照上游 #8659，将全量分组倍率/RPM 同步改为单事务，失败自动回滚，重复用户 ID 按最后一项去重。sqlmock 提交/回滚/去重回归重复 5 次和 repository 编译通过；提交 `1b5c8a05a12a340a5a92bfbaa5ea12fb642d4b13`、[Draft PR #81](https://github.com/CherryLover/sub2api/pull/81)、[CI](https://github.com/CherryLover/sub2api/actions/runs/44057689677)、[Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44057689733)。**仍待 GitHub 最终 SHA 的 PostgreSQL 集成、全部 CI 和 Scan 验收**。
  - 补充验证：为避免仅用 sqlmock 证明事务行为，新增真实 PostgreSQL 外键插入失败后的原倍率/RPM 保留回归；合并到既有 PR #81，不重复创建 PR。新提交 `4fbcb6313bacbf5664d4d3e8b5e96e9444dc240c`；[最终 SHA 的 CI](https://github.com/CherryLover/sub2api/actions/runs/44058398377) 和 [Scan](https://github.com/CherryLover/sub2api/actions/runs/44058398358) 待验收。本地 Docker 不可用，集成测试被跳过，未记通过。
  - 最终验收：目标 PR #81 在提交 `4fbcb6313bacbf5664d4d3e8b5e96e9444dc240c` 上的 [CI](https://github.com/CherryLover/sub2api/actions/runs/44058398377)（Go unit/integration、Frontend、Shell、golangci-lint）及 [Security Scan](https://github.com/CherryLover/sub2api/actions/runs/44058398358)（前后端）全部通过；PR 已转 Ready 并[留言请求复审](https://github.com/CherryLover/sub2api/pull/81#issuecomment-6944681503)，未合并或部署。
- [ ] **A-10｜P0 继续处理 Antigravity/Gemini 429/503**；开放 [#8878](https://github.com/Wei-Shaw/sub2api/pull/8878)、[#8793](https://github.com/Wei-Shaw/sub2api/pull/8793)、[#8781](https://github.com/Wei-Shaw/sub2api/pull/8781)、[#8744](https://github.com/Wei-Shaw/sub2api/pull/8744)、[#8627](https://github.com/Wei-Shaw/sub2api/pull/8627)、[#8730](https://github.com/Wei-Shaw/sub2api/pull/8730)。逐个证实假 429、账号级 404、首字节超时、请求级 403、坏请求逐号冷却；确保能无感切健康端点/账号但不错误惩罚整个池。不复制所有 Open PR 为一包。
  - 2026-11-17 再核对：上游 #8878/#8793/#8781/#8744/#8627/#8730 均仍为 Open，暂无可不经验证整包套用的已合并补丁。后续按伪 429、账号级 404、first-byte timeout、请求级 403 与坏请求逐号冷却分别复现后最小实现；保留精简版 #72 已合入修复，避免重复。
- [ ] **A-11｜P1 OAuth/setup-token 401 恢复与多实例并发租约**；开放 [#8856](https://github.com/Wei-Shaw/sub2api/pull/8856)、[#8774](https://github.com/Wei-Shaw/sub2api/pull/8774)、[#8685](https://github.com/Wei-Shaw/sub2api/pull/8685)、[#8671](https://github.com/Wei-Shaw/sub2api/pull/8671)。账号测试不能早期 401 永久停号，重启不能清其他实例租约；先对照/分开交付。
  - 2026-11-17 追加开发：已按上游已合并 [#8737](https://github.com/Wei-Shaw/sub2api/pull/8737) 实现 Claude OAuth 轮换令牌后的同号单次重试（Messages/Chat/Responses），及开放 [#8774](https://github.com/Wei-Shaw/sub2api/pull/8774) 的 setup-token 401 可恢复处理；提交 `c5c3d6c52`、`c0048497f`。带 `unit` 标记的多场景回归、现有 401 策略回归和 Go service/handler 编译通过。**多实例租约与 OpenAI OAuth 测试 401 仍未完成；最终 CI/Scan 待统一交付后验证，不勾选。**

**建议首批 PR 主题**：刷新令牌原子消费；并发槽及冷却恢复；RPM 跨组隔离；Claude 默认测试与 JSON Schema 兼容（可分开）。A-10/11 为独立高风险专题。

## 3. 第二阶段 B：最新模型和主线档位（P0/P1）

- [ ] **B-01｜Claude Fable 5.2 / Mythos 5.2**；[#8639](https://github.com/Wei-Shaw/sub2api/pull/8639)、[#8634](https://github.com/Wei-Shaw/sub2api/pull/8634)，v0.2.21。覆盖模型目录、Bedrock/OAuth 映射、前端选项、adaptive/signed/redacted thinking、Fast 2×、$8/$40 及缓存 5m/1h 价、客户端版本门禁与工具往返；Mythos 仍需真实上游 eligibility。
  - 2026-11-18 本地开发：基于上游已合并 #8639/#8634 最小适配 Fable/Mythos 5.2 目录、Bedrock 模型映射、Claude CLI 版本门槛 2.1.321、协议 Thinking/signed history、Fast 模式适用性、前端模型选项及独立 5m/1h 缓存写入默认价卡；保留 Lite 旧 Fable 型号原价（未机械套用上游旧价）。本地提交 `aed6cc09d3a1a15f0310fa5220a21f9fc533636b`，Claude/apicompat/domain 全包、相关 Service unit、Handler/Service 编译及前端 TypeScript/16 项 whitelist 测试通过。**Mythos eligibility 未在真实账号验证、价格仍需最终审计及统一 CI/Scan，暂不标完成。**
- [ ] **B-02｜Gemini 4 Carbon 完整支持**；[#8661](https://github.com/Wei-Shaw/sub2api/pull/8661)，v0.2.22。精简版缺模型；补原生 Gemini、CLI、Antigravity、目录/白名单/映射、thinking level、缓存/priority 价和上下文；注意 2026-12-01 价格调整，需要重新核实，不永久写死促销价。
  - 2026-11-17 继续开发：隔离 worktree `backport/consolidated-mainline-followup-20261117` 已按上游 #8661 补齐 Carbon 的 Gemini/CLI/Antigravity 模型目录、映射、前端白名单、thinking level 与官方促销期价卡，提交 `42ebdcfc918a4195474a75a4ab9ae87e64538392`；聚焦 Go 包测试重复 3 次及编译通过。**2026-12-01 价格调整尚需用户批准日期敏感的结算处理，并补综合回归/CI，暂不标完成、暂不提交 PR。**
- [ ] **B-03｜Gemini 4 Hydrogen**；已合并定价 [#8746](https://github.com/Wei-Shaw/sub2api/pull/8746)，完整模型/effort 仍开放 [#8778](https://github.com/Wei-Shaw/sub2api/pull/8778)。核对 Carbon 重叠，覆盖 `low/medium/high/xhigh` 与动态变体、映射/usage/价卡；未全链路验证不能称完整支持。
  - 2026-11-18 风险复核：上游 #8778 仍为 Open，跨约 35 个文件并包含调度、usage、价卡及 native/compat 多入口，对 Lite 不适合未经全链路验证机械移植；已合并 #8746 的价格部分仍待与 Carbon 促销期及实际账单口径统一确认，尚未修改 Hydrogen 业务代码。
- [ ] **B-04｜OpenAI Ultra Fast**；[#8812](https://github.com/Wei-Shaw/sub2api/pull/8812)，v0.2.22。`force_openai_ultrafast` 仅支持官方适用模型，HTTP/WS/Composite 一致；Sol 倍率从 2× 修正 6×。**强制档位及价格变化需用户确认**。
- [ ] **B-05｜Qwen3 Reranker 价卡**；[#7540](https://github.com/Wei-Shaw/sub2api/pull/7540)，v0.2.22。精简版已有 Qwen3-Embedding 兜底，Reranker 未完整覆盖。验证不同尺寸/前缀/未知型号、只有输入 token 计费；与 F-02 的路由同时评估。
- [ ] **B-06｜主流模型六维回归**：每个新增模型均核对 `/models`、model mapping/alias、白名单及前端、协议和 thinking/多模态、pricing+usage、Go/前端/模拟转发测试；旧型号 GPT-6.1 Terra、Gemini Argon、Claude 4.6、Grok 仍可调用。
- [ ] **B-07｜新定价变动追踪**；开放 [#7469](https://github.com/Wei-Shaw/sub2api/pull/7469) GPT-5.6 Sol 促销价及新提交；引用当前官方价与日期，不依赖过期 PR 自行猜测价。

## 4. 第三阶段 C：协议、长流、工具与无感切换（P1）

- [ ] **C-01｜Antigravity 长推理静默超时**；[#8673](https://github.com/Wei-Shaw/sub2api/pull/8673)：流式 `includeThoughts`、effort 映射、首内容截止时间；验证高思考量场景，保留现有 URL failover。
- [ ] **C-02｜Antigravity 兼容空流解绑**；[#8826](https://github.com/Wei-Shaw/sub2api/pull/8826)：keepalive 后 empty_stream/stream_timeout 只删仍绑定该失败账号的会话，不全号冷却。
  - 2026-11-18 代码核对：上游已合并 #8826 的前置 keepalive 已提交后清理粘性会话问题在当前 Lite `antigravity_gateway_compat_stream.go` 路径尚不复现：流首个有意义内容前不发送 `: ping`，空流/首内容前超时仍返回可切换的 `antigravityCompatEmptyStreamError`，因此不会发生已提交 200 后重新命中原会话绑定。**此处先记为 Lite 现有行为已避免该触发条件，不复制上游整包；若后续引入前置 keepalive 必须重新审查并补定向解绑回归。**
- [ ] **C-03｜Gemini 并行同名 tool calls**；[#8828](https://github.com/Wei-Shaw/sub2api/pull/8828)：不同 ID 分离、参数 delta/快照不拼错，Chat+Messages 流式与非流式回归。
- [ ] **C-04｜Gemini 工具 Schema 和结构化输出**；[#8759](https://github.com/Wei-Shaw/sub2api/pull/8759)、[#8662](https://github.com/Wei-Shaw/sub2api/pull/8662)：array items 与选定 type 匹配；Chat JSON Schema 进入原生 `generationConfig`，工具/图片禁用条件保持。
  - 2026-11-17 继续开发：上游 #8759/#8662 的核心内容已最小化适配至 Lite 的 Antigravity/Gemini；array `items` 与选定 type 一致、Chat `response_format` 进入原生 `generationConfig`，流式/缓冲与三协议模拟上游测试通过，本地提交 `3c366ab0d12064d0106ab8c5532e947916b7b0d8`。仍待最终统一 CI/Scan，不提前提交 PR。
- [ ] **C-05｜OpenAI SSE 首输出与错误传播**；[#8811](https://github.com/Wei-Shaw/sub2api/pull/8811)、[#8813](https://github.com/Wei-Shaw/sub2api/pull/8813)、[#8724](https://github.com/Wei-Shaw/sub2api/pull/8724)、[#8808](https://github.com/Wei-Shaw/sub2api/pull/8808)：首内容前 failover、上游 keepalive 及时下发、已提交 SSE 时返回 error event、spool 写盘失败回退有界内存。
  - 2026-11-17 部分完成：上游 #8813 的 OpenAI 上游 keepalive 可在首个语义输出前及时下发，且不修改 TTFT/首输出超时或阻断 failover；先复现失败后实现，目标和扩大后的 Go 回归 + service 编译通过，本地提交 `c72a30860910ef3cbea4e4437be966d33816113e`。本项还含 #8811/#8724/#8808 的独立路径，尚未全部开发/验收。
  - 2026-11-17 补充：上游 #8808 的首输出 spool 临时文件创建失败问题已先失败复现再修复：自动退化到原容量上限内的内存缓冲，不丢 usage/首 token，提交 `1bb4ea2fd`，targeted/expanded Go tests 与 service build 通过；仍需 #8811/#8724 和最终 CI/Scan，**C-05 未完成**。
  - 2026-11-18 继续开发：按已合并上游 #8724 实施在 HTTP 200/SSE keepalive 已提交后的上游错误，以 SSE `event:error` 而非 JSON 报告；未提交 SSE 时保留普通 HTTP 错误和 400 透传。首次回归复现了错误写入，修复后测试重复 3 次 + service/handler 编译通过；本地提交 `1653e119c1ce4b2625e6a00453f6ba5f56950fce`。#8811 等仍待审查，最终集中 CI/Scan 未执行，暂不勾选。
- [ ] **C-06｜流已输出后账号错误副作用**；[#8814](https://github.com/Wei-Shaw/sub2api/pull/8814)：401、额度耗尽、in-band terminal error 更新账号健康；请求级错误/503 容量不误停号。
  - 2026-11-18 已开发部分：适配已合并上游 #8814，在 OpenAI Chat/Messages 兼容流已下发语义内容后遇到认证错误时更新账号健康，但不非法切号重放；普通客户端错及请求级 503 容量降载不误停号。Lite 焦点测试重复 2 次、扩大回归和 service/handler build 通过，本地提交 `fcac6ab18a27833e14fcc17d4e4ccd8de1e29520`。更广账号/状态副作用边界与最终综合 CI 尚待验收。
- [ ] **C-07｜Codex Desktop 新上下文和工具 ID**；[#8622](https://github.com/Wei-Shaw/sub2api/pull/8622)、[#8769](https://github.com/Wei-Shaw/sub2api/pull/8769)、[#8754](https://github.com/Wei-Shaw/sub2api/pull/8754)：页面上下文/`browser_info`/`goal_objective`/`untrusted_context`/`additional_tools.id` 归一，保留工具关联和大整数。
- [ ] **C-08｜Codex WS/会话缓存身份**；[#8564](https://github.com/Wei-Shaw/sub2api/pull/8564)、[#8589](https://github.com/Wei-Shaw/sub2api/pull/8589)、[#8664](https://github.com/Wei-Shaw/sub2api/pull/8664)、[#8358](https://github.com/Wei-Shaw/sub2api/pull/8358)：后续轮次不能误重放首帧；工具循环 hash 与 fork 父缓存标识保持；搜索激活前切连接。
- [ ] **C-09｜Anthropic per-turn beta / effort / mid-system**；[#8742](https://github.com/Wei-Shaw/sub2api/pull/8742)、[#8739](https://github.com/Wei-Shaw/sub2api/pull/8739)、[#8703](https://github.com/Wei-Shaw/sub2api/pull/8703)：三兼容入口不丢输出控制，正确封顶和拒绝，assistant/工具/thinking 轮次保真。
- [ ] **C-10｜OpenAI Python regex/tool schema**；[#8772](https://github.com/Wei-Shaw/sub2api/pull/8772)：过滤不支持的 pattern/`patternProperties` 并保留 Python 合法 regex 与内嵌 schema。
  - 2026-11-18 本地开发：按上游已合并 #8772 仅针对 OpenAI Responses 实际工具 Schema 清除 Python 规则不接受的 lookaround、命名分组和特殊转义，移除无效 `patternProperties` key 时恢复可接受属性；保护 valid regex、实例 `default`、较大数字字面量及精简版局部字节编辑。提交 `9dd57edd464293e424ff363b1791320063b1d7e4`，聚焦及既有 schema 单测重复 3 次、Go service/handler build 通过；**仍需合并链路与最终统一 CI/Scan，暂不勾选。**
- [ ] **C-11｜Claude 后台轮换令牌后的 401**；[#8737](https://github.com/Wei-Shaw/sub2api/pull/8737)：比较实际发出 token 与最新持久化 token，至多同号重试一次；不要变成永久冷却。
  - 2026-11-17 本地实现：上游 #8737 的 Claude OAuth 发出旧 token 遇 401 时重读持久化 token、不同才单次同账号重试，不覆盖新凭据，保持精简版 Anthropic 401 默认不冷却策略；`c5c3d6c52`，unit 回归（全部三入口的流式/非流式、缓存与失败边界）重复 3 次、编译通过。**最终统一 CI/Scan 尚未执行，不提前勾选。**
- [ ] **C-12｜真实响应模型/服务档位与端点记录**；[#8745](https://github.com/Wei-Shaw/sub2api/pull/8745)、[#8727](https://github.com/Wei-Shaw/sub2api/pull/8727)、[#8728](https://github.com/Wei-Shaw/sub2api/pull/8728)、[#8796](https://github.com/Wei-Shaw/sub2api/pull/8796)、[#8741](https://github.com/Wei-Shaw/sub2api/pull/8741)、[#8786](https://github.com/Wei-Shaw/sub2api/pull/8786)；公开路由模型、真实响应模型、用户 metadata、实际请求 endpoint 彼此分开，不能错误计费。
- [ ] **C-13｜Grok 媒体与服务端搜索**；[#8669](https://github.com/Wei-Shaw/sub2api/pull/8669)、[#8834](https://github.com/Wei-Shaw/sub2api/pull/8834)，开放 [#8864](https://github.com/Wei-Shaw/sub2api/pull/8864)：错误码与消息脱敏、服务端 search 不能污染客户端工具调用和 search 成本。
- [ ] **C-14｜开放协议关键修复**；[#8832](https://github.com/Wei-Shaw/sub2api/pull/8832) Responses→Anthropic 末尾 assistant/thinking；[#8750](https://github.com/Wei-Shaw/sub2api/pull/8750) reasoning 历史泄漏；[#8809](https://github.com/Wei-Shaw/sub2api/pull/8809) signed thinking details；[#8770](https://github.com/Wei-Shaw/sub2api/pull/8770) WS 当前轮次恢复。严格复现/讨论依赖后决定是否提前回移。
  - 2026-11-18 已开发部分：针对上游仍开放 #8750，Chat→Responses 历史 reasoning 从可见 assistant content 的 `<thinking>` 包裹分离成独立 reasoning item，保留 visible text 与工具序列。`go test ./internal/pkg/apicompat` 全包通过，Go build 通过，本地提交 `3e564316891b50a097362baa3e78526e7f342909`。其他 Open PR 均仍待单独审查。

**阶段 C 共用验收**：真实 handler/forwarder+模拟上游的流式/缓冲、首轮/续轮、SSE 已提交/未提交、工具并行/加密内容、超时/取消/多账号切换、usage；不可只测纯转换函数。

## 5. 第四阶段 D：额度、账单与缓存准确性

- [ ] **D-01｜OpenAI 缓存写入 30m/12h 计量**；[#8606](https://github.com/Wei-Shaw/sub2api/pull/8606)，v0.2.21：按真实 `ttl_30m_tokens/ttl_12h_tokens` 落账；`cache_write_12h_price`；上游 SQL 迁移 250 不能直接占用精简版编号。**需用户确认迁移/定价行为**，不重算历史账单。
- [ ] **D-02｜Fable 7d_oi 周配额与自愈**；[#8644](https://github.com/Wei-Shaw/sub2api/pull/8644)、[#8723](https://github.com/Wei-Shaw/sub2api/pull/8723)、[#8779](https://github.com/Wei-Shaw/sub2api/pull/8779)、[#8698](https://github.com/Wei-Shaw/sub2api/pull/8698)、开放 [#8831](https://github.com/Wei-Shaw/sub2api/pull/8831)：5h 重置不删除 7d，真实恢复证据才清 Fable 模型冷却；防错清其他限额。
- [ ] **D-03｜Kimi 月度套餐额度**；[#8763](https://github.com/Wei-Shaw/sub2api/pull/8763)：`usages.limit_5h/7d/month_total/month_code`；合法空档位清理，错误 JSON/NaN/2xx 空响应不能误开放；**不恢复 OpenCode Go 商业模块**。
- [ ] **D-04｜DeepSeek 显式价格覆盖**；[#8825](https://github.com/Wei-Shaw/sub2api/pull/8825)：显式非负值与零值优先于官方缺省价，峰谷/组倍率/缓存/热重载一致。
- [ ] **D-05｜费用与使用记录口径**；[#8806](https://github.com/Wei-Shaw/sub2api/pull/8806)、[#8727](https://github.com/Wei-Shaw/sub2api/pull/8727)、[#8728](https://github.com/Wei-Shaw/sub2api/pull/8728)：`actual_cost` 不混入账号成本；上游实际 response model/tier 保真，用户页面不能泄露内部成本。
- [ ] **D-06｜OpenAI/Gemini 额度刷新真实性**；[#8747](https://github.com/Wei-Shaw/sub2api/pull/8747)、[#8740](https://github.com/Wei-Shaw/sub2api/pull/8740)、[#8581](https://github.com/Wei-Shaw/sub2api/pull/8581)：失败刷新不把旧额度标成当前，未知为未知；刷新时不闪烁/变 0。
  - 2026-11-17 部分完成：按已合并上游 [#8740](https://github.com/Wei-Shaw/sub2api/pull/8740) 修改 Gemini 配额展示，后台刷新且存在旧 usage 时继续显示配额条而非闪烁骨架；提交 `09aded53f`，Vue/TypeScript typecheck 通过。**OpenAI/Gemini 额度刷新真实性 #8747/#8581 尚待回移，不勾选。**
  - 2026-11-18 继续开发：按已合并上游 #8747 的 OpenAI quota 观测时间/失败可见性与 #8581 的 UI 保留旧值实现，刷新失败保留旧 `UpdatedAt`，本地窗口只显示“未知”而非伪 0%，前端显示缓存及错误状态并避免刷新时卸载额度条/表格行。此前 #8740 Gemini 配额条处理保留。提交 `d725ab3c979cc520be724e536be90a19edffcf2e`、`07abd5be19d72cccd5062bb664792dc7a9f608a0`，Go focus、Service/Handler build、Vue typecheck、AccountUsageCell 49 项测试通过；最终集成及 CI/Scan 待统一交付，暂不勾选。
- [ ] **D-07｜Vertex 多区域和 Gemini 会话粘性**；[#8827](https://github.com/Wei-Shaw/sub2api/pull/8827)、[#8822](https://github.com/Wei-Shaw/sub2api/pull/8822)：us/eu 正确 host、不支持模型 global fallback、缓存区域一致、按实际 1.1× 计价；更稳定的首 user 会话 hash，换号按实际 usage，不虚报缓存减免。**涉及路由和价格行为，需确认。**
- [ ] **D-08｜本轮价格总审计**：Fable/Mythos 5.2、Carbon/Hydrogen、Ultra Fast、Qwen3 Reranker、DeepSeek、开放 [#7469](https://github.com/Wei-Shaw/sub2api/pull/7469) Sol 促销价；按官方有效日期验证，不猜测也不追溯旧账单。

## 6. 第五阶段 E：资源预算/性能（先测再回移）

- [ ] **E-01｜请求体内存准入**：已有精简版 `request_body_memory_budget_bytes` 默认关闭，区别上游 [#8610](https://github.com/Wei-Shaw/sub2api/pull/8610) 自动 1/4、最低 1GiB；开放 [#8875](https://github.com/Wei-Shaw/sub2api/pull/8875) → [#8876](https://github.com/Wei-Shaw/sub2api/pull/8876) 发现 handler 后引用和 JSON 峰值 OOM。先用 2GiB/并发大图测试验证，**未批准不改变默认值**，按依赖审查。
- [ ] **E-02｜调度快照 bucket 合并**；[#8833](https://github.com/Wei-Shaw/sub2api/pull/8833)：缓存复用/DB fallback singleflight/Redis 读数；确保限流和账号停用即时可见，压力和失败恢复测试后再启用。
- [ ] **E-03｜按用户批量结算**；[#8816](https://github.com/Wei-Shaw/sub2api/pull/8816)：高风险事务，锁顺序、幂等、余额零界、失败逐条回退/停机排空；用户确认且真实 PostgreSQL 集成与并发压测前不回移。
- [ ] **E-04｜热路径零拷贝优化**；[#8653](https://github.com/Wei-Shaw/sub2api/pull/8653)、[#8667](https://github.com/Wei-Shaw/sub2api/pull/8667)、[#8735](https://github.com/Wei-Shaw/sub2api/pull/8735) 及开放 [#8874](https://github.com/Wei-Shaw/sub2api/pull/8874)、[#8872](https://github.com/Wei-Shaw/sub2api/pull/8872)、[#8870](https://github.com/Wei-Shaw/sub2api/pull/8870)、[#8868](https://github.com/Wei-Shaw/sub2api/pull/8868)：以 bytes/分配、保真与内存边界 benchmark 决定，不大面积机械合并。
- [ ] **E-05｜上游 TCP 死连接/价格启动/汇总清理**；[#8800](https://github.com/Wei-Shaw/sub2api/pull/8800)、[#8798](https://github.com/Wei-Shaw/sub2api/pull/8798)、[#8693](https://github.com/Wei-Shaw/sub2api/pull/8693)：检查精简版是否存在同路径；长 SSE、SOCKS5、Linux 选项、用量日志不丢记录。
  - 2026-11-18 已实施其中 TCP 安全项：依据上游已合并 #8800，将 TCP keepalive 配置为 idle 15s、interval 5s、count 3，Linux 尝试 TCP_USER_TIMEOUT=30s，其他系统不设置 Linux socket option。提交 `8271b7f63e94041ba54e02c2ebd86c37195df941`，Mac 端拨号/context/代理回归重复 3 次及 Linux amd64 交叉构建通过；**Linux 真机 socket 行为和 #8798/#8693 尚待验证，暂不勾选。**
- [ ] **E-06｜未合并资源安全相关 PR**：[#8685](https://github.com/Wei-Shaw/sub2api/pull/8685) Redis 多副本租约、[#8705](https://github.com/Wei-Shaw/sub2api/pull/8705) 长请求体持有、[#8537](https://github.com/Wei-Shaw/sub2api/pull/8537) 停机预算，按生产风险决定优先级。

## 7. 第六阶段 F：新增端点/平台（明确需分析，部分需用户确认）

- [ ] **F-01｜Rerank 端点**；开放 [#7157](https://github.com/Wei-Shaw/sub2api/pull/7157)，配套已合并 [#7540](https://github.com/Wei-Shaw/sub2api/pull/7540)：`POST /v1/rerank`、versioned base、账号能力、失败换号、`usage/meta.tokens` 计费；可作为精简版中转新能力优先评估。
- [ ] **F-02｜OpenAI Decisions API**；开放 [#8849](https://github.com/Wei-Shaw/sub2api/pull/8849)：`/v1/decisions`、按账号 capability 调度、输入/缓存计费、Composite 路由、迁移 251；先确认 API 是否需要与数据库 schema 变更。
- [ ] **F-03｜DeepSeek 应用工具/Files/多模态**；开放 [#8817](https://github.com/Wei-Shaw/sub2api/pull/8817)：search/fetch/OCR/ASR/TTS/Files 等，先拆可直接透传的核心端点，账单按官方用量，文件生命周期/大型模块需用户确认。
- [ ] **F-04｜Antigravity/Gemini 经 Responses 的 compact 与工具**；开放 [#8164](https://github.com/Wei-Shaw/sub2api/pull/8164)、[#8804](https://github.com/Wei-Shaw/sub2api/pull/8804)、[#8809](https://github.com/Wei-Shaw/sub2api/pull/8809)：Codex 长上下文、最大输出、ToolSearch、签名 reasoning、避免请求末尾非法模型轮次；主线能力不因未合并而不分析。
- [ ] **F-05｜国产 Embeddings / Composite effort**；开放 [#8773](https://github.com/Wei-Shaw/sub2api/pull/8773)、[#8309](https://github.com/Wei-Shaw/sub2api/pull/8309)、[#6359](https://github.com/Wei-Shaw/sub2api/pull/6359)；按精简版实际支持的 DeepSeek/Kimi/Zhipu 兼容入口与模型能力筛选；MCP 大范围需确认。
- [ ] **F-06｜Seedance/BytePlus、Meta Muse**；开放 [#8459](https://github.com/Wei-Shaw/sub2api/pull/8459)、[#8722](https://github.com/Wei-Shaw/sub2api/pull/8722)，已合并 [#8837](https://github.com/Wei-Shaw/sub2api/pull/8837)。不先恢复大型平台才补特定 Seedance 能力，先确认是否符合中转定位。
- [ ] **F-07｜OpenAI 透传账号按模型过滤**；[#8748](https://github.com/Wei-Shaw/sub2api/pull/8748)：开关默认关，避免 Qwen-only API Key 路由接 GPT；用映射源键/通配、模型目录、failover 验证，单独 PR。
- [ ] **F-08｜WebSearch 与外围功能分流**；如开放 Exa/MCP、支付、营销、排行榜、外部管理 Playground 默认不回移。若其中有可单独回移的安全/协议/计费核心修复，摘取最小补丁；**不恢复 OpenCode Go 商业化残留**。

## 8. 上游 Open PR 的重点审查门槛

| 分类 | 重点开放 PR | 必须回答的核心问题 |
|---|---|---|
| Antigravity 429/503 | [#8878](https://github.com/Wei-Shaw/sub2api/pull/8878)、[#8793](https://github.com/Wei-Shaw/sub2api/pull/8793)、[#8781](https://github.com/Wei-Shaw/sub2api/pull/8781)、[#8744](https://github.com/Wei-Shaw/sub2api/pull/8744)、[#8627](https://github.com/Wei-Shaw/sub2api/pull/8627) | 是请求导致、账号模型不支持、端点故障还是额度耗尽？哪一种应该换号、冷却或终止？ |
| 请求级 403/401 | [#8730](https://github.com/Wei-Shaw/sub2api/pull/8730)、[#8774](https://github.com/Wei-Shaw/sub2api/pull/8774)、[#8856](https://github.com/Wei-Shaw/sub2api/pull/8856) | 是否会逐号停用？真正授权失效时仍否正确拦截？ |
| 多实例安全/内存 OOM | [#8685](https://github.com/Wei-Shaw/sub2api/pull/8685)、[#8875](https://github.com/Wei-Shaw/sub2api/pull/8875)→[#8876](https://github.com/Wei-Shaw/sub2api/pull/8876) | 账号并发租约不丢，预算不过量，不改原默认值未经批准 |
| Claude/Responses 兼容 | [#8832](https://github.com/Wei-Shaw/sub2api/pull/8832)、[#8750](https://github.com/Wei-Shaw/sub2api/pull/8750)、[#8809](https://github.com/Wei-Shaw/sub2api/pull/8809)、[#8859](https://github.com/Wei-Shaw/sub2api/pull/8859) | thinking 与历史轮次可合法往返、不泄露推理到可见 content、不被上游 400 |
| 新主线模型/端点 | [#8778](https://github.com/Wei-Shaw/sub2api/pull/8778)、[#8164](https://github.com/Wei-Shaw/sub2api/pull/8164)、[#8849](https://github.com/Wei-Shaw/sub2api/pull/8849)、[#7157](https://github.com/Wei-Shaw/sub2api/pull/7157)、[#8817](https://github.com/Wei-Shaw/sub2api/pull/8817) | 是否可最小化接入精简版？需要什么依赖、计费、迁移、许可？ |
| 其它安全与正确性 | [#8633](https://github.com/Wei-Shaw/sub2api/pull/8633)、[#7469](https://github.com/Wei-Shaw/sub2api/pull/7469)、[#8770](https://github.com/Wei-Shaw/sub2api/pull/8770)、[#8864](https://github.com/Wei-Shaw/sub2api/pull/8864) | 有无隐私存储、错误价格、WS 轮次错误或服务端搜索计费误差？ |

- [ ] **OPEN-REVIEW** 以上每条记录最新状态/是否合并/是否被替代/与已回移重复，并给出明确“回移/等待/排除”结论。完整活跃 PR 名单见附录 B，不能漏查近期新增主线能力。

## 9. PR 拆分与验收流程（每个实际开发项都执行）

- [ ] 以最新 `lite/main` 为基线，不碰原冲突工作树；上游 merge SHA 与精简版源文件差异可追溯。
- [ ] 尽可能先写可在旧实现复现的失败测试；新模型至少提供六维兼容矩阵；不得把未合并 PR 的测试声明当作自己已通过。
- [ ] 后端焦点测试及需要的 Go unit、handler、service、repository；前端变更跑 ESLint/typecheck/Vitest/build；`gofmt`、`git diff --check` 清洁。
- [ ] 计费/Redis/并发/迁移要求 PostgreSQL+Redis 集成测试，SSE/WS 要实际 handler+模拟上游测试长流/取消/超时/换号，不能只测 helper。
- [ ] GitHub 最终 SHA 的 CI 包含 frontend、embed/build、Go unit、PostgreSQL/Redis integration、golangci-lint、部署脚本；Security Scan 后端+前端过关。Fork Actions 禁用必须明确标注，不得称通过。
- [ ] PR 标题/正文只用审核者能理解的功能主题，例如 `fix(scheduling): release slots on failed hydration`，不是“第几批/任务号”；正文包括问题、最小修复、原 PR 链接、范围、测试结果/Run 链接和风险。
- [ ] CI 失败必须修正并在最终 SHA 上重新跑；确认没有已有同源 PR 后提交/转 Ready 通知维护者。**不自动合并、不发布、不部署**。
- [ ] 每完成一个功能，在本清单勾选并写目标 PR、最终 SHA、CI/Scan URL、风险结论，更新 `cairn/LOG.md`、`cairn/ROADMAP.md` 和对应专题。
- [ ] 开始下一批前重新查上游 release/main/Open PR，新主流模型、新协议功能默认纳入候选；如果存在依赖/schema/计费范围冲突，先请求用户决定。

### 交付 PR 建议主题（非提交标题模板）

| 建议阶段 | 建议审核主题 | 依赖 |
|---|---|---|
| A | Refresh Token 原子兑换 | 单独安全 PR |
| A | 调度失败释放并发槽 + 冷却自愈 | 实际两项可拆开 |
| A | RPM 跨组限制与 JSON Schema 缺省修复 | 建议不同模块分别 PR |
| B | Claude 5.2 完整模型支持 | 目录→转换→价格→回归 |
| B | Gemini 4 Carbon；另审 Hydrogen | Carbon 稳定先交，Hydrogen 开放 PR 另核对 |
| B | GPT-6 Ultra Fast 及正确倍率 | 用户确认计费语义 |
| C | Antigravity 长流/空流/工具；OpenAI SSE/WS | 每个协议主题拆小 PR |
| D | Fable/Kimi 额度恢复、响应模型成本归因 | 避免与未批准的 12h 迁移混在一起 |
| D | 12h cache-write 计费 | schema/迁移、用户确认 |
| E | 资源预算与调度高并发 | 先压测、谨慎改默认值 |
| F | Decisions / Rerank / DeepSeek 工具 | 新 API 范围单独确认 |

## 10. 下一会话直接复制这段指令

> 使用 WebCodex 连接 Mac mini 的 Sub2API 项目。先阅读 `AGENTS.md`、`docs/maintenance/upstream-backport-workflow.md`、`cairn/ROADMAP.md` 和 `docs/maintenance/upstream-reviews/2026-11-17-v022-backport-development-checklist.md`。原 Mac mini checkout 有既有未提交变更和 `upstream_models.go` 冲突，**绝不能清理或覆盖**；只读检查原工作区，另从最新 `lite/main` 建立隔离 worktree。
>
> 首先执行 BOOT 清单，刷新 Wei-Shaw/sub2api 最新版本、main、正式版全部 PR 及 Open/Draft PR，并逐条对照精简版已有实现。随后按阶段 A 的 P0 缺口开始开发、测试、推送 Fork、跑目标仓库完整 CI/Security Scan，按明确功能主题提 PR、请求审核；每完成一项更新此清单的状态、SHA 与 PR/CI 证据，再审查新增上游 PR。开放 PR 不自动 cherry-pick，DB/schema/价格/默认行为、新平台大范围需先确认；不自动合并、发版或部署。精简版坚持中转核心，不引入无关商业模块；新主流模型能力默认进入候选。

---

## 附录 A：正式版新增的全部已合并 PR（逐条待核对）

说明：v0.2.20→v0.2.21 **33** 条；v0.2.21→v0.2.22 **70** 条。`[ ]` 是“尚待核对”，**不是必须逐条回移**。本附录由 first-parent 合并历史与 GitHub PR metadata 自动生成，以版本边界为准。


### v0.2.21：33 条合并 PR

- [ ] **MERGED-8639** [#8639](https://github.com/Wei-Shaw/sub2api/pull/8639) — feat: add Claude Fable 5.2 and Mythos 5.2 support（merge 10ba8f9b4；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8357** [#8357](https://github.com/Wei-Shaw/sub2api/pull/8357) — fix(securityaudit): 审计任务入队后立即唤醒本进程 worker（merge e2c981219；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8575** [#8575](https://github.com/Wei-Shaw/sub2api/pull/8575) — fix(openai): reject invalid UTF-8 Responses bodies（merge 2cc43fdef；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8379** [#8379](https://github.com/Wei-Shaw/sub2api/pull/8379) — fix(openai): preserve native Codex image error statuses（merge e6981f0ff；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8622** [#8622](https://github.com/Wei-Shaw/sub2api/pull/8622) — fix(openai): normalize Codex Desktop page context before tool validation（merge b045db4a4；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8367** [#8367](https://github.com/Wei-Shaw/sub2api/pull/8367) — feat(settings): 系统设置保存按钮改为底部吸附浮标（merge 41e4a6b10；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8642** [#8642](https://github.com/Wei-Shaw/sub2api/pull/8642) — fix(payment): support up to 6 decimals for balance recharge multiplier（merge 48b11bf74；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8581** [#8581](https://github.com/Wei-Shaw/sub2api/pull/8581) — fix(accounts): retain rows and quota values while refreshing（merge 51a0e89b8；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8346** [#8346](https://github.com/Wei-Shaw/sub2api/pull/8346) — perf(keys): API 密钥列表不再等待 30 天用量统计，并为其加覆盖索引（merge a87083659；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8161** [#8161](https://github.com/Wei-Shaw/sub2api/pull/8161) — fix(openai): apply mappings to synthesized reasoning effort（merge 5fe13c3c4；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8637** [#8637](https://github.com/Wei-Shaw/sub2api/pull/8637) — fix(auth): consume refresh tokens atomically during rotation（merge efdc399ec；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8631** [#8631](https://github.com/Wei-Shaw/sub2api/pull/8631) — fix(scheduling): release acquired slot when account hydration fails（merge b455228b4；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8566** [#8566](https://github.com/Wei-Shaw/sub2api/pull/8566) — fix(payment): configure EasyPay refund success codes per instance（merge d18e1dff6；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8568** [#8568](https://github.com/Wei-Shaw/sub2api/pull/8568) — fix(antigravity): isolate Pro Argon quota cooldowns（merge 968c1022e；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8621** [#8621](https://github.com/Wei-Shaw/sub2api/pull/8621) — fix(antigravity): keep parallel tool functionResponses contiguous before media（merge 6f6b54d31；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8584** [#8584](https://github.com/Wei-Shaw/sub2api/pull/8584) — fix(gemini): 兼容入口非流式响应不再把思考摘要混入正文（merge 350c80044；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8385** [#8385](https://github.com/Wei-Shaw/sub2api/pull/8385) — fix(gateway): honor account error policies in compatibility paths（merge 9de6dd766；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8589** [#8589](https://github.com/Wei-Shaw/sub2api/pull/8589) — fix(openai): stabilize Responses session seeds across tool loops（merge f7ddfb823；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8386** [#8386](https://github.com/Wei-Shaw/sub2api/pull/8386) — fix(apicompat): skip foreign CC Switch reasoning envelopes（merge 73e75a18d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8585** [#8585](https://github.com/Wei-Shaw/sub2api/pull/8585) — fix(apicompat): preserve sealed function call metadata（merge aa56a4606；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8579** [#8579](https://github.com/Wei-Shaw/sub2api/pull/8579) — fix(openai): honor explicit model cooldowns for capacity errors（merge 6e128ba40；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8576** [#8576](https://github.com/Wei-Shaw/sub2api/pull/8576) — fix(openai): keep Codex metadata events retry-safe（merge fc9d81138；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8565** [#8565](https://github.com/Wei-Shaw/sub2api/pull/8565) — fix(openai-ws): follow the TTFT mode for WebSocket first token（merge 32d0576ed；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8358** [#8358](https://github.com/Wei-Shaw/sub2api/pull/8358) — fix(openai-ws): reconnect pooled OAuth sockets before search activation（merge 9725718e1；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8564** [#8564](https://github.com/Wei-Shaw/sub2api/pull/8564) — fix(openai): stop native WS later-turn first-frame replay（merge 2fd34acc1；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8634** [#8634](https://github.com/Wei-Shaw/sub2api/pull/8634) — fix(pricing): add Claude Fable and Mythos 5.2 fallback rates（merge d216e4215；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8384** [#8384](https://github.com/Wei-Shaw/sub2api/pull/8384) — fix(pricing): retry sync after catalog hash mismatch（merge 1cae79565；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8381** [#8381](https://github.com/Wei-Shaw/sub2api/pull/8381) — fix: correct Sonnet 5.5 cache read fallback pricing（merge 722bbf6db；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8606** [#8606](https://github.com/Wei-Shaw/sub2api/pull/8606) — fix(openai): preserve cache-write TTLs in usage and billing（merge ee58c0013；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8590** [#8590](https://github.com/Wei-Shaw/sub2api/pull/8590) — feat(antigravity): 用量格子加查询按钮，强制查询跳过缓存 / Query button for antigravity usage; force skips the cache（merge 8ca1a613e；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8603** [#8603](https://github.com/Wei-Shaw/sub2api/pull/8603) — test(openai): isolate orphan precheck allocation test（merge 27ef2a713；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8607** [#8607](https://github.com/Wei-Shaw/sub2api/pull/8607) — docs(images): explain upstream output format mismatches（merge 8e649dd65；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8610** [#8610](https://github.com/Wei-Shaw/sub2api/pull/8610) — fix(gateway): auto-size request body memory budget from the memory limit（merge d7c050538；待判定：已覆盖 / 回移 / 暂缓 / 不适用）

### v0.2.22：70 条合并 PR

- [ ] **MERGED-7540** [#7540](https://github.com/Wei-Shaw/sub2api/pull/7540) — fix(billing): 为硅基流动 Qwen3-Embedding/Reranker 系列补兜底定价（merge b0fa9f3e6；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8812** [#8812](https://github.com/Wei-Shaw/sub2api/pull/8812) — feat(groups): 分组支持强制 OpenAI Ultra Fast 档位（merge 8b13851ac；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8747** [#8747](https://github.com/Wei-Shaw/sub2api/pull/8747) — fix(openai): preserve quota freshness when refresh fails（merge ef2920300；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8669** [#8669](https://github.com/Wei-Shaw/sub2api/pull/8669) — fix(grok): preserve upstream media error messages and codes（merge 7e4c614bc；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8772** [#8772](https://github.com/Wei-Shaw/sub2api/pull/8772) — fix(openai): drop tool schema patterns Python re rejects（merge 3e8e3254d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8724** [#8724](https://github.com/Wei-Shaw/sub2api/pull/8724) — fix(gateway): emit SSE errors after queue keepalives（merge f8d891d6f；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8748** [#8748](https://github.com/Wei-Shaw/sub2api/pull/8748) — feat(openai): optionally filter passthrough accounts by model（merge 7a19bbef7；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8834** [#8834](https://github.com/Wei-Shaw/sub2api/pull/8834) — fix(grok): keep xAI server-side search calls out of tool calls（merge 4328510f2；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8661** [#8661](https://github.com/Wei-Shaw/sub2api/pull/8661) — feat(gemini): add Gemini 4 Carbon support（merge 03c17491b；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8833** [#8833](https://github.com/Wei-Shaw/sub2api/pull/8833) — perf(scheduler): 调度快照加载与数据库回源按 bucket 合并，减少大分组选号的整组复制（merge 8ed701382；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8816** [#8816](https://github.com/Wei-Shaw/sub2api/pull/8816) — perf: 同一用户的扣费按用户通道合并结算，请求体热点路径改用零拷贝读取（merge b140d5db8；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8822** [#8822](https://github.com/Wei-Shaw/sub2api/pull/8822) — fix(gemini): 原生请求按稳定前缀生成粘性会话哈希，换号按上游实际用量计费（merge 04503e001；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8827** [#8827](https://github.com/Wei-Shaw/sub2api/pull/8827) — feat(gemini): Vertex 账号支持 us/eu 多区域端点，并按实际请求区域计价（merge f242a6ea2；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8746** [#8746](https://github.com/Wei-Shaw/sub2api/pull/8746) — feat(gemini): 补齐 Gemini 4 Hydrogen 计费（merge 1cf7eb48c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8840** [#8840](https://github.com/Wei-Shaw/sub2api/pull/8840) — fix(channels): persist Bedrock compatibility as a boolean（merge d70676756；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8837** [#8837](https://github.com/Wei-Shaw/sub2api/pull/8837) — fix(scheduler): keep openai_capabilities in the scheduler metadata projection so Seedance accounts stay schedulable（merge 6b7cbc63b；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8828** [#8828](https://github.com/Wei-Shaw/sub2api/pull/8828) — fix(gemini): separate same-name parallel tool calls in streams（merge 46b879265；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8826** [#8826](https://github.com/Wei-Shaw/sub2api/pull/8826) — fix(antigravity): keepalive 后空流释放会话粘性账号 / Release the sticky account after a committed empty compat stream（merge 4f3f91631；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8825** [#8825](https://github.com/Wei-Shaw/sub2api/pull/8825) — fix(pricing): honor explicit DeepSeek token price overrides（merge f2eaed31c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8814** [#8814](https://github.com/Wei-Shaw/sub2api/pull/8814) — fix(openai): apply account side effects to stream errors after output on compat paths（merge 992ac6330；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8813** [#8813](https://github.com/Wei-Shaw/sub2api/pull/8813) — fix(openai): forward upstream keepalive events before first output（merge 101c2462d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8811** [#8811](https://github.com/Wei-Shaw/sub2api/pull/8811) — fix(openai): fail over Messages bridge errors before first content（merge 1ca302abf；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8808** [#8808](https://github.com/Wei-Shaw/sub2api/pull/8808) — fix(openai): fall back to memory when first-output spool creation fails（merge 9750f8e03；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8806** [#8806](https://github.com/Wei-Shaw/sub2api/pull/8806) — fix(usage): 按账户筛选时模型与端点分布的实际扣费不再替换为账号成本（merge 568db32f2；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8800** [#8800](https://github.com/Wei-Shaw/sub2api/pull/8800) — fix(upstream): detect dead upstream connections in about 30s（merge 119b04f94；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8798** [#8798](https://github.com/Wei-Shaw/sub2api/pull/8798) — fix(pricing): check remote hash in background at startup（merge 857ab9508；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8796** [#8796](https://github.com/Wei-Shaw/sub2api/pull/8796) — fix(gateway): restore public model in composite-routed responses（merge 470c02b39；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8790** [#8790](https://github.com/Wei-Shaw/sub2api/pull/8790) — fix(websearch): honor account mode on every channel-toggle platform（merge 97957edd9；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8788** [#8788](https://github.com/Wei-Shaw/sub2api/pull/8788) — fix(gateway): stop duplicating headers on count_tokens passthrough（merge 63751ea4d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8786** [#8786](https://github.com/Wei-Shaw/sub2api/pull/8786) — fix(gateway): keep client metadata.user_id in Chat Completions to Anthropic（merge 1d585c94f；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8779** [#8779](https://github.com/Wei-Shaw/sub2api/pull/8779) — fix(ratelimit): lift Anthropic Fable model limit when the 7d_oi window recovers（merge 4cebc098c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8775** [#8775](https://github.com/Wei-Shaw/sub2api/pull/8775) — fix(web): stop caching settings-injected HTML（merge c7df3b693；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8769** [#8769](https://github.com/Wei-Shaw/sub2api/pull/8769) — fix(openai): normalize additional Codex context on HTTP Responses（merge d31a87a4e；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8763** [#8763](https://github.com/Wei-Shaw/sub2api/pull/8763) — fix(kimi): 解析新套餐月度额度并清理缺席窗口快照（merge 56848016c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8761** [#8761](https://github.com/Wei-Shaw/sub2api/pull/8761) — fix(apicompat): default json_schema strict=false when converting CC to Responses（merge 4763569af；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8759** [#8759](https://github.com/Wei-Shaw/sub2api/pull/8759) — fix(antigravity): match schema items to the selected type（merge 202740441；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8757** [#8757](https://github.com/Wei-Shaw/sub2api/pull/8757) — fix(claude): default account tests to claude-sonnet-4-6（merge 3b5310c5d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8754** [#8754](https://github.com/Wei-Shaw/sub2api/pull/8754) — fix(codex): preserve and normalize additional_tools IDs（merge e16a349eb；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8745** [#8745](https://github.com/Wei-Shaw/sub2api/pull/8745) — fix(gateway): record Anthropic response models in compatibility routes（merge 8a585c914；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8743** [#8743](https://github.com/Wei-Shaw/sub2api/pull/8743) — test(billing): make peak-rate usage tests independent of wall clock（merge 6247d33eb；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8742** [#8742](https://github.com/Wei-Shaw/sub2api/pull/8742) — fix(gateway): preserve message output config with per-turn beta（merge 6fd905f30；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8741** [#8741](https://github.com/Wei-Shaw/sub2api/pull/8741) — fix(gateway): record native Anthropic endpoints on error paths（merge e54a18654；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8740** [#8740](https://github.com/Wei-Shaw/sub2api/pull/8740) — fix(accounts): keep Gemini usage bars visible during refetch（merge 0563176a5；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8739** [#8739](https://github.com/Wei-Shaw/sub2api/pull/8739) — fix(gateway): enforce effort policies on Anthropic compatibility routes（merge c782537ee；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8738** [#8738](https://github.com/Wei-Shaw/sub2api/pull/8738) — fix(cn-quota): refresh usage cells with account snapshots（merge d7ae0544e；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8737** [#8737](https://github.com/Wei-Shaw/sub2api/pull/8737) — fix(claude): recover requests after concurrent token rotation（merge 5146728d2；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8736** [#8736](https://github.com/Wei-Shaw/sub2api/pull/8736) — fix(accounts): reject wildcard model whitelist entries（merge 472ea5752；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8735** [#8735](https://github.com/Wei-Shaw/sub2api/pull/8735) — perf(openai): avoid repeated full-body copies during OAuth metadata cleanup（merge eb25511c7；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8729** [#8729](https://github.com/Wei-Shaw/sub2api/pull/8729) — fix(scheduler): recover snapshot accounts after cooldown expiry（merge c6d9d48f9；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8728** [#8728](https://github.com/Wei-Shaw/sub2api/pull/8728) — fix(usage): retain response models through Chat fallbacks（merge a38ffc305；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8727** [#8727](https://github.com/Wei-Shaw/sub2api/pull/8727) — fix(usage): retain native Anthropic response observations（merge 32e428389；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8726** [#8726](https://github.com/Wei-Shaw/sub2api/pull/8726) — fix(gateway): retain sticky writes after client disconnects（merge 51e76a46c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8723** [#8723](https://github.com/Wei-Shaw/sub2api/pull/8723) — fix(anthropic): keep weekly usage readings across 5h rollovers（merge 90eb1744d；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8703** [#8703](https://github.com/Wei-Shaw/sub2api/pull/8703) — fix(apicompat): send mid-conversation system messages to Responses as developer（merge 0b473ad60；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8698** [#8698](https://github.com/Wei-Shaw/sub2api/pull/8698) — fix(usage): advance five-hour window start on active quota sync（merge 2df3dbcfb；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8693** [#8693](https://github.com/Wei-Shaw/sub2api/pull/8693) — fix(usage): 分组日汇总重建与保留期清理不阻塞 usage_logs 写入（merge fde504e34；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8684** [#8684](https://github.com/Wei-Shaw/sub2api/pull/8684) — fix(auth): 登录回填 email 身份失败时不再 panic（merge 910d4d86b；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8682** [#8682](https://github.com/Wei-Shaw/sub2api/pull/8682) — fix(apicompat): retain encrypted function call content（merge 89fce5446；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8678** [#8678](https://github.com/Wei-Shaw/sub2api/pull/8678) — fix: allow disabling concurrency wait pings（merge bd3a878ef；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8675** [#8675](https://github.com/Wei-Shaw/sub2api/pull/8675) — fix(bedrock): 按错误语义区分封号、用例缺失、限流与内容过滤，流中断保留 usage（merge 0ebc8b2fd；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8673** [#8673](https://github.com/Wei-Shaw/sub2api/pull/8673) — fix(antigravity): stream Gemini compat thoughts and anchor pre-content timeout after headers（merge 8da038854；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8667** [#8667](https://github.com/Wei-Shaw/sub2api/pull/8667) — perf(gateway): trim per-request JSON work on the Anthropic request path（merge 41c567b38；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8664** [#8664](https://github.com/Wei-Shaw/sub2api/pull/8664) — fix(openai): keep Codex forks on their parent's prompt cache key（merge a6e02a37a；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8662** [#8662](https://github.com/Wei-Shaw/sub2api/pull/8662) — fix(gemini): 兼容入口 chat 请求转发 response_format 结构化输出（merge 980ddd942；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8659** [#8659](https://github.com/Wei-Shaw/sub2api/pull/8659) — fix(billing): make group rate/RPM sync transactional（merge 0c641e5ed；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8653** [#8653](https://github.com/Wei-Shaw/sub2api/pull/8653) — perf(gateway): skip JSON decode for forwarded Anthropic stream events（merge 83a1f5dbe；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8651** [#8651](https://github.com/Wei-Shaw/sub2api/pull/8651) — fix(billing): scope RPM override to the group it belongs to（merge 314de5d0c；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8649** [#8649](https://github.com/Wei-Shaw/sub2api/pull/8649) — fix(payment): make Alipay refund request id stable across retries（merge d7d45924e；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8644** [#8644](https://github.com/Wei-Shaw/sub2api/pull/8644) — fix(usage): read Fable weekly quotas from scoped limits（merge a81a345a1；待判定：已覆盖 / 回移 / 暂缓 / 不适用）
- [ ] **MERGED-8767** [#8767](https://github.com/Wei-Shaw/sub2api/pull/8767) — chore(ci): bump Go toolchain to 1.27.4 for GO-2026-6859（merge e3151c520；待判定：已覆盖 / 回移 / 暂缓 / 不适用）

## 附录 B：近期活跃 Open/Draft PR 快照（逐条待分析）

> 数据来源：截至 2026-11-17，updated >= 2026-11-10，112 条；其中 Draft 2 条。以后状态变动要重新核对。这里的 [ ] 只代表待筛选，不意味着全部开发。

- [ ] **OPEN-8878** [#8878](https://github.com/Wei-Shaw/sub2api/pull/8878) — fix(antigravity): neutralize DeepSeek Harness identity and browser-context sentence（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8877** [#8877](https://github.com/Wei-Shaw/sub2api/pull/8877) — feat(frontend): API 密钥支持一键导入到 AI Switch（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8876** [#8876](https://github.com/Wei-Shaw/sub2api/pull/8876) — fix(gateway): admit JSON processing memory before rewriting requests（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8875** [#8875](https://github.com/Wei-Shaw/sub2api/pull/8875) — fix(openai): stop pinning large request bodies after the handler（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8874** [#8874](https://github.com/Wei-Shaw/sub2api/pull/8874) — perf(gateway): stop scanning every request body twice for lenient JSON（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8872** [#8872](https://github.com/Wei-Shaw/sub2api/pull/8872) — perf(openai): stop walking every request body in the tool-schema sanitizer（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8870** [#8870](https://github.com/Wei-Shaw/sub2api/pull/8870) — perf(openai): stop copying the whole input array when stripping namespaces（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8868** [#8868](https://github.com/Wei-Shaw/sub2api/pull/8868) — perf(gateway): parse the request body once instead of eight times（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8866** [#8866](https://github.com/Wei-Shaw/sub2api/pull/8866) — perf(openai): stop re-validating the same body when classifying image intent（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8864** [#8864](https://github.com/Wei-Shaw/sub2api/pull/8864) — fix(grok): bill search calls from xAI usage success counts（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8862** [#8862](https://github.com/Wei-Shaw/sub2api/pull/8862) — perf(gateway): skip the cache_control TTL walk when no 1h breakpoint exists（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8860** [#8860](https://github.com/Wei-Shaw/sub2api/pull/8860) — fix(accounts): refetch Claude and Gemini usage cells when auto-refresh updates a row（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8859** [#8859](https://github.com/Wei-Shaw/sub2api/pull/8859) — fix(composite): adapt thinking to the Claude 5.5 model a route rewrites to（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8858** [#8858](https://github.com/Wei-Shaw/sub2api/pull/8858) — fix(accounts): refresh Claude and Gemini usage snapshots（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8856** [#8856](https://github.com/Wei-Shaw/sub2api/pull/8856) — fix(openai): recover OAuth account tests after early token rejection（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8852** [#8852](https://github.com/Wei-Shaw/sub2api/pull/8852) — feat(admin): add a playground pinned to one upstream account（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8849** [#8849](https://github.com/Wei-Shaw/sub2api/pull/8849) — feat(openai): support Decisions API（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8845** [#8845](https://github.com/Wei-Shaw/sub2api/pull/8845) — perf(scheduler): omit expanded groups from account cache（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8843** [#8843](https://github.com/Wei-Shaw/sub2api/pull/8843) — test(oauth): make late refresh deadline deterministic（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8839** [#8839](https://github.com/Wei-Shaw/sub2api/pull/8839) — test(antigravity): stabilize repeated pre-content keepalives（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8838** [#8838](https://github.com/Wei-Shaw/sub2api/pull/8838) — feat(i18n): add complete Russian locale（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8836** [#8836](https://github.com/Wei-Shaw/sub2api/pull/8836) — fix(antigravity): 无重置时间的裸 429 只冷却失败的模型 / Cool only the failed model on a bare 429（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8835** [#8835](https://github.com/Wei-Shaw/sub2api/pull/8835) — fix(gateway): support Claude Code auto mode server-side classifier（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8832** [#8832](https://github.com/Wei-Shaw/sub2api/pull/8832) — fix(apicompat): Responses→Anthropic 不再以 assistant / thinking 结尾 / Never end converted history on assistant or a thinking block（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8831** [#8831](https://github.com/Wei-Shaw/sub2api/pull/8831) — fix(claude): 区分 Fable 重置和周重置 / Separate Fable and weekly Claude resets（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8829** [#8829](https://github.com/Wei-Shaw/sub2api/pull/8829) — fix(antigravity): strip the Claude Code dual-use security sentence that triggers a fake 429（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8820** [#8820](https://github.com/Wei-Shaw/sub2api/pull/8820) — fix(openai): rebuild empty buffered output from done items（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8817** [#8817](https://github.com/Wei-Shaw/sub2api/pull/8817) — feat(gateway): support DeepSeek application tool APIs (tools/files endpoints, server-tool billing, passthrough fixes)（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8809** [#8809](https://github.com/Wei-Shaw/sub2api/pull/8809) — feat(apicompat): carry Anthropic thinking blocks in Chat reasoning_details（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8807** [#8807](https://github.com/Wei-Shaw/sub2api/pull/8807) — fix(gateway): anchor the content session hash to the first message（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8804** [#8804](https://github.com/Wei-Shaw/sub2api/pull/8804) — fix(apicompat): map ToolSearch to native tool_search for Codex OAuth（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8803** [#8803](https://github.com/Wei-Shaw/sub2api/pull/8803) — feat(apicompat): return web_search sources in web_search_tool_result blocks（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8801** [#8801](https://github.com/Wei-Shaw/sub2api/pull/8801) — feat(websearch): add Exa as a web search emulation provider（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8794** [#8794](https://github.com/Wei-Shaw/sub2api/pull/8794) — feat(gateway): write Claude Code compaction requests at 5m cache TTL（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8793** [#8793](https://github.com/Wei-Shaw/sub2api/pull/8793) — fix(antigravity): neutralize Pi identity in system prompts（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8784** [#8784](https://github.com/Wei-Shaw/sub2api/pull/8784) — fix(usage-log): keep retrying best-effort usage log writes through DB stalls（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8781** [#8781](https://github.com/Wei-Shaw/sub2api/pull/8781) — fix(gateway): fail over on account-scoped Antigravity and Codex rejections（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8778** [#8778](https://github.com/Wei-Shaw/sub2api/pull/8778) — feat(gemini,antigravity): add Gemini 4 Hydrogen/Carbon support and dynamic effort variant routing（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8776** [#8776](https://github.com/Wei-Shaw/sub2api/pull/8776) — feat: 完善账号会话、订阅计费、模型 Skills 与管理体验（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8774** [#8774](https://github.com/Wei-Shaw/sub2api/pull/8774) — fix(ratelimit): keep setup-token accounts recoverable after 401（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8773** [#8773](https://github.com/Wei-Shaw/sub2api/pull/8773) — fix(embeddings): 国产平台分组按分组平台调度账号（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8771** [#8771](https://github.com/Wei-Shaw/sub2api/pull/8771) — feat(keys): choose CC Switch import client（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8770** [#8770](https://github.com/Wei-Shaw/sub2api/pull/8770) — fix(openai): rebuild native WS failover from bounded current-window history（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8765** [#8765](https://github.com/Wei-Shaw/sub2api/pull/8765) — fix(payment): bound closed-order payment recovery by the order deadline（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8756** [#8756](https://github.com/Wei-Shaw/sub2api/pull/8756) — test(streaming): synchronize TTFT and keepalive fixtures（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8753** [#8753](https://github.com/Wei-Shaw/sub2api/pull/8753) — feat(ops-ui): 账号追溯前端 —— 请求链路图、账号追溯面板与订阅台账、待续费看板（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8751** [#8751](https://github.com/Wei-Shaw/sub2api/pull/8751) — feat(ops): 账号追溯 —— 逐跳上游尝试、账号状态事件与订阅台账（后端）（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8750** [#8750](https://github.com/Wei-Shaw/sub2api/pull/8750) — fix(apicompat): 历史 reasoning 改为 reasoning item，避免泄漏进可见 content（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8749** [#8749](https://github.com/Wei-Shaw/sub2api/pull/8749) — fix(openai): preserve retries after chat role-only preambles（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8744** [#8744](https://github.com/Wei-Shaw/sub2api/pull/8744) — fix(antigravity): fail over on upstream first-byte timeout（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8734** [#8734](https://github.com/Wei-Shaw/sub2api/pull/8734) — fix(openai): record invalid_prompt policy blocks in risk control（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8731** [#8731](https://github.com/Wei-Shaw/sub2api/pull/8731) — fix(claude): sync Stainless versions with floored fingerprint UA（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8730** [#8730](https://github.com/Wei-Shaw/sub2api/pull/8730) — fix(gemini): 请求级 403 不再逐账号停用和换号重放（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8722** [#8722](https://github.com/Wei-Shaw/sub2api/pull/8722) — feat: add Meta Muse Code provider（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8721** [#8721](https://github.com/Wei-Shaw/sub2api/pull/8721) — feat(scheduler): 支持各平台按用户优先选择同组上游账号（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8705** [#8705](https://github.com/Wei-Shaw/sub2api/pull/8705) — fix(gateway): 减少长会话请求的内存分配与持有（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8704** [#8704](https://github.com/Wei-Shaw/sub2api/pull/8704) — fix(usage): keep token/cost detail tooltips inside the viewport on mobile（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8692** [#8692](https://github.com/Wei-Shaw/sub2api/pull/8692) — fix(gemini): keep empty compatibility completions unbilled（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8690** [#8690](https://github.com/Wei-Shaw/sub2api/pull/8690) — fix(gateway): allow bounded failover for unwritten buffered EOF（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8688** [#8688](https://github.com/Wei-Shaw/sub2api/pull/8688) — fix(gateway): retry narrowly classified relay upstream failures（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8689** [#8689](https://github.com/Wei-Shaw/sub2api/pull/8689) — fix(gateway): preserve configured pool accounts after retry exhaustion（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8687** [#8687](https://github.com/Wei-Shaw/sub2api/pull/8687) — docs(deploy): clarify binary updates and container image rollouts（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8685** [#8685](https://github.com/Wei-Shaw/sub2api/pull/8685) — fix(concurrency): preserve shared Redis leases during startup（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8686** [#8686](https://github.com/Wei-Shaw/sub2api/pull/8686) — fix(ops): exclude client failures from SLA error counts（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8671** [#8671](https://github.com/Wei-Shaw/sub2api/pull/8671) — fix(openai): protect rotated oauth credentials from enrichment timeout and race conditions（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-8665** [#8665](https://github.com/Wei-Shaw/sub2api/pull/8665) — fix(openai): 非 Codex 账号将 Responses input 中的 system 提升到 instructions（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8655** [#8655](https://github.com/Wei-Shaw/sub2api/pull/8655) — perf(redis): consolidate RPM increments and slot release round trips（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8648** [#8648](https://github.com/Wei-Shaw/sub2api/pull/8648) — feat(payment): add optional daily FX sync for recharge multiplier（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8647** [#8647](https://github.com/Wei-Shaw/sub2api/pull/8647) — fix(gateway): strip inline tool_definition blocks for CN Anthropic endpoints（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8633** [#8633](https://github.com/Wei-Shaw/sub2api/pull/8633) — fix(prompt-audit): stop persisting raw prompts in audit events（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8629** [#8629](https://github.com/Wei-Shaw/sub2api/pull/8629) — feat(systemone): 通过 Command Code 账号提供 Jev / Serve Jev through Command Code accounts（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8627** [#8627](https://github.com/Wei-Shaw/sub2api/pull/8627) — fix(gateway): 一个坏请求不再冷却它经过的所有账号 / Stop one bad request from cooling every account it touches（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8619** [#8619](https://github.com/Wei-Shaw/sub2api/pull/8619) — fix(antigravity): forward thinking config for Gemini via OpenAI-compatible endpoints（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8616** [#8616](https://github.com/Wei-Shaw/sub2api/pull/8616) — feat(dashboard): add API key dimension to usage analytics（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8598** [#8598](https://github.com/Wei-Shaw/sub2api/pull/8598) — test(billing): reuse rate resolver and isolate heap checks（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8597** [#8597](https://github.com/Wei-Shaw/sub2api/pull/8597) — test(gemini): isolate incremental text allocation checks（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8596** [#8596](https://github.com/Wei-Shaw/sub2api/pull/8596) — test(grok): synchronize free-quota refresh tests（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8570** [#8570](https://github.com/Wei-Shaw/sub2api/pull/8570) — fix(accounts): 支持零时长错误规则，避免请求参数错误触发账号冷却（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8569** [#8569](https://github.com/Wei-Shaw/sub2api/pull/8569) — fix(accounts): refresh OAuth quotas through read-only usage APIs（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-8537** [#8537](https://github.com/Wei-Shaw/sub2api/pull/8537) — fix(server): 优雅停机等待超时可配置，超时后强制关闭剩余连接，cleanup 增加总预算（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8512** [#8512](https://github.com/Wei-Shaw/sub2api/pull/8512) — feat(byteplus): add prepaid pools, usage status and billing queries（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8459** [#8459](https://github.com/Wei-Shaw/sub2api/pull/8459) — feat(byteplus): add ModelArk account provider for Seed and Seedance（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8425** [#8425](https://github.com/Wei-Shaw/sub2api/pull/8425) — feat(accounts): add quota matrix and cross-account overview（Open，updated 2026-11-13；待分析）
- [ ] **OPEN-8421** [#8421](https://github.com/Wei-Shaw/sub2api/pull/8421) — feat(openai): 重置卡到期前自动使用，防止忘记使用 / Auto-use reset credits before expiry（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8359** [#8359](https://github.com/Wei-Shaw/sub2api/pull/8359) — fix(opencode): 补齐容量与过载 429 重试 / Retry capacity and overload refusals（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8309** [#8309](https://github.com/Wei-Shaw/sub2api/pull/8309) — fix(composite): apply reasoning effort policies to CN targets（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8280** [#8280](https://github.com/Wei-Shaw/sub2api/pull/8280) — feat(subscriptions): add opt-in restart-on-renewal policy（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8246** [#8246](https://github.com/Wei-Shaw/sub2api/pull/8246) — fix(antigravity): keep parallel tool results contiguous across notices（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-8174** [#8174](https://github.com/Wei-Shaw/sub2api/pull/8174) — Preserve Anthropic cache creation TTL details（Open，updated 2026-11-10；待分析）
- [ ] **OPEN-8164** [#8164](https://github.com/Wei-Shaw/sub2api/pull/8164) — fix(gateway): support Responses compaction, client tools, and 64k output limit on Antigravity/Gemini（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-8073** [#8073](https://github.com/Wei-Shaw/sub2api/pull/8073) — feat(accounts): optional plain connectivity test for API key accounts（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-8072** [#8072](https://github.com/Wei-Shaw/sub2api/pull/8072) — feat: add experimental Free account image generation via ChatGPT web（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-8052** [#8052](https://github.com/Wei-Shaw/sub2api/pull/8052) — fix(streaming): forward native Anthropic ping heartbeats to chat clients（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-7962** [#7962](https://github.com/Wei-Shaw/sub2api/pull/7962) — fix(openai): propagate cancellation and separate delivery from consumption（Open，updated 2026-11-14；待分析）
- [ ] **OPEN-7870** [#7870](https://github.com/Wei-Shaw/sub2api/pull/7870) — fix(frontend): keep whitelist models when a rewrite mapping is deleted（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-7861** [#7861](https://github.com/Wei-Shaw/sub2api/pull/7861) — fix(apicompat): preserve chat audio input on supported routes（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-7704** [#7704](https://github.com/Wei-Shaw/sub2api/pull/7704) — feat: API key concurrency limits, bounded queues and request lifecycle（Open，updated 2026-11-16；待分析）
- [ ] **OPEN-7639** [#7639](https://github.com/Wei-Shaw/sub2api/pull/7639) — feat(muse): add native consumer-app text provider and account runtime（Draft，updated 2026-11-11；待分析）
- [ ] **OPEN-7608** [#7608](https://github.com/Wei-Shaw/sub2api/pull/7608) — fix(ollama): keep quota 429 cooldown until the usage probe learns the window reset（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-7469** [#7469](https://github.com/Wei-Shaw/sub2api/pull/7469) — fix(billing): update GPT-5.6 Sol promotional pricing（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-7157** [#7157](https://github.com/Wei-Shaw/sub2api/pull/7157) — feat: add /v1/rerank passthrough endpoint for OpenAI-compatible rerankers（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-7114** [#7114](https://github.com/Wei-Shaw/sub2api/pull/7114) — feat(composite, ollama): 统一 Ollama Cloud 平台与跨平台账号池（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-6983** [#6983](https://github.com/Wei-Shaw/sub2api/pull/6983) — feat(group): 分组支持自定义 Codex 模型（用于生成配置说明和CCS）（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-6630** [#6630](https://github.com/Wei-Shaw/sub2api/pull/6630) — 官网同步（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-6359** [#6359](https://github.com/Wei-Shaw/sub2api/pull/6359) — feat(zhipu): passthrough for Zhipu Coding Plan remote MCP servers（Open，updated 2026-11-17；待分析）
- [ ] **OPEN-5889** [#5889](https://github.com/Wei-Shaw/sub2api/pull/5889) — feat(openai): add strict raw Responses forwarding mode（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-5220** [#5220](https://github.com/Wei-Shaw/sub2api/pull/5220) — feat(simple): expose Composite routing（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-4848** [#4848](https://github.com/Wei-Shaw/sub2api/pull/4848) — fix(apicompat): emit rs-prefixed Responses reasoning item IDs（Draft，updated 2026-11-11；待分析）
- [ ] **OPEN-4286** [#4286](https://github.com/Wei-Shaw/sub2api/pull/4286) — 针对sse有了部分优化，防止出现错误（Open，updated 2026-11-12；待分析）
- [ ] **OPEN-3801** [#3801](https://github.com/Wei-Shaw/sub2api/pull/3801) — feat: add verified custom API domains（Open，updated 2026-11-11；待分析）
- [ ] **OPEN-3748** [#3748](https://github.com/Wei-Shaw/sub2api/pull/3748) — feat: add user daily usage leaderboard（Open，updated 2026-11-15；待分析）
- [ ] **OPEN-2344** [#2344](https://github.com/Wei-Shaw/sub2api/pull/2344) — Dev sub2（Open，updated 2026-11-16；待分析）

## 附录 C：下次 GitHub 快速核验命令

~~~bash
git fetch upstream main
git fetch lite main
git rev-parse upstream/main lite/main
.git/codex-tools/bin/gh release view -R Wei-Shaw/sub2api --json tagName,publishedAt,name
.git/codex-tools/bin/gh pr list -R Wei-Shaw/sub2api --state open --search "updated:>=2026-11-10" --limit 400 --json number,title,updatedAt,isDraft,url
.git/codex-tools/bin/gh pr list -R CherryLover/sub2api --state open --limit 100 --json number,title,url,isDraft
~~~

如上游更新了新 release、主流模型或 PR 状态，应扩充本清单并记录新 SHA、来源 URL、风险和明确取舍。


## 2026-11-21 续作进度（未统一验收）

后续协议、额度、账号探测及费用口径开发见 [统一续作记录](2026-11-21-unified-followup.md)。本轮尚未提交，定向结果不能替代最终全仓/真实数据库/CI；原审批边界与一个最终 PR 的要求保持不变。
