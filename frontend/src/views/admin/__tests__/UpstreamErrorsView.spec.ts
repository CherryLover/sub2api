import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import UpstreamErrorsView from '../UpstreamErrorsView.vue'
import type { OpsUpstreamErrorRequest, OpsUpstreamErrorRequestsSummary } from '@/api/admin/ops'

const { list, summary } = vi.hoisted(() => ({
  list: vi.fn(),
  summary: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  listUpstreamErrorRequests: list,
  getUpstreamErrorRequestsSummary: summary,
  getUpstreamErrorRequestsTrend: vi.fn(),
  default: {}
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const DataTableStub = defineComponent({
  props: ['columns', 'data', 'loading'],
  template: `<div><div v-for="row in data" :key="row.id" data-testid="row"><div v-for="col in columns" :key="col.key"><slot :name="'cell-' + col.key" :row="row">{{ row[col.key] }}</slot></div></div></div>`
})

const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: ['show', 'title', 'width', 'closeOnClickOutside'],
  emits: ['close'],
  template: '<section v-if="show" data-testid="detail"><slot /></section>'
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  props: ['page', 'pageSize', 'total'],
  emits: ['update:page', 'update:pageSize'],
  template: '<div />'
})

const recovered: OpsUpstreamErrorRequest = {
  id: 1,
  created_at: '2026-11-04T01:00:00Z',
  client_request_id: 'request-1',
  request_id: 'upstream-1',
  api_key_id: 8,
  api_key_name: 'Production',
  account_id: 2,
  account_name: 'Failed upstream',
  model: 'gpt-test',
  reasoning_effort: 'low',
  request_body_bytes: 1536,
  client_status_code: 200,
  outcome: 'recovered',
  final: { account_id: 3, account_name: 'Successful upstream' },
  attempts: [
    {
      account_id: 2,
      account_name: 'Failed upstream',
      upstream_status_code: 502,
      message: 'Concrete 502 diagnostic',
      detail: '{"error":{"message":"bad"}}',
      kind: 'http_error'
    },
    {
      account_id: 2,
      upstream_status_code: 503,
      message: 'retry exhausted',
      kind: 'retry_exhausted'
    }
  ]
}

const summaryResponse: OpsUpstreamErrorRequestsSummary = {
  total: 30,
  recovered: 18,
  failed: 12,
  attempts: 55,
  failed_by_status: [
    { status_code: 502, count: 7 },
    { status_code: 503, count: 5 }
  ]
}

function render() {
  return mount(UpstreamErrorsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        DataTable: DataTableStub,
        Select: SelectStub,
        BaseDialog: BaseDialogStub,
        Pagination: PaginationStub,
        UpstreamErrorTrendChart: true
      }
    }
  })
}

beforeEach(() => {
  list.mockReset().mockResolvedValue({ items: [recovered], total: 30, page: 1, page_size: 20 })
  summary.mockReset().mockResolvedValue(summaryResponse)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) }
  })
})

