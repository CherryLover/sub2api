<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-5 pb-8">
      <header
        class="page-header mb-0 rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="page-title flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
              <span
                class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-amber-50 text-amber-500 dark:bg-amber-900/30 dark:text-amber-400"
              >
                <Icon name="grid" size="sm" />
              </span>
              {{ t('admin.accounts.modelAvailability.title') }}
            </h1>
            <p class="page-description mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.modelAvailability.description') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <span
              v-if="lastUpdatedText"
              data-test="last-updated"
              class="whitespace-nowrap text-[11px] text-gray-400 dark:text-gray-500"
            >
              {{ lastUpdatedText }}
            </span>
            <button
              type="button"
              data-test="refresh-button"
              class="btn btn-secondary btn-sm whitespace-nowrap"
              :disabled="loading"
              @click="refresh"
            >
              <Icon name="refresh" size="xs" class="mr-1" />
              {{ loading ? t('admin.accounts.modelAvailability.refreshing') : t('admin.accounts.modelAvailability.refresh') }}
            </button>
          </div>
        </div>
        <p class="mt-2 text-[11px] leading-5 text-gray-400 dark:text-gray-500">
          {{ t('admin.accounts.modelAvailability.delayNotice') }}
          {{ t('admin.accounts.modelAvailability.modelSourceNotice') }}
        </p>
      </header>

      <!-- 汇总条 -->
      <section
        data-test="summary-bar"
        class="grid grid-cols-2 gap-3 rounded-2xl bg-white p-4 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 lg:grid-cols-4"
      >
        <div class="min-w-0">
          <div class="truncate whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.modelAvailability.summary.accounts') }}
          </div>
          <div data-test="summary-accounts" class="text-xl font-bold text-gray-900 dark:text-white">
            {{ summary.accountCount }}
          </div>
        </div>
        <div class="min-w-0">
          <div class="truncate whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.modelAvailability.summary.models') }}
          </div>
          <div data-test="summary-models" class="text-xl font-bold text-gray-900 dark:text-white">
            {{ summary.modelCount }}
          </div>
        </div>
        <div class="min-w-0">
          <div class="truncate whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.modelAvailability.summary.blockedAccounts') }}
          </div>
          <div
            data-test="summary-blocked-accounts"
            :class="[
              'text-xl font-bold',
              summary.blockedAccountCount > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'
            ]"
          >
            {{ summary.blockedAccountCount }}
          </div>
        </div>
        <div class="min-w-0">
          <div class="truncate whitespace-nowrap text-[11px] text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.modelAvailability.summary.limitedCombos') }}
          </div>
          <div
            data-test="summary-limited-cells"
            :class="[
              'text-xl font-bold',
              summary.limitedCellCount > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-900 dark:text-white'
            ]"
          >
            {{ summary.limitedCellCount }}
          </div>
        </div>
      </section>

      <!-- 过滤器 -->
      <section class="flex flex-wrap items-center gap-3">
        <select
          v-model="platformFilter"
          data-test="platform-filter"
          class="input h-9 w-auto min-w-[140px] max-w-[220px] py-1 text-sm"
        >
          <option value="">{{ t('admin.accounts.modelAvailability.filters.allPlatforms') }}</option>
          <option v-for="platform in platformOptions" :key="platform" :value="platform">{{ platform }}</option>
        </select>
        <label class="flex cursor-pointer items-center gap-2 whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">
          <input v-model="onlyIssues" data-test="only-issues" type="checkbox" class="h-4 w-4 rounded" />
          {{ t('admin.accounts.modelAvailability.filters.onlyIssues') }}
        </label>
        <span
          v-if="degradedNotice"
          data-test="degraded-notice"
          class="rounded-lg bg-amber-50 px-2 py-1 text-[11px] text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
        >
          {{ degradedNotice }}
        </span>
      </section>

      <!-- 加载 / 错误 / 空 / 矩阵 -->
      <section
        v-if="loading && !hasLoadedOnce"
        data-test="loading-state"
        class="rounded-2xl bg-white p-10 text-center text-sm text-gray-500 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:text-gray-400 dark:ring-dark-700"
      >
        {{ t('admin.accounts.modelAvailability.loading') }}
      </section>
      <section
        v-else-if="loadError"
        data-test="error-state"
        class="rounded-2xl bg-white p-10 text-center shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
      >
        <p class="text-sm font-medium text-red-600 dark:text-red-400">
          {{ t('admin.accounts.modelAvailability.error.title') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ loadError }}</p>
        <button type="button" data-test="retry-button" class="btn btn-secondary btn-sm mt-4" @click="refresh">
          {{ t('admin.accounts.modelAvailability.error.retry') }}
        </button>
      </section>
      <section
        v-else-if="view.rows.length === 0"
        data-test="empty-state"
        class="rounded-2xl bg-white p-10 text-center shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700"
      >
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ emptyTitle }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ emptyDescription }}
        </p>
      </section>
      <AvailabilityMatrixTable v-else :columns="view.columns" :rows="view.rows" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import AvailabilityMatrixTable from '@/components/admin/model-availability/AvailabilityMatrixTable.vue'
import {
  buildModelAvailabilityMatrix,
  filterIssuesOnly,
  type ModelAvailabilityInput
} from '@/components/admin/model-availability/modelAvailability'
import { useNowTick } from '@/composables/useNowTick'
import type { Account, AccountDiagnosis } from '@/types'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const { now } = useNowTick()

