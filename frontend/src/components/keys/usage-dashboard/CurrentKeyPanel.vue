<template>
  <section class="card overflow-hidden xl:sticky xl:top-[92px]" data-testid="current-key-panel">
    <div class="border-b border-gray-100 p-4 dark:border-dark-700">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('keyUsageDashboard.current.title') }}
          </p>
          <h2 class="mt-1 truncate text-base font-semibold text-gray-900 dark:text-white">
            {{ detail?.name || keyName || '—' }}
          </h2>
          <p v-if="(detail?.group_name || groupName)" class="mt-0.5 truncate text-xs text-gray-400 dark:text-gray-500">
            {{ detail?.group_name || groupName }}
          </p>
        </div>
        <span class="rounded-lg bg-primary-50 p-2 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
          <Icon name="chartBar" size="md" />
        </span>
      </div>
    </div>

    <div v-if="loading" class="space-y-4 p-4">
      <div class="grid grid-cols-2 gap-2">
        <div v-for="index in 6" :key="index" class="h-16 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" />
      </div>
      <div class="h-32 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" />
      <div class="space-y-2">
        <div v-for="index in 4" :key="index" class="h-9 animate-pulse rounded bg-gray-100 dark:bg-dark-700" />
      </div>
    </div>

    <div v-else-if="error" class="p-6 text-center">
      <Icon name="exclamationCircle" size="lg" class="mx-auto text-red-500" />
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">
        {{ t('keyUsageDashboard.current.loadFailed') }}
      </p>
      <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadDetail">
        <Icon name="refresh" size="sm" class="mr-1" />
        {{ t('keyUsageDashboard.retry') }}
      </button>
    </div>

    <div v-else-if="detail" class="space-y-5 p-4">
      <dl class="grid grid-cols-2 gap-2">
        <div
          v-for="metric in metricItems"
          :key="metric.label"
          class="min-w-0 rounded-lg bg-gray-50 p-2.5 dark:bg-dark-800"
        >
          <dt class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ metric.label }}</dt>
          <dd class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ metric.value }}</dd>
        </div>
      </dl>

      <div>
        <div class="mb-2 flex items-center justify-between">
          <h3 class="text-xs font-semibold text-gray-700 dark:text-gray-300">
            {{ t('keyUsageDashboard.current.hourlyDistribution') }}
          </h3>
          <span class="text-[10px] text-gray-400">{{ t('keyUsageDashboard.metrics.requests') }}</span>
        </div>
        <div class="flex h-24 items-end gap-[3px] border-b border-gray-200 pb-1 dark:border-dark-600">
          <div
            v-for="(hour, index) in detail.hour_of_day"
            :key="index"
            class="group relative flex h-full min-w-0 flex-1 items-end"
            :title="`${String(index).padStart(2, '0')}:00 · ${Math.round(hour.requests).toLocaleString()}`"
          >
            <div
              class="w-full rounded-t-[2px] transition-opacity group-hover:opacity-75"
              :class="index >= 9 && index <= 18
                ? 'bg-primary-500'
                : 'bg-primary-200'"
              :style="{ height: hourBarHeight(hour.requests) }"
            />
          </div>
        </div>
        <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-gray-500">
          <span>00</span>
          <span>06</span>
          <span>12</span>
          <span>18</span>
          <span>23</span>
        </div>
      </div>

      <div>
        <h3 class="mb-2 text-xs font-semibold text-gray-700 dark:text-gray-300">
          {{ t('keyUsageDashboard.current.modelComposition') }}
        </h3>
        <div v-if="modelItems.length > 0" class="space-y-3">
          <div class="flex h-2.5 gap-[2px] overflow-hidden rounded-full">
            <div
              v-for="item in modelItems"
              :key="item.model"
              class="h-full min-w-[2px] first:rounded-l-full last:rounded-r-full"
              :class="item.colorClass"
              :style="{ width: `${item.share}%` }"
              :title="`${item.label} · ${item.valueLabel} (${item.share.toFixed(1)}%)`"
            />
          </div>
          <div class="space-y-1.5">
            <div
              v-for="item in modelItems"
              :key="`list-${item.model}`"
              class="flex min-w-0 items-center gap-2 text-xs"
            >
              <span class="h-2 w-2 shrink-0 rounded-full" :class="item.colorClass" />
              <span class="min-w-0 flex-1 truncate text-gray-600 dark:text-gray-300" :title="item.label">
                {{ item.label }}
              </span>
              <span class="shrink-0 text-gray-400 dark:text-gray-500">{{ item.valueLabel }}</span>
              <span class="w-11 shrink-0 text-right font-medium text-gray-700 dark:text-gray-200">
                {{ item.share.toFixed(1) }}%
              </span>
            </div>
          </div>
        </div>
        <p v-else class="text-xs text-gray-400">{{ t('keyUsageDashboard.noData') }}</p>
      </div>

      <div>
        <h3 class="mb-2 text-xs font-semibold text-gray-700 dark:text-gray-300">
          {{ t('keyUsageDashboard.current.reasoningEfforts') }}
        </h3>
        <div v-if="effortItems.length > 0" class="flex flex-wrap gap-1.5">
          <span
            v-for="item in effortItems"
            :key="item.effort"
            class="inline-flex items-center gap-1 rounded-full px-2 py-1 text-[11px] font-medium"
            :class="effortClass(item.effort)"
          >
            {{ effortLabel(item.effort) }}
            <span class="opacity-70">{{ item.share.toFixed(0) }}%</span>
          </span>
        </div>
        <p v-else class="text-xs text-gray-400">{{ t('keyUsageDashboard.noData') }}</p>
      </div>

      <p v-if="detail.large_request_count > 0" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
        {{ t('keyUsageDashboard.current.largeRequests', {
          count: detail.large_request_count,
          size: formatBytes(detail.large_request_avg_bytes)
        }) }}
      </p>

      <RouterLink
        :to="{ path: '/usage', query: { api_key_id: String(detail.api_key_id) } }"
        class="inline-flex w-full items-center justify-center gap-1.5 rounded-lg border border-gray-200 px-3 py-2 text-xs font-medium text-gray-600 transition-colors hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300 dark:hover:border-primary-700 dark:hover:text-primary-300"
      >
        {{ t('keyUsageDashboard.current.viewUsageLogs') }}
        <Icon name="arrowRight" size="xs" />
      </RouterLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import {
  getKeyUsageDashboardDetail,
  type KeyUsageDashboardDetail,
  type KeyUsageDashboardMetric,
  type KeyUsageDashboardParams
} from '@/api/keyUsageDashboard'
import {
  MODEL_BAR_COLORS,
  formatBytes,
  formatCost,
  formatLatency,
  formatPercent,
  metricDisplay,
  metricValue,
  toFiniteNumber
} from './utils'

