import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

// ---------------------------------------------------------------------------
// Store mocks
//
// AppSidebar 只读 store 的少量字段（isAdmin / isSimpleMode / sidebarCollapsed /
// opsMonitoringEnabled / cachedPublicSettings）。这里用一个 reactive 的假 state
// 顶掉整个 store 模块，测试里直接改 state 就能驱动组件重新渲染。
// featureFlags.ts 里的 useAppStore 来自 '@/stores/app'，所以两个模块都要 mock，
// 且必须指向同一个假 store 实例。
// ---------------------------------------------------------------------------
interface MockState {
  isAdmin: boolean
  isSimpleMode: boolean
  sidebarCollapsed: boolean
  mobileOpen: boolean
  backendModeEnabled: boolean
  opsMonitoringEnabled: boolean
  publicSettings: Record<string, boolean> | null
}

const holder = vi.hoisted(() => ({ state: null as unknown as MockState }))

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')

  const state = reactive<MockState>({
    isAdmin: true,
    isSimpleMode: false,
    sidebarCollapsed: false,
    mobileOpen: false,
    backendModeEnabled: false,
    opsMonitoringEnabled: true,
    publicSettings: {
      channel_monitor_enabled: true,
      risk_control_enabled: true,
      available_channels_enabled: true
    }
  })
  holder.state = state

  const appStore = {
    sidebarScrollTop: 0,
    get sidebarCollapsed() {
      return state.sidebarCollapsed
    },
    get mobileOpen() {
      return state.mobileOpen
    },
    get backendModeEnabled() {
      return state.backendModeEnabled
    },
    get siteVersion() {
      return '0.0.0-test'
    },
    get cachedPublicSettings() {
      return state.publicSettings
    },
    toggleSidebar() {
      state.sidebarCollapsed = !state.sidebarCollapsed
    },
    setMobileOpen(open: boolean) {
      state.mobileOpen = open
    }
  }

  const authStore = {
    get isAdmin() {
      return state.isAdmin
    },
    get isSimpleMode() {
      return state.isSimpleMode
    }
  }

  const adminSettingsStore = {
    get opsMonitoringEnabled() {
      return state.opsMonitoringEnabled
    },
    fetch: vi.fn()
  }

  return {
    useAppStore: () => appStore,
    useAuthStore: () => authStore,
    useAdminSettingsStore: () => adminSettingsStore
  }
})

vi.mock('@/stores/app', async () => {
  const stores = await import('@/stores')
  return { useAppStore: stores.useAppStore }
})

// vitest 用的是 vue-i18n 运行时版（见 vitest.config.ts 的 alias），无法在运行时编译
// 消息。这里直接用真实的中文文案表查 key：既拿到真实标签，又能在 key 拼错/缺失时暴露出来。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const zh = (await import('@/i18n/locales/zh')).default as Record<string, unknown>

  const lookup = (key: string): unknown =>
    key.split('.').reduce<unknown>((acc, part) => {
      if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[part]
      return undefined
    }, zh)

  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh' },
      t: (key: string) => {
        const value = lookup(key)
        if (typeof value !== 'string') throw new Error(`missing locale key: ${key}`)
        return value
      }
    })
  }
})

import AppSidebar, { resetExpandedNavGroups } from '../AppSidebar.vue'

const LABELS = {
  dashboard: '仪表盘',
  ops: '运维监控',
  usage: '使用记录',
  users: '用户管理',
  adminApiKeys: '密钥总表',
  groups: '分组管理',
  channels: '渠道管理',
  channelPricing: '渠道定价',
  channelMonitor: '渠道监控',
  accounts: '账号管理',
  modelAvailability: '模型可用性',
  proxies: 'IP管理',
  securityAudit: '安全审计',
  promptAudit: '提示词审计',
  auditLogs: '操作日志',
  settings: '系统设置',
  personalKeys: 'API 密钥'
} as const

const GROUP_TITLES = {
  access: '接入配置',
  upstream: '上游资源',
  runtime: '运行状态',
  security: '安全与审计'
} as const

function resetState(): void {
  holder.state.isAdmin = true
  holder.state.isSimpleMode = false
  holder.state.sidebarCollapsed = false
  holder.state.mobileOpen = false
  holder.state.backendModeEnabled = false
  holder.state.opsMonitoringEnabled = true
  holder.state.publicSettings = {
    channel_monitor_enabled: true,
    risk_control_enabled: true,
    available_channels_enabled: true
  }
  resetExpandedNavGroups()
}

async function mountSidebar(path = '/admin/dashboard') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
  })
  await router.push(path)
  await router.isReady()

  return mount(AppSidebar, {
    global: {
      plugins: [router],
      stubs: { VersionBadge: true }
    }
  })
}

