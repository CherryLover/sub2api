# 2026-11-21 统一回移开发、失败复现和验收记录

> 状态：集中开发中，未达到最终 PR 门槛。原始冲突工作区只读；局部通过不等于全部完成。

## 固定基线
- 最后代码收尾提交：`660108582d14d71812441660515f328b51509b11`；backend tree `cb36d6609162363fe22b83085d6d51734d62c9e9`。本轮累计 81 个 Go 文件（38 个生产文件、43 个测试文件）；最终全仓复验逐一核对 2,778 个后端文件内容，测试期间没有源码变化。
- 本轮统一代码提交：`3acfbe004dede5e70754430c2090608cef1c26f4`，70 个 Go 文件（37 个生产文件、33 个测试文件），含前轮未提交改动；后续文档提交不改变该代码树。
- 隔离分支 `backport/unified-integration-20261120`，开始 HEAD `7b24c3d54e2df8fda903cec62c8d47d57e4ddba9`，已有 40 个提交。
- 上游 main `328db26f450edbdf9f8d4344a3ea903ac6396842`，v0.2.24；lite/main `8fe8e095ed8abfef9a7e9fda6de4ae45de425296`。
- #73–#81 仍 Open；不自动关闭或合并。最终只提交一个统一 PR，不发布、部署或自动合并。
- 初始 13 个修改/新增文件先保存 binary diff 与 untracked tar，再在原有内容上最小适配。

## 实际推进的修复
| 主题 | 上游来源 | 实施/证据 |
|---|---|---|
| 429/503、空流、冷却 | #8973/#8974/#8975/#8987 | 保留短重试换号、空流不冷却、单请求冷却隔离；最终 RetryInfo 同模型更新 |
| Gemini 轮次边界 | #8997 | 先复现 5 种分块失败；保留代码围栏/正常文本，截断泄漏轮次与后续工具；转换包 158 项通过 |
| Gemini 六入口终止 | #8980 最小拆分 | EOF 不能伪装成功；保留 usage、流内错误、refusal 终态及 lite 首内容前 failover/keepalive 定制 |
| 多实例 Redis 租约 | #8685，不叠加 #7804 | 先复现误删其他实例活跃槽位/等待计数/并发索引；只清过期租约，不引入心跳架构或 schema |
| OpenAI OAuth 轮换 | #8671 | 已轮换 token 即使可选补全/客户端超时也有界持久化；CAS 保护并发重新授权/代理变更；先失败复现 |
| 消息/会话/metadata | #8703/#8589/#8786 | 中间 system→developer；工具循环粘性稳定；合法 metadata.user_id 保留，OAuth 仍用自身身份 |
| 原生 Anthropic 端点与读错误 | #8741 + 本地复现 | 错误仍记录 /v1/messages；两兼容流读失败不合成成功终态，保留累计用量 |
| Codex 外部上下文 | #8622/#8769/#8994 | 验证后转换独立上下文；真实工具关联、未支持 bootstrap、重复 JSON key、大整数保护；新增 terminal_info 精确类型支持 |
| WS 后续轮次 | #8564 | 真 handler+WS 复现第二轮限额/重握手重放首轮；后续轮次保护与首轮 failover 分开；本地全仓和竞态通过 |
| additional_tools | #8754 | 复现 store=false 丢声明 ID；OAuth HTTP/map 与原始 WS 路径最小适配，保留 API Key/大整数/无关字段；本地全仓通过 |
| Antigravity 长思考 | #8673 最小拆分 | 四个失败用例复现缺 includeThoughts/低 effort 选择；保留 lite 读流计时与首内容前不发 keepalive 的策略，补配置/映射，随后全仓通过 |
| 测试维护 | 本地 | 保留 47 处连接池 cleanup；Claude 夹具相对最低版本；Grok reasoning、GPT-6.2 目录断言匹配已实现行为 |

