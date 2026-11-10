<template>
  <div class="card p-4">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.rankingTrend.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ granularityLabel }}</p>
      </div>
      <div class="inline-flex items-center rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800">
        <button
          v-for="opt in metricOptions"
          :key="opt.value"
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="metric === opt.value
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="metric = opt.value"
        >
          {{ t(opt.label) }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex h-56 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="hasData && chartData" class="h-56">
      <Line :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="flex h-56 items-center justify-center text-sm text-gray-400 dark:text-gray-500">
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>

    <div
      v-if="hasData && !loading"
      class="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400"
    >
      <span>
        {{ t('admin.usage.rankingTrend.peak') }}
        <span class="font-medium text-gray-700 dark:text-gray-200">{{ peakLabel }}</span>
      </span>
      <span>
        {{ t('admin.usage.rankingTrend.average') }}
        <span class="font-medium text-gray-700 dark:text-gray-200">{{ averageLabel }}</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatCompactNumber, formatCostFixed } from '@/utils/format'
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const props = defineProps<{
  trendData: TrendDataPoint[]
  granularity: 'hour' | 'day'
  loading?: boolean
}>()

const { t } = useI18n()

type Metric = 'requests' | 'tokens' | 'cost'
const metric = ref<Metric>('requests')

const metricOptions: { value: Metric; label: string }[] = [
  { value: 'requests', label: 'admin.usage.rankingTrend.metricRequests' },
  { value: 'tokens', label: 'admin.usage.rankingTrend.metricTokens' },
  { value: 'cost', label: 'admin.usage.rankingTrend.metricCost' }
]

const METRIC_STYLE: Record<Metric, { color: string; label: string }> = {
  requests: { color: '#14b8a6', label: 'Requests' },
  tokens: { color: '#3b82f6', label: 'Tokens' },
  cost: { color: '#8b5cf6', label: 'Cost' }
}

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))
const textColor = computed(() => (isDarkMode.value ? '#e5e7eb' : '#374151'))
const gridColor = computed(() => (isDarkMode.value ? '#374151' : '#e5e7eb'))

const hasData = computed(() => (props.trendData?.length ?? 0) > 0)

const granularityLabel = computed(() =>
  props.granularity === 'hour'
    ? t('admin.usage.rankingTrend.hourly')
    : t('admin.usage.rankingTrend.daily')
)

const metricValue = (d: TrendDataPoint): number => {
  if (metric.value === 'requests') return d.requests
  if (metric.value === 'tokens') return d.total_tokens
  return d.actual_cost
}

const formatMetric = (value: number): string => {
  if (metric.value === 'requests') return formatCompactNumber(value)
  if (metric.value === 'tokens') return formatCompactNumber(value)
  return `$${formatCostFixed(value, 2)}`
}

const labelOf = (date: string): string => {
  if (props.granularity === 'hour') {
    const parts = date.split(' ')
    return parts.length > 1 ? parts[1] : date
  }
  return date.length >= 10 ? date.slice(5) : date
}

const chartData = computed(() => {
  if (!hasData.value) return null
  const style = METRIC_STYLE[metric.value]
  const values = props.trendData.map(metricValue)
  return {
    labels: props.trendData.map((d) => labelOf(d.date)),
    datasets: [
      {
        label: t(`admin.usage.rankingTrend.metric${metric.value === 'requests' ? 'Requests' : metric.value === 'tokens' ? 'Tokens' : 'Cost'}`),
        data: values,
        borderColor: style.color,
        backgroundColor: `${style.color}1f`,
        fill: true,
        tension: 0.3,
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 4
      }
    ]
  }
})

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' as const },
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.dataset.label}: ${formatMetric(Number(context.raw))}`
      }
    }
  },
  scales: {
    x: {
      grid: { display: false },
      ticks: { color: textColor.value, font: { size: 10 }, maxRotation: 0, autoSkipPadding: 12 }
    },
    y: {
      beginAtZero: true,
      grid: { color: gridColor.value },
      ticks: {
        color: textColor.value,
        font: { size: 10 },
        callback: (value: string | number) => formatMetric(Number(value))
      }
    }
  }
}))

const peakIndex = computed(() => {
  const rows = props.trendData ?? []
  let idx = -1
  let best = -1
  rows.forEach((row, i) => {
    const v = metricValue(row)
    if (v > best) {
      best = v
      idx = i
    }
  })
  return idx
})

const peakLabel = computed(() => {
  const row = props.trendData?.[peakIndex.value]
  if (!row) return '-'
  return `${labelOf(row.date)} · ${formatMetric(metricValue(row))}`
})

const averageLabel = computed(() => {
  const rows = props.trendData ?? []
  if (rows.length === 0) return '-'
  const total = rows.reduce((sum, row) => sum + metricValue(row), 0)
  return formatMetric(total / rows.length)
})
</script>
