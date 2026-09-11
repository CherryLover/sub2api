<template>
  <aside
    class="sidebar"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { '-translate-x-full lg:translate-x-0': !mobileOpen }
    ]"
  >
    <!-- Logo/Brand -->
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': sidebarCollapsed }">
      <!-- Custom Logo or Default Logo -->
      <router-link
        :to="homePath"
        class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl shadow-glow transition-opacity hover:opacity-80"
        @click="handleMenuItemClick()"
      >
        <img src="/logo.svg" alt="Logo" class="h-full w-full object-contain" />
      </router-link>
      <div class="sidebar-brand" :class="{ 'sidebar-brand-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
        <router-link
          :to="homePath"
          class="sidebar-brand-title text-lg font-bold text-gray-900 transition-colors hover:text-primary-600 dark:text-white dark:hover:text-primary-400"
          @click="handleMenuItemClick()"
        >
          {{ SITE_NAME }}
        </router-link>
        <!-- Version Badge -->
        <VersionBadge :version="siteVersion" />
      </div>
    </div>

    <!-- Navigation -->
    <nav ref="sidebarNavRef" class="sidebar-nav scrollbar-hide">
      <!-- Admin View: Admin menu first, then personal menu -->
      <template v-if="isAdmin">
        <!--
          Admin sections.
          完整模式：四个「纯标题」分组（接入配置 / 上游资源 / 运行状态 / 安全与审计）+
          末尾一个无标题分组放系统设置。
          简单模式：退化成一个无标题分组，沿用原来的平铺精简列表。
          无标题分组（section.label 为空）永远展开，不渲染可点击的标题。
        -->
        <div
          v-for="section in adminNavSections"
          :key="section.key"
          class="sidebar-section"
          :class="{ 'sidebar-section-grouped': !!section.label }"
          :data-section="section.key"
        >
          <!-- 分组标题：纯标题，只切换展开/折叠，不导航；侧边栏收起时降级为分隔线 -->
          <button
            v-if="section.label"
            type="button"
            class="sidebar-section-title sidebar-section-toggle"
            :class="{
              'sidebar-section-title-collapsed': sidebarCollapsed,
              'sidebar-section-toggle-active':
                !sidebarCollapsed && !isSectionExpanded(section) && isSectionActive(section)
            }"
            :disabled="sidebarCollapsed"
            :aria-expanded="isSectionExpanded(section) ? 'true' : 'false'"
            :title="sidebarCollapsed ? section.label : undefined"
            data-section-toggle
            @click="toggleSection(section)"
          >
            <span
              class="sidebar-section-title-text"
              :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }"
              :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
            >
              {{ section.label }}
            </span>
            <ChevronDownIcon
              v-if="!sidebarCollapsed"
              class="h-3.5 w-3.5 flex-shrink-0 transition-transform duration-200"
              :class="isSectionExpanded(section) ? 'rotate-180' : ''"
            />
          </button>

          <template v-if="isSectionExpanded(section)">
            <template v-for="item in section.items" :key="item.path">
              <!-- Collapsible group (has children) -->
              <template v-if="item.children?.length">
                <button
                  type="button"
                  class="sidebar-link mb-1 w-full"
                  :class="{
                    'sidebar-link-active': isGroupActive(item) && !isGroupExpanded(item),
                    'sidebar-link-collapsed': sidebarCollapsed
                  }"
                  :title="sidebarCollapsed ? item.label : undefined"
                  @click="handleGroupClick(item)"
                >
                  <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
                  <span
                    class="sidebar-label sidebar-label-flex"
                    :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
                    :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
                  >
                    <span class="min-w-0 truncate">{{ item.label }}</span>
                    <ChevronDownIcon
                      class="h-4 w-4 flex-shrink-0 transition-transform duration-200"
                      :class="isGroupExpanded(item) ? 'rotate-180' : ''"
                    />
                  </span>
                </button>
                <!-- Children -->
                <div v-if="!sidebarCollapsed && isGroupExpanded(item)" class="mb-1 ml-4 border-l border-gray-200 pl-2 dark:border-dark-600">
                  <router-link
                    v-for="child in item.children"
                    :key="child.path"
                    :to="child.path"
                    class="sidebar-link mb-0.5 py-1.5 text-sm"
                    :class="{ 'sidebar-link-active': route.path === child.path }"
                    @click="handleMenuItemClick()"
                  >
                    <component :is="child.icon" class="h-4 w-4 flex-shrink-0" />
                    <span>{{ child.label }}</span>
                  </router-link>
                </div>
              </template>
              <!-- Normal item (no children) -->
              <router-link
                v-else
                :to="item.path"
                class="sidebar-link mb-1"
                :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
                :title="sidebarCollapsed ? item.label : undefined"
                @click="handleMenuItemClick()"
              >
                <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
                <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
              </router-link>
            </template>
          </template>
        </div>

        <!-- Personal Section for Admin (hidden in simple mode) -->
        <div v-if="!authStore.isSimpleMode" class="sidebar-section">
          <div class="sidebar-section-title" :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
            <span class="sidebar-section-title-text" :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }">
              {{ t('nav.myAccount') }}
            </span>
          </div>

          <router-link
            v-for="item in personalNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined"
            @click="handleMenuItemClick()"
          >
            <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
          </router-link>
        </div>
      </template>

      <!-- Regular User View -->
      <template v-else-if="!appStore.backendModeEnabled">
        <div class="sidebar-section">
          <router-link
            v-for="item in userNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined"
            @click="handleMenuItemClick()"
          >
            <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
          </router-link>
        </div>
      </template>
    </nav>

    <!-- Bottom Section -->
    <div class="mt-auto border-t border-gray-100 p-3 dark:border-dark-800">
      <!-- Theme Toggle -->
      <button
        @click="toggleTheme"
        class="sidebar-link mb-2 w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? (isDark ? t('nav.lightMode') : t('nav.darkMode')) : undefined"
      >
        <SunIcon v-if="isDark" class="h-5 w-5 flex-shrink-0 text-amber-500" />
        <MoonIcon v-else class="h-5 w-5 flex-shrink-0" />
        <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{
          isDark ? t('nav.lightMode') : t('nav.darkMode')
        }}</span>
      </button>

      <!-- Collapse Button -->
      <button
        @click="toggleSidebar"
        class="sidebar-link w-full"
        :class="{ 'sidebar-link-collapsed': sidebarCollapsed }"
        :title="sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
      >
        <ChevronDoubleLeftIcon v-if="!sidebarCollapsed" class="h-5 w-5 flex-shrink-0" />
        <ChevronDoubleRightIcon v-else class="h-5 w-5 flex-shrink-0" />
        <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ t('nav.collapse') }}</span>
      </button>
    </div>
  </aside>

  <!-- Mobile Overlay -->
  <transition name="fade">
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-30 bg-black/50 lg:hidden"
      @click="closeMobile"
    ></div>
  </transition>
