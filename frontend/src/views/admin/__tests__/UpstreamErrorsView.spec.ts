import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import UpstreamErrorsView from '../UpstreamErrorsView.vue'
import type { OpsUpstreamErrorRequest } from '@/api/admin/ops'

const { list } = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('@/api/admin/ops', () => ({ listUpstreamErrorRequests: list, default: {} }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const DataTableStub = defineComponent({
  props: ['columns', 'data', 'loading'],
  template: `<div><div v-for="row in data" :key="row.id" data-testid="row"><div v-for="col in columns" :key="col.key"><slot :name="'cell-' + col.key" :row="row">{{ row[col.key] }}</slot></div></div></div>`
})
const SelectStub = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`
})
const recovered: OpsUpstreamErrorRequest = {
  id: 1, created_at: '2026-11-04T01:00:00Z', client_request_id: 'request-1', request_id: 'upstream-1',
  api_key_id: 8, api_key_name: 'Production', account_id: 2, account_name: 'Failed upstream', model: 'gpt-test',
  client_status_code: 200, outcome: 'recovered', final: { account_id: 3, account_name: 'Successful upstream' },
  attempts: [
    { account_id: 2, account_name: 'Failed upstream', upstream_status_code: 502, message: 'Concrete 502 diagnostic', detail: '<script>not html</script>', kind: 'http_error' },
    { account_id: 2, upstream_status_code: 503, message: 'retry exhausted', kind: 'retry_exhausted' }
  ]
}
function render() {
  return mount(UpstreamErrorsView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, DataTable: DataTableStub, Select: SelectStub,
    BaseDialog: { props: ['show'], template: '<section v-if="show" data-testid="detail"><slot /></section>' },
    Pagination: { name: 'Pagination', props: ['page', 'pageSize', 'total'], emits: ['update:page', 'update:pageSize'], template: '<div />' }
  } } })
}
beforeEach(() => {
  list.mockReset().mockResolvedValue({ items: [recovered], total: 30, page: 1, page_size: 20 })
})
describe('UpstreamErrorsView', () => {
  it('loads the latest 24h with server pagination and renders Key, failing and successful accounts', async () => {
    const wrapper = render(); await flushPromises()
    const params = list.mock.calls[0]![0]
    expect(params).toMatchObject({ page: 1, page_size: 20 })
    expect(new Date(params.end_time).getTime() - new Date(params.start_time).getTime()).toBe(86400000)
    expect(wrapper.text()).toContain('Production #8')
    expect(wrapper.text()).toContain('Failed upstream #2')
    expect(wrapper.text()).toContain('Successful upstream #3')
    expect(wrapper.text()).toContain('502 / 503')
    wrapper.unmount()
  })
  it('opens available diagnostics and safely renders text rather than HTML', async () => {
    const wrapper = render(); await flushPromises()
    const button = wrapper.findAll('button').find(b => b.text() === 'admin.upstreamErrors.details')!
    await button.trigger('click')
    const detail = wrapper.get('[data-testid="detail"]')
    expect(detail.text()).toContain('request-1')
    expect(detail.text()).toContain('Concrete 502 diagnostic')
    expect(detail.text()).toContain('<script>not html</script>')
    expect(detail.find('script').exists()).toBe(false)
    wrapper.unmount()
  })
  it('filters matching status, outcome, Key and failing account on search', async () => {
    const wrapper = render(); await flushPromises()
    const selects = wrapper.findAll('select')
    await selects[1]!.setValue('503'); await selects[2]!.setValue('failed')
    const inputs = wrapper.findAll('input')
    await inputs[0]!.setValue('8'); await inputs[1]!.setValue('2')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, status_code: 503, outcome: 'failed', api_key_id: 8, account_id: 2 })
    wrapper.unmount()
  })
  it('paginates server-side and resets page when changing page size', async () => {
    const wrapper = render(); await flushPromises()
    wrapper.findComponent({ name: 'Pagination' }).vm.$emit('update:page', 2)
    await flushPromises()
    expect(list.mock.calls.at(-1)![0].page).toBe(2)
    wrapper.findComponent({ name: 'Pagination' }).vm.$emit('update:pageSize', 50)
    await flushPromises()
    expect(list.mock.calls.at(-1)![0]).toMatchObject({ page: 1, page_size: 50 })
    wrapper.unmount()
  })
  it('does not imply a successful final account for failed requests or missing usage', async () => {
    list.mockResolvedValue({ items: [{ ...recovered, outcome: 'failed', client_status_code: 503 }, { ...recovered, id: 2, final: null }], total: 2 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).not.toContain('Successful upstream')
    expect(wrapper.text()).toContain('admin.upstreamErrors.noSuccess')
    expect(wrapper.text()).toContain('admin.upstreamErrors.notRecorded')
    wrapper.unmount()
  })
  it('shows a retryable load error rather than stale data', async () => {
    list.mockRejectedValue(new Error('monitoring disabled'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.upstreamErrors.loadFailed')
    list.mockResolvedValue({ items: [recovered], total: 1 })
    await wrapper.get('[role="alert"] button').trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Production')
    wrapper.unmount()
  })
  it('ignores stale responses when a newer search finishes first', async () => {
    let resolveOld!: (value: unknown) => void
    list.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    resolveOld({ items: [{ ...recovered, api_key_name: 'stale key' }], total: 1 }); await flushPromises()
    expect(wrapper.text()).toContain('Production')
    expect(wrapper.text()).not.toContain('stale key')
    wrapper.unmount()
  })
})