const ACCOUNT_PAGE_SIZE = 200
const MAX_ACCOUNT_PAGES = 10
const MODEL_FETCH_CONCURRENCY = 6

const loading = ref(false)
const hasLoadedOnce = ref(false)
const loadError = ref<string | null>(null)
const accounts = ref<Account[]>([])
const diagnostics = ref<Record<string, AccountDiagnosis>>({})
/** 账号 ID → 模型清单；缺键表示该账号的清单没取到 */
const accountModels = ref<Record<number, string[]>>({})
const diagnosticsFailed = ref(false)
const modelListFailures = ref(0)
const updatedAt = ref<number | null>(null)

const platformFilter = ref('')
const onlyIssues = ref(false)

/**
 * 限流解除时间只显示到分钟，没必要跟着 5 秒心跳整表重算。
 * 按分钟对齐后，过期的限流最多一分钟内自己消失，而几千个格子不会每 5 秒重渲染一次。
 */
const nowMinute = computed(() => Math.floor(now.value / 60_000) * 60_000)

const platformOptions = computed(() => {
  const set = new Set<string>()
  for (const account of accounts.value) {
    if (account.platform) set.add(account.platform)
  }
  return [...set].sort()
})

const inputs = computed<ModelAvailabilityInput[]>(() =>
  accounts.value
    .filter((account) => !platformFilter.value || account.platform === platformFilter.value)
    .map((account) => ({
      account,
      diagnosis: diagnostics.value[String(account.id)] ?? null,
      models: accountModels.value[account.id]
    }))
)

const matrix = computed(() => buildModelAvailabilityMatrix(inputs.value, nowMinute.value))
const summary = computed(() => matrix.value.summary)
const view = computed(() => (onlyIssues.value ? filterIssuesOnly(matrix.value) : matrix.value))

const lastUpdatedText = computed(() =>
  updatedAt.value
    ? t('admin.accounts.modelAvailability.lastUpdated', { time: formatDateTime(new Date(updatedAt.value)) })
    : ''
)

const degradedNotice = computed(() => {
  const notes: string[] = []
  if (diagnosticsFailed.value) notes.push(t('admin.accounts.modelAvailability.diagnosticsUnavailable'))
  if (modelListFailures.value > 0) {
    notes.push(
      t('admin.accounts.modelAvailability.modelListPartiallyUnavailable', { count: modelListFailures.value })
    )
  }
  return notes.join(' ')
})

const emptyTitle = computed(() =>
  onlyIssues.value
    ? t('admin.accounts.modelAvailability.empty.noIssuesTitle')
    : t('admin.accounts.modelAvailability.empty.title')
)

const emptyDescription = computed(() =>
  onlyIssues.value
    ? t('admin.accounts.modelAvailability.empty.noIssuesDescription')
    : t('admin.accounts.modelAvailability.empty.description')
)

/** 按固定并发跑一批请求：账号多的时候不要一口气打出上百个请求 */
async function mapWithConcurrency<T, R>(
  items: T[],
  limit: number,
  task: (item: T) => Promise<R>
): Promise<R[]> {
  const results = new Array<R>(items.length)
  let cursor = 0
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (cursor < items.length) {
      const index = cursor
      cursor += 1
      results[index] = await task(items[index])
    }
  })
  await Promise.all(workers)
  return results
}

async function loadAllAccounts(): Promise<Account[]> {
  const collected: Account[] = []
  for (let page = 1; page <= MAX_ACCOUNT_PAGES; page += 1) {
    const response = await adminAPI.accounts.list(page, ACCOUNT_PAGE_SIZE)
    const items = response?.items ?? []
    collected.push(...items)
    const total = response?.total ?? collected.length
    if (items.length === 0 || collected.length >= total) break
  }
  return collected
}

async function refresh(): Promise<void> {
  if (loading.value) return
  loading.value = true
  loadError.value = null
  diagnosticsFailed.value = false
  modelListFailures.value = 0

  try {
    const list = await loadAllAccounts()
    accounts.value = list

    const ids = list.map((account) => account.id)

    // 诊断和模型清单都是「有更好、没有也能看」的增强数据，任何一路失败都不该让整页空白
    const [diagnosisResult, modelEntries] = await Promise.all([
      ids.length > 0
        ? adminAPI.accounts.getBatchDiagnostics(ids).catch(() => {
            diagnosticsFailed.value = true
            return null
          })
        : Promise.resolve(null),
      mapWithConcurrency(list, MODEL_FETCH_CONCURRENCY, async (account) => {
        try {
          const models = await adminAPI.accounts.getAvailableModels(account.id)
          return [account.id, (models ?? []).map((model) => model?.id).filter(Boolean) as string[]] as const
        } catch {
          modelListFailures.value += 1
          return [account.id, null] as const
        }
      })
    ])

    diagnostics.value = diagnosisResult?.diagnostics ?? {}
    const nextModels: Record<number, string[]> = {}
    for (const [id, models] of modelEntries) {
      if (models) nextModels[id] = models
    }
    accountModels.value = nextModels
    updatedAt.value = Date.now()
    hasLoadedOnce.value = true
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : String(error)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)

defineExpose({ refresh })
</script>