const { t } = useI18n()

const props = defineProps<{
  apiKeyId?: number
  keyName?: string
  groupName?: string
  params: KeyUsageDashboardParams
  metric: KeyUsageDashboardMetric
}>()

const detail = ref<KeyUsageDashboardDetail | null>(null)
const loading = ref(false)
const error = ref(false)
let controller: AbortController | undefined
let generation = 0

const metricItems = computed(() => {
  if (!detail.value) return []
  return [
    { label: t('keyUsageDashboard.metrics.cost'), value: formatCost(detail.value.totals.cost) },
    { label: t('keyUsageDashboard.metrics.requests'), value: Math.round(detail.value.totals.requests).toLocaleString() },
    { label: t('keyUsageDashboard.current.cacheHit'), value: formatPercent(detail.value.cache_hit_rate) },
    { label: t('keyUsageDashboard.current.averageFirstToken'), value: formatLatency(detail.value.avg_first_token_ms) },
    { label: t('keyUsageDashboard.current.errorRate'), value: formatPercent(detail.value.error_rate, 2) },
    { label: t('keyUsageDashboard.current.peakRpm'), value: Math.round(detail.value.peak_rpm).toLocaleString() }
  ]
})

const maxHourRequests = computed(() =>
  Math.max(0, ...(detail.value?.hour_of_day || []).map((hour) => toFiniteNumber(hour.requests)))
)

const modelItems = computed(() => {
  const models = detail.value?.models || []
  const total = models.reduce((sum, model) => sum + metricValue(model.metrics, props.metric), 0)
  return models.map((model, index) => {
    const value = metricValue(model.metrics, props.metric)
    const isOther = ['其他', 'other', 'others'].includes(model.model.toLowerCase())
    return {
      model: model.model,
      label: isOther ? t('keyUsageDashboard.current.other') : model.model,
      valueLabel: metricDisplay(value, props.metric),
      share: total > 0 ? (value / total) * 100 : 0,
      colorClass: isOther
        ? 'bg-gray-400'
        : MODEL_BAR_COLORS[index % MODEL_BAR_COLORS.length]
    }
  })
})

const totalEffortRequests = computed(() =>
  (detail.value?.efforts || []).reduce((sum, effort) => sum + toFiniteNumber(effort.requests), 0)
)

const effortItems = computed(() =>
  (detail.value?.efforts || []).map((effort) => ({
    effort: effort.effort || 'unknown',
    share: totalEffortRequests.value > 0
      ? (toFiniteNumber(effort.requests) / totalEffortRequests.value) * 100
      : 0
  }))
)

function hourBarHeight(requests: number): string {
  if (maxHourRequests.value <= 0) return '2px'
  return `${Math.max(2, (toFiniteNumber(requests) / maxHourRequests.value) * 100)}%`
}

function effortLabel(effort: string): string {
  const normalized = effort.toLowerCase()
  if (normalized === 'low') return t('keyUsageDashboard.current.effortLow')
  if (normalized === 'medium') return t('keyUsageDashboard.current.effortMedium')
  if (normalized === 'high') return t('keyUsageDashboard.current.effortHigh')
  if (normalized === 'xhigh') return t('keyUsageDashboard.current.effortXHigh')
  return t('keyUsageDashboard.current.effortUnknown')
}

function effortClass(effort: string): string {
  switch (effort.toLowerCase()) {
    case 'low':
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
    case 'medium':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'high':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300'
    case 'xhigh':
      return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
  }
}

function isAbortError(value: unknown): boolean {
  if (!value || typeof value !== 'object') return false
  const { name, code } = value as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

async function loadDetail(): Promise<void> {
  if (!props.apiKeyId) {
    detail.value = null
    return
  }
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  error.value = false
  detail.value = null
  try {
    const result = await getKeyUsageDashboardDetail(
      props.apiKeyId,
      props.params,
      { signal: controller.signal }
    )
    if (current === generation) detail.value = result
  } catch (requestError) {
    if (current !== generation || isAbortError(requestError)) return
    error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

watch(
  () => [props.apiKeyId, props.params.range, props.params.group_id, props.params.model],
  () => { void loadDetail() },
  { immediate: true }
)

onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>