## 本地验证证据与限制
- service 在 10m 上限约 208s 结束；原先 100/180s 是整个包过短上限，不能凭空闲 WS worker 栈认定死锁根因。
- 六入口 Gemini/Antigravity+Redis 扩大回归：5 包、1434 测试/子测试通过。
- 全仓 `go test -tags=unit ./... -count=1 -timeout=10m`：52 个有测试的包通过，19,657 个测试/子测试通过，14 个按条件跳过。另 36 个包没有可运行测试。原 handler 上游额外 bootstrap 依赖及 GPT-5.6 首项旧断言均已修正。
- 关键 OAuth、租约、流式、真实 handler/WS、上下文和工具 ID 的 `-race`：182 项通过；随后新增 terminal_info 和 AG thinking 的竞态补验 60 项通过。两次测试范围有重叠，不把 182+60 宣称成去重数量。
- 全仓 unit 的 14 个跳过：3 个 TLS 实网测试、1 个 Linux 专属抓包样本、9 个需要显式 PostgreSQL/Redis 环境的 Prompt Audit 集成测试、1 个需要真实 OPENAI_API_KEY 的计数比对。它们未在本机运行，不声称全通过。
- 前端 typecheck、lint:check、227 文件/1841 Vitest、生产 build 通过；构建仅有体积/Browserslist 提示。
- embed 构建及 5 组部署脚本 fixture 通过。Apple container 测试使用临时目录和 fake container PATH；没有真实部署、删除生产卷或发布镜像。
- 新增 4 个 PostgreSQL OAuth CAS 集成用例：保存/调度 outbox、并发重新授权、影子账号与 outbox 原子回滚。原子性用例改用 autocommit 连接，避免测试自身 SAVEPOINT 回滚掩盖生产部分写入。已编译通过，未实际执行数据库；Redis 启动租约和 PostgreSQL TTL 等用例也已准备，待真实环境执行。
- 本机 Docker/OrbStack 引擎未运行，因此真实 PostgreSQL/Redis 尚未执行。没有启动生产服务或用 skip 替代实测。
- GitHub 验证遇到服务端阻塞：权限 API 虽显示 `enabled:true` 且工作流 `active`，但 CI (`354018963`) 和 Security Scan (`354018960`) 的实际 dispatch 都返回 HTTP 422：`Actions has been disabled for this repository.` 对已推送 SHA `94da627f36b72bd7170f9f319dfd32ddce52f7ce` 查询得到 0 个 runs。之前的“enabled”观察只表示配置返回值，不代表工作流已成功启动。
- 本地后端 `govulncheck ./...`（Darwin）和 Linux/amd64 源码目标扫描均退出 0：无代码可达漏洞；7 个未被代码调用的 required-module 漏洞提示仍保留。它们不是 GitHub Security Scan 或 Linux 实机执行。
- 前端原始生产依赖审计有 2 high / 12 moderate / 2 low。按仓库既有门禁运行后通过；两个 SheetJS high (`GHSA-4r6h-8v6p-xvw6`、`GHSA-5pgg-2g8v-p4x9`) 使用既有只写导出豁免，截止 2027-01-06，本轮没有新增、延期或放宽豁免。通过门禁不等于依赖零漏洞。
- 本机 Python 3.9 不能直接求值门禁脚本的 `str | None` 注解；使用 future annotations 编译标志执行同一脚本、原参数和原豁免文件后通过，没有修改门禁策略。
- 最后代码树全仓 unit：19,657 通过、14 显式跳过、0 失败，52 个有测试的包通过。`golangci-lint v2.9.0 --max-same-issues=0 --max-issues-per-linter=0`：0 issues，退出 0；原配置未变。处理 Builder 写入返回值、旧 GPT-6.2 测试格式和旧测试 Fatal 后显式退出，没有改业务控制流或关闭检查。
- Lint 首次只显示同类问题上限内的结果；取消显示上限后完整修正并复验。受影响 Antigravity/OpenAI 包另有 315 项 race 通过；与前述 182/60 项有重叠，不累加成唯一数量。

## 剩余范围，不能标为完成
1. 本轮本地 unit、真实 handler 模拟、race、embed/build、脚本和本地安全门禁已取得上述证据；真实 PostgreSQL/Redis 与最终 SHA GitHub CI/Security 仍阻塞。后续剩余功能完成后还需统一重验。
2. #8856 账号测试恢复与已有早期 401 差异；#8742/#8739 beta/effort；#8664/#8358 缓存身份/搜索重连；#8745/#8727/#8728/#8796 实际响应模型。
3. 原清单其余 Gemini 首字节/403/404 边界、Fable/Kimi/DeepSeek 额度价格、Grok/账单、请求体内存及快照必须有逐项证据，未读完不能标已覆盖。
4. Hydrogen 完整链路、真实模型资格不能由价卡推定；#8778 已关闭未合并。
5. Ultra Fast/Carbon 日期倍率、30m/12h 缓存 schema/计价、Vertex 默认路由、按用户批结算须先用户决策，未批准不改。
6. Rerank、Decisions API、DeepSeek Files/多模态、新平台只评估范围，不擅自扩产品边界。
7. 历史开放 PR 初始快照上限 1000 条，不能说全部历史开放项已逐项审完；已另按更新时间降序分页确认 2026-11-20 起更新的 24 条 Open/Draft PR 全部进入近期筛查快照，截止范围没有分页遗漏。

