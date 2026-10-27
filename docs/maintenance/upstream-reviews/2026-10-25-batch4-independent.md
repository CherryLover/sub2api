# 第四批独立回移：用量 / 计费 / 断连可靠性 (2026-10-25)

## 独立基线

- 上游：Wei-Shaw/sub2api，各项参考 PR 如下。
- 精简版 PR 目标：CherryLover/sub2api main，创建分支时 SHA 为 `6af365be55481a5a14d86222eefc8e742b70d738` (v0.1.206)。
- 开发分支：`backport/2026-10-25-billing-independent`，直接基于 main，不包含第一、第二、第三批任何祖先提交，也不等待对应 PR 合并。
- 为使本批独立执行安全审计，附加与前三批相同的 Go 1.26.10 / DOMPurify 3.4.17 / follow-redirects 1.16.1 安全版本升级。没有复制其它批次的核心功能修复。

## B01–B08 核验及回移

| 编号 | 上游来源 | 精简版范围与结果 | 本批提交 | 验证 |
|---|---|---|---|---|
| B01 | [#8168](https://github.com/Wei-Shaw/sub2api/pull/8168) | Gemini 客户端真正断连后保留已见 token；非完整生图不按完成张数收费；其它上游读错不误判为断连 | `54c356063` | 定向单测通过 |
| B02 | [#8210](https://github.com/Wei-Shaw/sub2api/pull/8210) | Anthropic 协议 Chat / Responses 中客户端断连后有界读取上游 usage；idle、错误帧、EOF 完整分类；handler 部分 usage 至多记一次；保留现有 heartbeat 不重置超时 | `48de13791` | service / handler 定向单测通过 |
| B03 | [#8177](https://github.com/Wei-Shaw/sub2api/pull/8177) | Antigravity/Gemini 中 prompt 缺失但 cached 大于零时 clamp 非缓存输入为零，缓存读取保留 | `90add6dc9` | 定向单测通过 |
| B04 | [#8170](https://github.com/Wei-Shaw/sub2api/pull/8170) | Grok 4.7 Fast / Build Fast 静态和回退价格；200k token 进入高价档，LiteLLM 精简版显式阈值和倍率 | `cdf3707dd` | 定向单测通过 |
| B05 | [#7792](https://github.com/Wei-Shaw/sub2api/pull/7792) | OpenAI WS v2 图片输入 token 和 hosted image_gen 输出 token 同步到逐轮及汇总计费 | `2ef2e52b1` | relay / service 定向单测通过 |
| B06 | [#7779](https://github.com/Wei-Shaw/sub2api/pull/7779) | WS 每轮重取同一 API Key/同一 Group 的当前价格快照，安全保护换组或读取失败，利润门和记录使用同一快照 | `11c8de741` | handler 定向单测通过 |
| B07 | [#7573](https://github.com/Wei-Shaw/sub2api/pull/7573) | 频道图片价 nil 继承模型目录价，显式 0 保留免费的语义，兼容区间定价 | `4c082a9ae` | 定向单测通过 |
| B08 | [#7380](https://github.com/Wei-Shaw/sub2api/pull/7380) | Anthropic/国产协议兼容输出中的 cache/input usage 与账单一致，不重复处理终态/EOF | `2f49ec074` | 定向单测通过 |

## 保留的独立修复和裁剪边界

- 保留精简版原有 Gemini 传输 failover、缓存签名、用量计算、WS 会话调度；不会取消数据库写失败时的账号内存冷却。
- 保留 Anthropic SSE heartbeat 不重置读间隔超时的精简版行为；断连排水使用单独边界。
- 只回移保留的 API 兼容与计费可靠性能力，不增加支付、会员、营销、推广、独立运营模块或新平台入口。
- B02 的用户断连和上游错误严格区分；无可观测用量不得伪造扣费。
- 在 PR 创建前必须以**最终提交 SHA** 验证本地完整 Go unit、GitHub CI、Security Scan；以旧批次的绿色测试替代是不允许的。
- 本批 PR 仅交由项目维护者合并，不自动发布镜像或部署。

## CI / 验收结果

待独立分支最终 SHA 推送后填入 GitHub Actions 精确运行链接与全部结论。
