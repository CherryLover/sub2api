# 2026-10-25 第二批独立回移台账

目标仓库：CherryLover/sub2api；基线：lite/main 6af365be55481a5a14d86222eefc8e742b70d738（v0.1.206）

分支：backport/2026-10-24-core-routing（重整后直接以精简版 main 为祖先）。

本批与首批安全 PR #32 没有 Git/代码合并前置依赖。原堆叠版本 SHA 1d93b2d9b3fff3829cab00d8c6096cabad552910 留作备份；本独立分支对应原第二批与首批之间的净代码增量，不能用原堆叠 CI 代替本 PR CI。两批都应按独立 PR 审查、等待维护者合并。已在首批处理的 R02 Cloudflare 1010 在本批不重复实施。

## 本批范围与原上游依据

| ID | 上游依据 | 原精简版差异与决策 | 原实现证据 | 独立重整状态 |
|---|---|---|---|---|
| R01 | [#8305](https://github.com/Wei-Shaw/sub2api/pull/8305) | Codex overload / Retry-After / 空 SSE；核心完成：Codex overload/Retry-After/空流与官方 CLI 身份配对；全量单测通过，待最终CI | c1d8024e6、b94b6eb1b、347c85c30 | 代码已整理，最终 CI 待验证 |
| R03 | [#7448](https://github.com/Wei-Shaw/sub2api/pull/7448) | Gemini 传输故障快速换号；已适配，DB 写失败保留内存冷却；待 CI | 91274f178、47e584651 | 代码已整理，最终 CI 待验证 |
| R04 | [#8076](https://github.com/Wei-Shaw/sub2api/pull/8076) | Vertex SA 删除和 401 错误策略复用；已适配：Vertex SA 删除识别及策略只检查一次；待 CI | b24435409 | 代码已整理，最终 CI 待验证 |
| R05 | [#8093](https://github.com/Wei-Shaw/sub2api/pull/8093) | Gemini 4 Argon、Hub UA、付费档位和目录；核心完成：Gemini 4 Argon/UA/定价、档位识别、同服务端点模型列表；排除未保留的元数据增强模块 | 05e000819、4db90f26d | 代码已整理，最终 CI 待验证 |
| R06 | [#8121](https://github.com/Wei-Shaw/sub2api/pull/8121) | GPT-5.5 OAuth 退役与 API Key 保留；核心完成：OAuth 退役选号、公开目录和管理候选列表过滤；API Key 保留 | 9cd199046、85b36bcf7、9aaf778aa | 代码已整理，最终 CI 待验证 |
| R07 | [#8088](https://github.com/Wei-Shaw/sub2api/pull/8088) | Codex 个人和 Team 工作区导入隔离；已适配：Codex 多 workspace 隔离；待 CI | 5a2c67303 | 代码已整理，最终 CI 待验证 |
| R08 | [#8079](https://github.com/Wei-Shaw/sub2api/pull/8079) | Anthropic 协议独立 Base URL；已适配：Anthropic 协议专用 URL；待 CI | eda32f120 | 代码已整理，最终 CI 待验证 |
| R09 | [#7303](https://github.com/Wei-Shaw/sub2api/pull/7303) | 上游 HTTP 响应 body 关闭生命周期；已回移；HTTP body lifecycle 定向测试通过，待完整 CI | 064343083 | 代码已整理，最终 CI 待验证 |
| R10 | [#7195](https://github.com/Wei-Shaw/sub2api/pull/7195) | 健康但手动暂停的 OAuth 继续刷新；已适配：后台刷新暂停的有效 OAuth；待 CI | f18a83922 | 代码已整理，最终 CI 待验证 |
| R11 | [#7427](https://github.com/Wei-Shaw/sub2api/pull/7427) | 传统 OpenAI 调度 previous_response 粘性；已核验精简版有等价实现；回归通过 | 精简版既有 openai_account_scheduler.go, TestOpenAIGatewayService_LegacySchedulerRoutesPreviousResponseOwner | 代码已整理，最终 CI 待验证 |
| R12 | [#7308](https://github.com/Wei-Shaw/sub2api/pull/7308) | 非 GPT 模型 Codex 提示词去 GPT 身份；已适配：非 GPT 模型目录移除 GPT 自我声明；待 CI | d5ec55307 | 代码已整理，最终 CI 待验证 |
| R13 | [#7411](https://github.com/Wei-Shaw/sub2api/pull/7411) | Claude Agent SDK 身份提示兼容；已适配 Antigravity SDK 前缀身份处理；定向测试通过，待CI | 7b0618dee | 代码已整理，最终 CI 待验证 |
| R14 | [#7256](https://github.com/Wei-Shaw/sub2api/pull/7256) | Antigravity Claude attribution 文本兼容；已适配 Antigravity billing attribution 清理；定向测试通过，待CI | 7b0618dee | 代码已整理，最终 CI 待验证 |
| R15 | [#6689](https://github.com/Wei-Shaw/sub2api/pull/6689) | Antigravity web_search 与函数工具混用；已适配：Gemini / Claude 包装路由优先函数工具；定向测试通过，待CI | 3a1424f45 | 代码已整理，最终 CI 待验证 |
| R16 | [#7703](https://github.com/Wei-Shaw/sub2api/pull/7703) | thinking_signature_invalid 安全恢复；现有 HTTP/WS Forward 完成：特定错误安全恢复及隔离失效密文摘要记忆；其它 WS ingress 路径不额外扩容 | 10aab5090、ed0437739 | 代码已整理，最终 CI 待验证 |
| R17 | [#7939](https://github.com/Wei-Shaw/sub2api/pull/7939) | Codex 历史 web_search_call 压缩恢复；已适配：OAuth 历史 web_search_call 补 hosted tool；定向通过，待CI | bc0bdc8c7 | 代码已整理，最终 CI 待验证 |
| R18 | [#7609](https://github.com/Wei-Shaw/sub2api/pull/7609) | 客户端断开统一标记 499；核心请求入口和保留服务完成：499/Google CANCELLED/监控跳过纯取消；持续防回归 | 240a2119d、d8d68f93a、ce3105672 | 代码已整理，最终 CI 待验证 |
| R19 | [#7339](https://github.com/Wei-Shaw/sub2api/pull/7339) | Responses 流内 error.status 解析；已适配：SSE 内嵌 status/status_code；待 CI | c1d8024e6 | 代码已整理，最终 CI 待验证 |
| L01 | [#5937](https://github.com/Wei-Shaw/sub2api/pull/5937) / [#8181](https://github.com/Wei-Shaw/sub2api/pull/8181) (均未合并) | Antigravity 有序端点 404/429 故障转移；精简版最小完成：付费权限端点的 429、限定 Gemini 3.7 catalog-404 回退；普通404不重放 | b548d23c8 | 代码已整理，最终 CI 待验证 |

## 依赖和边界

1. 不引入首批 S01–S04 的提交，不复制支付、充值、站点 OAuth、新平台、营销、更新器。
2. R03 数据库写入失败仍保留账号内存冷却，强制保持既有精简版调度安全逻辑。
3. L01 仅在账号已知可访问的备选 URL 上处理特定 429/catalog-404，禁止免费账号错误路由至 paid-only 端点、一般 404 重放。
4. R05 同端点模型列表发现只使用保留的模型列表功能，不回移上游额外的模型 metadata 管理子系统。
5. 自包含的 CI/依赖安全基线：Go 1.26.10、DOMPurify 3.4.17、follow-redirects 1.16.1，使用精简版 main 的旧 Go/锁文件将触发已知安全扫描告警。相关工具链变更与首批 PR #32 的依赖升级相同，属于独立 CI 环境要求；两批 PR 无代码祖先或合并顺序依赖，维护者可按任意顺序合并，重复安全基线将自动收敛。

## 验证追踪

- 独立分支构建前：原堆叠 PR #33 HEAD 1d93b2d9b3 的 fork/目标 CI 与 Security Scan 曾通过；仅作历史参考。
- 独立分支代码编译：Go1.26.10，go test -tags=unit ./internal/service ./internal/handler ./internal/pkg/antigravity -run '^$' 通过。
- 独立分支完整 CI/Security Scan：尚未运行；**不得复用原堆叠 PR #33 的旧成功检查**。
- 正式 PR：将复用目标仓库现有 #33，重写其分支 HEAD 以移除第一批祖先提交；保留 #32 原状。需要最终分支 SHA 的 CI、Security Scan 全通过方可视为交付。
