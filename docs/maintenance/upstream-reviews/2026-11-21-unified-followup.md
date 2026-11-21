# 2026-11-21 统一回移续作：协议、额度与并发正确性

> **后续进展**：调度 epoch 合并、请求内存与历史转换修复，以及真实 PostgreSQL/Redis 运行结果已补充到[2026-11-21 继续开发记录](2026-11-21-scheduler-memory-integration.md)。下方分阶段红/绿日志保留为历史证据，不代表最终清单已结案。

> 当前状态：代码集中开发与验证中，尚未达到最终 PR 门槛。本文件不把定向通过外推成全仓或真实数据库通过。

## 工作区与边界

- 原始 `/Users/leobot/Projects/sub2api` 仅只读，未清理、覆盖或处理其合并冲突。
- 沿用隔离 worktree `/Users/leobot/Projects/.webcodex-worktrees/sub2api-edb4965d` 与分支 `backport/unified-integration-20261120`。
- 本轮起始和当前已提交 HEAD：`29509a944ed6e1c7c7845bcbcbe2d9f4d0a838d3`；本轮改动尚未提交。初始未提交改动已保存 binary diff 与 untracked tar 后再继续适配。
- 上游 v0.2.24 / `328db26f450edbdf9f8d4344a3ea903ac6396842`，Lite `8fe8e095ed8abfef9a7e9fda6de4ae45de425296`；以固定来源审查，不能称全部历史开放 PR 已审完。
- 用户要求先收尾开发、后处理 GitHub 执行权限。本轮未重新授权、未建新 PR、未合并、未发布或部署。

## 已开发的相关修复（待最终统一验收）

| 范围 | 来源 | 精简版适配与验证重点 |
|---|---|---|
| 响应模型、公开别名、统计端点 | #8745/#8727/#8728/#8796 | 接续已有未提交观察代码；公开模型与实际上游模型分别保存，不覆盖用户计费身份 |
| Fable 周额度与恢复证据 | #8644/#8723/#8779 | 缺失/null 不当作零，超过阈值标记阻止恢复；精确 CAS 不能清掉更新后的限额 |
| 五小时窗口 | #8698 | 更新起止边界，旧窗口费用不算进新窗口；新增 SQL 和真实 PG 夹具 |
| GPT-6.2 缓存前缀 | #8985 | 以实际映射模型处理网关默认指令；保留显式指令、compact、其他模型、图片桥及大整数 |
| 消息级 Anthropic beta | #8742 | 最终出站 header 与 messages[].output_config 一致；不隐式增加 Lite 默认 beta |
| Codex fork 缓存身份 | #8664 | Lite 自有 fingerprint 的 session/full 模式识别父头与子 metadata；不引入上游另一套身份框架 |
| Antigravity 账号目录 404 | #8781 局部 | 保留先换 URL 的顺序，再对精确目录拒绝进行账号级切换；普通 404 不泛化；不采用政策拒绝误报猜测 |
| Antigravity 首字节期限 | #8744 局部 | 可配置保护默认 0 保持现状；取消、迟到数据、关闭一次、所有重试/收集器和心跳后错误帧；不新增前置心跳 |
| Grok 媒体错误 | #8669 | Grok 专属嵌套/平面错误提取、脱敏、规则优先级；不改变其他平台提取规则 |
| Grok 搜索转换与计量 | #8834/#8864 | xs_/as_ 服务端调用不成为客户端工具；成功次数（含 0）优先，失败项/重复/迟到事件不重复收费；未改费率 |
| Kimi 月额度 | #8763 | monthly total/code、有效空档位清理、异常/超大/残缺响应保留旧快照；调度和界面一致；旧行请求不能覆盖新账号 |
| DeepSeek 显式价格 | #8825 范围核对 | Lite 没有上游 override-file 入口，不扩新配置产品；验证既有分组自定义非负价及显式 0 在峰谷保持优先 |
| WS 搜索激活 | #8358 | 已初始化无搜索 socket 在激活时重连；仅使用完整且独立限制为 16MiB 的当前窗口历史；未知链和超限不猜测重放 |
| OpenAI 账号探测恢复 | #8856 最小重写 | 同号最多刷新一次、原请求与 TLS 保持；成功证明绑定实际凭据；父/影子双重 CAS、outbox 原子化；保留独立冷却和手工停用 |
| 实际扣费与账号成本 | #8806 | 按账号筛选也不把账号成本填入 actual_cost；内部成本独立列，用户端隐藏；账号额度仍使用账号成本 |
| API Key 无状态流重试 | #8990 | 首输出前最多换一次其他 Key；工具结果/托管工具/关联响应/取消/歧义 JSON 不重放，分块 JSON 不提前提交 |
| 调度 DB 同桶回源 | #8833 最小适配 | 已按 bucket+epoch 合并；取消隔离、有界期限、服务停止和独立切片回归通过，未引入 stale grace；详见续作记录 |
| 价格启动检查 | #8798 | 接续启动异步远端 hash 核验，保留既有价格来源与收费策略 |

