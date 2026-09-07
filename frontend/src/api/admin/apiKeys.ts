/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { ApiKey, PaginatedResponse } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

/**
 * 管理端密钥总表的筛选条件。
 * group_id 传 0 表示「无分组」；status 取 active/inactive/quota_exhausted/expired；
 * search 按名称/密钥模糊匹配；sort_by 白名单与用户侧 /keys 相同（含 today_cost、last_used_at）。
 */
export interface AdminApiKeyListFilters {
  user_id?: number
  group_id?: number | string
  status?: string
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

/**
 * 管理员更新单个密钥：字段不传则不改，IP 名单传空数组表示清空。
 * status 只接受 active / inactive。
 */
export interface AdminUpdateApiKeyRequest {
  group_id?: number // 0=解绑，>0=绑定，不传=不改
  reset_rate_limit_usage?: boolean
  status?: 'active' | 'inactive'
  ip_whitelist?: string[]
  ip_blacklist?: string[]
}

/**
 * List all API keys across users (admin only).
 * GET /admin/api-keys — 每行 key 为服务端掩码，user / group 已填充。
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: AdminApiKeyListFilters,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>('/admin/api-keys', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

/**
 * Update an API key's admin-managed fields (group / status / IP rules).
 * PUT /admin/api-keys/:id
 */
export async function update(id: number, payload: AdminUpdateApiKeyRequest): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, payload)
  return data
}

/**
 * Delete an API key (admin only).
 * DELETE /admin/api-keys/:id
 */
export async function remove(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/api-keys/${id}`)
  return data
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  return update(id, { group_id: groupId === null ? 0 : groupId })
}

export const apiKeysAPI = {
  list,
  update,
  remove,
  updateApiKeyGroup
}

export default apiKeysAPI