</template>

<script lang="ts">
import { ref } from 'vue'

/**
 * 管理端导航分组的稳定 key。
 * - access / upstream / runtime / security：四个「纯标题」分组（不可点击跳转，只归类）
 * - system：无标题分组，系统设置固定留在最下面，不属于任何分组
 * - simple：无标题分组，简单模式下的平铺精简列表
 */
export const AdminNavSectionKeys = {
  access: 'access',
  upstream: 'upstream',
  runtime: 'runtime',
  security: 'security',
  system: 'system',
  simple: 'simple'
} as const

/**
 * 默认展开的分组：管理员日常都会用到的三组默认展开（接入配置 / 上游资源 / 运行状态），
 * 低频的「安全与审计」默认折叠，避免首屏比改版前更长。
 */
export const DEFAULT_EXPANDED_SECTIONS: string[] = [
  AdminNavSectionKeys.access,
  AdminNavSectionKeys.upstream,
  AdminNavSectionKeys.runtime
]

/** 分组展开状态在 expandedNavGroups 里的 key，和菜单项分组（用 path 做 key）区分开。 */
export function sectionStateKey(key: string): string {
  return `section:${key}`
}

/**
 * 展开/折叠状态放在模块作用域而不是 setup 内部。
 * AppSidebar 随 AppLayout 挂在每个页面组件内部，路由切换会整体重新挂载
 * （这也是滚动位置要存进 appStore 的原因）。放模块作用域后，用户折叠某个分组的选择
 * 能在整个会话里保持，而不是点一次菜单就被重置回默认值。
 * 只做会话内记忆，不新增 localStorage 持久化——沿用原来的做法。
 */
export const expandedNavGroups = ref<Set<string>>(
  new Set(DEFAULT_EXPANDED_SECTIONS.map(sectionStateKey))
)

