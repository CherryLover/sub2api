<template>
  <section class="card overflow-hidden" data-testid="usage-heatmap">
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('keyUsageDashboard.heatmap.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('keyUsageDashboard.heatmap.subtitle') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <div
          v-if="range !== 'today'"
          class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800"
        >
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium"
            :class="viewMode === 'hour'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 dark:text-gray-400'"
            :aria-pressed="viewMode === 'hour'"
            @click="viewMode = 'hour'"
          >
            {{ t('keyUsageDashboard.heatmap.byHour') }}
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium"
            :class="viewMode === 'date'
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 dark:text-gray-400'"
            :aria-pressed="viewMode === 'date'"
            @click="viewMode = 'date'"
          >
            {{ t('keyUsageDashboard.heatmap.byDate') }}
          </button>
        </div>
        <label class="relative">
          <Icon name="search" size="sm" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            v-model="search"
            type="search"
            class="input min-h-9 w-44 py-1.5 pl-8 text-xs"
            :placeholder="t('keyUsageDashboard.heatmap.filterKeys')"
            data-testid="usage-heatmap-search"
          />
        </label>
      </div>
    </div>

    <div class="max-h-[460px] overflow-auto" data-testid="usage-heatmap-scroll">
      <table
        class="w-full table-fixed border-separate border-spacing-0 text-xs"
        :style="{ minWidth: `${176 + columnLabels.length * 16}px` }"
      >
        <thead>
          <tr>
            <th
              class="sticky left-0 top-0 z-30 w-44 min-w-44 max-w-44 border-b border-r border-gray-200 bg-gray-50 px-3 py-2 text-left font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
            >
              {{ t('keyUsageDashboard.heatmap.key') }}
            </th>
            <th
              v-for="(label, index) in columnLabels"
              :key="`${viewMode}-${label}-${index}`"
              class="sticky top-0 z-20 border-b border-gray-200 bg-gray-50 px-1 py-2 text-center font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
              :title="columnTitle(label, index)"
            >
              {{ compactColumnLabel(label, index) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visibleRows" :key="row.api_key_id">
            <th
              class="sticky left-0 z-10 w-44 min-w-44 max-w-44 border-b border-r border-gray-100 bg-white px-3 py-2 text-left font-medium dark:border-dark-700 dark:bg-dark-900"
              :class="row.api_key_id === selectedKeyId
                ? 'text-primary-700 dark:text-primary-300'
                : 'text-gray-700 dark:text-gray-300'"
              scope="row"
            >
              <button
                type="button"
                class="flex w-full min-w-0 items-center gap-2 text-left"
                @click="emit('select', row.api_key_id)"
              >
                <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="row.totals.requests > 0 ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-500'" />
                <span class="truncate" :title="row.name">{{ row.name }}</span>
                <span v-if="row.group_name" class="ml-auto max-w-20 truncate text-[10px] font-normal text-gray-400">
                  {{ row.group_name }}
                </span>
              </button>
            </th>
            <td
              v-for="(label, index) in columnLabels"
              :key="`${row.api_key_id}-${viewMode}-${index}`"
              class="border-b border-gray-100 p-0.5 dark:border-dark-700"
            >
              <button
                type="button"
                class="h-8 min-h-8 w-full rounded-[3px] transition-[filter] hover:brightness-95 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 dark:focus:ring-offset-dark-900"
                :class="heatmapClass(cellValue(row, index), maxValue)"
                :title="cellTitle(row, label, index)"
                :aria-label="cellTitle(row, label, index)"
                :data-testid="`usage-cell-${row.api_key_id}-${index}`"
                @click="selectCell(row, label, index)"
              />
            </td>
          </tr>
          <tr v-if="visibleRows.length === 0">
            <td :colspan="columnLabels.length + 1" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('keyUsageDashboard.heatmap.empty') }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 px-4 py-3 dark:border-dark-700">
      <p
        v-if="selectedCell"
        class="min-w-0 flex-1 truncate text-xs text-gray-600 dark:text-gray-300"
        data-testid="usage-heatmap-cell-detail"
      >
        <span class="font-medium text-gray-900 dark:text-white">{{ selectedCell.keyName }}</span>
        <span class="mx-1.5 text-gray-400">·</span>
        <span>{{ selectedCell.label }}</span>
        <span class="mx-1.5 text-gray-400">·</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ selectedCell.value }}</span>
      </p>
      <p v-else class="text-xs text-gray-400 dark:text-gray-500">
        {{ t('keyUsageDashboard.heatmap.clickCellHint') }}
      </p>
      <button
        v-if="sortedRows.length > 15"
        type="button"
        class="ml-auto inline-flex items-center gap-1 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
        data-testid="usage-heatmap-expand"
        @click="expanded = !expanded"
      >
        <Icon :name="expanded ? 'chevronUp' : 'chevronDown'" size="xs" />
        {{
          expanded
            ? t('keyUsageDashboard.heatmap.collapse')
            : t('keyUsageDashboard.heatmap.showAll', {
                total: sortedRows.length,
                idle: idleCount
              })
        }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type {
  KeyUsageDashboardKey,
  KeyUsageDashboardMetric,
  KeyUsageDashboardRange
} from '@/api/keyUsageDashboard'
import {
  heatmapClass,
  metricDisplay,
  metricValue
} from './utils'

const { t } = useI18n()

const props = defineProps<{
  keys: KeyUsageDashboardKey[]
  seriesLabels: string[]
  range: KeyUsageDashboardRange
  metric: KeyUsageDashboardMetric
  selectedKeyId?: number
}>()

const emit = defineEmits<{
  select: [apiKeyId: number]
}>()

const viewMode = ref<'hour' | 'date'>('hour')
const expanded = ref(false)
const search = ref('')
const selectedCell = ref<{ keyName: string; label: string; value: string } | null>(null)

watch(() => props.range, (range) => {
  if (range === 'today') viewMode.value = 'hour'
})

const columnLabels = computed(() => {
  if (viewMode.value === 'date') return props.seriesLabels
  return Array.from({ length: 24 }, (_, hour) => `${hour}:00`)
})

const sortedRows = computed(() => [...props.keys].sort((left, right) =>
  metricValue(right.totals, props.metric) - metricValue(left.totals, props.metric)
))

const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return sortedRows.value
  return sortedRows.value.filter((row) =>
    row.name.toLowerCase().includes(query) ||
    (row.group_name || '').toLowerCase().includes(query)
  )
})

