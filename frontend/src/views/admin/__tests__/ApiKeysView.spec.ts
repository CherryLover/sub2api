import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'
import ApiKeysView from '../ApiKeysView.vue'

const {
  listApiKeys,
  updateApiKey,
  removeApiKey,
  getBatchApiKeysUsage,
  getAllGroups,
  searchUsers,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  updateApiKey: vi.fn(),
  removeApiKey: vi.fn(),
  getBatchApiKeysUsage: vi.fn(),
  getAllGroups: vi.fn(),
  searchUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

const routerMock = vi.hoisted(() => ({
  push: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    apiKeys: {
      list: listApiKeys,
      update: updateApiKey,
      remove: removeApiKey,
    },
    dashboard: {
      getBatchApiKeysUsage,
    },
    groups: {
      getAll: getAllGroups,
    },
    usage: {
      searchUsers,
    },
  },
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

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (params) {
          return `${key}:${JSON.stringify(params)}`
        }
        return key
      },
    }),
  }
})

// 服务端已经掩码（前 6 + 后 4），前端原样显示
const MASKED_KEY = 'sk-abc...wxyz'

const createApiKey = (overrides: Partial<ApiKey> = {}): ApiKey => ({
  id: 1,
  user_id: 7,
  key: MASKED_KEY,
  name: 'prod-key',
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  current_concurrency: 0,
  user: { id: 7, email: 'owner@example.com', username: 'owner' },
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
  ...overrides,
})

const AppLayoutStub = { template: '<div><slot /></div>' }

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
      <button data-test="sort-usage" @click="$emit('sort', 'usage', 'desc')">Sort Usage</button>
      <button data-test="sort-last-used-at" @click="$emit('sort', 'last_used_at', 'asc')">Sort Last Used</button>
      <button data-test="sort-name" @click="$emit('sort', 'name', 'asc')">Sort Name</button>
      <div v-for="row in data" :key="row.id" data-test="row">
        <div data-test="cell-name"><slot name="cell-name" :value="row.name" :row="row" /></div>
        <div data-test="cell-user"><slot name="cell-user" :value="row.user" :row="row" /></div>
        <div data-test="cell-key"><slot name="cell-key" :value="row.key" :row="row" /></div>
        <div data-test="cell-status"><slot name="cell-status" :value="row.status" :row="row" /></div>
        <div data-test="cell-usage"><slot name="cell-usage" :value="null" :row="row" /></div>
        <div data-test="row-actions"><slot name="cell-actions" :value="null" :row="row" /></div>
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
  template: '<div data-test="pagination"></div>',
}

const BaseDialogStub = {
  name: 'BaseDialog',
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show" data-test="base-dialog"><slot /><slot name="footer" /></div>',
}

const ConfirmDialogStub = {
  name: 'ConfirmDialog',
  props: ['show', 'title', 'message'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="confirm-dialog">
      <span data-test="confirm-message">{{ message }}</span>
      <button data-test="confirm-ok" @click="$emit('confirm')">ok</button>
      <button data-test="confirm-cancel" @click="$emit('cancel')">cancel</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async () => {
  const wrapper = mount(ApiKeysView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        GroupBadge: true,
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

/** 最后一次 list 调用的 filters 参数（第三个位置参数） */
const lastListFilters = () => {
  const call = listApiKeys.mock.calls.at(-1)
  if (!call) throw new Error('list not called')
  return call[2] as Record<string, unknown>
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

beforeEach(() => {
  localStorage.clear()

  listApiKeys.mockReset()
  updateApiKey.mockReset()
  removeApiKey.mockReset()
  getBatchApiKeysUsage.mockReset()
  getAllGroups.mockReset()
  searchUsers.mockReset()
  showError.mockReset()
  showSuccess.mockReset()
  routerMock.push.mockClear()

  listApiKeys.mockResolvedValue({
    items: [createApiKey()],
    total: 1,
    page: 1,
    page_size: 20,
    pages: 1,
  })
  getBatchApiKeysUsage.mockResolvedValue({
    stats: { '1': { api_key_id: 1, today_actual_cost: 1.5, total_actual_cost: 12.25 } },
  })
  getAllGroups.mockResolvedValue([])
  searchUsers.mockResolvedValue([])
  updateApiKey.mockImplementation(async (id: number, payload: Record<string, unknown>) => ({
    api_key: createApiKey({ id, ...(payload as Partial<ApiKey>) }),
    auto_granted_group_access: false,
  }))
  removeApiKey.mockResolvedValue({ message: 'ok' })
})

describe('admin ApiKeysView 列渲染', () => {
  it('按方案顺序渲染 9 列，用户 / 密钥 / 分组列不可排序', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'user',
      'key',
      'group',
      'usage',
      'status',
      'last_used_at',
      'created_at',
      'actions',
    ])

    const meta = Object.fromEntries(visibleColumnMeta(wrapper).map((col) => [col.key, col.sortable]))
    expect(meta.user).toBe(false)
    expect(meta.key).toBe(false)
    expect(meta.group).toBe(false)
    expect(meta.actions).toBe(false)
    expect(meta.usage).toBe(true)
    expect(meta.last_used_at).toBe(true)
    expect(meta.created_at).toBe(true)
  })

  it('密钥列原样显示服务端掩码值，所属用户列显示邮箱与用户名', async () => {
    const wrapper = await mountView()

    expect(wrapper.get('[data-test="masked-key"]').text()).toBe(MASKED_KEY)
    // 不做二次脱敏，也不出现明文形态的完整密钥
    expect(wrapper.get('[data-test="cell-key"]').text()).not.toContain('sk-abcdefghijklmnop')

    const userCell = wrapper.get('[data-test="cell-user"]').text()
    expect(userCell).toContain('owner@example.com')
    expect(userCell).toContain('owner')
  })

  it('用量列用管理端批量接口按当前页密钥 ID 拉取', async () => {
    const wrapper = await mountView()

    expect(getBatchApiKeysUsage).toHaveBeenCalledTimes(1)
    expect(getBatchApiKeysUsage.mock.calls[0][0]).toEqual([1])
    const usageCell = wrapper.get('[data-test="cell-usage"]').text()
    expect(usageCell).toContain('$1.5000')
    expect(usageCell).toContain('$12.2500')
  })
})

describe('admin ApiKeysView 排序参数透传', () => {
  it('默认按 created_at desc 请求', async () => {
    await mountView()

    expect(listApiKeys).toHaveBeenCalledTimes(1)
    expect(listApiKeys.mock.calls[0][0]).toBe(1)
    expect(lastListFilters()).toMatchObject({ sort_by: 'created_at', sort_order: 'desc' })
    expect(lastListFilters().user_id).toBeUndefined()
  })

  it('点击「用量」列排序时透传 sort_by=today_cost', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-test="sort-usage"]').trigger('click')
    await flushPromises()

    expect(lastListFilters()).toMatchObject({ sort_by: 'today_cost', sort_order: 'desc' })
  })

  it('点击「最后使用时间」列排序时透传 sort_by=last_used_at', async () => {
    const wrapper = await mountView()

    await wrapper.get('[data-test="sort-last-used-at"]').trigger('click')
    await flushPromises()

    expect(lastListFilters()).toMatchObject({ sort_by: 'last_used_at', sort_order: 'asc' })
  })
})

