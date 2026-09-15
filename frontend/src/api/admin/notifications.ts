import { apiClient } from '../client'

/**
 * Bark 推送通知（iOS）配置。
 *
 * 接口契约：/api/v1/admin/notifications/bark
 * - GET  返回当前配置，device_key 永远为空串，用 has_device_key / device_key_count 表示已配置几个设备
 * - PUT  保存配置，device_key 留空表示保留已存的
 * - POST /test 用请求体里的（未保存的）值做一次连通性测试并发一条测试通知
 *
 * device_key 支持逗号分隔的多个设备，一条通知会同时推给所有人。字段本身仍是一个字符串，
 * 后端整串加密存一个值，所以老的单 Key 配置天然兼容。
 */

export type BarkLevel = 'active' | 'timeSensitive' | 'passive' | 'critical'

export interface BarkNotifyConfig {
  enabled: boolean
  server_url: string
  /** 后端永远回空串，前端只用它承载「本次要写入的新 Key」 */
  device_key: string
  has_device_key: boolean
  /** 已配置的设备数量；加密密钥换过导致解不开时会是 0（此时 has_device_key 仍为 true） */
  device_key_count: number
  group: string
  level: BarkLevel
  sound: string
  click_url: string
  notify_on_resolve: boolean
  updated_at?: string
}

export interface UpdateBarkNotifyConfigRequest {
  enabled: boolean
  server_url: string
  /** 留空表示保留已存的设备 Key；多个设备用逗号分隔 */
  device_key: string
  group: string
  level: BarkLevel
  sound: string
  click_url: string
  notify_on_resolve: boolean
}

export interface TestBarkNotifyRequest extends UpdateBarkNotifyConfigRequest {
  title?: string
  body?: string
}

/** 单个设备的推送结果。masked_key 是打码片段，后端绝不回显完整 Key。 */
export interface BarkDevicePushOutcome {
  index: number
  masked_key: string
  ok: boolean
  status_code: number
  message: string
  latency_ms: number
}

export interface TestBarkNotifyResponse {
  /** 只要有一个设备收到就是 true；全部失败时接口直接返回 502 */
  ok: boolean
  ping_ok: boolean
  /** 取第一个成功设备的回执 */
  status_code: number
  message: string
  latency_ms: number
  device_count?: number
  success_count?: number
  failure_count?: number
  devices?: BarkDevicePushOutcome[]
}

/**
 * 定时报错汇总推送配置。
 *
 * 接口契约：/api/v1/admin/notifications/error-digest
 * - GET  返回当前配置（从未保存过时是后端默认值）
 * - PUT  保存配置，保存成功后后端立刻按新的 cron 表达式重建定时任务
 * - POST /test 立刻按最近 24 小时算一份并推送，不看 enabled，也不看 skip_when_empty
 *
 * 推送走的是上面那条 Bark 通道；Bark 没启用时试推不报错，只会回 pushed=false。
 */
export interface ErrorDigestConfig {
  enabled: boolean
  /** 五字段 cron 表达式，时区跟随后端配置 */
  schedule: string
  /** 区间内没有报错时是否跳过推送 */
  skip_when_empty: boolean
  /** 正文里展开几个密钥，其余折成「另有 N 个密钥共 M 次」 */
  top_keys: number
  updated_at?: string
}

export interface UpdateErrorDigestConfigRequest {
  enabled: boolean
  schedule: string
  skip_when_empty: boolean
  top_keys: number
}

/** 一个密钥下的一种错误：标签已经是「上游过载(529)」这种人话形式 */
export interface ErrorDigestTypeLine {
  label: string
  count: number
}

export interface ErrorDigestGroup {
  title: string
  total: number
  types: ErrorDigestTypeLine[]
}

export interface ErrorDigestSummary {
  start: string
  end: string
  total: number
  /** 真故障（is_business_limited=false） */
  sla: number
  /** 业务拦截（余额不足、额度用尽之类） */
  limited: number
  groups: ErrorDigestGroup[]
  hidden_groups: number
  hidden_total: number
}

export interface TestErrorDigestResponse {
  /** Bark 没启用时是 false，reason 会是 bark_disabled */
  pushed: boolean
  reason?: string
  title: string
  body: string
  summary: ErrorDigestSummary | null
}

export async function getErrorDigestConfig(): Promise<ErrorDigestConfig> {
  const { data } = await apiClient.get<ErrorDigestConfig>('/admin/notifications/error-digest')
  return data
}

export async function updateErrorDigestConfig(
  req: UpdateErrorDigestConfigRequest,
): Promise<ErrorDigestConfig> {
  const { data } = await apiClient.put<ErrorDigestConfig>(
    '/admin/notifications/error-digest',
    req,
  )
  return data
}

export async function testErrorDigest(): Promise<TestErrorDigestResponse> {
  const { data } = await apiClient.post<TestErrorDigestResponse>(
    '/admin/notifications/error-digest/test',
  )
  return data
}

export async function getBarkConfig(): Promise<BarkNotifyConfig> {
  const { data } = await apiClient.get<BarkNotifyConfig>('/admin/notifications/bark')
  return data
}

export async function updateBarkConfig(
  req: UpdateBarkNotifyConfigRequest,
): Promise<BarkNotifyConfig> {
  const { data } = await apiClient.put<BarkNotifyConfig>('/admin/notifications/bark', req)
  return data
}

export async function testBark(req: TestBarkNotifyRequest): Promise<TestBarkNotifyResponse> {
  const { data } = await apiClient.post<TestBarkNotifyResponse>(
    '/admin/notifications/bark/test',
    req,
  )
  return data
}

export const notificationsAPI = {
  getBarkConfig,
  updateBarkConfig,
  testBark,
  getErrorDigestConfig,
  updateErrorDigestConfig,
  testErrorDigest,
}

export default notificationsAPI
