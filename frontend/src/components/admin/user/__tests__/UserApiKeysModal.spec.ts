import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { ApiKey } from '@/types'

const apiMocks = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  routerPush: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      getUserApiKeys: apiMocks.getUserApiKeys,
    },

  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: apiMocks.routerPush }),
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
      },
    },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('UserApiKeysModal', () => {
  it('打开时拉取该用户的密钥并显示状态文案', async () => {
    const wrapper = await mountAndOpen()

    expect(apiMocks.getUserApiKeys).toHaveBeenCalledWith(99)
    expect(wrapper.get('[data-test="key-status-11"]').text()).toBe('keys.status.active')
  })

  it('用户 Key 弹窗只读，不提供分组变更、启停、IP 编辑或删除', async () => {
    const wrapper = await mountAndOpen([
      createApiKey({ ip_whitelist: ['192.168.1.1'], ip_blacklist: ['1.2.3.4'] }),
    ])

    expect(wrapper.find('[data-test="key-toggle-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="key-ip-rules-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="key-delete-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="ip-editor-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="key-usage-11"]').exists()).toBe(true)
  })

  it('查看用量跳到全局用量页并同时带用户与 Key 筛选', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="key-usage-11"]').trigger('click')

    expect(apiMocks.routerPush).toHaveBeenCalledWith({
      path: '/admin/usage',
      query: { user_id: '99', api_key_id: '11' },
    })
  })
})