const visibleRows = computed(() =>
  expanded.value ? filteredRows.value : filteredRows.value.slice(0, 15)
)

const idleCount = computed(() => props.keys.filter((row) => row.totals.requests === 0).length)

const maxValue = computed(() => {
  let maximum = 0
  for (const row of sortedRows.value) {
    const cells = viewMode.value === 'hour' ? row.hour_of_day : row.series
    for (const cell of cells || []) {
      maximum = Math.max(maximum, metricValue(cell, props.metric))
    }
  }
  return maximum
})

function cellValue(row: KeyUsageDashboardKey, index: number): number {
  const cells = viewMode.value === 'hour' ? row.hour_of_day : row.series
  return metricValue(cells?.[index], props.metric)
}

function columnTitle(label: string, index: number): string {
  return viewMode.value === 'hour' ? `${String(index).padStart(2, '0')}:00` : label
}

function compactColumnLabel(label: string, index: number): string {
  if (viewMode.value === 'hour') return index % 2 === 0 ? String(index) : ''
  if (props.range === '30d') return index % 3 === 0 ? label : ''
  return label
}

function cellTitle(row: KeyUsageDashboardKey, label: string, index: number): string {
  const value = cellValue(row, index)
  return `${row.name} · ${columnTitle(label, index)} · ${metricDisplay(value, props.metric)}`
}

function selectCell(row: KeyUsageDashboardKey, label: string, index: number): void {
  selectedCell.value = {
    keyName: row.name,
    label: columnTitle(label, index),
    value: metricDisplay(cellValue(row, index), props.metric)
  }
}

</script>