/** 渲染出来的分组 key，按 DOM 顺序。 */
function sectionKeys(wrapper: VueWrapper): (string | undefined)[] {
  return wrapper.findAll('[data-section]').map((node) => node.attributes('data-section'))
}

/** 分组标题文案；无标题分组返回 null。 */
function sectionTitle(wrapper: VueWrapper, key: string): string | null {
  const toggle = wrapper.find(`[data-section="${key}"] [data-section-toggle]`)
  return toggle.exists() ? toggle.text() : null
}

/** 分组下的一级菜单项文案（不含二级子项）。 */
function sectionEntries(wrapper: VueWrapper, key: string): string[] {
  const section = wrapper.find(`[data-section="${key}"]`)
  if (!section.exists()) return []
  return Array.from(
    section.element.querySelectorAll(':scope > a.sidebar-link, :scope > button.sidebar-link')
  ).map((node) => (node.textContent ?? '').trim())
}

/** 分组下某个可折叠菜单项展开后的二级子项文案。 */
function sectionChildEntries(wrapper: VueWrapper, key: string): string[] {
  const section = wrapper.find(`[data-section="${key}"]`)
  if (!section.exists()) return []
  return Array.from(section.element.querySelectorAll('.border-l a')).map((node) =>
    (node.textContent ?? '').trim()
  )
}

function isSectionExpanded(wrapper: VueWrapper, key: string): boolean {
  const toggle = wrapper.find(`[data-section="${key}"] [data-section-toggle]`)
  return toggle.exists() ? toggle.attributes('aria-expanded') === 'true' : true
}

async function expandSection(wrapper: VueWrapper, key: string): Promise<void> {
  if (!isSectionExpanded(wrapper, key)) {
    await wrapper.find(`[data-section="${key}"] [data-section-toggle]`).trigger('click')
  }
}

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar admin nav grouping', () => {
  beforeEach(() => {
    resetState()
  })

  it('renders the four admin groups in order, plus an untitled section for settings', async () => {
    const wrapper = await mountSidebar()

    expect(sectionKeys(wrapper)).toEqual(['access', 'upstream', 'runtime', 'security', 'system'])
    expect(sectionTitle(wrapper, 'access')).toBe(GROUP_TITLES.access)
    expect(sectionTitle(wrapper, 'upstream')).toBe(GROUP_TITLES.upstream)
    expect(sectionTitle(wrapper, 'runtime')).toBe(GROUP_TITLES.runtime)
    expect(sectionTitle(wrapper, 'security')).toBe(GROUP_TITLES.security)
    expect(sectionTitle(wrapper, 'system')).toBeNull()
  })

  it('puts every admin entry in the expected group', async () => {
    const wrapper = await mountSidebar()
    await expandSection(wrapper, 'security')

    expect(sectionEntries(wrapper, 'access')).toEqual([
      LABELS.users,
      LABELS.adminApiKeys,
      LABELS.groups,
      LABELS.channels
    ])
    expect(sectionEntries(wrapper, 'upstream')).toEqual([
      LABELS.accounts,
      LABELS.modelAvailability,
      LABELS.proxies
    ])
    expect(sectionEntries(wrapper, 'runtime')).toEqual([
      LABELS.dashboard,
      LABELS.ops,
      LABELS.usage
    ])
    expect(sectionEntries(wrapper, 'security')).toEqual([LABELS.securityAudit, LABELS.auditLogs])
  })

  it('keeps the existing sub-items under 渠道管理 and 安全审计', async () => {
    const wrapper = await mountSidebar()

    // 渠道管理：点父项只展开，不跳转
    await wrapper.find(`[data-section="access"] button.sidebar-link`).trigger('click')
    expect(sectionChildEntries(wrapper, 'access')).toEqual([
      LABELS.channelPricing,
      LABELS.channelMonitor
    ])

    await expandSection(wrapper, 'security')
    await wrapper.find(`[data-section="security"] button.sidebar-link`).trigger('click')
    expect(sectionChildEntries(wrapper, 'security')).toEqual([LABELS.promptAudit])
  })

  it('links 模型可用性 to /admin/model-availability', async () => {
    const wrapper = await mountSidebar()

    const link = wrapper.find('a[href="/admin/model-availability"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toBe(LABELS.modelAvailability)
  })

  it('keeps 系统设置 at the bottom and outside every group', async () => {
    const wrapper = await mountSidebar()
    await expandSection(wrapper, 'security')

    const keys = sectionKeys(wrapper)
    expect(keys[keys.length - 1]).toBe('system')
    expect(sectionEntries(wrapper, 'system')).toEqual([LABELS.settings])

    for (const key of ['access', 'upstream', 'runtime', 'security']) {
      expect(sectionEntries(wrapper, key)).not.toContain(LABELS.settings)
    }
  })
})