## 验证记录与限制

- 定向红→绿证据存放于本轮隔离 evidence 目录，索引 `followup-test-index.json`。测试标签与真实 pass/fail/skip 结果见下方，不把旧阶段日志当作最终代码树结论。
- 上一轮前端月额度与旧请求隔离、成本显示/隐私已完成 typecheck/lint、228 文件/1,846 测试及 build；本次后端续作没有再改前端。
- 2026-11-21 续作已取得真实 PostgreSQL 18.1 / Redis 8.4.0 仓储整包 1,384 通过、1 原有跳过；审计整包 207 通过、零跳过。使用独立临时 Unix socket，不启动 OrbStack。后续最终树验证见[续作记录](2026-11-21-scheduler-memory-integration.md)。
- 本次最后代码树已复核：Go unit 20,215 pass/14 skip，关键 race 190 pass，Lint 0 issues，embed 构建成功；真实仓储 1,384 pass/1 skip、审计 207 pass/0 skip。3,508 源文件清单无漂移，详见续作记录及验证 JSON；这不等于整个剩余回移范围或远端 CI 已完成。
- GitHub CI/Security 原阻塞为实际 dispatch HTTP 422，元数据 enabled 不代表可执行；按用户顺序留待代码收尾后再处理。

## 仍需决策或审查的范围

- 迁移编号与新 schema、按用户批量结算、30m/12h 缓存计价、Ultra Fast/Carbon 日期费率、Vertex 默认路由和价格：没有用户批准，不改。
- Hydrogen 已关闭未合并的 #8778 不当作已发布全协议能力；实网套餐资格不能由价卡/模型列表推定。
- Rerank、Decisions API、DeepSeek Files/多模态和新增大平台仅评估，不因“全部开发”自动恢复商业化外围模块。
- #8739 的上游 deny/逐模型 effort 依赖与 Lite 现有 OpenAI/Composite 配置边界不同，需独立最小适配或记录不适用部分，不能直接复制签名。
- #8875 的请求/审计内存所有权和 #8832 的历史尾部修复已继续实施并取得红→绿证据；#8833 stale grace 未采用，#8693 迁移、#8705/#8876 其余预算、#8537 停机以及 #8809/#8770 仍待独立审查或开发，详见[续作记录](2026-11-21-scheduler-memory-integration.md)。

## 本地阶段测试索引

- `combined-followup-green`：{"run": 477, "output": 1194, "pause": 6, "pass": 477, "cont": 6}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/pkg/claude", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/repository", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "pass"}]。
- `grok-green-kimi-red`：{"run": 1165, "output": 2719, "pass": 1156, "pause": 131, "cont": 131, "fail": 9}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/pkg/apicompat", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "fail"}]。
- `kimi-monthly-green`：{"run": 78, "output": 240, "pause": 30, "pass": 77, "cont": 30, "fail": 1}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "fail"}]。
- `probe-ws-quota-expanded`：{"run": 145, "output": 413, "pass": 142, "pause": 3, "fail": 3, "cont": 3}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/repository", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "fail"}]。
- `probe-green-usage-cost-red-v2`：{"run": 37, "output": 154, "pass": 34, "fail": 3}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/repository", "action": "fail"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "fail"}]。
- `usage-cost-green-stream-replay-red`：{"run": 4, "output": 8, "pass": 4}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/repository", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "pass"}]。
- `responses-failed-frame-red`：{"run": 8, "output": 147, "fail": 7, "pass": 1}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "fail"}]。
- `responses-failed-frame-green-v2`：{"run": 22, "output": 64, "pass": 22}；包终态 [{"package": "github.com/Wei-Shaw/sub2api/internal/handler", "action": "pass"}, {"package": "github.com/Wei-Shaw/sub2api/internal/service", "action": "pass"}]。

