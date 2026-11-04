import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { listUpstreamErrorRequests, type OpsUpstreamErrorRequestParams } from '@/api/admin/ops'

const params: OpsUpstreamErrorRequestParams = {
  page: 2,
  page_size: 20,
  start_time: '2026-11-03T00:00:00Z',
  end_time: '2026-11-04T00:00:00Z',
  status_code: 503,
  api_key_id: 7,
  account_id: 9,
  outcome: 'recovered'
}

describe('upstream error request API', () => {
  beforeEach(() => get.mockReset())

  it('forwards all filters and cancellation signal to the dedicated endpoint', async () => {
    const response = { items: [], total: 21, page: 2, page_size: 20 }
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: response })
    await expect(listUpstreamErrorRequests(params, { signal: controller.signal })).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests', {
      params,
      signal: controller.signal
    })
  })

  it('works without request options', async () => {
    const response = { items: [], total: 0, page: 2, page_size: 20 }
    get.mockResolvedValueOnce({ data: response })
    await expect(listUpstreamErrorRequests(params)).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests', { params, signal: undefined })
  })

  it('propagates transport errors instead of returning a misleading empty result', async () => {
    const error = new Error('network failed')
    get.mockRejectedValueOnce(error)
    await expect(listUpstreamErrorRequests(params)).rejects.toBe(error)
  })
})