describe('AppSidebar admin group expand/collapse', () => {
  beforeEach(() => {
    resetState()
  })

  it('expands the three daily-use groups by default and collapses 安全与审计', async () => {
    const wrapper = await mountSidebar()

    expect(isSectionExpanded(wrapper, 'access')).toBe(true)
    expect(isSectionExpanded(wrapper, 'upstream')).toBe(true)
    expect(isSectionExpanded(wrapper, 'runtime')).toBe(true)
    expect(isSectionExpanded(wrapper, 'security')).toBe(false)

    // 折叠的分组不渲染菜单项，但标题还在
    expect(sectionEntries(wrapper, 'security')).toEqual([])
    expect(sectionTitle(wrapper, 'security')).toBe(GROUP_TITLES.security)
  })

  it('toggles a group open and closed on title click', async () => {
    const wrapper = await mountSidebar()
    const toggle = wrapper.find('[data-section="runtime"] [data-section-toggle]')

    await toggle.trigger('click')
    expect(isSectionExpanded(wrapper, 'runtime')).toBe(false)
    expect(sectionEntries(wrapper, 'runtime')).toEqual([])

    await toggle.trigger('click')
    expect(isSectionExpanded(wrapper, 'runtime')).toBe(true)
    expect(sectionEntries(wrapper, 'runtime')).toEqual([
      LABELS.dashboard,
      LABELS.ops,
      LABELS.usage
    ])
  })

  it('does not navigate when a group title is clicked', async () => {
    const wrapper = await mountSidebar()
    const toggle = wrapper.find('[data-section="access"] [data-section-toggle]')

    expect(toggle.element.tagName).toBe('BUTTON')
    expect(toggle.attributes('href')).toBeUndefined()

    await toggle.trigger('click')
    expect(wrapper.vm.$route.path).toBe('/admin/dashboard')
  })

  it('remembers the collapsed state across remounts (AppSidebar remounts on every route change)', async () => {
    const first = await mountSidebar('/admin/dashboard')
    await first.find('[data-section="access"] [data-section-toggle]').trigger('click')
    expect(isSectionExpanded(first, 'access')).toBe(false)
    first.unmount()

    const second = await mountSidebar('/admin/accounts')
    expect(isSectionExpanded(second, 'access')).toBe(false)
    expect(sectionEntries(second, 'access')).toEqual([])
  })
})

describe('AppSidebar collapsed rail', () => {
  beforeEach(() => {
    resetState()
    holder.state.sidebarCollapsed = true
  })

  it('degrades group titles to a divider and hides the chevron', async () => {
    const wrapper = await mountSidebar()
    const toggle = wrapper.find('[data-section="access"] [data-section-toggle]')

    expect(toggle.classes()).toContain('sidebar-section-title-collapsed')
    expect(toggle.attributes('disabled')).toBeDefined()
    expect(toggle.find('.sidebar-section-title-text').attributes('aria-hidden')).toBe('true')
    expect(toggle.find('.sidebar-section-title-text').classes()).toContain(
      'sidebar-section-title-text-collapsed'
    )
    // 收起态没有展开箭头（唯一的 svg 就是箭头）
    expect(toggle.find('svg').exists()).toBe(false)
  })

  it('still renders every entry as an icon, including default-collapsed groups', async () => {
    const wrapper = await mountSidebar()

    expect(sectionEntries(wrapper, 'security')).toEqual([LABELS.securityAudit, LABELS.auditLogs])
    expect(sectionEntries(wrapper, 'upstream')).toEqual([
      LABELS.accounts,
      LABELS.modelAvailability,
      LABELS.proxies
    ])
    // 标签被 CSS 收掉，但 DOM 里仍在（收起动画沿用原有的 sidebar-label 处理）
    const firstLink = wrapper.find('[data-section="upstream"] a.sidebar-link')
    expect(firstLink.classes()).toContain('sidebar-link-collapsed')
    expect(firstLink.find('.sidebar-label').classes()).toContain('sidebar-label-collapsed')
  })

  it('ignores clicks on a group title while collapsed', async () => {
    const wrapper = await mountSidebar()
    const toggle = wrapper.find('[data-section="access"] [data-section-toggle]')

    await toggle.trigger('click')
    expect(sectionEntries(wrapper, 'access')).toEqual([
      LABELS.users,
      LABELS.adminApiKeys,
      LABELS.groups,
      LABELS.channels
    ])
  })
})