## 固定上游来源

Open/Draft 来源仍属于未经上游最终合并的变更；本地失败复现与测试支持本轮最小适配，不代表上游已批准。

| 来源 | 审查时状态 | 固定 head SHA | 适配范围 |
|---|---|---|---|
| [#8973](https://github.com/Wei-Shaw/sub2api/pull/8973) | MERGED | `e0328a62acf0ecf55622c297069eec93cca045e5` | 既有单请求冷却隔离等价覆盖，未重复开发 |
| [#8974](https://github.com/Wei-Shaw/sub2api/pull/8974) | OPEN | `0ad0c55fcaadca0ab7b16998c44ab0a42a57a3f7` | 503 容量短重试和换号，保留单账号模式 |
| [#8975](https://github.com/Wei-Shaw/sub2api/pull/8975) | OPEN | `c00fb0bd300566bf372525167f302aeab8d45ab2` | 空流不冷却共享账号 |
| [#8987](https://github.com/Wei-Shaw/sub2api/pull/8987) | OPEN | `5f9c4d0135e03cea65661095dea795a61d0d748c` | 最终同模型 RetryInfo 控制冷却 |
| [#8997](https://github.com/Wei-Shaw/sub2api/pull/8997) | OPEN | `affad30adda69482fe16295d3bcf4fab6ab0dc71` | 轮次边界保护/终态/工具隔离 |
| [#8980](https://github.com/Wei-Shaw/sub2api/pull/8980) | OPEN | `c2fb1383e34f5424c5251ce0bf7066ba6bf58feb` | 六入口截断和拒绝终态最小拆分；不宣称该 PR 所有附带行为均引入 |
| [#8685](https://github.com/Wei-Shaw/sub2api/pull/8685) | OPEN | `5b3d826cf45d8e895cf546f9b4add4eed8d22b0e` | 保留活跃跨实例 Redis 租约与等待状态 |
| [#8671](https://github.com/Wei-Shaw/sub2api/pull/8671) | MERGED | `fb52c6e136a9401d5c078bade0533b32715706f7` | OpenAI 轮换不可撤销后的有界 CAS 持久化 |
| [#8703](https://github.com/Wei-Shaw/sub2api/pull/8703) | MERGED | `6c79fdfc7ab55e2c067b475ee4a498efd12423c2` | 中间 system 消息保真 |
| [#8589](https://github.com/Wei-Shaw/sub2api/pull/8589) | MERGED | `85f43b640a5f92dab03599974cb19ee1a5005414` | 工具循环粘性种子稳定 |
| [#8741](https://github.com/Wei-Shaw/sub2api/pull/8741) | MERGED | `6d7e6b9d842a9a791398cf1c5f41c19e2a09dba6` | 原生 Anthropic 端点记录及附加读取错误回归 |
| [#8786](https://github.com/Wei-Shaw/sub2api/pull/8786) | MERGED | `1f2242436293c1a64e5e17b17ca1033af902f232` | API Key 合法 metadata.user_id 保留，OAuth 维持自身身份 |
| [#8622](https://github.com/Wei-Shaw/sub2api/pull/8622) | MERGED | `b2a695d617d490242a39b5f245cf2a50ae5d9993` | 独立页面上下文适配 |
| [#8769](https://github.com/Wei-Shaw/sub2api/pull/8769) | MERGED | `a0a03be96d81fd1aae4a989500cd4a2752212e80` | Codex 外部上下文，不引入 automation/delegation 产品能力 |
| [#8564](https://github.com/Wei-Shaw/sub2api/pull/8564) | MERGED | `0578fb160e5b77ecbfd7139b5b7761c39d73fd78` | 后续 WS 轮次不可重放首帧 |
| [#8754](https://github.com/Wei-Shaw/sub2api/pull/8754) | MERGED | `8dcdfff22a7ad9568a1c2f88900809dd8eeb4f56` | additional_tools 声明和引用 ID 保真 |
| [#8673](https://github.com/Wei-Shaw/sub2api/pull/8673) | MERGED | `e055d7bafc5b7eda444e0ec7c7aed99e2c802de1` | includeThoughts/effort 最小适配，保留 lite 原计时策略 |
| [#8994](https://github.com/Wei-Shaw/sub2api/pull/8994) | OPEN | `88b1924bd9711a458c5e78deb45c0178725e0810` | 终端上下文及命名/类型/call_id 边界 |

## 新增开放 PR 的范围决定

| 来源 | 本轮决定和理由 |
|---|---|
| #8994 | 已最小适配终端上下文，并先复现三个失败，再补类型/调用关联/重复键回归 |
| #8990 | 首输出前 API Key Responses 安全重试和多行 SSE；已审阅条件与回放预算，需与 lite 已有首输出预算做专门合并回归，尚未实现 |
| #8985 | GPT-6.2 默认指令缓存前缀；保留显式客户端指令是必要条件，尚需与已存在的 lite 默认指令/模型映射验证，未引入 |
| #8989 | 新插件模型目录/配置修订/管理桥接体系，会扩产品边界，暂不回移，需范围确认 |
| #8995 | 运维错误详情显示请求者，非核心转发修复，暂缓 |
| #8983 | 原生 Gemini 音频单独价桶及目录价格应用，会改变实际费用；列为计费政策待确认，不静默改变 |
| #8977 | 新的按查询收费 Google Search grounding 开关及管理 UI，列为计费/产品范围待确认 |

## 既有 40 个提交的保留清点

下表核对提交存在性、变更文件及当前全仓测试覆盖；不是把每项都宣称生产/套餐资格或完整协议已验收。模型全链路和政策争议仍按剩余范围处理。

| 提交 SHA | 具体主题 | 变更文件数 / 测试文件数 |
|---|---|---|
| `66d55467da8fefb44a40003c9070ee3c9ff6d6e7` | fix(scheduling): release concurrency slot on hydration failure | 2 / 1 |
| `1b68ce71f35d6184fa198ce2a6f8df6b8c9740d2` | fix(auth): atomically consume refresh tokens during rotation | 6 / 3 |
| `e796e2613d2db6be24653243f669479a1b16977d` | fix(billing): scope RPM overrides to their configured group | 2 / 1 |
| `e5363c2c68e0f6de41ec5d9ffe7f95b6685b607d` | fix(apicompat): preserve non-strict JSON Schema default | 2 / 1 |
| `c1c8222e21d587bf13bde914c0133bfcebf262a6` | fix(claude): use Sonnet 4.6 for default account tests | 2 / 1 |
| `6392d0936cd6358738c1611b358ca218b8a33af8` | fix(openai): reject invalid UTF-8 before Responses routing | 2 / 1 |
| `e22bacfae8366582d6a7971e102e12465658a2c6` | fix(gateway): apply account error rules in Anthropic compat paths | 3 / 1 |
| `bfb386047f733c511cd764f40746df66384cab39` | fix(scheduler): restore accounts when transient cooldowns expire | 9 / 8 |
| `13776ca8d5014a006ff5b26c34cc6358a398e04b` | test(scheduler): verify configured cooldown membership with PostgreSQL and Redis | 1 / 1 |
| `4a8d09b1fbcf7a6a72b52118cace731310f9642f` | test(scheduler): isolate configured-membership assertions from shared fixtures | 1 / 1 |
| `65b7e87cdee5fa300281dda60081d566d1f72e0d` | fix(billing): make group rate and RPM sync transactional | 2 / 1 |
| `64e96aa42c8f77491afda18a0b3fc907e57ecc87` | test(billing): verify group sync rollback against PostgreSQL | 1 / 1 |
| `5e8bdab0e9a6e4a098db92008f2d809c5cde5d94` | fix(gateway): prevent request-wide account cooldowns and normalize Pi system identity | 4 / 2 |
| `ecc0d2ec572978b86194a76c713d0612b80e5926` | fix(gemini): preserve independent same-name parallel tool calls in streams | 4 / 1 |
| `90a780f6e9fddf23993e278f723c78187896ac6d` | docs(backport): submit one consolidated PR after full validation | 1 / 0 |
| `3c366ab0d12064d0106ab8c5532e947916b7b0d8` | fix(gemini): preserve structured response formats and valid tool schemas | 5 / 2 |
| `42ebdcfc918a4195474a75a4ab9ae87e64538392` | feat(gemini): add Carbon catalog, routing, thinking levels and introductory rates | 19 / 8 |
| `c72a30860910ef3cbea4e4437be966d33816113e` | fix(openai): forward SSE keepalives without delaying first output failover | 3 / 2 |
| `1bb4ea2fd9b52775700875da5f5458aa82c52971` | fix(openai): keep first-output buffering bounded when temporary storage fails | 2 / 1 |
| `9513e877463871dcafb30e9b3b10afe4328044e3` | fix(claude): retry once with rotated OAuth token across gateway protocols | 4 / 1 |
| `7ee08e51caad45f2e198b8e9fe9badbc375739c4` | fix(auth): keep setup-token accounts recoverable after transient 401 | 3 / 2 |
| `4fa5d07e476cfe700692948ccc496e438fb4ade2` | fix(gemini): keep quota bars visible while usage refreshes | 1 / 0 |
| `fcac6ab18a27833e14fcc17d4e4ccd8de1e29520` | fix(openai): account health updates on terminal compatibility stream errors | 4 / 1 |
| `3e564316891b50a097362baa3e78526e7f342909` | fix(apicompat): keep prior reasoning separate from visible assistant text | 3 / 2 |
| `d725ab3c979cc520be724e536be90a19edffcf2e` | fix(quota): preserve OpenAI upstream observation freshness across failed refreshes | 9 / 3 |
| `07abd5be19d72cccd5062bb664792dc7a9f608a0` | fix(frontend): keep account quota and rows visible during refresh | 3 / 1 |
| `1653e119c1ce4b2625e6a00453f6ba5f56950fce` | fix(gateway): emit SSE error events when upstream fails after keepalive | 2 / 1 |
| `9dd57edd464293e424ff363b1791320063b1d7e4` | fix(openai): sanitize unsupported Python regexes in Responses tool schemas | 4 / 1 |
| `aed6cc09d3a1a15f0310fa5220a21f9fc533636b` | feat(claude): add Fable and Mythos 5.2 models and protocol support | 17 / 4 |
| `8271b7f63e94041ba54e02c2ebd86c37195df941` | fix(upstream): shorten dead TCP connection detection with bounded keepalive | 6 / 2 |
| `6814647ff1a72fb478cf7b7816bb5bc6cdee1afc` | fix(openai): bound stream idle failover to one account switch | 2 / 1 |
| `9a91f0db6437a02e5bed76eba6c336c12945208e` | feat(openai): add GPT-6.2 Sol and Astra standard model compatibility | 14 / 2 |
| `76383eac0840ca34179d8d9d20ae265dfd2572fa` | perf(openai): batch raw chat SSE flushes before upstream reads | 3 / 1 |
| `100b58376a5dd7cb71f17e901919ee3af0ebdaff` | fix(anthropic): preserve sparse cache-creation TTL snapshots in passthrough | 4 / 1 |
| `02fe90cb93c1183c8ea3fd9a88985bfe5328670d` | fix(anthropic): preserve cache creation TTL usage across compatibility streams | 21 / 5 |
| `fc994ef2a4a3a2c245c7d8c63ea25c3e281bc071` | perf(ollama): preserve Chat reasoning with single-pass request edits | 3 / 1 |
| `731d551983aec148aa05e7080802971351c8bf13` | fix(billing): reconcile Anthropic cache TTL detail with usage logs | 2 / 1 |
| `449cf40b66bfdba6bff49d63961c69b2439f3906` | test(openai): verify GPT-6.2 WebSocket eligibility and Composite routing | 1 / 1 |
| `fa74ef0701d1f497021a14121e9a13c6db52c75f` | fix(antigravity): retain streamed usage and add cache TTL database regression | 3 / 2 |
| `7b24c3d54e2df8fda903cec62c8d47d57e4ddba9` | test(billing): cover TTL-only Anthropic cache in account statistics | 1 / 1 |