/** 仅供测试使用：把展开状态复位到默认值。 */
export function resetExpandedNavGroups(): void {
  expandedNavGroups.value = new Set(DEFAULT_EXPANDED_SECTIONS.map(sectionStateKey))
}
</script>

<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import { SITE_NAME } from '@/constants/site'
import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'

interface NavItem {
  path: string
  label: string
  icon: unknown
  hideInSimpleMode?: boolean
  children?: NavItem[]
  /**
   * When true, the parent item only toggles the expand/collapse state and
   * does NOT navigate to its `path`. The `path` is purely a stable key.
   */
  expandOnly?: boolean
  /**
   * 可选的功能开关 getter。返回 false 时菜单项被隐藏；返回 undefined/true 时显示。
   * 宽容策略（undefined → 显示）避免 public settings 未加载完成时菜单闪烁消失。
   * Getter 里访问的 reactive 来源（store / composable）会被 computed 自动追踪，
   * 开关切换时菜单自动更新。
   */
  featureFlag?: () => boolean | undefined
}

/**
 * 侧边栏的一段分组。
 * - `label` 有值：渲染成可折叠的「纯标题」（不可点击跳转，只切换展开状态）
 * - `label` 为空：无标题分组，永远展开（用于系统设置、简单模式的平铺列表）
 */
interface NavSection {
  /** 稳定 key，用于展开状态记忆与测试定位 */
  key: string
  label?: string
  items: NavItem[]
}

// applyFeatureFlags 递归过滤掉 featureFlag() === false 的节点（含子节点）。
// 使用 `!== false` 宽容语义：undefined（设置未加载）或 true 都视为显示。
function applyFeatureFlags(items: NavItem[]): NavItem[] {
  const out: NavItem[] = []
  for (const item of items) {
    if (item.featureFlag && item.featureFlag() === false) continue
    if (item.children) {
      out.push({ ...item, children: applyFeatureFlags(item.children) })
    } else {
      out.push(item)
    }
  }
  return out
}

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const adminSettingsStore = useAdminSettingsStore()

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const mobileOpen = computed(() => appStore.mobileOpen)
const isAdmin = computed(() => authStore.isAdmin)
const sidebarNavRef = ref<HTMLElement | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))

const homePath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))

// 展开状态见文件顶部的 expandedNavGroups（模块作用域，跨路由切换保持）。
// 菜单项分组用 item.path 做 key，标题分组用 sectionStateKey(section.key)。

const siteVersion = computed(() => appStore.siteVersion)

// SVG Icon Components
const DashboardIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z'
        })
      ]
    )
}

const KeyIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z'
        })
      ]
    )
}

const ChartIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z'
        })
      ]
    )
}

const UserIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z'
        })
      ]
    )
}

const UsersIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z'
        })
      ]
    )
}

const FolderIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z'
        })
      ]
    )
}

const ChannelIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M6.429 9.75L2.25 12l4.179 2.25m0-4.5l5.571 3 5.571-3m-11.142 0L2.25 7.5 12 2.25l9.75 5.25-4.179 2.25m0 0l4.179 2.25L12 17.25 2.25 12m15.321-2.25l4.179 2.25L12 17.25l-9.75-5.25'
        })
      ]
    )
}

const GlobeIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418'
        })
      ]
    )
}

// 模型可用性：用芯片图标表达"模型/算力"，和账号(地球)、代理(服务器)区分开
const CpuChipIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M8.25 3v1.5M4.5 8.25H3m18 0h-1.5M4.5 12H3m18 0h-1.5m-15 3.75H3m18 0h-1.5M8.25 19.5V21M12 3v1.5m0 15V21m3.75-18v1.5m0 15V21m-9-1.5h10.5a2.25 2.25 0 002.25-2.25V6.75a2.25 2.25 0 00-2.25-2.25H6.75A2.25 2.25 0 004.5 6.75v10.5a2.25 2.25 0 002.25 2.25zm.75-12h9v9h-9v-9z'
        })
      ]
    )
}

const ServerIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z'
        })
      ]
    )
}

const CogIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z'
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z'
        })
      ]
    )
}

const SunIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z'
        })
      ]
    )
}

const MoonIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z'
        })
      ]
    )
}

const ChevronDoubleLeftIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm18.75 4.5-7.5 7.5 7.5 7.5m-6-15L5.25 12l7.5 7.5'
        })
      ]
    )
}

const ChevronDoubleRightIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm5.25 4.5 7.5 7.5-7.5 7.5m6-15 7.5 7.5-7.5 7.5'
        })
      ]
    )
}

const SignalIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.348 14.651a3.75 3.75 0 010-5.303m5.304 0a3.75 3.75 0 010 5.303m-7.425 2.122a6.75 6.75 0 010-9.546m9.546 0a6.75 6.75 0 010 9.546M5.106 18.894c-3.808-3.807-3.808-9.98 0-13.788m13.788 0c3.808 3.807 3.808 9.98 0 13.788M12 12h.008v.008H12V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z'
        })
      ]
    )
}

const ShieldIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'
        })
      ]
    )
}

const PriceTagIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.568 3H5.25A2.25 2.25 0 003 5.25v4.318c0 .597.237 1.17.659 1.591l9.581 9.581c.699.699 1.78.872 2.607.33a18.095 18.095 0 005.223-5.223c.542-.827.369-1.908-.33-2.607L11.16 3.66A2.25 2.25 0 009.568 3z'
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M6 6h.008v.008H6V6z'
        })
      ]
    )
}

const ChevronDownIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm19.5 8.25-7.5 7.5-7.5-7.5'
        })
      ]
    )
}

// Public-settings flags go through the registry in utils/featureFlags.ts,
// which handles the opt-in vs opt-out fallback when settings haven't loaded
// yet. Admin-only flags (not in public settings) stay inline below.
const flagChannelMonitor = makeSidebarFlag(FeatureFlags.channelMonitor)
const flagAvailableChannels = makeSidebarFlag(FeatureFlags.availableChannels)
const flagRiskControl = makeSidebarFlag(FeatureFlags.riskControl)
const flagOpsMonitoring = () => adminSettingsStore.opsMonitoringEnabled

// buildSelfNavItems 构造用户自己的导航项（用户端主菜单和管理员的"我的账户"子菜单共享这组声明）。
// userSide=true 是普通用户的主菜单，包含仪表盘和用量；false 是管理员的"我的账户"区，
// 两者都不含——管理端已有 /admin/dashboard 与 /admin/usage，用户端的这两页对管理员
// 只是重复入口（路由守卫也会把管理员从 /dashboard、/usage 转到管理端对应页）。
// /keys 管理员自己也用，保留。
//
// 条目顺序：密钥 → 用量 → 可用渠道 → 渠道状态 → 资料。
// 可用渠道紧挨渠道状态之上，让用户"先看自己能用什么、再看对应状态"。
function buildSelfNavItems(userSide: boolean): NavItem[] {
  const items: NavItem[] = []
  if (userSide) {
    items.push({ path: '/dashboard', label: t('nav.dashboard'), icon: DashboardIcon })
  }
  items.push({ path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon })
  if (userSide) {
    items.push({ path: '/usage', label: t('nav.usage'), icon: ChartIcon, hideInSimpleMode: true })
  }
  items.push(
    { path: '/available-channels', label: t('nav.availableChannels'), icon: ChannelIcon, hideInSimpleMode: true, featureFlag: flagAvailableChannels },
    { path: '/monitor', label: t('nav.channelStatus'), icon: SignalIcon, featureFlag: flagChannelMonitor },
    { path: '/profile', label: t('nav.profile'), icon: UserIcon },
  )
  return items
}

// finalizeNav 合并三重过滤：featureFlag 过滤 + simple 模式过滤。
function finalizeNav(items: NavItem[]): NavItem[] {
  const visible = applyFeatureFlags(items)
  return authStore.isSimpleMode ? visible.filter(item => !item.hideInSimpleMode) : visible
}

// User navigation items (for regular users)
const userNavItems = computed((): NavItem[] => finalizeNav(buildSelfNavItems(true)))

// Personal navigation items (for admin's "My Account" section, without Dashboard/Usage).
// Admins access 可用渠道 from this section just like regular users — there is no
// separate admin entry, since the page is purely a user-facing view.
const personalNavItems = computed((): NavItem[] => finalizeNav(buildSelfNavItems(false)))