describe('AppSidebar feature flags', () => {
  beforeEach(() => {
    resetState()
  })

  it('drops 运维监控 when ops monitoring is disabled', async () => {
    holder.state.opsMonitoringEnabled = false
    const wrapper = await mountSidebar()

    expect(sectionEntries(wrapper, 'runtime')).toEqual([LABELS.dashboard, LABELS.usage])
    expect(wrapper.text()).not.toContain(LABELS.ops)
  })

  it('drops the 渠道监控 sub-item when channel monitor is disabled', async () => {
    holder.state.publicSettings = { ...holder.state.publicSettings, channel_monitor_enabled: false }
    const wrapper = await mountSidebar()

    await wrapper.find('[data-section="access"] button.sidebar-link').trigger('click')
    expect(sectionChildEntries(wrapper, 'access')).toEqual([LABELS.channelPricing])
    expect(sectionEntries(wrapper, 'access')).toContain(LABELS.channels)
  })

  it('drops 安全审计 when risk control is disabled but keeps the group for 操作日志', async () => {
    holder.state.publicSettings = { ...holder.state.publicSettings, risk_control_enabled: false }
    const wrapper = await mountSidebar()
    await expandSection(wrapper, 'security')

    expect(sectionEntries(wrapper, 'security')).toEqual([LABELS.auditLogs])
    expect(wrapper.text()).not.toContain(LABELS.promptAudit)
  })

  it('never renders a titled group with zero entries', async () => {
    const combos = [
      { ops: true, channelMonitor: true, riskControl: true },
      { ops: false, channelMonitor: true, riskControl: true },
      { ops: true, channelMonitor: false, riskControl: true },
      { ops: true, channelMonitor: true, riskControl: false },
      { ops: false, channelMonitor: false, riskControl: false }
    ]

    for (const combo of combos) {
      resetState()
      holder.state.opsMonitoringEnabled = combo.ops
      holder.state.publicSettings = {
        channel_monitor_enabled: combo.channelMonitor,
        risk_control_enabled: combo.riskControl,
        available_channels_enabled: true
      }

      const wrapper = await mountSidebar()
      for (const key of sectionKeys(wrapper)) {
        if (!key || sectionTitle(wrapper, key) === null) continue
        await expandSection(wrapper, key)
        expect(sectionEntries(wrapper, key).length, `group ${key} is an empty shell`).toBeGreaterThan(0)
      }
      wrapper.unmount()
    }
  })
})

describe('AppSidebar simple mode', () => {
  beforeEach(() => {
    resetState()
    holder.state.isSimpleMode = true
  })

  it('keeps the flat trimmed list without any group titles', async () => {
    const wrapper = await mountSidebar()

    expect(sectionKeys(wrapper)).toEqual(['simple'])
    expect(wrapper.findAll('[data-section-toggle]')).toHaveLength(0)
    expect(sectionEntries(wrapper, 'simple')).toEqual([
      LABELS.dashboard,
      LABELS.ops,
      LABELS.accounts,
      LABELS.proxies,
      LABELS.securityAudit,
      LABELS.usage,
      LABELS.personalKeys,
      LABELS.settings
    ])
  })

  it('hides hideInSimpleMode entries, including the new 模型可用性', async () => {
    const wrapper = await mountSidebar()
    const text = wrapper.text()

    for (const label of [
      LABELS.users,
      LABELS.adminApiKeys,
      LABELS.groups,
      LABELS.channels,
      LABELS.modelAvailability,
      LABELS.auditLogs
    ]) {
      expect(text, `${label} should be hidden in simple mode`).not.toContain(label)
    }
    // 简单模式没有"我的账户"区
    expect(text).not.toContain('我的账户')
  })

  it('still honors feature flags in simple mode', async () => {
    holder.state.opsMonitoringEnabled = false
    const wrapper = await mountSidebar()

    expect(sectionEntries(wrapper, 'simple')).toEqual([
      LABELS.dashboard,
      LABELS.accounts,
      LABELS.proxies,
      LABELS.securityAudit,
      LABELS.usage,
      LABELS.personalKeys,
      LABELS.settings
    ])
  })
})

describe('AppSidebar non-admin view', () => {
  beforeEach(() => {
    resetState()
    holder.state.isAdmin = false
  })

  it('renders the user menu without admin groups', async () => {
    const wrapper = await mountSidebar('/dashboard')

    expect(sectionKeys(wrapper)).toEqual([])
    expect(wrapper.findAll('[data-section-toggle]')).toHaveLength(0)
    expect(wrapper.text()).toContain(LABELS.personalKeys)
    expect(wrapper.text()).not.toContain(GROUP_TITLES.access)
  })
})