## 固定来源

| PR | 状态 | 审查 head SHA |
|---|---|---|
| [#8745](https://github.com/Wei-Shaw/sub2api/pull/8745) | MERGED | `90f115132490b39be090459455cdb51fdda43df9` |
| [#8727](https://github.com/Wei-Shaw/sub2api/pull/8727) | MERGED | `d30179f4b5c8f56750fea3d6a7a3f74d50796230` |
| [#8728](https://github.com/Wei-Shaw/sub2api/pull/8728) | MERGED | `ac981240cc44f4e61bf76aa1b67eb1a5436aab30` |
| [#8796](https://github.com/Wei-Shaw/sub2api/pull/8796) | MERGED | `7349750c017ca0d4efbb63cc109c6692fcc4d228` |
| [#8644](https://github.com/Wei-Shaw/sub2api/pull/8644) | MERGED | `c115dc27ab2e21e121874555b630ba77348badcf` |
| [#8723](https://github.com/Wei-Shaw/sub2api/pull/8723) | MERGED | `c829317fc5987b5d0608c861230b45325dd70d31` |
| [#8779](https://github.com/Wei-Shaw/sub2api/pull/8779) | MERGED | `4114fedd1892ee8b14aec40c716f5685edf9663a` |
| [#8698](https://github.com/Wei-Shaw/sub2api/pull/8698) | MERGED | `fa910c1c3842a627a3b9372bf73e7cee11b26593` |
| [#8985](https://github.com/Wei-Shaw/sub2api/pull/8985) | OPEN | `840a8503cceb49bc8a1b9bcdfa55d5902af8e35d` |
| [#8742](https://github.com/Wei-Shaw/sub2api/pull/8742) | MERGED | `bd89d3d943c0b2dafaf927f6665ea90ccfaaada8` |
| [#8664](https://github.com/Wei-Shaw/sub2api/pull/8664) | MERGED | `74ba91aa6b6d130d0b0029fabffed6ce3425dffe` |
| [#8781](https://github.com/Wei-Shaw/sub2api/pull/8781) | OPEN | `f00fa7af0606088a6972fc4188f61435e45aa2aa` |
| [#8744](https://github.com/Wei-Shaw/sub2api/pull/8744) | OPEN | `3c9cd3a88ab5dc1f07eb298553cb9eb6bfabd965` |
| [#8669](https://github.com/Wei-Shaw/sub2api/pull/8669) | MERGED | `03a489e43e5103a488387e6b96602bff57a73af7` |
| [#8834](https://github.com/Wei-Shaw/sub2api/pull/8834) | MERGED | `70ad8365d9b5a0be5ebaf3c0065ff385f8c081fd` |
| [#8864](https://github.com/Wei-Shaw/sub2api/pull/8864) | MERGED | `2c587f78b07e35f23e79ac63d180a06234bee050` |
| [#8763](https://github.com/Wei-Shaw/sub2api/pull/8763) | MERGED | `694d09379b24d93a4a57ad5dda1c0bf8d940707d` |
| [#8825](https://github.com/Wei-Shaw/sub2api/pull/8825) | MERGED | `b7878328b699bd5f5b61e10df2db74a3723dfc7a` |
| [#8358](https://github.com/Wei-Shaw/sub2api/pull/8358) | MERGED | `9c7380732df8ad4420b8be161504552a52b585a5` |
| [#8856](https://github.com/Wei-Shaw/sub2api/pull/8856) | OPEN | `d3c49f1ec122c9986ed618b464544773178e62ce` |
| [#8806](https://github.com/Wei-Shaw/sub2api/pull/8806) | MERGED | `d05e8ef033db09bef1c77fe579801591322bc7cb` |
| [#8990](https://github.com/Wei-Shaw/sub2api/pull/8990) | OPEN | `f0d1d6a27ec5cc0c31419b98d98fbbf97bed918f` |
| [#8833](https://github.com/Wei-Shaw/sub2api/pull/8833) | MERGED | `7f3fe5bfde146f940aaad88e89f6e8944e1f5ceb` |
| [#8798](https://github.com/Wei-Shaw/sub2api/pull/8798) | MERGED | `c45ef4116f0cb680a6fc87bb2e75d78869140e81` |
| [#8693](https://github.com/Wei-Shaw/sub2api/pull/8693) | MERGED | `829bf2cdb9a6a235a5d447ccd92405dbbc123577` |