// buildAdminNavCatalog 是管理端每个入口的唯一声明处。
// 简单模式的平铺精简列表和完整模式的分组视图都从这里取，避免两套声明各自漂移。
function buildAdminNavCatalog(): Record<string, NavItem> {
  return {
    dashboard: { path: '/admin/dashboard', label: t('nav.dashboard'), icon: DashboardIcon },
    ops: { path: '/admin/ops', label: t('nav.ops'), icon: ChartIcon, featureFlag: flagOpsMonitoring },
    users: { path: '/admin/users', label: t('nav.users'), icon: UsersIcon, hideInSimpleMode: true },
    apiKeys: { path: '/admin/api-keys', label: t('nav.adminApiKeys'), icon: KeyIcon, hideInSimpleMode: true },
    groups: { path: '/admin/groups', label: t('nav.groups'), icon: FolderIcon, hideInSimpleMode: true },
    channels: {
      path: '/admin/channels',
      label: t('nav.channelManagement'),
      icon: ChannelIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      children: [
        { path: '/admin/channels/pricing', label: t('nav.channelPricing'), icon: PriceTagIcon },
        { path: '/admin/channels/monitor', label: t('nav.channelMonitor'), icon: SignalIcon, featureFlag: flagChannelMonitor },
      ],
    },
    accounts: { path: '/admin/accounts', label: t('nav.accounts'), icon: GlobeIcon },
    // 模型可用性：进阶排查页，和简单模式的"只保留最常用入口"定位不符，故简单模式下隐藏。
    modelAvailability: {
      path: '/admin/model-availability',
      label: t('nav.modelAvailability'),
      icon: CpuChipIcon,
      hideInSimpleMode: true,
    },
    proxies: { path: '/admin/proxies', label: t('nav.proxies'), icon: ServerIcon },
    securityAudit: {
      path: '/admin/security-audit',
      label: t('nav.securityAudit'),
      icon: ShieldIcon,
      expandOnly: true,
      featureFlag: flagRiskControl,
      children: [
        { path: '/admin/prompt-audit', label: t('nav.promptAudit'), icon: ShieldIcon },
      ],
    },
    usage: { path: '/admin/usage', label: t('nav.usage'), icon: ChartIcon },
    auditLogs: { path: '/admin/audit-logs', label: t('nav.auditLogs'), icon: ShieldIcon, hideInSimpleMode: true },
    settings: { path: '/admin/settings', label: t('nav.settings'), icon: CogIcon },
  }
}

// 简单模式下的管理端菜单：保持改版前的平铺精简列表（顺序与内容都不变）。
// hideInSimpleMode 的项会被过滤掉，最后补上 API 密钥和系统设置。
const adminSimpleNavItems = computed((): NavItem[] => {
  const c = buildAdminNavCatalog()
  const baseItems: NavItem[] = [
    c.dashboard, c.ops, c.users, c.apiKeys, c.groups, c.channels,
    c.accounts, c.modelAvailability, c.proxies, c.securityAudit, c.usage, c.auditLogs,
  ]

  const filtered = applyFeatureFlags(baseItems).filter(item => !item.hideInSimpleMode)
  filtered.push({ path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon })
  filtered.push(c.settings)
  return filtered
})

// 管理端导航分组。
// 完整模式：四个按"你在做什么事"划分的纯标题分组 + 末尾无标题分组（系统设置）。
// 简单模式：一个无标题分组，内容就是原来的平铺精简列表。
// featureFlag / hideInSimpleMode 过滤后为空的分组会被整段移除，不留空壳标题。
const adminNavSections = computed((): NavSection[] => {
  if (authStore.isSimpleMode) {
    return [{ key: AdminNavSectionKeys.simple, items: adminSimpleNavItems.value }]
  }

  const c = buildAdminNavCatalog()
  const grouped: NavSection[] = [
    {
      key: AdminNavSectionKeys.access,
      label: t('nav.groupAccess'),
      items: [c.users, c.apiKeys, c.groups, c.channels],
    },
    {
      key: AdminNavSectionKeys.upstream,
      label: t('nav.groupUpstream'),
      items: [c.accounts, c.modelAvailability, c.proxies],
    },
    {
      key: AdminNavSectionKeys.runtime,
      label: t('nav.groupRuntime'),
      items: [c.dashboard, c.ops, c.usage],
    },
    {
      key: AdminNavSectionKeys.security,
      label: t('nav.groupSecurity'),
      items: [c.securityAudit, c.auditLogs],
    },
  ]

  const sections = grouped
    .map(section => ({ ...section, items: finalizeNav(section.items) }))
    .filter(section => section.items.length > 0)

  // 系统设置单独留在最下面，不进任何分组
  sections.push({ key: AdminNavSectionKeys.system, items: [c.settings] })
  return sections
})

