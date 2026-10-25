# 第三批独立回移台账 (2026-10-25)

- 基线：CherryLover/sub2api main 6af365be55481a5a14d86222eefc8e742b70d738 (v0.1.206)
- 独立开发分支：backport/2026-10-25-protocol-independent，父提交为 main，不依赖 PR #32/#33 的代码、提交或合并时间。
- 原堆叠第三批 SHA e551dbc03be3d21c40a9aa80de3847246c74d0f4 已留作历史备份，仅提取第三批相对第二批的代码净增量。
- 安全依赖基线：Go 1.26.10，DOMPurify 3.4.17，follow-redirects 1.16.1（独立执行漏洞扫描的自包含环境要求；其它批次可任意顺序合并）。
- 未经完整验收不提前创建正式第三批 PR；PR 一旦提交只指向目标 main，无 stacked base。

## 候选状态

| 编号 | 上游来源 | 变更说明 | 原实施状态 | 独立分支代码状态 |
|---|---|---|---|---|
| C01 | [#8114](https://github.com/Wei-Shaw/sub2api/pull/8114) | Gemini thinking 努力等级和摘要输出 | 待核验 | 已实现：Gemini 2.5 thinkingBudget、3+ thinkingLevel、流式思考输出；保留精简版音频转换；定向单测通过，待完整 CI |
| C02 | [#8188](https://github.com/Wei-Shaw/sub2api/pull/8188) | Schema const/examples 清理 | 已适配 const/examples 及 schema 关键字名称保留；定向测试通过，待CI | 已提取，待全量验证 |
| C03 | [#8224](https://github.com/Wei-Shaw/sub2api/pull/8224) | DeepSeek 图片输入去重 | 待基础能力：当前 lite 不含上游 DeepSeek 图片归一化转换；不能直接套去重补丁 | 已实现：DeepSeek Responses 去除 input_image.url 冗余别名，保留 image_url，其他平台不改；定向单测通过，待完整 CI |
| C04 | [#8167](https://github.com/Wei-Shaw/sub2api/pull/8167) | Gemini 生图沿用分组开关 | 待核验 | 已实现：Gemini / Antigravity / Composite 生图分组门禁、响应模态识别、入口/转发双重校验；定向单测通过，待完整 CI |
| C05 | [#8140](https://github.com/Wei-Shaw/sub2api/pull/8140) | Responses created_at 补齐 | 已补常规和 passthrough Responses 的缺失 created_at；定向测试通过，待CI | 已提取，待全量验证 |
| C06 | [#7077](https://github.com/Wei-Shaw/sub2api/pull/7077) | Antigravity prefixItems/tuple Schema | 已适配 prefixItems/tuple 清理；定向测试通过，待CI | 已提取，待全量验证 |
| C07 | [#7272](https://github.com/Wei-Shaw/sub2api/pull/7272) | function_call_arguments.done 参数一致 | 已回移工具参数 done 完整值；定向测试通过，待CI | 已提取，待全量验证 |
| C08 | [#7785](https://github.com/Wei-Shaw/sub2api/pull/7785) | Chat named tool_choice 转 Responses | 已适配 Chat 指定函数 tool_choice 展平；定向测试通过，待CI | 已提取，待全量验证 |
| C09 | [#7786](https://github.com/Wei-Shaw/sub2api/pull/7786) | Chat developer 角色保持 | 已回移 developer 角色保持；定向测试通过，待CI | 已提取，待全量验证 |
| C10 | [#7787](https://github.com/Wei-Shaw/sub2api/pull/7787) | 传统 function_call 与结果 ID 配对 | 已适配旧 function_call / function 结果稳定 ID 配对，保留音频转换；定向测试通过，待CI | 已提取，待全量验证 |
| C11 | [#7788](https://github.com/Wei-Shaw/sub2api/pull/7788) | Responses refusal 到 Chat 转换 | 已适配 refusal 非流式和流式转换；定向测试通过，待CI | 已提取，待全量验证 |
| C12 | [#7789](https://github.com/Wei-Shaw/sub2api/pull/7789) | Anthropic 聚合流负索引 panic | 已适配四条 Anthropic 缓冲流负索引防 panic；定向测试通过，待CI | 已提取，待全量验证 |
| C13 | [#6925](https://github.com/Wei-Shaw/sub2api/pull/6925) | done 和终态消息恢复文本 | 待核验 | 已实现：output_text.done 和终态文本恢复，避免增量文本重复输出；定向单测通过，待完整 CI |
| C14 | [#7246](https://github.com/Wei-Shaw/sub2api/pull/7246) | DeepSeek 并行工具图片与顺序 | 待核验 | 已实现：DeepSeek Responses 工具返回图片提升为后续 user 图文，保留并行工具输出顺序；定向单测通过，待完整 CI |
| C15 | [#7635](https://github.com/Wei-Shaw/sub2api/pull/7635) | Chat to Responses 类型标记 | 已回移 Chat 转 Responses message.type；定向测试通过，待CI | 已提取，待全量验证 |
| C16 | [#7854](https://github.com/Wei-Shaw/sub2api/pull/7854) | Chat 兼容流 SSE ping flush | 已回移 Chat SSE ping 注释转发和 flush；定向测试通过，待CI | 已提取，待全量验证 |
| C17 | [#8080](https://github.com/Wei-Shaw/sub2api/pull/8080) | OpenAI API Key Messages web_search 模拟 | 待核验 | 已实现：仅 OpenAI API Key Messages 模拟 web_search，OAuth 原生检索保留，复用配置开关；定向单测通过，待完整 CI |
| C18 | [#7094](https://github.com/Wei-Shaw/sub2api/pull/7094) | Responses to Chat system/developer 顺序 | 待核验 | 有限回移：只合并会话开头连续的 system/developer；会话中途高级指令不降级为 user；定向单测通过，待完整 CI |

## 当前保留边界

- C01/C03/C04/C13/C14/C17 已按精简版保留入口适配；C18 采用只合并开头高级指令的安全子集。不得为兼容上游而将中途 developer/system 静默降级为 user。
- C03/C14 仅落在已有 DeepSeek 官方 Responses 请求变换：图片字段去重、工具输出图片提升为用户内容及并行调用顺序保持；非 DeepSeek 路径不变。
- Gemini 映射及 upstream_models.go 另一工作树上的未合并冲突始终保持原状。
- 本批 PR 须基于独立最终 SHA 经 CI 和 Security Scan 全通过，失败不得作为通过处理。


## 独立基线首次 CI 修补记录

- SHA 75f0dae65：基于 lite/main 的第三批无前置依赖版本已推送；对应 fork CI #40182912990：lint 报 8 个 errcheck，完整 Go 单测复现 TestOpenAIStreamingPassthroughScannerErrorFlushesWrittenResidual 失败。Security Scan #40182913073 前后端成功。
- 本次修复：测试用例仅输出 output_text.delta，没有 response 生命周期对象，因此不能断言 created_at 必定存在；保留其原始输出、scan error 和 flush 行为检查。补齐 schema 类型断言的 comma-ok、strings.Builder 的错误值处理，以符合现有 lint。
- 修改后定向 Go 单测 (internal/service, internal/pkg/antigravity) 全部通过。独立第三批整体尚未完成，不提交 PR，不擅自合并。


## 第三批核心补齐（2026-10-25）

- 656a216c7（C04）：现有 Gemini/Antigravity/Composite 生图开关禁用时拒绝上游出图模型或 IMAGE 模态，countTokens/纯文本不拦；保留精简版服务构造接口，定向通过。
- f8e3a8724（C17）：复用 websearch.Manager 搜索模拟，允许显式开启的 OpenAI API Key Messages 使用，OAuth 原生搜索不动；定向通过。
- bd70e449b（C13/C01）：C13 恢复只存在于 done 或终态的文本、不重复已流出 delta；C01 Gemini 2.5 与 3+ 思考参数、流式 thinking、历史块清理，保留精简版音频入口；定向通过。
- 37e0f8dac（C14/C03）：DeepSeek 原生 Responses 将工具图片整理到 user 图文，保持工具批次顺序；input_image 只留 image_url。非 DeepSeek 不变，定向测试通过。
- a52fafae1（C18）：只合并会话开头连续 system/developer 指令。上游 #7094 对会话中途高级指令静默降成 user、会改变指令优先级，精简版明确不采纳。其它部分无需等待本子项决策。
- 各批 PR 独立基于精简版 main，完整 CI/Security Scan 须验证本批最终 SHA；不能借用旧批检查。