describe('UpstreamErrorsView', () => {
  it('loads the latest 24h and renders summary, model tags, accounts and status codes', async () => {
    const wrapper = render()
    await flushPromises()

    const params = list.mock.calls[0]![0]
    expect(params).toMatchObject({ page: 1, page_size: 20 })
    expect(new Date(params.end_time).getTime() - new Date(params.start_time).getTime()).toBe(86_400_000)
    expect(wrapper.get('[data-testid="summary-total"]').text()).toContain('30')
    expect(wrapper.get('[data-testid="summary-recovered"]').text()).toContain('18')
    expect(wrapper.get('[data-testid="summary-failed"]').text()).toContain('12')
    expect(wrapper.get('[data-testid="model-reasoning-effort"]').text()).toBe('low')
    expect(wrapper.get('[data-testid="model-request-size"]').text()).toBe('1.5 KB')
    expect(wrapper.text()).toContain('Production #8')
    expect(wrapper.text()).toContain('Failed upstream #2')
    expect(wrapper.text()).toContain('Successful upstream #3')
    expect(wrapper.text()).toContain('502 / 503')
    wrapper.unmount()
  })

  it('queries immediately when the time capsule changes and resets to page one', async () => {
    const wrapper = render()
    await flushPromises()
    wrapper.findComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    expect(list.mock.calls.at(-1)![0].page).toBe(2)

    await wrapper.get('[data-testid="time-range-15m"]').trigger('click')
    await flushPromises()
    const params = list.mock.calls.at(-1)![0]
    expect(params.page).toBe(1)
    expect(new Date(params.end_time).getTime() - new Date(params.start_time).getTime()).toBe(15 * 60_000)
    expect(summary.mock.calls.at(-1)![0].start_time).toBe(params.start_time)
    wrapper.unmount()
  })

  it('queries immediately when status or outcome selects change', async () => {
    const wrapper = render()
    await flushPromises()
    const selects = wrapper.findAll('select')

    await selects[0]!.setValue('429')
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, status_code: 429 })

    await selects[1]!.setValue('failed')
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, outcome: 'failed' })
    wrapper.unmount()
  })

  it('debounces numeric inputs for 400ms and applies on Enter immediately', async () => {
    vi.useFakeTimers()
    const wrapper = render()
    await flushPromises()
    const initialCalls = list.mock.calls.length

    await wrapper.get('[data-testid="key-id-filter"]').setValue('8')
    vi.advanceTimersByTime(399)
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(initialCalls)

    vi.advanceTimersByTime(1)
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, api_key_id: 8 })

    await wrapper.get('[data-testid="account-id-filter"]').setValue('2')
    await wrapper.get('[data-testid="account-id-filter"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, account_id: 2 })
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('renders failed status capsules and applies final status plus failed outcome', async () => {
    const wrapper = render()
    await flushPromises()
    const capsule = wrapper.get('[data-testid="failed-status-502"]')
    expect(capsule.text()).toBe('502 7')

    await capsule.trigger('click')
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({
      page: 1,
      final_status_code: 502,
      outcome: 'failed'
    })
    expect(wrapper.text()).toContain('admin.upstreamErrors.finalStatusFilter')
    expect(wrapper.get('[data-testid="clear-final-status"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('opens diagnostics with outside-close enabled and switches formatted/raw content', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.upstreamErrors.details')!.trigger('click')

    const dialog = wrapper.findComponent(BaseDialogStub)
    expect(dialog.props('closeOnClickOutside')).toBe(true)
    const detail = wrapper.get('[data-testid="detail"]')
    expect(detail.text()).toContain('gpt-test')
    expect(detail.text()).toContain('low')
    expect(detail.text()).toContain('1.5 KB')

    const content = wrapper.get('[data-testid="content-attempt-0-detail"]')
    expect(content.text()).toContain('\n  "error"')
    await wrapper.get('[data-testid="format-mode-raw"]').trigger('click')
    expect(content.text()).toBe('{"error":{"message":"bad"}}')
    wrapper.unmount()
  })

  it('copies the currently displayed content and shows temporary success text', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.upstreamErrors.details')!.trigger('click')
    await wrapper.get('[data-testid="copy-attempt-0-detail"]').trigger('click')
    await flushPromises()

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('{\n  "error": {\n    "message": "bad"\n  }\n}')
    expect(wrapper.get('[data-testid="copy-attempt-0-detail"]').text()).toContain('admin.upstreamErrors.copied')
    wrapper.unmount()
  })

  it('shows the truncation hint when a block is exactly 2048 bytes', async () => {
    list.mockResolvedValue({
      items: [{ ...recovered, attempts: [{ ...recovered.attempts[0], detail: 'x'.repeat(2048) }] }],
      total: 1
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.upstreamErrors.details')!.trigger('click')

    expect(wrapper.get('[data-testid="truncated-hint"]').text()).toBe('admin.upstreamErrors.truncated')
    wrapper.unmount()
  })

  it('shows a retryable load error rather than stale data', async () => {
    list.mockRejectedValue(new Error('monitoring disabled'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.upstreamErrors.loadFailed')

    list.mockResolvedValue({ items: [recovered], total: 1, page: 1, page_size: 20 })
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Production #8')
    wrapper.unmount()
  })

  it('ignores stale list responses when a newer filter request finishes first', async () => {
    let resolveOld!: (value: unknown) => void
    list.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    await wrapper.get('[data-testid="time-range-15m"]').trigger('click')
    await flushPromises()

    resolveOld({ items: [{ ...recovered, api_key_name: 'stale key' }], total: 1, page: 1, page_size: 20 })
    await flushPromises()
    expect(wrapper.text()).toContain('Production')
    expect(wrapper.text()).not.toContain('stale key')
    wrapper.unmount()
  })

  it('resets to page one when page size changes', async () => {
    const wrapper = render()
    await flushPromises()
    wrapper.findComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    wrapper.findComponent(PaginationStub).vm.$emit('update:pageSize', 50)
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, page_size: 50 })
    wrapper.unmount()
  })
})
