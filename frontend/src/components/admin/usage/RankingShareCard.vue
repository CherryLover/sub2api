<template>
  <div class="card p-4">
    <div class="mb-3 flex flex-wrap items-start justify-between gap-2">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.rankingShare.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
          {{ mode === 'key' ? t('admin.usage.rankingShare.subtitleKeys') : t('admin.usage.rankingShare.subtitleUser') }}
        </p>
      </div>
      <span class="text-xs tabular-nums text-gray-400 dark:text-gray-500">{{ t('admin.usage.rankingShare.top') }}</span>
    </div>

    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="items.length === 0" class="flex h-48 items-center justify-center text-sm text-gray-400 dark:text-gray-500">
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
    <div v-else class="flex flex-col items-center gap-4 sm:flex-row sm:gap-5">
      <div class="relative h-40 w-40 shrink-0">
        <Doughnut :data="chartData" :options="chartOptions" />
        <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">${{ formatCostFixed(totalCost, 2) }}</span>
          <span class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('admin.usage.rankingShare.total') }}</span>
        </div>
      </div>

      <div class="w-full min-w-0 flex-1 space-y-1.5">
        <div
          v-for="item in items"
          :key="item.key"
          class="flex items-center gap-2 text-xs"
        >
          <span class="h-1.5 w-1.5 shrink-0 rounded-full" :style="{ backgroundColor: item.color }" />
          <span class="truncate text-gray-600 dark:text-gray-300" :title="item.label">{{ item.label }}</span>
          <span class="ml-auto shrink-0 tabular-nums text-gray-500 dark:text-gray-400">{{ item.pct }}%</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatCostFixed } from '@/utils/format'
import { getUserBreakdown, getAPIKeyBreakdown } from '@/api/admin/dashboard'

ChartJS.register(ArcElement, Tooltip, Legend)

const props = defineProps<{
  mode: 'user' | 'key'
  startDate: string
  endDate: string
  filters: Record<string, unknown>
  totalCost: number
}>()

const { t } = useI18n()

const PALETTE = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16'
]
const TOP_N = 6

const loading = ref(false)
const rows = ref<{ label: string; cost: number }[]>([])
let reqSeq = 0

const share = computed(() => {
  const total = props.totalCost > 0 ? props.totalCost : rows.value.reduce((sum, r) => sum + r.cost, 0)
  if (total <= 0) return { items: [] as { key: string; label: string; cost: number; pct: number; color: string }[] }
  const top = rows.value.slice(0, TOP_N)
  const rest = rows.value.slice(TOP_N).reduce((sum, r) => sum + r.cost, 0)
  const items = top.map((row, i) => ({
    key: `${i}-${row.label}`,
    label: row.label,
    cost: row.cost,
    pct: Number(((row.cost / total) * 100).toFixed(1)),
    color: PALETTE[i % PALETTE.length]
  }))
  const pctSum = items.reduce((sum, item) => sum + item.pct, 0)
  const restPct = Number(Math.max(100 - pctSum, 0).toFixed(1))
  const restTotal = rows.value.slice(TOP_N)
  if (restTotal.length > 0) {
    items.push({
      key: 'other',
      label: t('admin.usage.rankingShare.other', { count: restTotal.length }),
      cost: rest,
      pct: restPct,
      color: '#cbd5e1'
    })
  } else if (restPct > 0.05) {
    items.push({
      key: 'other',
      label: t('admin.usage.rankingShare.otherRest'),
      cost: Math.max(total - top.reduce((sum, r) => sum + r.cost, 0), 0),
      pct: restPct,
      color: '#cbd5e1'
    })
  }
  return { items }
})

const items = computed(() => share.value.items)

const chartData = computed(() => ({
  labels: items.value.map((item) => item.label),
  datasets: [
    {
      data: items.value.map((item) => item.cost),
      backgroundColor: items.value.map((item) => item.color),
      borderWidth: 0,
      hoverOffset: 4
    }
  ]
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  cutout: '68%',
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.label}: $${formatCostFixed(Number(context.raw), 2)}`
      }
    }
  }
}))

const load = async () => {
  const seq = ++reqSeq
  loading.value = true
  try {
    const params = {
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      sort_by: 'actual_cost' as const,
      limit: 200
    }
    if (props.mode === 'key') {
      const res = await getAPIKeyBreakdown(params)
      if (seq !== reqSeq) return
      rows.value = (res.api_keys || []).map((item) => ({
        label: item.name || `#${item.api_key_id}`,
        cost: item.actual_cost
      }))
    } else {
      const res = await getUserBreakdown(params)
      if (seq !== reqSeq) return
      rows.value = (res.users || []).map((item) => ({
        label: item.email || `User #${item.user_id}`,
        cost: item.actual_cost
      }))
    }
  } catch {
    if (seq === reqSeq) rows.value = []
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

watch(
  () => [props.mode, props.startDate, props.endDate, JSON.stringify(props.filters)],
  () => load(),
  { immediate: true }
)
</script>
