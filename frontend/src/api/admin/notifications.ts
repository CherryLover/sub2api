import { apiClient } from '../client'

/**
 * Bark 推送通知（iOS）配置。
 *
 * 接口契约：/api/v1/admin/notifications/bark
 * - GET  返回当前配置，device_key 永远为空串，用 has_device_key 表示是否已配置
 * - PUT  保存配置，device_key 留空表示保留已存的
 * - POST /test 用请求体里的（未保存的）值做一次连通性测试并发一条测试通知
 */

export type BarkLevel = 'active' | 'timeSensitive' | 'passive' | 'critical'

export interface BarkNotifyConfig {
  enabled: boolean
  server_url: string
  /** 后端永远回空串，前端只用它承载「本次要写入的新 Key」 */
  device_key: string
  has_device_key: boolean
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
  /** 留空表示保留已存的设备 Key */
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

export interface TestBarkNotifyResponse {
  ok: boolean
  ping_ok: boolean
  status_code: number
  message: string
  latency_ms: number
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
}

export default notificationsAPI
