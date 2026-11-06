<template>
  <section class="card p-4" data-testid="key-usage-trend">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-2">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('keyUsageDashboard.trend.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('keyUsageDashboard.trend.subtitle', { metric: metricLabel }) }}
        </p>
      </div>
      <span class="rounded-full bg-gray-100 px-2.5 py-1 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">
        {{ t('keyUsageDashboard.trend.topKeys') }}
      </span>
    </div>
    <div v-if="chartData" class="h-72">
      <Line :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="flex h-72 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('keyUsageDashboard.noData') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type {
  KeyUsageDashboardKey,
  KeyUsageDashboardMetric,
  KeyUsageMetric
} from '@/api/keyUsageDashboard'
import {
  KEY_USAGE_OTHER_COLOR,
  KEY_USAGE_SERIES_COLORS,
  METRIC_LABEL_KEYS,
  formatCompactNumber,
  formatCost,
  metricDisplay,
  metricValue
} from './utils'

interface TrendRow {
  key: KeyUsageDashboardKey
  color: string
  series: number[]
}

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  keys: KeyUsageDashboardKey[]
  seriesLabels: string[]
  metric: KeyUsageDashboardMetric
}>()

const darkMode = ref(false)
let observer: MutationObserver | undefined

const syncDarkMode = (): void => {
  darkMode.value = document.documentElement.classList.contains('dark')
}

onMounted(() => {
  syncDarkMode()
  observer = new MutationObserver(syncDarkMode)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => observer?.disconnect())

const metricLabel = computed(() => t(METRIC_LABEL_KEYS[props.metric]))

const trendRows = computed<TrendRow[]>(() => {
  const columns = props.seriesLabels.length
  const sorted = [...props.keys]
    .filter((key) => metricValue(key.totals, props.metric) > 0)
    .sort((left, right) =>
      metricValue(right.totals, props.metric) - metricValue(left.totals, props.metric)
    )
  const leading = sorted.slice(0, 5)
  const trailing = sorted.slice(5)
  const result: TrendRow[] = leading.map((key, index) => ({
    key,
    color: KEY_USAGE_SERIES_COLORS[index],
    series: Array.from({ length: columns }, (_, column) =>
      metricValue(key.series?.[column], props.metric)
    )
  }))

  if (trailing.length > 0) {
    const other = Array.from({ length: columns }, (_, column) =>
      trailing.reduce(
        (sum, key) => sum + metricValue(key.series?.[column], props.metric),
        0
      )
    )
    result.push({
      key: {
        api_key_id: -1,
        name: t('keyUsageDashboard.trend.other'),
        group_name: '',
        status: 'active',
        last_used_at: null,
        totals: trailing.reduce<KeyUsageMetric>(
          (sum, key) => ({
            cost: sum.cost + key.totals.cost,
            requests: sum.requests + key.totals.requests,
            tokens: sum.tokens + key.totals.tokens
          }),
          { cost: 0, requests: 0, tokens: 0 }
        ),
        cost_per_request: 0,
        cache_hit_rate: 0,
        avg_first_token_ms: 0,
        error_count: 0,
        error_rate: 0,
        peak_rpm: 0,
        hour_of_day: [],
        series: []
      },
      color: KEY_USAGE_OTHER_COLOR,
      series: other
    })
  }
  return result
})

const chartData = computed(() => {
  if (props.seriesLabels.length === 0 || trendRows.value.length === 0) return null
  return {
    labels: props.seriesLabels,
    datasets: trendRows.value.map((row) => ({
      label: row.key.name,
      data: row.series,
      borderColor: row.color,
      backgroundColor: `${row.color}20`,
      fill: true,
      stack: 'usage',
      borderWidth: 2,
      pointRadius: 0,
      pointHoverRadius: 4,
      pointHoverBackgroundColor: row.color,
      pointHoverBorderColor: darkMode.value ? '#0f172a' : '#ffffff',
      pointHoverBorderWidth: 2,
      tension: 0.25
    }))
  }
})

const tickColor = computed(() => darkMode.value ? '#9ca3af' : '#6b7280')
const gridColor = computed(() => darkMode.value ? '#334155' : '#e5e7eb')
const legendColor = computed(() => darkMode.value ? '#e5e7eb' : '#374151')

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: legendColor.value,
        usePointStyle: true,
        pointStyle: 'line' as const,
        boxWidth: 18,
        boxHeight: 2,
        padding: 14,
        font: { size: 11 }
      }
    },
    tooltip: {
      mode: 'index' as const,
      intersect: false,
      callbacks: {
        label: (context: { dataset: { label?: string }; raw: unknown }) =>
          `${context.dataset.label || ''}: ${metricDisplay(Number(context.raw), props.metric)}`,
        footer: (items: Array<{ dataIndex: number }>) => {
          const dataIndex = items[0]?.dataIndex
          if (dataIndex === undefined) return ''
          const total = trendRows.value.reduce(
            (sum, row) => sum + (row.series[dataIndex] || 0),
            0
          )
          return t('keyUsageDashboard.trend.total', {
            value: metricDisplay(total, props.metric)
          })
        }
      }
    }
  },
  scales: {
    x: {
      stacked: true,
      grid: { color: gridColor.value, drawTicks: false },
      border: { display: false },
      ticks: { color: tickColor.value, maxRotation: 0, autoSkip: true, font: { size: 10 } }
    },
    y: {
      stacked: true,
      beginAtZero: true,
      grid: { color: gridColor.value },
      border: { display: false },
      ticks: {
        color: tickColor.value,
        font: { size: 10 },
        callback: (value: string | number) => props.metric === 'cost'
          ? formatCost(Number(value))
          : formatCompactNumber(Number(value))
      }
    }
  }
}))
</script>
