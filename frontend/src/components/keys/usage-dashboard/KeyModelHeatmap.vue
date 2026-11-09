<template>
  <section class="card overflow-hidden" data-testid="key-model-heatmap">
    <div class="border-b border-gray-100 p-4 dark:border-dark-700">
      <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('keyUsageDashboard.keyModel.title') }}
      </h2>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('keyUsageDashboard.keyModel.subtitle', { metric: metricLabel }) }}
      </p>
    </div>
    <div class="overflow-x-auto">
      <table class="min-w-max border-separate border-spacing-0 text-xs">
        <thead>
          <tr>
            <th class="sticky left-0 z-10 min-w-40 border-b border-r border-gray-200 bg-gray-50 px-3 py-2 text-left font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400">
              {{ t('keyUsageDashboard.details.key') }}
            </th>
            <th
              v-for="model in models.names"
              :key="model"
              class="min-w-24 border-b border-gray-200 bg-gray-50 px-2 py-2 text-center font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
              :title="model"
            >
              <span class="block max-w-36 truncate">{{ model }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visibleRows" :key="row.apiKeyId">
            <th
              class="sticky left-0 z-10 max-w-52 border-b border-r border-gray-100 bg-white px-3 py-1.5 text-left font-medium text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
              scope="row"
            >
              <span class="block truncate" :title="row.name">{{ row.name }}</span>
            </th>
            <td
              v-for="columnIndex in row.cells.length"
              :key="`${row.apiKeyId}-${columnIndex}`"
              class="border-b border-gray-100 p-0.5 text-center dark:border-dark-700"
            >
              <div
                class="rounded-[3px] px-1 py-1.5 font-medium"
                :class="cellPercent(row, columnIndex - 1) > 0
                  ? `${heatmapClass(cellPercent(row, columnIndex - 1), 100)} text-gray-900 dark:text-white`
                  : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-dark-500'"
                :title="`${row.name} · ${models.names[columnIndex - 1] || ''} · ${formatPercentValue(row, columnIndex - 1)}`"
              >
                {{ formatPercentValue(row, columnIndex - 1) }}
              </div>
            </td>
          </tr>
          <tr v-if="visibleRows.length === 0">
            <td :colspan="models.names.length + 1" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('keyUsageDashboard.noData') }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div
      v-if="rows.length > initialRowLimit"
      class="border-t border-gray-100 px-4 py-3 text-right dark:border-dark-700"
    >
      <button
        type="button"
        class="inline-flex items-center gap-1 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
        data-testid="key-model-expand"
        @click="expanded = !expanded"
      >
        <Icon :name="expanded ? 'chevronUp' : 'chevronDown'" size="xs" />
        {{
          expanded
            ? t('keyUsageDashboard.keyModel.collapse')
            : t('keyUsageDashboard.keyModel.showAll', { total: rows.length })
        }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type {
  KeyUsageDashboardKey,
  KeyUsageDashboardMetric,
  KeyUsageMetric
} from '@/api/keyUsageDashboard'
import { METRIC_LABEL_KEYS, heatmapClass, metricValue } from './utils'

const { t } = useI18n()
const initialRowLimit = 10
const expanded = ref(false)

const props = defineProps<{
  keys: KeyUsageDashboardKey[]
  models: {
    names: string[]
    keys: Array<{ api_key_id: number; cells: KeyUsageMetric[] }>
  }
  metric: KeyUsageDashboardMetric
}>()

const metricLabel = computed(() => t(METRIC_LABEL_KEYS[props.metric]))

const rows = computed(() => {
  const keyMap = new Map(props.keys.map((key) => [key.api_key_id, key]))
  return props.models.keys
    .map((modelKey) => ({
      apiKeyId: modelKey.api_key_id,
      name: keyMap.get(modelKey.api_key_id)?.name || `#${modelKey.api_key_id}`,
      cells: modelKey.cells || []
    }))
    .sort((left, right) => {
      const leftTotal = left.cells.reduce((sum, cell) => sum + metricValue(cell, props.metric), 0)
      const rightTotal = right.cells.reduce((sum, cell) => sum + metricValue(cell, props.metric), 0)
      return rightTotal - leftTotal
    })
})

const visibleRows = computed(() =>
  expanded.value ? rows.value : rows.value.slice(0, initialRowLimit)
)

function cellPercent(
  row: { cells: KeyUsageMetric[] },
  index: number
): number {
  const total = row.cells.reduce((sum, cell) => sum + metricValue(cell, props.metric), 0)
  if (total <= 0) return 0
  return (metricValue(row.cells[index], props.metric) / total) * 100
}

function formatPercentValue(row: { cells: KeyUsageMetric[] }, index: number): string {
  const percentage = cellPercent(row, index)
  return percentage > 0 ? `${percentage.toFixed(1)}%` : '—'
}
</script>
