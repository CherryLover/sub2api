<template>
  <section class="card overflow-hidden" data-testid="key-usage-details-table">
    <div class="flex flex-wrap items-start justify-between gap-2 border-b border-gray-100 p-4 dark:border-dark-700">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('keyUsageDashboard.details.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('keyUsageDashboard.details.subtitle') }}
        </p>
      </div>
      <span class="rounded-full bg-gray-100 px-2.5 py-1 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
        {{ t('keyUsageDashboard.details.count', { count: keys.length }) }}
      </span>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full min-w-[1120px] divide-y divide-gray-200 text-xs dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th
              v-for="column in columns"
              :key="column.id"
              scope="col"
              class="whitespace-nowrap px-3 py-2.5 text-left font-medium text-gray-500 dark:text-gray-400"
              :class="column.numeric ? 'text-right' : 'text-left'"
              :aria-sort="column.sortable ? ariaSort(column.key) : undefined"
            >
              <button
                v-if="column.sortable"
                type="button"
                class="inline-flex w-full items-center gap-1"
                :class="column.numeric ? 'justify-end' : 'justify-start'"
                :data-testid="`usage-sort-${column.key}`"
                @click="toggleSort(column.key)"
              >
                <span>{{ column.label }}</span>
                <Icon
                  v-if="sortKey === column.key"
                  :name="sortOrder === 'asc' ? 'arrowUp' : 'arrowDown'"
                  size="xs"
                  class="text-primary-500"
                />
                <Icon v-else name="arrowsUpDown" size="xs" class="text-gray-300 dark:text-dark-500" />
              </button>
              <span v-else>{{ column.label }}</span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr
            v-for="key in sortedKeys"
            :key="key.api_key_id"
            class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-800"
            :class="[
              key.totals.requests === 0 ? 'opacity-60' : '',
              key.api_key_id === selectedKeyId
                ? 'bg-primary-50/60 dark:bg-primary-900/10'
                : 'bg-white dark:bg-dark-900'
            ]"
            :data-testid="`usage-detail-row-${key.api_key_id}`"
            @click="emit('select', key.api_key_id)"
          >
            <td class="max-w-52 px-3 py-2.5">
              <button
                type="button"
                class="block max-w-full truncate text-left font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300"
                :title="key.name"
                @click.stop="emit('select', key.api_key_id)"
              >
                {{ key.name }}
              </button>
              <span class="mt-0.5 block truncate text-[10px] text-gray-400 dark:text-gray-500">
                {{ key.group_name || t('keys.noGroup') }}
              </span>
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? formatCost(key.totals.cost) : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? Math.round(key.totals.requests).toLocaleString() : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? formatCompactNumber(key.totals.tokens) : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? formatCost(key.cost_per_request) : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? formatPercent(key.cache_hit_rate) : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? formatLatency(key.avg_first_token_ms) : '—' }}
            </td>
            <td class="px-3 py-2.5 text-right">
              <span
                v-if="hasUsage(key)"
                class="inline-flex min-w-14 justify-center rounded px-1.5 py-1 font-medium"
                :class="errorRateClass(key.error_rate)"
              >
                {{ formatPercent(key.error_rate, 2) }}
              </span>
              <span v-else>—</span>
            </td>
            <td class="px-3 py-2.5 text-right text-gray-700 dark:text-gray-300">
              {{ hasUsage(key) ? Math.round(key.peak_rpm).toLocaleString() : '—' }}
            </td>
            <td class="px-3 py-2.5">
              <svg
                v-if="hasUsage(key) && sparklineHasData(key)"
                class="h-6 w-24 text-primary-500"
                viewBox="0 0 96 24"
                fill="none"
                aria-hidden="true"
              >
                <polyline
                  :points="sparklinePoints(key)"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
              <span v-else class="text-gray-400">—</span>
            </td>
          </tr>
          <tr v-if="keys.length === 0">
            <td :colspan="columns.length" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('keyUsageDashboard.noData') }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type {
  KeyUsageDashboardKey,
  KeyUsageDashboardMetric
} from '@/api/keyUsageDashboard'
import {
  formatCompactNumber,
  formatCost,
  formatLatency,
  formatPercent,
  metricValue,
  toFiniteNumber
} from './utils'

type SortKey =
  | 'name'
  | 'cost'
  | 'requests'
  | 'tokens'
  | 'cost_per_request'
  | 'cache_hit_rate'
  | 'avg_first_token_ms'
  | 'error_rate'
  | 'peak_rpm'