function toggleSidebar() {
  appStore.toggleSidebar()
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

function handleMenuItemClick() {
  if (mobileOpen.value) {
    setTimeout(() => {
      appStore.setMobileOpen(false)
    }, 150)
  }
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function isGroupActive(item: NavItem): boolean {
  if (!item.children) return false
  return item.children.some(child => route.path === child.path)
}

function isGroupExpanded(item: NavItem): boolean {
  return expandedNavGroups.value.has(item.path) || isGroupActive(item)
}

function toggleGroup(item: NavItem) {
  if (expandedNavGroups.value.has(item.path)) {
    expandedNavGroups.value.delete(item.path)
  } else {
    expandedNavGroups.value.add(item.path)
  }
}

/** 分组里是否有条目命中当前路由（含二级子项）。 */
function isSectionActive(section: NavSection): boolean {
  return section.items.some(item => {
    if (item.children?.length) {
      return isActive(item.path) || item.children.some(child => isActive(child.path))
    }
    return isActive(item.path)
  })
}

/**
 * 分组是否展开：
 * - 无标题分组（系统设置 / 简单模式列表）永远展开；
 * - 侧边栏收起成图标条时强制展开，否则图标会跟着标题一起消失；
 * - 其余情况读会话内记忆的展开状态。
 */
function isSectionExpanded(section: NavSection): boolean {
  if (!section.label) return true
  if (sidebarCollapsed.value) return true
  return expandedNavGroups.value.has(sectionStateKey(section.key))
}

/** 点击分组标题：只切换展开状态，不导航；侧边栏收起时不响应。 */
function toggleSection(section: NavSection) {
  if (sidebarCollapsed.value || !section.label) return
  const key = sectionStateKey(section.key)
  if (expandedNavGroups.value.has(key)) {
    expandedNavGroups.value.delete(key)
  } else {
    expandedNavGroups.value.add(key)
  }
}

/**
 * Click handler for collapsible parent items.
 * - When sidebar is collapsed: do nothing (children are not visible).
 * - When `expandOnly` is true: only toggle expand state.
 * - Otherwise (default): navigate to the parent path
 *   (router-link semantics) and ensure the group is expanded.
 */
function handleGroupClick(item: NavItem) {
  if (sidebarCollapsed.value) return
  if (item.expandOnly) {
    toggleGroup(item)
    return
  }
  // Push to path and ensure expanded
  if (route.path !== item.path) {
    router.push(item.path)
  }
  if (!expandedNavGroups.value.has(item.path)) {
    expandedNavGroups.value.add(item.path)
  }
}

// Initialize theme
const savedTheme = localStorage.getItem('theme')
if (
  savedTheme === 'dark' ||
  (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
) {
  isDark.value = true
  document.documentElement.classList.add('dark')
}

// Fetch admin settings (for feature-gated nav items like Ops).
watch(
  isAdmin,
  (v) => {
    if (v) {
      adminSettingsStore.fetch()
    }
  },
  { immediate: true }
)

onMounted(() => {
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
  // Restore sidebar scroll position after route change re-mounts the component
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  white-space: nowrap;
  transition:
    max-width 0.22s ease,
    opacity 0.14s ease,
    transform 0.14s ease;
  max-width: 12rem;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(229 231 235);
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity 0.18s ease;
}

.dark .sidebar-section-title::after {
  background: rgb(55 65 81);
}

/* 可折叠的分组标题：纯标题，只切换展开状态，不是链接 */
.sidebar-section-toggle {
  width: 100%;
  justify-content: space-between;
  gap: 0.5rem;
  padding-top: 0.25rem;
  padding-bottom: 0.25rem;
  border-radius: 0.5rem;
  cursor: pointer;
  transition: color 0.16s ease;
}

.sidebar-section-toggle:disabled {
  cursor: default;
}

.sidebar-section-toggle:hover:not(:disabled) {
  color: rgb(75 85 99);
}

.dark .sidebar-section-toggle:hover:not(:disabled) {
  color: rgb(209 213 219);
}

/* 分组被折叠、但当前页面就在这个分组里时，给标题一点提示色 */
.sidebar-section-toggle-active {
  color: rgb(79 70 229);
}

.dark .sidebar-section-toggle-active {
  color: rgb(129 140 248);
}

/* 分组之间的间距比大区块（管理端 / 我的账户）小一些，避免侧边栏被拉得过长 */
.sidebar-section-grouped {
  margin-bottom: 0.75rem;
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
  transition-delay: 0.08s;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width 0.2s ease,
    opacity 0.12s ease,
    transform 0.12s ease;
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}
</style>
