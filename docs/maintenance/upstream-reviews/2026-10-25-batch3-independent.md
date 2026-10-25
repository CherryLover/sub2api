# 第三批独立回移台账 (2026-10-25)

- 基线：CherryLover/sub2api main 6af365be55481a5a14d86222eefc8e742b70d738 (v0.1.206)
- 独立开发分支：backport/2026-10-25-protocol-independent，父提交为 main，不依赖 PR #32/#33 的代码、提交或合并时间。
- 原堆叠第三批 SHA e551dbc03be3d21c40a9aa80de3847246c74d0f4 已留作历史备份，仅提取第三批相对第二批的代码净增量。
- 安全依赖基线：Go 1.26.10，DOMPurify 3.4.17，follow-redirects 1.16.1（独立执行漏洞扫描的自包含环境要求；其它批次可任意顺序合并）。
- 未经完整验收不提前创建正式第三批 PR；PR 一旦提交只指向目标 main，无 stacked base。

## 候选状态

| 编号 | 上游来源 | 变更说明 | 原实施状态 | 独立分支代码状态 |
|---|---|---|---|---|
| C01 | [#8114](https://github.com/Wei-Shaw/sub2api/pull/8114) | Gemini thinking 努力等级和摘要输出 | 待核验 | 未实施，待核验 |
| C02 | [#8188](https://github.com/Wei-Shaw/sub2api/pull/8188) | Schema const/examples 清理 | 已适配 const/examples 及 schema 关键字名称保留；定向测试通过，待CI | 已提取，待全量验证 |
| C03 | [#8224](https://github.com/Wei-Shaw/sub2api/pull/8224) | DeepSeek 图片输入去重 | 待基础能力：当前 lite 不含上游 DeepSeek 图片归一化转换；不能直接套去重补丁 | 未实施，待核验 |
| C04 | [#8167](https://github.com/Wei-Shaw/sub2api/pull/8167) | Gemini 生图沿用分组开关 | 待核验 | 未实施，待核验 |
| C05 | [#8140](https://github.com/Wei-Shaw/sub2api/pull/8140) | Responses created_at 补齐 | 已补常规和 passthrough Responses 的缺失 created_at；定向测试通过，待CI | 已提取，待全量验证 |
| C06 | [#7077](https://github.com/Wei-Shaw/sub2api/pull/7077) | Antigravity prefixItems/tuple Schema | 已适配 prefixItems/tuple 清理；定向测试通过，待CI | 已提取，待全量验证 |
| C07 | [#7272](https://github.com/Wei-Shaw/sub2api/pull/7272) | function_call_arguments.done 参数一致 | 已回移工具参数 done 完整值；定向测试通过，待CI | 已提取，待全量验证 |
| C08 | [#7785](https://github.com/Wei-Shaw/sub2api/pull/7785) | Chat named tool_choice 转 Responses | 已适配 Chat 指定函数 tool_choice 展平；定向测试通过，待CI | 已提取，待全量验证 |
| C09 | [#7786](https://github.com/Wei-Shaw/sub2api/pull/7786) | Chat developer 角色保持 | 已回移 developer 角色保持；定向测试通过，待CI | 已提取，待全量验证 |
| C10 | [#7787](https://github.com/Wei-Shaw/sub2api/pull/7787) | 传统 function_call 与结果 ID 配对 | 已适配旧 function_call / function 结果稳定 ID 配对，保留音频转换；定向测试通过，待CI | 已提取，待全量验证 |
| C11 | [#7788](https://github.com/Wei-Shaw/sub2api/pull/7788) | Responses refusal 到 Chat 转换 | 已适配 refusal 非流式和流式转换；定向测试通过，待CI | 已提取，待全量验证 |
| C12 | [#7789](https://github.com/Wei-Shaw/sub2api/pull/7789) | Anthropic 聚合流负索引 panic | 已适配四条 Anthropic 缓冲流负索引防 panic；定向测试通过，待CI | 已提取，待全量验证 |
| C13 | [#6925](https://github.com/Wei-Shaw/sub2api/pull/6925) | done 和终态消息恢复文本 | 待核验 | 未实施，待核验 |
| C14 | [#7246](https://github.com/Wei-Shaw/sub2api/pull/7246) | DeepSeek 并行工具图片与顺序 | 待核验 | 未实施，待核验 |
| C15 | [#7635](https://github.com/Wei-Shaw/sub2api/pull/7635) | Chat to Responses 类型标记 | 已回移 Chat 转 Responses message.type；定向测试通过，待CI | 已提取，待全量验证 |
| C16 | [#7854](https://github.com/Wei-Shaw/sub2api/pull/7854) | Chat 兼容流 SSE ping flush | 已回移 Chat SSE ping 注释转发和 flush；定向测试通过，待CI | 已提取，待全量验证 |
| C17 | [#8080](https://github.com/Wei-Shaw/sub2api/pull/8080) | OpenAI API Key Messages web_search 模拟 | 待核验 | 未实施，待核验 |
| C18 | [#7094](https://github.com/Wei-Shaw/sub2api/pull/7094) | Responses to Chat system/developer 顺序 | 待核验 | 未实施，待核验 |

## 当前保留边界

- C01/C03/C04/C13/C14/C17/C18 未完成的项目须继续按精简版保留中转能力回移，不引入已裁剪的商业模块、支付、营销等。
- C03、C14 对 DeepSeek 图片输入和并行工具的补丁必须先查 lite 是否存在相应基础行为，禁止扩张产品边界。
- Gemini 映射及 upstream_models.go 另一工作树上的未合并冲突始终保持原状。
- 本批 PR 须基于独立最终 SHA 经 CI 和 Security Scan 全通过，失败不得作为通过处理。
