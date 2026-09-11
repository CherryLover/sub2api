import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getBatchDiagnostics,
  clearRuntimeBlocks,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getBatchDiagnostics: vi.fn(),
  clearRuntimeBlocks: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

const { showError, showSuccess, showInfo } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getBatchDiagnostics,
      clearRuntimeBlocks,
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

// Keep interpolation params visible so we can assert on the cleared count.
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${JSON.stringify(params)}` : key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" :data-test="'row-' + row.id" />
    </div>
  `
}

// Only renders while :show is true, so at most one dialog is in the DOM at a time.
const ConfirmDialogStub = {
  props: ['show', 'title', 'message', 'confirmText', 'cancelText', 'danger'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="confirm-dialog">
      <div data-test="confirm-title">{{ title }}</div>
      <div data-test="confirm-message">{{ message }}</div>
      <slot />
      <button data-test="confirm-ok" @click="$emit('confirm')">{{ confirmText }}</button>
      <button data-test="confirm-cancel" @click="$emit('cancel')">{{ cancelText }}</button>
    </div>
  `
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        HelpTooltip: true,
        Pagination: true,
        ConfirmDialog: ConfirmDialogStub,
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountRecentErrorsCell: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        Icon: true
      }
    }
  })
}

const baseAccount = {
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 1,
  priority: 0,
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

const clearedCounts = {
  account_runtime_blocks: 3,
  model_transient_cooldowns: 5,
  proxy_quarantines: 1,
  grok_model_quota_blocks: 0,
  grok_team_rate_limits: 0,
  grok_free_quota_gates: 2
}

// The tools menu lives inside <Teleport to="body">, so it is outside the wrapper's own tree.
const findMenuItem = () =>
  document.querySelector<HTMLButtonElement>('[data-test="clear-runtime-blocks"]')

async function openToolsMenu(wrapper: VueWrapper) {
  await wrapper.find('button[title="admin.accounts.moreActions"]').trigger('click')
  await flushPromises()
}

async function openConfirmDialog(wrapper: VueWrapper) {
  await openToolsMenu(wrapper)
  const menuItem = findMenuItem()
  expect(menuItem).not.toBeNull()
  menuItem!.click()
  await flushPromises()
}

describe('admin AccountsView runtime block reset', () => {
  let wrapper: VueWrapper | null = null

  beforeEach(() => {
    localStorage.clear()
    document.body.innerHTML = ''

    for (const fn of [
      listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getBatchDiagnostics,
      clearRuntimeBlocks,
      getAllProxies,
      getAllGroups,
      showError,
      showSuccess,
      showInfo
    ]) {
      fn.mockReset()
    }

    listAccounts.mockResolvedValue({
      items: [
        { ...baseAccount, id: 6, name: 'benched' },
        { ...baseAccount, id: 7, name: 'healthy' }
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getBatchDiagnostics.mockResolvedValue({
      window_minutes: 15,
      generated_at: '2026-09-11T09:00:00Z',
      diagnostics: {}
    })
    clearRuntimeBlocks.mockResolvedValue({
      scope: 'all',
      account_ids: [],
      cleared: clearedCounts,
      total_cleared: 11
    })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    document.body.innerHTML = ''
  })

  it('asks for confirmation before touching the API, and explains what is cleared', async () => {
    wrapper = mountView()
    await flushPromises()

    await openToolsMenu(wrapper)
    const menuItem = findMenuItem()
    expect(menuItem).not.toBeNull()
    expect(menuItem!.textContent).toContain('admin.accounts.runtimeBlocks.clearAction')

    menuItem!.click()
    await flushPromises()

    const dialog = wrapper.find('[data-test="confirm-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.find('[data-test="confirm-title"]').text()).toBe(
      'admin.accounts.runtimeBlocks.clearTitle'
    )
    expect(dialog.find('[data-test="confirm-message"]').text()).toBe(
      'admin.accounts.runtimeBlocks.clearConfirmMessage'
    )

    // In-memory only / DB untouched / accounts return to the pool.
    const details = dialog.findAll('[data-test="clear-runtime-blocks-details"] li').map(li => li.text())
    expect(details).toEqual([
      'admin.accounts.runtimeBlocks.clearConfirmScope',
      'admin.accounts.runtimeBlocks.clearConfirmSafety',
      'admin.accounts.runtimeBlocks.clearConfirmEffect'
    ])

    // Nothing happens until the user confirms.
    expect(clearRuntimeBlocks).not.toHaveBeenCalled()
  })

  it('cancelling closes the dialog without calling the API', async () => {
    wrapper = mountView()
    await flushPromises()
    await openConfirmDialog(wrapper)

    await wrapper.find('[data-test="confirm-cancel"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
    expect(clearRuntimeBlocks).not.toHaveBeenCalled()
  })

  it('confirming clears every account (no account_ids in the request)', async () => {
    wrapper = mountView()
    await flushPromises()
    await openConfirmDialog(wrapper)

    await wrapper.find('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(clearRuntimeBlocks).toHaveBeenCalledTimes(1)
    // Called with no argument => the API layer sends {} => backend treats it as "all accounts".
    expect(clearRuntimeBlocks.mock.calls[0]).toEqual([])
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('reports how many blocks were cleared and refreshes the list + diagnostics', async () => {
    wrapper = mountView()
    await flushPromises()

    const listCallsBefore = listAccounts.mock.calls.length
    const diagnosticsCallsBefore = getBatchDiagnostics.mock.calls.length

    await openConfirmDialog(wrapper)
    await wrapper.find('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith(
      'admin.accounts.runtimeBlocks.clearSuccess|{"count":11}'
    )
    expect(showError).not.toHaveBeenCalled()

    expect(listAccounts.mock.calls.length).toBe(listCallsBefore + 1)
    expect(getBatchDiagnostics.mock.calls.length).toBe(diagnosticsCallsBefore + 1)
  })

  it('total_cleared = 0 reads as "nothing was blocked", not as a failure', async () => {
    clearRuntimeBlocks.mockResolvedValue({
      scope: 'all',
      account_ids: [],
      cleared: {
        account_runtime_blocks: 0,
        model_transient_cooldowns: 0,
        proxy_quarantines: 0,
        grok_model_quota_blocks: 0,
        grok_team_rate_limits: 0,
        grok_free_quota_gates: 0
      },
      total_cleared: 0
    })

    wrapper = mountView()
    await flushPromises()
    await openConfirmDialog(wrapper)
    await wrapper.find('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.runtimeBlocks.clearNothing')
    expect(showError).not.toHaveBeenCalled()
  })

  it('surfaces API failures through the error toast and leaves the list alone', async () => {
    clearRuntimeBlocks.mockRejectedValue({ message: 'runtime reset unavailable' })

    wrapper = mountView()
    await flushPromises()

    const listCallsBefore = listAccounts.mock.calls.length

    await openConfirmDialog(wrapper)
    await wrapper.find('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('runtime reset unavailable')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(listAccounts.mock.calls.length).toBe(listCallsBefore)
  })

  it('disables the menu entry while the request is in flight and never double-fires', async () => {
    let resolveClear: ((value: unknown) => void) | undefined
    clearRuntimeBlocks.mockImplementation(
      () =>
        new Promise(resolve => {
          resolveClear = resolve
        })
    )

    wrapper = mountView()
    await flushPromises()
    await openConfirmDialog(wrapper)
    await wrapper.find('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    // Re-open the menu mid-flight: the entry is disabled and shows the in-progress label.
    await openToolsMenu(wrapper)
    const pendingItem = findMenuItem()
    expect(pendingItem).not.toBeNull()
    expect(pendingItem!.disabled).toBe(true)
    expect(pendingItem!.textContent).toContain('admin.accounts.runtimeBlocks.clearing')

    // Clicking again while disabled must not re-open the dialog or re-issue the request.
    pendingItem!.click()
    await flushPromises()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
    expect(clearRuntimeBlocks).toHaveBeenCalledTimes(1)

    resolveClear?.({
      scope: 'all',
      account_ids: [],
      cleared: clearedCounts,
      total_cleared: 11
    })
    await flushPromises()

    // The menu was never closed by the ignored click, so the entry is still there — now re-enabled.
    expect(findMenuItem()!.disabled).toBe(false)
    expect(showSuccess).toHaveBeenCalledWith(
      'admin.accounts.runtimeBlocks.clearSuccess|{"count":11}'
    )
  })
})
