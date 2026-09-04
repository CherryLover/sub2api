import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import KeysView from '../KeysView.vue'

const {
  listKeys,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  showError,
  showSuccess,
  copyToClipboard,
  createKeyUsageSession,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  createKeyUsageSession: vi.fn(),
}))

const routerMock = vi.hoisted(() => ({
  resolve: vi.fn((to: { path: string; query?: Record<string, string> }) => ({
    href: `${to.path}?${new URLSearchParams(to.query ?? {}).toString()}`,
  })),
  push: vi.fn(() => Promise.resolve()),
}))

const messages: Record<string, string> = {
  'common.actions': 'Actions',
  'common.name': 'Name',
  'common.refresh': 'Refresh',
  'common.status': 'Status',
  'keys.apiKey': 'API Key',
  'keys.allGroups': 'All Groups',
  'keys.allStatus': 'All Status',
  'keys.columnSettings': 'Column Settings',
  'keys.createKey': 'Create API Key',
  'keys.created': 'Created',
  'keys.expiresAt': 'Expires',
  'keys.group': 'Group',
  'keys.id': 'ID',
  'keys.currentConcurrency': 'Current Concurrency',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
  'keys.viewUsage': 'View Usage',
  'keys.usageUnavailableInactive': 'Disabled keys cannot look up usage',
  'keys.failedToOpenUsage': 'Failed to open usage lookup',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('@/api/keyUsage', () => ({
  createKeyUsageSession,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => routerMock,
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-06-27T00:00:00Z',
  updated_at: '2026-06-27T00:00:00Z',
  current_concurrency: 3,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const AppLayoutStub = {
  template: '<div><slot /></div>',
}

const TablePageLayoutStub = {
  template: `
    <div>
      <slot name="filters" />
      <slot name="actions" />
      <slot name="table" />
      <slot name="pagination" />
    </div>
  `,
}

const DataTableStub = {
  name: 'DataTable',
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <div
          v-if="columns.some((col) => col.key === 'last_used_ip')"
          data-test="last-used-ip"
        >
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
        <div data-test="row-actions">
          <slot name="cell-actions" :value="null" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"></select>',
}

const SearchInputStub = {
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

const PaginationStub = {
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div>
      <button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: true,
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

const visibleColumnKeys = (wrapper: VueWrapper) =>
  wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)

const visibleColumnMeta = (wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> =>
  JSON.parse(wrapper.get('[data-test="columns-meta"]').text())

const getButtonByText = (wrapper: VueWrapper, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`Button not found: ${text}`)
  }
  return button
}

/** Stand-in for the about:blank tab that openKeyUsage opens synchronously and navigates later. */
const createPopup = () => ({
  opener: {} as unknown,
  location: { replace: vi.fn() },
  close: vi.fn(),
})

let popup: ReturnType<typeof createPopup>
let openSpy: MockInstance<typeof window.open>

beforeEach(() => {
  localStorage.clear()

  listKeys.mockReset()
  getPublicSettings.mockReset()
  getDashboardApiKeysUsage.mockReset()
  getAvailableGroups.mockReset()
  getUserGroupRates.mockReset()
  showError.mockReset()
  showSuccess.mockReset()
  copyToClipboard.mockReset()
  createKeyUsageSession.mockReset()
  routerMock.resolve.mockClear()
  routerMock.push.mockClear()
  popup = createPopup()
  openSpy = vi.spyOn(window, 'open').mockImplementation(() => popup as unknown as Window)

  listKeys.mockResolvedValue({
    items: [createApiKey()],
    total: 1,
    page: 1,
    page_size: 20,
    pages: 1,
  })
  getPublicSettings.mockResolvedValue({})
  getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
  getAvailableGroups.mockResolvedValue([])
  getUserGroupRates.mockResolvedValue({})
})

afterEach(() => {
  openSpy.mockRestore()
})

describe('user KeysView column settings', () => {
  it('uses the default API key columns with low-frequency columns hidden', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'status',
      'created_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Rate Limit').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'ID').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    listKeys.mockResolvedValueOnce({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
  })

  it('restores column preferences from localStorage on mount', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('3')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('API Key')
    expect(columnMenuText).toContain('ID')
    expect(columnMenuText).toContain('Current Concurrency')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })

  it('renders the current concurrency value', async () => {
    const wrapper = await mountView()

    expect(wrapper.get('[data-test="current-concurrency"]').text()).toBe('3')
  })

  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('keeps filters and selected page size when sorting by current concurrency', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 42, name: 'OpenAI' }])
    const wrapper = await mountView()

    await wrapper.get('[data-test="page-size-50"]').trigger('click')
    await flushPromises()

    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()

    listKeys.mockClear()

    await wrapper.get('[data-test="sort-current-concurrency"]').trigger('click')
    await flushPromises()

    expect(listKeys).toHaveBeenLastCalledWith(
      1,
      50,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: 'current_concurrency',
        sort_order: 'asc',
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})

describe('user KeysView usage lookup', () => {
  const getUsageButton = (wrapper: VueWrapper) => getButtonByText(wrapper, 'View Usage')

  const listSingleKey = (overrides: Partial<ApiKey>) =>
    listKeys.mockResolvedValueOnce({
      items: [{ ...createApiKey(), ...overrides }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

  it('opens a blank tab before the exchange, then navigates it to the usage page with the token only', async () => {
    createKeyUsageSession.mockResolvedValue({ token: 'tok-1', expires_at: '2026-10-04T00:00:00Z' })
    const wrapper = await mountView()

    await getUsageButton(wrapper).trigger('click')
    await flushPromises()

    // The tab is opened inside the click gesture (before any await) so Safari does not block it.
    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(openSpy).toHaveBeenCalledWith('about:blank', '_blank')
    expect(openSpy.mock.invocationCallOrder[0]).toBeLessThan(createKeyUsageSession.mock.invocationCallOrder[0])
    // noopener cannot be used (it hides the window handle), so the link is severed by hand.
    expect(popup.opener).toBeNull()

    expect(createKeyUsageSession).toHaveBeenCalledTimes(1)
    expect(createKeyUsageSession).toHaveBeenCalledWith('sk-test-key', 'Failed to open usage lookup')
    expect(routerMock.resolve).toHaveBeenCalledWith({ path: '/key-usage', query: { t: 'tok-1' } })

    expect(popup.location.replace).toHaveBeenCalledTimes(1)
    const url = String(popup.location.replace.mock.calls[0][0])
    expect(url).toContain('/key-usage?')
    expect(url).toContain('t=tok-1')
    // The raw key must never be written into the URL.
    expect(url).not.toContain('sk-test-key')

    expect(popup.close).not.toHaveBeenCalled()
    expect(routerMock.push).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('disables the button while the token exchange is in flight', async () => {
    let resolveSession: (value: { token: string; expires_at: string | null }) => void = () => {}
    createKeyUsageSession.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSession = resolve
        })
    )
    const wrapper = await mountView()

    await getUsageButton(wrapper).trigger('click')
    await nextTick()
    expect(getUsageButton(wrapper).attributes('disabled')).toBeDefined()

    // A second click while pending must neither open another tab nor start another exchange.
    await getUsageButton(wrapper).trigger('click')
    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(createKeyUsageSession).toHaveBeenCalledTimes(1)

    resolveSession({ token: 'tok-2', expires_at: null })
    await flushPromises()
    expect(getUsageButton(wrapper).attributes('disabled')).toBeUndefined()
    expect(popup.location.replace).toHaveBeenCalledTimes(1)
  })

  it('closes the blank tab and shows the backend error when the exchange fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    createKeyUsageSession.mockRejectedValue(new Error('Invalid or expired key'))
    const wrapper = await mountView()

    await getUsageButton(wrapper).trigger('click')
    await flushPromises()

    expect(popup.close).toHaveBeenCalledTimes(1)
    expect(popup.location.replace).not.toHaveBeenCalled()
    expect(routerMock.push).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('Invalid or expired key')
    expect(getUsageButton(wrapper).attributes('disabled')).toBeUndefined()

    consoleError.mockRestore()
  })

  it('falls back to navigating the current tab when the popup is blocked', async () => {
    openSpy.mockImplementation(() => null)
    createKeyUsageSession.mockResolvedValue({ token: 'tok-3', expires_at: null })
    const wrapper = await mountView()

    await getUsageButton(wrapper).trigger('click')
    await flushPromises()

    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(routerMock.push).toHaveBeenCalledTimes(1)
    expect(routerMock.push).toHaveBeenCalledWith({ path: '/key-usage', query: { t: 'tok-3' } })
    expect(JSON.stringify(routerMock.push.mock.calls)).not.toContain('sk-test-key')
    expect(popup.location.replace).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('disables the button for a disabled key and explains why', async () => {
    listSingleKey({ status: 'inactive' })
    const wrapper = await mountView()

    const button = getUsageButton(wrapper)
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('Disabled keys cannot look up usage')

    // The backend answers 401 for inactive keys, so no tab and no request are ever started.
    await button.trigger('click')
    expect(openSpy).not.toHaveBeenCalled()
    expect(createKeyUsageSession).not.toHaveBeenCalled()
  })

  it('keeps the button enabled for a quota-exhausted key', async () => {
    listSingleKey({ status: 'quota_exhausted' })
    const wrapper = await mountView()

    const button = getUsageButton(wrapper)
    expect(button.attributes('disabled')).toBeUndefined()
    expect(button.attributes('title')).toBeUndefined()
  })
})
