# Gemini 显式上下文缓存（精简版）

## 本次增加的能力

在原生 Gemini `/v1beta` 入口提供缓存创建、查询、分页列表、续期和删除。缓存元数据、TTL、调用者 API Key、用户、分组、持有上游账号和创建时的渠道模型映射均持久化到 PostgreSQL。实际内容仍保存在上游，不在本地数据库另存一份提示词正文。

| 方法 | 路径 | 行为 |
|---|---|---|
| POST | `/v1beta/cachedContents` | 创建上游缓存，确认计费及本地绑定事务成功后返回公开资源名 |
| GET | `/v1beta/cachedContents` | 只列出当前 Key、当前分组中仍有效的缓存；支持 `pageSize`、`pageToken` |
| GET | `/v1beta/cachedContents/{id}` | 查询本地持久化的公开元数据，不返回上游私有 ID 或缓存正文 |
| PATCH | `/v1beta/cachedContents/{id}` | 只允许修改 `ttl` 或 `expireTime`；支持对应的 `updateMask` |
| DELETE | `/v1beta/cachedContents/{id}` | 删除上游资源并软删除本地绑定，不另收请求费 |

这些路由沿用原有 API Key 鉴权、IP 限制、用户和分组状态校验、请求体大小限制。创建中的提示内容在任何上游调用、并发占用和计费之前经过现有安全审计。

## 支持的账号和配置

支持官方 AI Studio API Key、Vertex Service Account JSON，以及管理员显式声明支持同一缓存协议的第三方 Gemini API Key 网关。Gemini CLI / Code Assist OAuth、Antigravity、Vertex API-key-only 模式不作为缓存持有者。本实现不恢复精简版已经移除的用户余额、订阅和外围运营模块。

原生缓存入口要求 Gemini 分组；不开放强制平台别名或 composite 分组的跨平台缓存路由。

```yaml
gateway:
  gemini_cached_content_max_ttl_seconds: 86400
```

未指定 TTL 时默认一小时，全局默认最大 TTL 为 24 小时。第三方网关可在现有账号管理 API 的 `credentials` 中添加以下字段；更新时须保留账号已有凭据，不可用该片段覆盖整份凭据：

```json
{
  "explicit_cache_upstream": true,
  "explicit_cache_upstream_max_ttl_seconds": 86400
}
```

只有确实提供 `cachedContents` 接口、且能维持缓存账号绑定语义的网关才应启用。自定义 `base_url` 仍受原有上游 URL 安全策略限制。有效 TTL 上限取全局与该账号上限的较小值。

## 调用示例

先准备 `cache-create.json`。其中内容量及模型需要符合实际上游的缓存要求；过短、不可缓存或不支持的模型会被上游拒绝。

```json
{
  "model": "models/gemini-2.5-pro",
  "displayName": "technical-reference",
  "ttl": "3600s",
  "contents": [
    {"role": "user", "parts": [{"text": "替换为实际需要缓存的完整参考材料"}]}
  ]
}
```

使用自己的网关地址和 Key：

```sh
export SUB2API_URL='https://your-sub2api.example'
# 在本地安全设置 SUB2API_KEY；不要把真实 Key 写进版本库或共享日志。

curl --fail-with-body -sS \
  -H "x-goog-api-key: $SUB2API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @cache-create.json \
  "$SUB2API_URL/v1beta/cachedContents"
```

创建返回 `name: "cachedContents/<公开ID>"`。随后引用该名称，而不是上游的缓存 ID：

```sh
export CACHE_NAME='cachedContents/替换为创建返回的公开ID'

curl --fail-with-body -sS \
  -H "x-goog-api-key: $SUB2API_KEY" \
  -H 'Content-Type: application/json' \
  --data "{\"cachedContent\":\"$CACHE_NAME\",\"contents\":[{\"role\":\"user\",\"parts\":[{\"text\":\"请总结参考材料\"}]}]}" \
  "$SUB2API_URL/v1beta/models/gemini-2.5-pro:generateContent"

curl --fail-with-body -sS -H "x-goog-api-key: $SUB2API_KEY" \
  "$SUB2API_URL/v1beta/cachedContents?pageSize=20"

curl --fail-with-body -sS -H "x-goog-api-key: $SUB2API_KEY" \
  "$SUB2API_URL/v1beta/$CACHE_NAME"

curl --fail-with-body -sS -X PATCH \
  -H "x-goog-api-key: $SUB2API_KEY" -H 'Content-Type: application/json' \
  --data '{"ttl":"7200s"}' \
  "$SUB2API_URL/v1beta/$CACHE_NAME?updateMask=ttl"

curl --fail-with-body -sS -X DELETE -H "x-goog-api-key: $SUB2API_KEY" \
  "$SUB2API_URL/v1beta/$CACHE_NAME"
```

