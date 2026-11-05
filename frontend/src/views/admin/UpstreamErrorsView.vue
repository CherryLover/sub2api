<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import UpstreamErrorTrendChart from '@/components/admin/upstream-errors/UpstreamErrorTrendChart.vue'
import {
  getUpstreamErrorRequestsSummary,
  listUpstreamErrorRequests,
  type OpsUpstreamErrorAttempt,
  type OpsUpstreamErrorFilterParams,
  type OpsUpstreamErrorRequest,
  type OpsUpstreamErrorRequestParams,
  type OpsUpstreamErrorRequestsSummary,
  type OpsUpstreamErrorTrendFilters
} from '@/api/admin/ops'
import { formatBytes } from '@/utils/format'
import { formatDateTime } from './ops/utils/opsFormatters'

const { t } = useI18n()
const prefix = 'admin.upstreamErrors.'
const items = ref<OpsUpstreamErrorRequest[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const loadError = ref(false)
const timeRange = ref('24h')
const status = ref('')
const outcome = ref('')
const finalStatus = ref('')
const keyId = ref('')
const accountId = ref('')
const summary = ref<OpsUpstreamErrorRequestsSummary | null>(null)
const summaryLoading = ref(false)
const trendRefreshKey = ref(0)
const selected = ref<OpsUpstreamErrorRequest | null>(null)
const formattedMode = ref(true)
const copyFeedback = ref<Record<string, 'copied' | 'failed'>>({})

let controller: AbortController | undefined
let generation = 0
let summaryController: AbortController | undefined
let summaryGeneration = 0
let idDebounceTimer: ReturnType<typeof setTimeout> | undefined
const copyTimers = new Map<string, ReturnType<typeof setTimeout>>()

const columns = computed(() => [
  { key: 'created_at', label: t(prefix + 'time') },
  { key: 'api_key', label: t(prefix + 'key') },
  { key: 'account', label: t(prefix + 'failedAccount') },
  { key: 'model', label: t('usage.model') },
  { key: 'errors', label: t(prefix + 'events') },
  { key: 'outcome', label: t(prefix + 'outcome') },
  { key: 'final', label: t(prefix + 'finalAccount') },
  { key: 'actions', label: t(prefix + 'details') }
])

const timeOptions = computed(() => [
  { value: '5m', minutes: 5, label: t(prefix + 'range5m') },
  { value: '15m', minutes: 15, label: t(prefix + 'range15m') },
  { value: '30m', minutes: 30, label: t(prefix + 'range30m') },
  { value: '1h', minutes: 60, label: t(prefix + 'range1h') },
  { value: '6h', minutes: 360, label: t(prefix + 'range6h') },
  { value: '24h', minutes: 1440, label: t(prefix + 'range24h') },
  { value: '3d', minutes: 4320, label: t(prefix + 'range3d') },
  { value: '7d', minutes: 10080, label: t(prefix + 'range7d') },
  { value: '14d', minutes: 20160, label: t(prefix + 'range14d') },
  { value: '30d', minutes: 43200, label: t(prefix + 'range30d') }
])
const activeRange = ref(timeWindow())

const statusOptions = computed(() => [
  { value: '', label: t(prefix + 'allStatusCodes') },
  ...[400, 401, 403, 408, 429, 500, 502, 503, 504, 529].map(code => ({
    value: String(code),
    label: String(code)
  }))
])

const outcomeOptions = computed(() => [
  { value: '', label: t(prefix + 'allOutcomes') },
  { value: 'recovered', label: t(prefix + 'recovered') },
  { value: 'failed', label: t(prefix + 'failed') }
])

const trendFilters = computed<OpsUpstreamErrorTrendFilters>(() => {
  const filters: OpsUpstreamErrorTrendFilters = {}
  if (status.value) filters.status_code = Number(status.value)
  if (finalStatus.value) filters.final_status_code = Number(finalStatus.value)
  if (outcome.value) filters.outcome = outcome.value as 'recovered' | 'failed'
  if (Number(keyId.value) > 0) filters.api_key_id = Number(keyId.value)
  if (Number(accountId.value) > 0) filters.account_id = Number(accountId.value)
  return filters
})

const recoveryRate = computed(() => {
  if (!summary.value?.total) return '0'
  return ((summary.value.recovered / summary.value.total) * 100).toFixed(1)
})

const accountLabel = (id: number | null | undefined, name?: string) => id ? `${name || '#'}${name ? ' #' : ''}${id}` : t(prefix + 'notRecorded')
const statusCodes = (row: OpsUpstreamErrorRequest) => [...new Set(row.attempts.map(attempt => attempt.upstream_status_code).filter((code): code is number => code != null))].join(' / ') || '—'
const outcomeClass = (row: OpsUpstreamErrorRequest) => row.outcome === 'recovered'
  ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300'
  : 'bg-rose-100 text-rose-800 dark:bg-rose-900/30 dark:text-rose-300'
const requestSize = (row: OpsUpstreamErrorRequest) => row.request_body_bytes == null
  ? '—'
  : formatBytes(row.request_body_bytes)
const effortClass = (effort?: string) => {
  switch ((effort || '').toLowerCase()) {
    case 'low':
      return 'bg-slate-100 text-slate-700 dark:bg-slate-700/50 dark:text-slate-200'
    case 'medium':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'high':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300'
    case 'xhigh':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
  }
}

function timeWindow() {
  const end = new Date()
  const minutes = timeOptions.value.find(option => option.value === timeRange.value)?.minutes || 1440
  return {
    start_time: new Date(end.getTime() - minutes * 60_000).toISOString(),
    end_time: end.toISOString()
  }
}

function filterParams(): OpsUpstreamErrorFilterParams {
  return { ...activeRange.value, ...trendFilters.value }
}

async function fetchData(): Promise<void> {
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  loadError.value = false
  const params: OpsUpstreamErrorRequestParams = {
    ...filterParams(),
    page: page.value,
    page_size: pageSize.value
  }
  try {
    const result = await listUpstreamErrorRequests(params, { signal: controller.signal })
    if (current !== generation) return
    items.value = result.items || []
    total.value = result.total
  } catch {
    if (current !== generation) return
    items.value = []
    total.value = 0
    loadError.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

async function fetchSummary(): Promise<void> {
  summaryController?.abort()
  const current = ++summaryGeneration
  summaryController = new AbortController()
  summaryLoading.value = true
  try {
    const result = await getUpstreamErrorRequestsSummary(filterParams(), {
      signal: summaryController.signal
    })
    if (current === summaryGeneration) summary.value = result
  } catch {
    if (current === summaryGeneration) summary.value = null
  } finally {
    if (current === summaryGeneration) summaryLoading.value = false
  }
}

function clearIdDebounce(): void {
  if (!idDebounceTimer) return
  clearTimeout(idDebounceTimer)
  idDebounceTimer = undefined
}

function refreshForFilters(): void {
  clearIdDebounce()
  activeRange.value = timeWindow()
  page.value = 1
  void fetchData()
  void fetchSummary()
}

function refreshAll(): void {
  activeRange.value = timeWindow()
  page.value = 1
  trendRefreshKey.value++
  void fetchData()
  void fetchSummary()
}

function changeTime(value: string): void {
  if (timeRange.value === value) return
  timeRange.value = value
  refreshForFilters()
}

function onSelectFilterChange(): void {
  refreshForFilters()
}

function scheduleIdFilter(): void {
  clearIdDebounce()
  idDebounceTimer = setTimeout(() => {
    idDebounceTimer = undefined
    refreshForFilters()
  }, 400)
}

function applyIdFilterNow(): void {
  refreshForFilters()
}

function clearFinalStatus(): void {
  finalStatus.value = ''
  refreshForFilters()
}

function selectFailedStatus(code: number): void {
  finalStatus.value = String(code)
  outcome.value = 'failed'
  refreshForFilters()
}

function changePage(value: number): void {
  page.value = value
  void fetchData()
}

function changePageSize(value: number): void {
  pageSize.value = value
  page.value = 1
  void fetchData()
}

function openDetails(row: OpsUpstreamErrorRequest): void {
  selected.value = row
  formattedMode.value = true
  copyFeedback.value = {}
}

function closeDetails(): void {
  selected.value = null
}

interface ContentBlock {
  key: string
  title: string
  content: string
}

function attemptBlocks(attempt: OpsUpstreamErrorAttempt, index: number): ContentBlock[] {
  const blocks: ContentBlock[] = []
  if (attempt.message) {
    blocks.push({ key: `attempt-${index}-message`, title: t(prefix + 'eventMessage'), content: attempt.message })
  }
  if (attempt.detail) {
    blocks.push({ key: `attempt-${index}-detail`, title: t(prefix + 'eventDetail'), content: attempt.detail })
  }
  if (attempt.upstream_response_body) {
    blocks.push({ key: `attempt-${index}-response`, title: t(prefix + 'responseBody'), content: attempt.upstream_response_body })
  }
  return blocks
}

function displayContent(content: string): string {
  if (!formattedMode.value) return content
  try {
    return JSON.stringify(JSON.parse(content), null, 2)
  } catch {
    return content
  }
}

function isTruncated(content: string): boolean {
  return new TextEncoder().encode(content).length === 2048
}

async function copyBlock(block: ContentBlock): Promise<void> {
  const existingTimer = copyTimers.get(block.key)
  if (existingTimer) clearTimeout(existingTimer)
  try {
    if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
    await navigator.clipboard.writeText(displayContent(block.content))
    copyFeedback.value = { ...copyFeedback.value, [block.key]: 'copied' }
  } catch {
    copyFeedback.value = { ...copyFeedback.value, [block.key]: 'failed' }
  }
  copyTimers.set(block.key, setTimeout(() => {
    const next = { ...copyFeedback.value }
    delete next[block.key]
    copyFeedback.value = next
    copyTimers.delete(block.key)
  }, 1600))
}

onMounted(() => {
  void fetchData()
  void fetchSummary()
})

onBeforeUnmount(() => {
  generation++
  summaryGeneration++
  controller?.abort()
  summaryController?.abort()
  clearIdDebounce()
  copyTimers.forEach(timer => clearTimeout(timer))
  copyTimers.clear()
})
</script>

<template>
  <AppLayout>
    <div class="space-y-4">
      <section class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t(prefix + 'title') }}</h2>
            <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">{{ t(prefix + 'description') }}</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || summaryLoading" @click="refreshAll">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div class="mt-5">
          <span class="input-label">{{ t(prefix + 'range') }}</span>
          <div class="flex flex-wrap gap-1.5" role="group" :aria-label="t(prefix + 'range')">
            <button
              v-for="option in timeOptions"
              :key="option.value"
              type="button"
              class="rounded-full border px-3 py-1.5 text-xs font-medium transition-colors"
              :class="timeRange === option.value
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:text-gray-900 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-dark-500 dark:hover:text-white'"
              :aria-pressed="timeRange === option.value"
              :data-testid="`time-range-${option.value}`"
              @click="changeTime(option.value)"
            >
              {{ option.label }}
            </button>
          </div>
        </div>

        <div class="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <label>
            <span class="input-label">{{ t(prefix + 'status') }}</span>
            <Select v-model="status" :options="statusOptions" @change="onSelectFilterChange" />
          </label>
          <label>
            <span class="input-label">{{ t(prefix + 'outcome') }}</span>
            <Select v-model="outcome" :options="outcomeOptions" @change="onSelectFilterChange" />
          </label>
          <label>
            <span class="input-label">{{ t(prefix + 'keyId') }}</span>
            <input
              v-model="keyId"
              type="number"
              min="1"
              step="1"
              class="input"
              data-testid="key-id-filter"
              @input="scheduleIdFilter"
              @keydown.enter.prevent="applyIdFilterNow"
            />
          </label>
          <label>
            <span class="input-label">{{ t(prefix + 'accountId') }}</span>
            <input
              v-model="accountId"
              type="number"
              min="1"
              step="1"
              class="input"
              data-testid="account-id-filter"
              @input="scheduleIdFilter"
              @keydown.enter.prevent="applyIdFilterNow"
            />
          </label>
          <div v-if="finalStatus" class="flex items-end">
            <span class="inline-flex max-w-full items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-medium text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">
              <span class="truncate">{{ t(prefix + 'finalStatusFilter', { code: finalStatus }) }}</span>
              <button
                type="button"
                class="rounded-full p-0.5 hover:bg-amber-100 dark:hover:bg-amber-800/40"
                :aria-label="t(prefix + 'clearFinalStatus')"
                data-testid="clear-final-status"
                @click="clearFinalStatus"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
        </div>

        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t(prefix + 'retentionHint') }}</p>
      </section>

      <div class="grid gap-4 md:grid-cols-3">
        <section class="card p-4 sm:p-5" data-testid="summary-total">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(prefix + 'upstreamErrorRequests') }}</p>
              <p class="mt-2 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ summary?.total ?? 0 }}</p>
            </div>
            <span class="rounded-lg bg-slate-100 p-2 text-slate-600 dark:bg-slate-800 dark:text-slate-300">
              <Icon name="document" size="md" />
            </span>
          </div>
          <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t(prefix + 'failedAttempts', { n: summary?.attempts ?? 0 }) }}</p>
        </section>

        <section class="card p-4 sm:p-5" data-testid="summary-recovered">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(prefix + 'recoveredRequests') }}</p>
              <p class="mt-2 text-2xl font-semibold tabular-nums text-teal-600 dark:text-teal-400">{{ summary?.recovered ?? 0 }}</p>
            </div>
            <span class="rounded-lg bg-teal-100 p-2 text-teal-600 dark:bg-teal-900/30 dark:text-teal-400">
              <Icon name="checkCircle" size="md" />
            </span>
          </div>
          <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full bg-teal-500 transition-[width]" :style="{ width: `${recoveryRate}%` }" />
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t(prefix + 'recoveryRate', { rate: recoveryRate }) }}</p>
        </section>

        <section class="card p-4 sm:p-5" data-testid="summary-failed">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(prefix + 'finalFailedRequests') }}</p>
              <p class="mt-2 text-2xl font-semibold tabular-nums text-rose-600 dark:text-rose-400">{{ summary?.failed ?? 0 }}</p>
            </div>
            <span class="rounded-lg bg-rose-100 p-2 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400">
              <Icon name="xCircle" size="md" />
            </span>
          </div>
          <div class="mt-3 flex flex-wrap gap-1.5">
            <button
              v-for="entry in summary?.failed_by_status || []"
              :key="entry.status_code"
              type="button"
              class="rounded-full bg-rose-50 px-2.5 py-1 text-xs font-medium text-rose-700 transition-colors hover:bg-rose-100 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
              :data-testid="`failed-status-${entry.status_code}`"
              @click="selectFailedStatus(entry.status_code)"
            >
              {{ entry.status_code }} {{ entry.count }}
            </button>
            <span v-if="!summary?.failed_by_status?.length" class="text-xs text-gray-400">—</span>
          </div>
        </section>
      </div>

      <UpstreamErrorTrendChart :filters="trendFilters" :refresh-key="trendRefreshKey" />

      <div v-if="loadError" role="alert" class="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800 dark:border-rose-800 dark:bg-rose-950/30 dark:text-rose-300">
        {{ t(prefix + 'loadFailed') }}
        <button type="button" class="ml-2 inline-flex items-center gap-1 underline" @click="refreshAll">
          <Icon name="refresh" size="xs" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div class="card overflow-hidden">
        <DataTable :columns="columns" :data="items" :loading="loading" row-key="id">
          <template #cell-created_at="{ row }"><div class="whitespace-nowrap text-sm">{{ formatDateTime(row.created_at) }}</div></template>
          <template #cell-api_key="{ row }"><span class="break-all">{{ accountLabel(row.api_key_id, row.api_key_name) }}</span></template>
          <template #cell-account="{ row }"><span class="break-all">{{ accountLabel(row.account_id, row.account_name) }}</span></template>
          <template #cell-model="{ row }">
            <div class="min-w-[180px]">
              <div class="break-all font-medium text-gray-900 dark:text-white">{{ row.model || '—' }}</div>
              <div class="mt-1.5 flex flex-wrap gap-1.5">
                <span
                  class="inline-flex rounded px-1.5 py-0.5 text-[11px] font-medium"
                  :class="effortClass(row.reasoning_effort)"
                  :title="t(prefix + 'reasoningEffort')"
                  data-testid="model-reasoning-effort"
                >
                  {{ row.reasoning_effort || '—' }}
                </span>
                <span
                  class="inline-flex rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-200"
                  :title="t(prefix + 'requestSize')"
                  data-testid="model-request-size"
                >
                  {{ requestSize(row) }}
                </span>
              </div>
            </div>
          </template>
          <template #cell-errors="{ row }"><div class="font-mono text-rose-600 dark:text-rose-300">{{ statusCodes(row) }}</div><div class="mt-1 text-xs text-gray-500">{{ t(prefix + 'eventCount', { n: row.attempts.length }) }}</div></template>
          <template #cell-outcome="{ row }"><span class="inline-flex whitespace-nowrap rounded px-2 py-1 text-xs font-medium" :class="outcomeClass(row)">{{ t(prefix + row.outcome) }} · {{ row.client_status_code || '—' }}</span></template>
          <template #cell-final="{ row }"><span class="break-all">{{ row.outcome === 'recovered' && row.final ? accountLabel(row.final.account_id, row.final.account_name) : t(prefix + (row.outcome === 'recovered' ? 'notRecorded' : 'noSuccess')) }}</span></template>
          <template #cell-actions="{ row }">
            <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1 whitespace-nowrap" @click="openDetails(row)">
              <Icon name="eye" size="sm" />
              {{ t(prefix + 'details') }}
            </button>
          </template>
          <template #empty><p class="py-10 text-sm text-gray-500">{{ t(prefix + 'empty') }}</p></template>
        </DataTable>
        <Pagination v-if="total > 0" :page="page" :page-size="pageSize" :total="total" @update:page="changePage" @update:page-size="changePageSize" />
      </div>
    </div>

    <BaseDialog
      :show="selected !== null"
      :title="t(prefix + 'details')"
      width="full"
      :close-on-click-outside="true"
      @close="closeDetails"
    >
      <div v-if="selected" class="space-y-5">
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-900/60">
          <p class="min-w-0 break-all text-sm text-gray-700 dark:text-dark-200">
            <span class="font-medium text-gray-900 dark:text-white">{{ selected.model || '—' }}</span>
            <span class="mx-1.5 text-gray-400">·</span>
            <span>{{ t(prefix + 'reasoningEffort') }} {{ selected.reasoning_effort || '—' }}</span>
            <span class="mx-1.5 text-gray-400">·</span>
            <span>{{ t(prefix + 'requestSize') }} {{ requestSize(selected) }}</span>
          </p>
          <div class="inline-flex shrink-0 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-600 dark:bg-dark-800">
            <button
              type="button"
              class="rounded-md px-2.5 py-1 text-xs font-medium"
              :class="formattedMode ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'text-gray-500 dark:text-dark-300'"
              :aria-pressed="formattedMode"
              data-testid="format-mode-formatted"
              @click="formattedMode = true"
            >
              {{ t(prefix + 'formatted') }}
            </button>
            <button
              type="button"
              class="rounded-md px-2.5 py-1 text-xs font-medium"
              :class="!formattedMode ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'text-gray-500 dark:text-dark-300'"
              :aria-pressed="!formattedMode"
              data-testid="format-mode-raw"
              @click="formattedMode = false"
            >
              {{ t(prefix + 'raw') }}
            </button>
          </div>
        </div>

        <dl class="grid gap-3 break-all text-sm sm:grid-cols-2">
          <div><dt class="text-gray-500">{{ t(prefix + 'requestId') }}</dt><dd class="font-mono">{{ selected.client_request_id || selected.request_id || '—' }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'key') }}</dt><dd>{{ accountLabel(selected.api_key_id, selected.api_key_name) }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'outcome') }}</dt><dd>{{ t(prefix + selected.outcome) }} · HTTP {{ selected.client_status_code || '—' }}</dd></div>
          <div><dt class="text-gray-500">{{ t(prefix + 'finalAccount') }}</dt><dd>{{ selected.outcome === 'recovered' && selected.final ? accountLabel(selected.final.account_id, selected.final.account_name) : t(prefix + (selected.outcome === 'recovered' ? 'notRecorded' : 'noSuccess')) }}</dd></div>
        </dl>

        <p class="text-xs text-gray-500">{{ t(prefix + 'eventsHint') }}</p>
        <ol class="space-y-3">
          <li v-for="(attempt, index) in selected.attempts" :key="`${selected.id}-${index}`" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex flex-wrap items-center gap-2 text-sm">
              <span class="font-mono text-gray-400">#{{ index + 1 }}</span>
              <span class="font-medium">{{ accountLabel(attempt.account_id, attempt.account_name) }}</span>
              <span class="font-mono text-rose-600 dark:text-rose-300">{{ attempt.upstream_status_code || '—' }}</span>
              <span class="text-xs text-gray-500">{{ attempt.kind }} · {{ attempt.at_unix_ms ? formatDateTime(new Date(attempt.at_unix_ms).toISOString()) : t(prefix + 'notRecorded') }}</span>
            </div>
            <p v-if="attempt.upstream_url" class="mt-2 break-all font-mono text-xs text-gray-500">{{ attempt.upstream_url }}</p>
            <p v-if="attempt.upstream_request_id" class="mt-1 break-all font-mono text-xs text-gray-500">{{ t(prefix + 'upstreamRequestId') }}: {{ attempt.upstream_request_id }}</p>
            <p v-if="attempt.stage || attempt.scope || attempt.reason" class="mt-2 break-words font-mono text-xs text-gray-500">{{ [attempt.stage, attempt.scope, attempt.reason].filter(Boolean).join(' · ') }}</p>

            <div v-for="block in attemptBlocks(attempt, index)" :key="block.key" class="mt-3">
              <div class="mb-1.5 flex flex-wrap items-center justify-between gap-2">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ block.title }}</span>
                  <span
                    v-if="isTruncated(block.content)"
                    class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-900/30 dark:text-amber-300"
                    data-testid="truncated-hint"
                  >
                    {{ t(prefix + 'truncated') }}
                  </span>
                </div>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white"
                  :data-testid="`copy-${block.key}`"
                  @click="copyBlock(block)"
                >
                  <Icon name="copy" size="xs" />
                  {{ copyFeedback[block.key] === 'copied'
                    ? t(prefix + 'copied')
                    : copyFeedback[block.key] === 'failed'
                      ? t(prefix + 'copyFailed')
                      : t(prefix + 'copy') }}
                </button>
              </div>
              <pre
                class="max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-100"
                :data-testid="`content-${block.key}`"
              >{{ displayContent(block.content) }}</pre>
            </div>
          </li>
        </ol>
      </div>
    </BaseDialog>
  </AppLayout>
</template>
