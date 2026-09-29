<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <SearchInput
            v-model="filterSearch"
            :placeholder="t('admin.apiKeys.searchPlaceholder')"
            class="w-full sm:w-64"
            @search="onFilterChange"
          />

          <!-- 用户筛选：输入邮箱模糊搜索后从下拉里选一个 -->
          <div ref="userSearchRef" class="relative w-full sm:w-56">
            <input
              v-model="userKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.apiKeys.userFilterPlaceholder')"
              data-test="user-filter-input"
              @input="debounceUserSearch"
              @focus="onUserFocus"
            />
            <button
              v-if="filterUserId"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              :aria-label="t('admin.apiKeys.clearUserFilter')"
              data-test="user-filter-clear"
              @click="clearUserFilter"
            >
              ✕
            </button>
            <div
              v-if="showUserDropdown && userResults.length > 0"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                v-for="u in userResults"
                :key="u.id"
                type="button"
                class="flex w-full items-center justify-between px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                :data-test="`user-option-${u.id}`"
                @click="selectUser(u)"
              >
                <span class="truncate">{{ u.email }}</span>
                <span class="ml-2 shrink-0 text-xs text-gray-400">#{{ u.id }}</span>
              </button>
            </div>
          </div>

          <Select
            :model-value="filterGroupId"
            class="w-44"
            :options="groupFilterOptions"
            @update:model-value="onGroupFilterChange"
          />
          <Select
            :model-value="filterStatus"
            class="w-40"
            :options="statusFilterOptions"
            @update:model-value="onStatusFilterChange"
          />
        </div>
      </template>

      <template #actions>
        <div class="flex justify-end gap-3">
          <button
            @click="loadApiKeys"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="apiKeys"
          :loading="loading"
          row-key="id"
          :actions-count="1"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ value, row }">
            <div class="flex items-center gap-1.5">
              <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <Icon
                v-if="hasIpRules(row)"
                name="shield"
                size="sm"
                class="text-blue-500"
                :title="t('admin.apiKeys.ipRestrictionEnabled')"
              />
            </div>
          </template>

          <template #cell-user="{ row }">
            <div v-if="row.user" class="min-w-0">
              <div class="truncate text-sm text-gray-900 dark:text-white">{{ row.user.email }}</div>
              <div v-if="row.user.username" class="truncate text-xs text-gray-500 dark:text-dark-400">
                {{ row.user.username }}
              </div>
            </div>
            <span v-else class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ row.user_id }}</span>
          </template>

          <!-- 密钥由服务端掩码后返回，这里原样显示，不再本地脱敏也不提供复制 -->
          <template #cell-key="{ value }">
            <code class="code text-xs" data-test="masked-key">{{ value }}</code>
          </template>

          <template #cell-group="{ row }">
            <GroupBadge
              v-if="row.group_id && row.group"
              :name="row.group.name"
              :platform="row.group.platform"
              :rate-multiplier="row.group.rate_multiplier"
              :peak-rate-enabled="row.group.peak_rate_enabled"
              :peak-start="row.group.peak_start"
              :peak-end="row.group.peak_end"
              :peak-rate-multiplier="row.group.peak_rate_multiplier"
            />
            <span v-else class="text-sm italic text-gray-400">{{ t('admin.apiKeys.noGroup') }}</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="text-sm">
              <div class="flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.apiKeys.today') }}:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.today_actual_cost ?? 0).toFixed(4) }}
                </span>
              </div>
              <div class="mt-0.5 flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.apiKeys.total') }}:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.total_actual_cost ?? 0).toFixed(4) }}
                </span>
              </div>
              <!-- 累计消费：quota_used 是后端无条件累加的用量账本 -->
              <div class="mt-0.5 flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.apiKeys.spent') }}:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (row.quota_used ?? 0).toFixed(4) }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span :class="[
              'badge',
              value === 'active' ? 'badge-success' :
              value === 'quota_exhausted' ? 'badge-warning' :
              value === 'expired' ? 'badge-danger' :
              'badge-gray'
            ]">
              {{ t('keys.status.' + value) }}
            </span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span v-if="value" class="text-sm text-gray-500 dark:text-dark-400">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <button
              @click="openKeyUsage(row)"
              class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-primary-50 hover:text-primary-600 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
            >
              <Icon name="chartBar" size="sm" />
              <span class="text-xs">{{ t('admin.apiKeys.viewUsage') }}</span>
            </button>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.apiKeys.noKeys')"
              :description="t('admin.apiKeys.noKeysDescription')"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { adminAPI } from '@/api/admin'
import type { AdminApiKeyListFilters } from '@/api/admin/apiKeys'
import type { BatchApiKeyUsageStats } from '@/api/admin/dashboard'
import type { SimpleUser } from '@/api/admin/usage'
import type { ApiKey, AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

// ---------- 列定义 ----------
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.apiKeys.columns.name'), sortable: true },
  { key: 'user', label: t('admin.apiKeys.columns.user'), sortable: false },
  { key: 'key', label: t('admin.apiKeys.columns.key'), sortable: false },
  { key: 'group', label: t('admin.apiKeys.columns.group'), sortable: false },
  { key: 'usage', label: t('admin.apiKeys.columns.usage'), sortable: true },
  { key: 'status', label: t('admin.apiKeys.columns.status'), sortable: true },
  { key: 'last_used_at', label: t('admin.apiKeys.columns.lastUsedAt'), sortable: true },
  { key: 'created_at', label: t('admin.apiKeys.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.apiKeys.columns.actions'), sortable: false }
])