`ttl` 表示从当前时刻起的剩余有效期，**不是**在旧到期时间上直接累加。`streamGenerateContent` 和 `countTokens` 的缓存引用也使用相同绑定。原生 `countTokens` 的嵌套 `generateContentRequest.cachedContent` 亦受验证。

## 隔离与调度边界

同一用户的另一个 API Key 也不能读取、续期、删除或引用原 Key 的缓存。不存在、过期和无权访问统一返回不可区分的缓存错误，避免资源枚举。查询与列表只返回网关公开 ID、模型、标签、token 数和时间信息，不暴露上游资源名或账号凭据。

引用缓存的生成请求只允许选择创建它的那个上游账号，仍执行当前账号可用性、并发和利润策略检查。账号不可用、退出分组、凭据模式不再支持缓存或模型映射改变时，不会把请求改发到另一个账号。缓存上游失败时，`countTokens` 不用不含缓存正文的本地估算冒充成功。

读取和删除已经创建的缓存，允许额度耗尽但其他鉴权条件仍有效的 Key 操作；创建和续期仍受额度限制。过期 Key、停用 Key、停用用户和停用分组不会获得豁免。

## 计费与一致性

创建费用包括输入请求的现有计费规则及缓存存储费。存储量按 `token 数 × 存储小时数` 计算；模型价格表可通过 `cache_storage_cost_per_token_per_hour` 配置 USD/token/hour 单价，没有显式价格时按回移的 Gemini 系列内置价格兜底。无法确定存储价格时拒绝创建或续期，不静默提供免费存储。

续期仅为超过本地原到期时间的增量存储收费，不再次收取输入 token 或按次请求费。缩短 TTL 或提前删除不退还已经结算的存储费。费用沿用精简版现有分组/用户倍率和 API Key 额度账本；Simple Mode 保持原有计费豁免及可选 Key 消费窗口行为。

缓存绑定或到期时间、Key/账号计费、去重记录及完整 usage log 在同一个 PostgreSQL 事务中提交。事务锁定最新 Key 行，重新核对所属用户/分组、状态、有效期、总额度和 5h/1d/7d 消费窗口；并发创建或续期不能一起越过这些 Key 限制。用户×平台额度的预检查和衍生计数沿用精简版已有路径，不宣称改变全站普通请求的并发额度语义。

每次创建得到一个新资源，不把客户端反复调用 POST 当作同一请求。对于同一资源的同一次结算，服务端 `gcache:create:` / `gcache:patch:` 金额事件 ID 保证不重复扣款；客户端随意复用 `X-Request-ID` 不会吞掉其他资源的费用。

上游 HTTP 调用和本地数据库无法构成跨系统事务。创建后的结算失败会尝试删除上游资源；发生提交结果不明时先查本地绑定，避免删掉已经付费成功的缓存。续期使用持久化修改租约，失败或进程退出后由恢复任务回滚到已结算的旧到期时间；不能把未结算的延长立即提供给客户端。未知上游创建结果不会自动换号重建；无法确认的孤立上游资源最终受请求 TTL 限制，运营方可结合日志核查。

## 迁移与审核

新增 `244_gemini_cached_contents.sql`，只新增缓存元数据表、归属索引和修改租约字段，不更改原有业务表结构。降级时可保留此表等待资源过期，不建议在仍有活跃缓存或待恢复操作时直接删表。

重点审核入口：`gemini_cached_contents_handler.go`、`gemini_cache_billing.go`、`gemini_cache_settlement.go`、`gemini_cached_content.go`。对应单元测试覆盖权限、引用改写、参数、价格、错误响应和审计顺序；真实 PostgreSQL 集成测试覆盖原子结算、重复扣款、并发超支、窗口到期、失败回滚及最新 Key 状态。