interface SortColumn {
  id: string
  key: SortKey
  label: string
  sortable: boolean
  numeric: boolean
}

const { t } = useI18n()

const props = defineProps<{
  keys: KeyUsageDashboardKey[]
  metric: KeyUsageDashboardMetric
  selectedKeyId?: number
}>()

const emit = defineEmits<{
  select: [apiKeyId: number]
}>()

const sortKey = ref<SortKey>(metricToSortKey(props.metric))
const sortOrder = ref<'asc' | 'desc'>('desc')

watch(() => props.metric, (metric) => {
  sortKey.value = metricToSortKey(metric)
  sortOrder.value = 'desc'
})

const columns = computed<SortColumn[]>(() => [
  { id: 'name', key: 'name', label: t('keyUsageDashboard.details.key'), sortable: true, numeric: false },
  { id: 'cost', key: 'cost', label: t('keyUsageDashboard.metrics.cost'), sortable: true, numeric: true },
  { id: 'requests', key: 'requests', label: t('keyUsageDashboard.metrics.requests'), sortable: true, numeric: true },
  { id: 'tokens', key: 'tokens', label: t('keyUsageDashboard.metrics.tokens'), sortable: true, numeric: true },
  { id: 'cost-per-request', key: 'cost_per_request', label: t('keyUsageDashboard.details.averageCost'), sortable: true, numeric: true },
  { id: 'cache-hit-rate', key: 'cache_hit_rate', label: t('keyUsageDashboard.current.cacheHit'), sortable: true, numeric: true },
  { id: 'first-token', key: 'avg_first_token_ms', label: t('keyUsageDashboard.current.averageFirstToken'), sortable: true, numeric: true },
  { id: 'error-rate', key: 'error_rate', label: t('keyUsageDashboard.current.errorRate'), sortable: true, numeric: true },
  { id: 'peak-rpm', key: 'peak_rpm', label: t('keyUsageDashboard.current.peakRpm'), sortable: true, numeric: true },
  { id: 'trend', key: 'name', label: t('keyUsageDashboard.details.trend'), sortable: false, numeric: false }
])

const sortedKeys = computed(() => {
  return [...props.keys].sort((left, right) => {
    const leftValue = sortValue(left)
    const rightValue = sortValue(right)
    const comparison = typeof leftValue === 'string'
      ? leftValue.localeCompare(String(rightValue))
      : toFiniteNumber(leftValue) - toFiniteNumber(rightValue)
    return sortOrder.value === 'asc' ? comparison : -comparison
  })
})

function metricToSortKey(metric: KeyUsageDashboardMetric): SortKey {
  if (metric === 'cost') return 'cost'
  if (metric === 'requests') return 'requests'
  return 'tokens'
}

function sortValue(key: KeyUsageDashboardKey): string | number {
  switch (sortKey.value) {
    case 'name':
      return key.name
    case 'cost':
      return key.totals.cost
    case 'requests':
      return key.totals.requests
    case 'tokens':
      return key.totals.tokens
    case 'cost_per_request':
      return key.cost_per_request
    case 'cache_hit_rate':
      return key.cache_hit_rate
    case 'avg_first_token_ms':
      return key.avg_first_token_ms
    case 'error_rate':
      return key.error_rate
    case 'peak_rpm':
      return key.peak_rpm
  }
}

function toggleSort(key: SortKey): void {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
    return
  }
  sortKey.value = key
  sortOrder.value = key === 'name' ? 'asc' : 'desc'
}

function ariaSort(key: SortKey): 'ascending' | 'descending' | 'none' {
  if (sortKey.value !== key) return 'none'
  return sortOrder.value === 'asc' ? 'ascending' : 'descending'
}

function hasUsage(key: KeyUsageDashboardKey): boolean {
  return key.totals.requests > 0
}

function errorRateClass(errorRate: number): string {
  if (errorRate >= 0.05) {
    return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  }
  if (errorRate >= 0.025) {
    return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
  }
  return 'text-gray-700 dark:text-gray-300'
}

function sparklineHasData(key: KeyUsageDashboardKey): boolean {
  return (key.series || []).some((point) => metricValue(point, props.metric) > 0)
}

function sparklinePoints(key: KeyUsageDashboardKey): string {
  const values = (key.series || []).map((point) => metricValue(point, props.metric))
  if (values.length === 0) return ''
  const max = Math.max(...values)
  if (max <= 0) return ''
  return values.map((value, index) => {
    const x = values.length === 1 ? 48 : (index / (values.length - 1)) * 92 + 2
    const y = 22 - (value / max) * 20
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}
</script>