describe('admin ApiKeysView 用户筛选', () => {
  it('输入邮箱搜索用户并选中后按 user_id 重新拉取；清除后去掉 user_id', async () => {
    searchUsers.mockResolvedValue([{ id: 42, email: 'alice@example.com', deleted: false }])
    const wrapper = await mountView()

    const input = wrapper.get('[data-test="user-filter-input"]')
    await input.setValue('alice')
    await wait(350)
    await flushPromises()

    expect(searchUsers).toHaveBeenCalledWith('alice')

    await wrapper.get('[data-test="user-option-42"]').trigger('click')
    await flushPromises()

    expect(lastListFilters()).toMatchObject({ user_id: 42, sort_by: 'created_at', sort_order: 'desc' })
    expect(listApiKeys.mock.calls.at(-1)?.[0]).toBe(1)

    await wrapper.get('[data-test="user-filter-clear"]').trigger('click')
    await flushPromises()

    expect(lastListFilters().user_id).toBeUndefined()
  })
})

describe('admin ApiKeysView 行操作', () => {
  it('删除需先确认，确认后调用管理端删除接口并刷新列表', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)

    await getButtonByText(wrapper, 'common.delete').trigger('click')
    await nextTick()

    expect(removeApiKey).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="confirm-message"]').text()).toContain('prod-key')
    expect(wrapper.get('[data-test="confirm-message"]').text()).toContain('owner@example.com')

    const listCallsBefore = listApiKeys.mock.calls.length
    await wrapper.get('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(removeApiKey).toHaveBeenCalledWith(1)
    expect(showSuccess).toHaveBeenCalledWith('admin.apiKeys.keyDeleted')
    expect(listApiKeys.mock.calls.length).toBe(listCallsBefore + 1)
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('取消删除不调用接口', async () => {
    const wrapper = await mountView()

    await getButtonByText(wrapper, 'common.delete').trigger('click')
    await nextTick()
    await wrapper.get('[data-test="confirm-cancel"]').trigger('click')
    await nextTick()

    expect(removeApiKey).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('启停只提交 status 字段', async () => {
    const wrapper = await mountView()

    await getButtonByText(wrapper, 'admin.apiKeys.disable').trigger('click')
    await flushPromises()

    expect(updateApiKey).toHaveBeenCalledWith(1, { status: 'inactive' })
    expect(showSuccess).toHaveBeenCalledWith('admin.apiKeys.keyDisabled')
    expect(wrapper.get('[data-test="cell-status"]').text()).toBe('keys.status.inactive')
  })

  it('「查用量」跳到管理端用量页并带上 api_key_id', async () => {
    const wrapper = await mountView()

    await getButtonByText(wrapper, 'admin.apiKeys.viewUsage').trigger('click')

    expect(routerMock.push).toHaveBeenCalledWith({
      path: '/admin/usage',
      query: { api_key_id: '1', user_id: '7' },
    })
  })

  it('编辑弹窗提交 IP 名单；分组未改动时不带 group_id', async () => {
    const wrapper = await mountView()

    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-test="base-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-test="edit-ip-whitelist"]').setValue('1.2.3.4\n10.0.0.0/8\n')
    await wrapper.get('[data-test="edit-ip-blacklist"]').setValue('')
    await wrapper.get('[data-test="edit-submit"]').trigger('click')
    await flushPromises()

    expect(updateApiKey).toHaveBeenCalledWith(1, {
      ip_whitelist: ['1.2.3.4', '10.0.0.0/8'],
      ip_blacklist: [],
    })
    expect(updateApiKey.mock.calls[0][1]).not.toHaveProperty('group_id')
    expect(wrapper.find('[data-test="base-dialog"]').exists()).toBe(false)
  })
})
