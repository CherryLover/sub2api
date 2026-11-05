import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'
import type { OpsErrorDetail } from '@/api/admin/ops'

const { getUpstreamErrorDetail, getRequestErrorDetail, listRequestErrorUpstreamErrors } = vi.hoisted(() => ({
  getUpstreamErrorDetail: vi.fn(),
  getRequestErrorDetail: vi.fn(),
  listRequestErrorUpstreamErrors: vi.fn(),
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getUpstreamErrorDetail,
    getRequestErrorDetail,
    listRequestErrorUpstreamErrors,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

function detailWithSize(requestBodyBytes: number | null): OpsErrorDetail {
  return {
    id: 12,
    created_at: '2026-11-05T10:00:00Z',
    phase: 'upstream',
    type: 'upstream_error',
    error_owner: 'provider',
    error_source: 'upstream_http',
    severity: 'error',
    status_code: 502,
    platform: 'openai',
    model: 'gpt-5',
    resolved: false,
    resolved_at: null,
    resolved_by_user_id: null,
    resolved_by_user_name: '',
    client_request_id: 'client-12',
    request_id: 'request-12',
    message: 'Upstream request failed',
    user_id: null,
    user_email: '',
    api_key_id: null,
    account_id: null,
    account_name: '',
    group_id: null,
    group_name: '',
    client_ip: null,
    request_path: '/v1/responses',
    stream: false,
    inbound_endpoint: '/v1/responses',
    upstream_endpoint: '',
    requested_model: 'gpt-5',
    upstream_model: '',
    request_type: 1,
    user_agent: 'test',
    request_body_bytes: requestBodyBytes,
    error_body: '',
    is_business_limited: false,
  }
}

function mountModal() {
  return mount(OpsErrorDetailModal, {
    props: {
      show: true,
      errorId: 12,
      errorType: 'upstream',
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: true,
      },
    },
  })
}

describe('OpsErrorDetailModal request body size', () => {
  beforeEach(() => {
    getUpstreamErrorDetail.mockReset()
    getRequestErrorDetail.mockReset()
    listRequestErrorUpstreamErrors.mockReset()
  })

  it('formats a recorded request body size', async () => {
    getUpstreamErrorDetail.mockResolvedValue(detailWithSize(8995454))

    const wrapper = mountModal()
    await flushPromises()

    expect(getUpstreamErrorDetail).toHaveBeenCalledWith(12)
    expect(wrapper.find('[data-testid="detail-request-size"]').text()).toBe('8.6 MB')
  })

  it('shows a dash when the request body size was not recorded', async () => {
    getUpstreamErrorDetail.mockResolvedValue(detailWithSize(null))

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.find('[data-testid="detail-request-size"]').text()).toBe('—')
  })
})
