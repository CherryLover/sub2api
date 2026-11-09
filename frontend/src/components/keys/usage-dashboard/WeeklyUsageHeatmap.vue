<template>
  <section class="card overflow-hidden" data-testid="weekly-usage-heatmap">
    <div class="border-b border-gray-100 p-4 dark:border-dark-700">
      <div class="flex items-center gap-2">
        <Icon name="calendar" size="sm" class="text-primary-500" />
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('keyUsageDashboard.weekly.title') }}
        </h2>
      </div>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ subtitle }}
      </p>
    </div>
    <div class="overflow-x-auto">
      <table
        class="w-full table-fixed border-separate border-spacing-0 text-xs"
        style="min-width: 440px"
      >
        <thead>
          <tr>
            <th class="sticky left-0 z-10 w-14 min-w-14 max-w-14 border-b border-r border-gray-200 bg-gray-50 px-2 py-2 text-left font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400">
              {{ t('keyUsageDashboard.weekly.day') }}
            </th>
            <th
              v-for="hour in 24"
              :key="hour"
              class="border-b border-gray-200 bg-gray-50 px-0.5 py-2 text-center font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
            >
              {{ (hour - 1) % 3 === 0 ? String(hour - 1).padStart(2, '0') : '' }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(day, dayIndex) in week" :key="dayIndex">
            <th class="sticky left-0 z-10 w-14 min-w-14 max-w-14 border-b border-r border-gray-100 bg-white px-2 py-1.5 text-left font-medium text-gray-600 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">
              {{ dayLabels[dayIndex] }}
            </th>
            <td
              v-for="(cell, hourIndex) in day"
              :key="`${dayIndex}-${hourIndex}`"
              class="border-b border-gray-100 p-0.5 dark:border-dark-700"
            >
              <div
                tabindex="0"
                role="img"
                class="h-7 min-h-7 rounded-[3px] outline-none transition-[filter] hover:brightness-95 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 dark:focus:ring-offset-dark-900"
                :class="heatmapClass(metricValue(cell, metric), maxValue)"
                :title="cellTitle(dayIndex, hourIndex, cell)"
                :aria-label="cellTitle(dayIndex, hourIndex, cell)"
                :data-testid="`week-cell-${dayIndex}-${hourIndex}`"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type {
  KeyUsageDashboardMetric,
  KeyUsageDashboardRange,
  KeyUsageMetric
} from '@/api/keyUsageDashboard'
import {
  METRIC_LABEL_KEYS,
  heatmapClass,
  metricDisplay,
  metricValue
} from './utils'

const { t } = useI18n()

const props = defineProps<{
  week: KeyUsageMetric[][]
  range: KeyUsageDashboardRange
  metric: KeyUsageDashboardMetric
}>()

const dayLabels = computed(() => [
  t('keyUsageDashboard.weekly.monday'),
  t('keyUsageDashboard.weekly.tuesday'),
  t('keyUsageDashboard.weekly.wednesday'),
  t('keyUsageDashboard.weekly.thursday'),
  t('keyUsageDashboard.weekly.friday'),
  t('keyUsageDashboard.weekly.saturday'),
  t('keyUsageDashboard.weekly.sunday')
])

const subtitle = computed(() => props.range === 'today'
  ? t('keyUsageDashboard.weekly.todayScope', { metric: t(METRIC_LABEL_KEYS[props.metric]) })
  : t('keyUsageDashboard.weekly.scope', { metric: t(METRIC_LABEL_KEYS[props.metric]) })
)

const maxValue = computed(() => {
  let maximum = 0
  for (const day of props.week || []) {
    for (const cell of day || []) {
      maximum = Math.max(maximum, metricValue(cell, props.metric))
    }
  }
  return maximum
})

function cellTitle(dayIndex: number, hourIndex: number, cell: KeyUsageMetric): string {
  return `${dayLabels.value[dayIndex] || ''} ${String(hourIndex).padStart(2, '0')}:00 · ${metricDisplay(metricValue(cell, props.metric), props.metric)}`
}
</script>
