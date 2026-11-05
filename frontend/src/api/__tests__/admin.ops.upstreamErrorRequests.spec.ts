import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import {
  getUpstreamErrorRequestsSummary,
  getUpstreamErrorRequestsTrend,
  listUpstreamErrorRequests,
  type OpsUpstreamErrorFilterParams,
  type OpsUpstreamErrorRequestParams,
  type OpsUpstreamErrorTrendParams
} from '@/api/admin/ops'

const params: OpsUpstreamErrorRequestParams = {
  page: 2,
  page_size: 20,
  start_time: '2026-11-03T00:00:00Z',
  end_time: '2026-11-04T00:00:00Z',
  status_code: 529,
  final_status_code: 503,
  api_key_id: 7,
  account_id: 9,
  outcome: 'recovered'
}

const filterParams: OpsUpstreamErrorFilterParams = {
  start_time: params.start_time,
  end_time: params.end_time,
  status_code: params.status_code,
  final_status_code: params.final_status_code,
  api_key_id: params.api_key_id,
  account_id: params.account_id,
  outcome: params.outcome
}

const trendParams: OpsUpstreamErrorTrendParams = {
  status_code: 400,
  final_status_code: 502,
  api_key_id: 7,
  account_id: 9,
  outcome: 'failed',
  days: 30
}

describe('upstream error request API', () => {
  beforeEach(() => get.mockReset())

  it('forwards all filters and cancellation signal to the request list endpoint', async () => {
    const response = { items: [], total: 21, page: 2, page_size: 20 }
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: response })
    await expect(listUpstreamErrorRequests(params, { signal: controller.signal })).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests', {
      params,
      signal: controller.signal
    })
  })

  it('forwards filters and cancellation signal to the summary endpoint', async () => {
    const response = {
      total: 3,
      recovered: 1,
      failed: 2,
      attempts: 5,
      failed_by_status: [{ status_code: 503, count: 2 }]
    }
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: response })

    await expect(getUpstreamErrorRequestsSummary(filterParams, { signal: controller.signal })).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests/summary', {
      params: filterParams,
      signal: controller.signal
    })
  })

  it('forwards non-time filters and days to the trend endpoint', async () => {
    const response = {
      days: [{ date: '2026-11-05', total: 2, recovered: 1, failed: 1 }]
    }
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: response })

    await expect(getUpstreamErrorRequestsTrend(trendParams, { signal: controller.signal })).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests/trend', {
      params: trendParams,
      signal: controller.signal
    })
  })

  it('works without request options', async () => {
    const response = { items: [], total: 0, page: 2, page_size: 20 }
    get.mockResolvedValueOnce({ data: response })
    await expect(listUpstreamErrorRequests(params)).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/ops/upstream-error-requests', { params, signal: undefined })

    get.mockResolvedValueOnce({
      data: { total: 0, recovered: 0, failed: 0, attempts: 0, failed_by_status: [] }
    })
    await getUpstreamErrorRequestsSummary(filterParams)
    expect(get).toHaveBeenLastCalledWith('/admin/ops/upstream-error-requests/summary', {
      params: filterParams,
      signal: undefined
    })
  })

  it('propagates transport errors instead of returning a misleading empty result', async () => {
    const error = new Error('network failed')
    get.mockRejectedValueOnce(error)
    await expect(listUpstreamErrorRequests(params)).rejects.toBe(error)
  })
})
