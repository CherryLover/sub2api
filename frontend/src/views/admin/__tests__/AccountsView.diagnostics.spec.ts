import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getBatchDiagnostics,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getBatchDiagnostics: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      getBatchDiagnostics,
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
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

// Render the status + recent_errors cells for every row and expose the column keys.
const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <span data-test="column-keys">{{ columns.map(c => c.key).join(',') }}</span>
      <div v-for="row in data" :key="row.id" :data-test="'row-' + row.id">
        <slot name="cell-status" :row="row" />
        <slot name="cell-recent_errors" :row="row" />
      </div>
    </div>
  `
}

const AccountStatusIndicatorStub = {
  props: ['account', 'diagnosis'],
  template: `<div data-test="indicator">{{ diagnosis ? diagnosis.scheduling.blocks.map(b => b.source).join('|') : 'none' }}</div>`
}

const AccountRecentErrorsCellStub = {
  props: ['errors', 'loading'],
  template: `<div data-test="errors-cell">{{ errors ? errors.total : 'null' }}</div>`
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
        ConfirmDialog: true,
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
        AccountStatusIndicator: AccountStatusIndicatorStub,
        AccountRecentErrorsCell: AccountRecentErrorsCellStub,
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

describe('admin AccountsView diagnostics', () => {
  beforeEach(() => {
    localStorage.clear()

    for (const fn of [listAccounts, listWithEtag, getBatchTodayStats, getBatchDiagnostics, getAllProxies, getAllGroups]) {
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
      generated_at: '2026-09-10T09:00:00Z',
      diagnostics: {
        '6': {
          scheduling: {
            schedulable: false,
            blocks: [{ source: 'runtime_block', until: '2099-09-15T07:10:53Z' }]
          },
          recent_errors: {
            total: 18,
            by_status: [{ status_code: 502, count: 14 }, { status_code: 429, count: 4 }],
            last: { at: '2026-09-10T08:58:00Z', status_code: 200, upstream_status_code: 502 }
          }
        }
      }
    })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('places the recent_errors column right after status/schedulable and shows it by default', async () => {
    const wrapper = mountView()
    await flushPromises()

    const keys = wrapper.find('[data-test="column-keys"]').text().split(',')
    const statusIdx = keys.indexOf('status')
    expect(keys.slice(statusIdx, statusIdx + 3)).toEqual(['status', 'schedulable', 'recent_errors'])
  })

  it('loads diagnostics for the current page and feeds both cells', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getBatchDiagnostics).toHaveBeenCalledWith([6, 7])

    const benched = wrapper.find('[data-test="row-6"]')
    expect(benched.find('[data-test="indicator"]').text()).toBe('runtime_block')
    expect(benched.find('[data-test="errors-cell"]').text()).toBe('18')

    // Missing from the response → no diagnosis, cell falls back to "-"
    const healthy = wrapper.find('[data-test="row-7"]')
    expect(healthy.find('[data-test="indicator"]').text()).toBe('none')
    expect(healthy.find('[data-test="errors-cell"]').text()).toBe('null')
  })

  it('skips the request when both the status and recent_errors columns are hidden', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['status', 'recent_errors', 'scheduler_score']))
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')

    mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalled()
    expect(getBatchDiagnostics).not.toHaveBeenCalled()
  })

  it('still requests diagnostics when only the status column is visible', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['recent_errors', 'scheduler_score']))
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')

    mountView()
    await flushPromises()

    expect(getBatchDiagnostics).toHaveBeenCalledTimes(1)
  })

  it('a failed diagnostics request does not break the list', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getBatchDiagnostics.mockRejectedValue(new Error('boom'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="row-6"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="row-7"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="row-6"] [data-test="errors-cell"]').text()).toBe('null')
    expect(consoleError).toHaveBeenCalledWith('Failed to load account diagnostics:', expect.any(Error))
    consoleError.mockRestore()
  })
})
