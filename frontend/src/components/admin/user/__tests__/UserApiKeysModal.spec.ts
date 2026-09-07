import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey } from '@/types'

const apiMocks = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getAllGroups: vi.fn(),
  updateApiKey: vi.fn(),
  removeApiKey: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      getUserApiKeys: apiMocks.getUserApiKeys,
    },
    groups: {
      getAll: apiMocks.getAllGroups,
    },
    apiKeys: {
      update: apiMocks.updateApiKey,
      remove: apiMocks.removeApiKey,
      updateApiKeyGroup: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: apiMocks.showError,
    showSuccess: apiMocks.showSuccess,
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

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show', 'title', 'width'],
    emits: ['close'],
    template: '<div v-if="show" data-test="base-dialog"><slot /><slot name="footer" /></div>',
  },
}))

vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: {
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
  },
}))

import UserApiKeysModal from '../UserApiKeysModal.vue'

const createApiKey = (overrides: Partial<ApiKey> = {}): ApiKey => ({
  id: 11,
  user_id: 99,
  key: 'sk-full-plaintext-key-0123456789abcdef',
  name: 'mobile',
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

const makeUser = () => ({ id: 99, email: 'u@example.com', username: 'u' }) as any

/** 挂载并触发 show：false → true，确保 watch 被激活 */
async function mountAndOpen(keys: ApiKey[] = [createApiKey()]) {
  apiMocks.getUserApiKeys.mockResolvedValue({ items: keys, total: keys.length, page: 1, page_size: 20, pages: 1 })
  const wrapper = mount(UserApiKeysModal, {
    props: { show: false, user: makeUser() },
    global: {
      stubs: {
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: true,
      },
    },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.getAllGroups.mockResolvedValue([])
  apiMocks.updateApiKey.mockImplementation(async (id: number, payload: Record<string, unknown>) => ({
    api_key: createApiKey({ id, ...(payload as Partial<ApiKey>) }),
    auto_granted_group_access: false,
  }))
  apiMocks.removeApiKey.mockResolvedValue({ message: 'ok' })
})

describe('UserApiKeysModal', () => {
  it('打开时拉取该用户的密钥并显示状态文案', async () => {
    const wrapper = await mountAndOpen()

    expect(apiMocks.getUserApiKeys).toHaveBeenCalledWith(99)
    expect(wrapper.get('[data-test="key-status-11"]').text()).toBe('keys.status.active')
  })

  it('启停只提交 status 字段，并用返回的密钥更新卡片', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="key-toggle-11"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateApiKey).toHaveBeenCalledTimes(1)
    expect(apiMocks.updateApiKey).toHaveBeenCalledWith(11, { status: 'inactive' })
    expect(apiMocks.showSuccess).toHaveBeenCalledWith('admin.apiKeys.keyDisabled')
    expect(wrapper.get('[data-test="key-status-11"]').text()).toBe('keys.status.inactive')
    // 再点一次变回启用
    await wrapper.get('[data-test="key-toggle-11"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateApiKey).toHaveBeenLastCalledWith(11, { status: 'active' })
  })

  it('删除先弹确认；取消不调接口，确认后调用删除接口并从列表移除', async () => {
    const wrapper = await mountAndOpen()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)

    await wrapper.get('[data-test="key-delete-11"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="confirm-message"]').text()).toContain('mobile')
    expect(wrapper.get('[data-test="confirm-message"]').text()).toContain('u@example.com')

    await wrapper.get('[data-test="confirm-cancel"]').trigger('click')
    await nextTick()
    expect(apiMocks.removeApiKey).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)

    await wrapper.get('[data-test="key-delete-11"]').trigger('click')
    await nextTick()
    await wrapper.get('[data-test="confirm-ok"]').trigger('click')
    await flushPromises()

    expect(apiMocks.removeApiKey).toHaveBeenCalledWith(11)
    expect(apiMocks.showSuccess).toHaveBeenCalledWith('admin.apiKeys.keyDeleted')
    expect(wrapper.find('[data-test="key-toggle-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('IP 名单编辑：回填现有规则，保存时按行拆分并提交 ip_whitelist / ip_blacklist', async () => {
    const wrapper = await mountAndOpen([
      createApiKey({ ip_whitelist: ['192.168.1.1'], ip_blacklist: [] }),
    ])

    await wrapper.get('[data-test="key-ip-rules-11"]').trigger('click')
    await nextTick()
    const editor = wrapper.get('[data-test="ip-editor-11"]')
    expect((editor.get('[data-test="ip-whitelist-input"]').element as HTMLTextAreaElement).value).toBe('192.168.1.1')

    await editor.get('[data-test="ip-whitelist-input"]').setValue('192.168.1.1\n 10.0.0.0/8 \n\n')
    await editor.get('[data-test="ip-blacklist-input"]').setValue('1.2.3.4')
    await editor.get('[data-test="ip-rules-save"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateApiKey).toHaveBeenCalledWith(11, {
      ip_whitelist: ['192.168.1.1', '10.0.0.0/8'],
      ip_blacklist: ['1.2.3.4'],
    })
    expect(apiMocks.updateApiKey.mock.calls[0][1]).not.toHaveProperty('status')
    expect(apiMocks.updateApiKey.mock.calls[0][1]).not.toHaveProperty('group_id')
    expect(apiMocks.showSuccess).toHaveBeenCalledWith('admin.apiKeys.keyUpdated')
    // 保存后编辑区收起
    expect(wrapper.find('[data-test="ip-editor-11"]').exists()).toBe(false)
  })

  it('清空 IP 名单时提交空数组（而不是省略字段）', async () => {
    const wrapper = await mountAndOpen([
      createApiKey({ ip_whitelist: ['192.168.1.1'], ip_blacklist: ['1.2.3.4'] }),
    ])

    await wrapper.get('[data-test="key-ip-rules-11"]').trigger('click')
    await nextTick()
    await wrapper.get('[data-test="ip-whitelist-input"]').setValue('')
    await wrapper.get('[data-test="ip-blacklist-input"]').setValue('')
    await wrapper.get('[data-test="ip-rules-save"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateApiKey).toHaveBeenCalledWith(11, { ip_whitelist: [], ip_blacklist: [] })
  })
})
