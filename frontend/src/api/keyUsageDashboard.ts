import { apiClient } from './client'

export type KeyUsageDashboardRange = 'today' | '7d' | '30d'
export type KeyUsageDashboardMetric = 'cost' | 'requests' | 'tokens'

export interface KeyUsageMetric {
  cost: number
  requests: number
  tokens: number
}

export interface KeyUsageDashboardParams {
  range: KeyUsageDashboardRange
  group_id?: number
  model?: string
}

export interface KeyUsageDashboardAlert {
  type: 'high_error_rate' | 'large_request' | 'usage_spike' | 'idle'
  api_key_id: number
  api_key_name: string
  value: number
  count: number
}

export interface KeyUsageDashboardKey {
  api_key_id: number
  name: string
  group_name: string
  status: string
  last_used_at: string | null
  totals: KeyUsageMetric
  cost_per_request: number
  cache_hit_rate: number
  avg_first_token_ms: number
  error_count: number
  error_rate: number
  peak_rpm: number
  hour_of_day: KeyUsageMetric[]
  series: KeyUsageMetric[]
}

export interface KeyUsageDashboardPeakHour {
  hour: number
  requests: number
}

export interface KeyUsageDashboardResponse {
  range: KeyUsageDashboardRange
  start: string
  end: string
  totals: KeyUsageMetric
  prev_totals: KeyUsageMetric
  active_keys: number
  total_keys: number
  avg_cost_per_request: number
  prev_avg_cost_per_request: number
  cache_hit_rate: number
  prev_cache_hit_rate: number
  error_rate: number
  prev_error_rate: number
  peak_hours: KeyUsageDashboardPeakHour[]
  alerts: KeyUsageDashboardAlert[]
  keys: KeyUsageDashboardKey[]
  series_labels: string[]
  week: KeyUsageMetric[][]
  models: {
    names: string[]
    keys: Array<{
      api_key_id: number
      cells: KeyUsageMetric[]
    }>
  }
}

export interface KeyUsageModelUsage {
  model: string
  metrics: KeyUsageMetric
}

export interface KeyUsageEffortUsage {
  effort: string
  requests: number
}

export interface KeyUsageDashboardDetail {
  api_key_id: number
  name: string
  group_name: string
  totals: KeyUsageMetric
  cache_hit_rate: number
  avg_first_token_ms: number
  error_rate: number
  peak_rpm: number
  large_request_count: number
  large_request_avg_bytes: number
  hour_of_day: KeyUsageMetric[]
  models: KeyUsageModelUsage[]
  efforts: KeyUsageEffortUsage[]
}

export async function getKeyUsageDashboard(
  params: KeyUsageDashboardParams,
  config: { signal?: AbortSignal } = {}
): Promise<KeyUsageDashboardResponse> {
  const { data } = await apiClient.get<KeyUsageDashboardResponse>('/usage/key-dashboard', {
    ...config,
    params
  })
  return data
}

export async function getKeyUsageDashboardDetail(
  apiKeyId: number,
  params: KeyUsageDashboardParams,
  config: { signal?: AbortSignal } = {}
): Promise<KeyUsageDashboardDetail> {
  const { data } = await apiClient.get<KeyUsageDashboardDetail>(
    `/usage/key-dashboard/keys/${apiKeyId}`,
    {
      ...config,
      params
    }
  )
  return data
}