// 表头列 key 与后端 sort_by 不同名的映射：「用量」列按今日用量排序
const SORT_BY_COLUMN: Record<string, string> = { usage: 'today_cost' }

// ---------- 列表状态 ----------
const apiKeys = ref<ApiKey[]>([])
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})
const sortState = ref({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// ---------- 筛选 ----------
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')
const filterUserId = ref<number | null>(null)

const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const showUserDropdown = ref(false)
const userSearchRef = ref<HTMLElement | null>(null)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null
let userSearchSequence = 0

const groupFilterOptions = computed(() => [
  { value: '', label: t('admin.apiKeys.allGroups') },
  { value: 0, label: t('admin.apiKeys.noGroup') },
  ...groups.value.map((g) => ({ value: g.id, label: g.name }))
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.apiKeys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') }
])

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null | undefined) => {
  filterGroupId.value = value === null || value === undefined || typeof value === 'boolean' ? '' : value
  onFilterChange()
}

const onStatusFilterChange = (value: string | number | boolean | null | undefined) => {
  filterStatus.value = typeof value === 'string' ? value : ''
  onFilterChange()
}

const clearPendingUserSearch = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
    userSearchTimeout = null
  }
  userSearchSequence += 1
}

const debounceUserSearch = () => {
  clearPendingUserSearch()
  const query = userKeyword.value.trim()
  if (!query) {
    userResults.value = []
    showUserDropdown.value = false
    // 清空输入即视为取消用户筛选
    if (filterUserId.value !== null) {
      filterUserId.value = null
      onFilterChange()
    }
    return
  }
  const sequence = userSearchSequence
  userSearchTimeout = setTimeout(async () => {
    userSearchTimeout = null
    try {
      const results = await adminAPI.usage.searchUsers(query)
      if (sequence !== userSearchSequence) return
      userResults.value = results
      showUserDropdown.value = results.length > 0
    } catch {
      if (sequence !== userSearchSequence) return
      userResults.value = []
    }
  }, 300)
}

const onUserFocus = () => {
  if (userResults.value.length > 0) showUserDropdown.value = true
}

const selectUser = (u: SimpleUser) => {
  clearPendingUserSearch()
  userKeyword.value = u.email
  showUserDropdown.value = false
  filterUserId.value = u.id
  onFilterChange()
}

const clearUserFilter = () => {
  clearPendingUserSearch()
  userKeyword.value = ''
  userResults.value = []
  showUserDropdown.value = false
  filterUserId.value = null
  onFilterChange()
}

const onDocumentClick = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (userSearchRef.value && target && !userSearchRef.value.contains(target)) {
    showUserDropdown.value = false
  }
}

// ---------- 加载 ----------
let abortController: AbortController | null = null

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const buildFilters = (): AdminApiKeyListFilters => {
  const filters: AdminApiKeyListFilters = {
    sort_by: sortState.value.sort_by,
    sort_order: sortState.value.sort_order
  }
  if (filterSearch.value) filters.search = filterSearch.value
  if (filterStatus.value) filters.status = filterStatus.value
  if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
  if (filterUserId.value !== null) filters.user_id = filterUserId.value
  return filters
}

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  try {
    const response = await adminAPI.apiKeys.list(
      pagination.value.page,
      pagination.value.page_size,
      buildFilters(),
      { signal }
    )
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    if (response.items.length > 0) {
      const keyIds = response.items.map((k) => k.id)
      try {
        const usageResponse = await adminAPI.dashboard.getBatchApiKeysUsage(keyIds, { signal })
        if (signal.aborted) return
        usageStats.value = usageResponse.stats
      } catch (e) {
        if (!isAbortError(e)) {
          console.error('Failed to load API key usage stats:', e)
        }
      }
    } else {
      usageStats.value = {}
    }
  } catch (error) {
    if (isAbortError(error)) return
    appStore.showError(t('admin.apiKeys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.value.sort_by = SORT_BY_COLUMN[key] ?? key
  sortState.value.sort_order = order
  pagination.value.page = 1
  loadApiKeys()
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

// ---------- 行操作 ----------
const hasIpRules = (key: ApiKey) =>
  (key.ip_whitelist?.length ?? 0) > 0 || (key.ip_blacklist?.length ?? 0) > 0

const openKeyUsage = (key: ApiKey) => {
  const query: Record<string, string> = { api_key_id: String(key.id) }
  if (key.user_id) query.user_id = String(key.user_id)
  router.push({ path: '/admin/usage', query })
}

onMounted(() => {
  loadApiKeys()
  loadGroups()
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  clearPendingUserSearch()
  abortController?.abort()
  document.removeEventListener('click', onDocumentClick)
})
</script>
