<template>
  <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-6" data-testid="usage-stats-cards">
    <article
      v-for="card in cards"
      :key="card.key"
      class="card min-h-[116px] p-4"
      :data-testid="`usage-stat-${card.key}`"
    >
      <div class="flex items-start justify-between gap-2">
        <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</p>
        <span class="rounded-lg bg-gray-100 p-1.5 text-gray-500 dark:bg-dark-700 dark:text-dark-300">
          <Icon :name="card.icon" size="sm" />
        </span>
      </div>
      <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</p>
      <div class="mt-2 min-h-5 text-xs text-gray-500 dark:text-gray-400">
        <span v-if="card.delta !== undefined" class="inline-flex items-center gap-1">
          <span class="inline-flex items-center gap-1" :class="deltaClass(card.delta, card.lowerIsBetter)">
            <Icon
              v-if="card.delta !== 0"
              :name="card.delta > 0 ? 'arrowUp' : 'arrowDown'"
              size="xs"
            />
            <span>{{ formatDelta(card.delta) }}</span>
          </span>
          <span class="text-gray-400 dark:text-gray-500">{{ t('keyUsageDashboard.stats.vsPrevious') }}</span>
        </span>
        <span v-else>{{ card.hint }}</span>
      </div>
    </article>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { KeyUsageDashboardMetric, KeyUsageDashboardResponse } from '@/api/keyUsageDashboard'
import {
  METRIC_LABEL_KEYS,
  formatCompactNumber,
  formatCost,
  formatPercent,
  metricValue,
  toFiniteNumber
} from './utils'

const { t } = useI18n()

const props = defineProps<{
  data: KeyUsageDashboardResponse
  metric: KeyUsageDashboardMetric
}>()

interface StatCard {
  key: string
  label: string
  value: string
  hint?: string
  delta?: number
  lowerIsBetter?: boolean
  icon: 'dollar' | 'key' | 'calculator' | 'database' | 'xCircle' | 'clock' | 'trendingUp'
}

function percentageChange(current: number, previous: number): number | undefined {
  const currentValue = toFiniteNumber(current)
  const previousValue = toFiniteNumber(previous)
  if (previousValue === 0) return currentValue === 0 ? 0 : undefined
  return ((currentValue - previousValue) / previousValue) * 100
}

function metricDisplay(value: number): string {
  if (props.metric === 'cost') return formatCost(value)
  if (props.metric === 'tokens') return formatCompactNumber(value)
  return Math.round(value).toLocaleString()
}

function peakHourLabel(hour: number | undefined): string {
  return hour === undefined ? '—' : `${String(hour).padStart(2, '0')}:00`
}

function formatDelta(value: number | undefined): string {
  if (value === undefined) return '—'
  return `${value >= 0 ? '+' : ''}${value.toFixed(1)}%`
}

function deltaClass(delta: number | undefined, lowerIsBetter = false): string {
  if (delta === undefined || delta === 0) return 'text-gray-500 dark:text-gray-400'
  const isGood = lowerIsBetter ? delta < 0 : delta > 0
  return isGood ? 'text-[#059669] dark:text-emerald-400' : 'text-[#dc2626] dark:text-red-400'
}

const cards = computed<StatCard[]>(() => {
  const totalMetric = props.data.totals
  const prevMetric = props.data.prev_totals
  const peakHours = [...(props.data.peak_hours || [])]
    .sort((a, b) => b.requests - a.requests)
    .slice(0, 3)
  const activeRate = props.data.total_keys > 0
    ? (props.data.active_keys / props.data.total_keys) * 100
    : 0
  const totalDelta = percentageChange(
    metricValue(totalMetric, props.metric),
    metricValue(prevMetric, props.metric)
  )
  const averageCostDelta = percentageChange(
    props.data.avg_cost_per_request,
    props.data.prev_avg_cost_per_request
  )
  const cacheHitDelta = percentageChange(
    props.data.cache_hit_rate,
    props.data.prev_cache_hit_rate
  )
  const errorRateDelta = percentageChange(
    props.data.error_rate,
    props.data.prev_error_rate
  )
  const noBaseline = t('keyUsageDashboard.stats.noBaseline')

  return [
    {
      key: 'total',
      label: t('keyUsageDashboard.stats.total', { metric: t(METRIC_LABEL_KEYS[props.metric]) }),
      value: metricDisplay(metricValue(totalMetric, props.metric)),
      delta: totalDelta,
      hint: totalDelta === undefined ? noBaseline : undefined,
      icon: props.metric === 'cost' ? 'dollar' : props.metric === 'requests' ? 'trendingUp' : 'database'
    },
    {
      key: 'active-keys',
      label: t('keyUsageDashboard.stats.activeKeys'),
      value: `${props.data.active_keys} / ${props.data.total_keys}`,
      hint: t('keyUsageDashboard.stats.activeKeyRate', { rate: activeRate.toFixed(0) }),
      icon: 'key'
    },
    {
      key: 'average-cost',
      label: t('keyUsageDashboard.stats.averageCost'),
      value: formatCost(props.data.avg_cost_per_request),
      delta: averageCostDelta,
      hint: averageCostDelta === undefined ? noBaseline : undefined,
      lowerIsBetter: true,
      icon: 'calculator'
    },
    {
      key: 'cache-hit',
      label: t('keyUsageDashboard.stats.cacheHitRate'),
      value: formatPercent(props.data.cache_hit_rate),
      delta: cacheHitDelta,
      hint: cacheHitDelta === undefined ? noBaseline : undefined,
      icon: 'database'
    },
    {
      key: 'error-rate',
      label: t('keyUsageDashboard.stats.errorRate'),
      value: formatPercent(props.data.error_rate, 2),
      delta: errorRateDelta,
      hint: errorRateDelta === undefined ? noBaseline : undefined,
      lowerIsBetter: true,
      icon: 'xCircle'
    },
    {
      key: 'peak-hours',
      label: t('keyUsageDashboard.stats.peakHours'),
      value: peakHourLabel(peakHours[0]?.hour),
      hint: peakHours.slice(1)
        .map((item) => `${peakHourLabel(item.hour)} · ${formatCompactNumber(item.requests)}`)
        .join('  '),
      icon: 'clock'
    }
  ]
})
</script>
