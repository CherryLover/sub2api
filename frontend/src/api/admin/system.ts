/**
 * System API endpoints for admin operations
 *
 * 应用内更新检查/在线升级/回滚已整套移除（内部部署由镜像或部署脚本升级），
 * 这里只剩版本号读取与服务重启。
 */

import { apiClient } from '../client'

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/system/restart')
  return data
}

export const systemAPI = {
  getVersion,
  restartService
}

export default systemAPI
