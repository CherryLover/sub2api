# Codex 图片直连与 JSON/SSE 兼容

## 使用范围与默认行为

本次补齐精简版缺少的 Codex 原生图片生成、编辑转发，实现上游 #8028 所需的直连基础。只有 **OpenAI OAuth 账号**明确启用直连且实际映射模型在支持列表内时才使用此路径。其他供应商、普通 API Key 以及未启用的 OAuth 账号保持原有行为；未启用时仍使用现有 Responses 图片桥接。

在现有账号管理 API 的 `extra` 中设置以下字段可按账号启用；请保留账号原有 `extra` 数据，不要用这一小段覆盖完整对象：

```json
{
  "codex_direct_images_enabled": true
}
```

这是账号级选择，不是全局强制切换。当前匹配的模型系列由 `usesCodexDirectImages` 明确列举，包括 `gpt-image-1.5`、`gpt-image-2`、`gpt-image-2.5-flare` 和 `gpt-image-2.5-sunburst` 及代码列出的日期版本；不把未知或任意前缀模型自动转到原生图片接口。

## 接口及行为

客户端继续使用原有 `/v1/images/generations`、`/v1/images/edits`，无需更换入站 URL。网关按选定账号发送到 Codex 原生 `/backend-api/codex/images/generations` 或 `/backend-api/codex/images/edits`，带上现有 OAuth 身份与账号相关请求头，不误带仅用于 Responses 的协议头。

生成请求支持已有模型、提示词、数量、尺寸、质量、背景、输出格式等参数；编辑请求把图片 URL、Base64 data URI、multipart 上传及遮罩转换成原生 Images 请求结构。实际转发模型由账号模型映射确定，计费用量保留该实际模型，而不是把客户端别名错误当成上游模型。

| 客户端请求 | 上游响应 | 本次处理 |
|---|---|---|
| 非流式 | JSON | 校验整个图片数组，返回兼容 Images JSON，保留用量和实际图片尺寸 |
| `stream: true` | 原生 SSE | 复用现有 Images 流处理、错误判断和用量提取 |
| `stream: true` | `application/json` | 先完整读取和校验 JSON，再输出合法 SSE 完成事件 |

JSON→SSE 转换分别使用 `image_generation.completed`、`image_edit.completed`。多张图片分别产生完成事件；JSON 中的换行不会破坏单条 SSE 的 `data:` 帧。图片数据为空、格式错误、Base64 错误、数组中混入无效条目或响应超过配置上限时，在输出第一条成功 SSE 之前拒绝该响应，避免一半成功、一半错误。

`response_format: url` 沿用精简版已有的图片 data URL 兼容处理，不承诺替用户上传图片到外部对象存储。

## 计费与故障边界

沿用现有图片计费管线，记录输入、输出、缓存读取 token 和实际图片数、尺寸。在 Images 端点没有单独输出分类时，将输出 token 归入图片输出；缓存明细只使用上游明确给出的信息，不凭总缓存量猜测图文占比。

上游已经生成了图片后，客户端断开连接不会把该次上游用量自动归零，也不会因为断开而再次生成同一批图片。未写出有效结果前的上游失败走已有错误和失败转移机制；已写出图片的情况保留已发生用量，不通过反复重试掩盖部分完成结果。

关闭账号的 `codex_direct_images_enabled` 即恢复原有 Responses 桥接路径，不修改任何生产发布或镜像设置。

## 验证入口

`openai_images_direct_protocol_test.go` 覆盖生成/编辑、JSON/流式请求、模型映射、完整数组验证、响应体大小限制和客户端断开后的用量保留；原有 OAuth Images 与 API Key Images 回归继续保留。审核时重点检查 JSON 首字节前验证、模型映射后的计费字段、SSE 完成事件类型以及账号级开关默认关闭。
