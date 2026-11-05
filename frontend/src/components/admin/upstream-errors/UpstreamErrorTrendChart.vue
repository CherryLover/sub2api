<template>
  <section class="card p-4 sm:p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <Icon name="chart" size="md" class="text-gray-500 dark:text-dark-300" />
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t(prefix + 'dailyTrend') }}
        </h3>
      </div>
      <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900">
        <button
          v-for="option in dayOptions"
          :key="option"
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="days === option
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'"
          :aria-pressed="days === option"
          :data-testid="`trend-days-${option}`"
          @click="setDays(option)"
        >
          {{ t(prefix + `trend${option}d`) }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex h-72 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="error"
      class="flex h-72 items-center justify-center text-sm text-rose-600 dark:text-rose-300"
    >
      {{ t(prefix + 'trendLoadFailed') }}
    </div>
    <div v-else-if="points.length > 0 && chartData" class="mt-4 h-72">
      <Line :data="chartData" :options="lineOptions" />
    </div>
    <div
      v-else
      class="flex h-72 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t(prefix + 'trendNoData') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Title,
  Tooltip
} from 'chart.js'
import { Line } from 'vue-chartjs'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import {
  getUpstreamErrorRequestsTrend,
  type OpsUpstreamErrorTrendDays,
  type OpsUpstreamErrorTrendFilters,
  type OpsUpstreamErrorTrendPoint
} from '@/api/admin/ops'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const props = withDefaults(defineProps<{
  filters: OpsUpstreamErrorTrendFilters
  refreshKey?: number
}>(), {
  refreshKey: 0
})

const { t } = useI18n()
const prefix = 'admin.upstreamErrors.'
const dayOptions: OpsUpstreamErrorTrendDays[] = [7, 14, 30]
const days = ref<OpsUpstreamErrorTrendDays>(14)
const points = ref<OpsUpstreamErrorTrendPoint[]>([])
const loading = ref(false)
const error = ref(false)
const isDarkMode = ref(false)

let controller: AbortController | undefined
let generation = 0
let themeObserver: MutationObserver | undefined

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  total: '#475569',
  recovered: '#0d9488',
  failed: '#dc2626'
}))

const chartData = computed(() => {
  if (points.value.length === 0) return null
  return {
    labels: points.value.map((point) => point.date),
    datasets: [
      {
        label: t(prefix + 'trendTotal'),
        data: points.value.map((point) => point.total),
        borderColor: chartColors.value.total,
        backgroundColor: chartColors.value.total,
        tension: 0.28
      },
      {
        label: t(prefix + 'trendRecovered'),
        data: points.value.map((point) => point.recovered),
        borderColor: chartColors.value.recovered,
        backgroundColor: chartColors.value.recovered,
        tension: 0.28
      },
      {
        label: t(prefix + 'trendFailed'),
        data: points.value.map((point) => point.failed),
        borderColor: chartColors.value.failed,
        backgroundColor: chartColors.value.failed,
        borderDash: [6, 4],
        tension: 0.28
      }
    ]
  }
})

const lineOptions = computed(() => ({
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
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 16,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      mode: 'index' as const,
      intersect: false
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        maxRotation: 0,
        autoSkip: true,
        font: {
          size: 10
        }
      }
    },
    y: {
      beginAtZero: true,
      precision: 0,
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    }
  }
}))

function syncTheme(): void {
  isDarkMode.value = document.documentElement.classList.contains('dark')
}

async function fetchTrend(): Promise<void> {
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  error.value = false
  try {
    const result = await getUpstreamErrorRequestsTrend(
      { ...props.filters, days: days.value },
      { signal: controller.signal }
    )
    if (current !== generation) return
    points.value = result.days || []
  } catch {
    if (current !== generation) return
    points.value = []
    error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

function setDays(value: OpsUpstreamErrorTrendDays): void {
  if (days.value === value) return
  days.value = value
  void fetchTrend()
}

watch(
  () => props.filters,
  () => { void fetchTrend() },
  { deep: true, immediate: true }
)
watch(
  () => props.refreshKey,
  () => { void fetchTrend() }
)

onMounted(() => {
  syncTheme()
  themeObserver = new MutationObserver(syncTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  generation++
  controller?.abort()
  themeObserver?.disconnect()
})
</script>
