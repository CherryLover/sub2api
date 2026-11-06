<template>
  <section
    class="sticky top-0 z-30 rounded-xl border border-gray-200 bg-white/95 p-3 shadow-sm backdrop-blur dark:border-dark-600 dark:bg-dark-900/95"
    data-testid="usage-dashboard-filters"
  >
    <div class="flex flex-wrap items-end gap-3">
      <div class="min-w-0">
        <span class="input-label">{{ t('keyUsageDashboard.filters.range') }}</span>
        <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800">
          <button
            v-for="option in rangeOptions"
            :key="option.value"
            type="button"
            class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
            :class="range === option.value
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            :aria-pressed="range === option.value"
            :data-testid="`key-usage-range-${option.value}`"
            @click="emit('update:range', option.value)"
          >
            {{ option.label }}
          </button>
        </div>
      </div>

      <div class="min-w-0">
        <span class="input-label">{{ t('keyUsageDashboard.filters.metric') }}</span>
        <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800">
          <button
            v-for="option in metricOptions"
            :key="option.value"
            type="button"
            class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
            :class="metric === option.value
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            :aria-pressed="metric === option.value"
            :data-testid="`key-usage-metric-${option.value}`"
            @click="emit('update:metric', option.value)"
          >
            {{ option.label }}
          </button>
        </div>
      </div>

      <label class="w-40">
        <span class="input-label">{{ t('keyUsageDashboard.filters.group') }}</span>
        <Select
          :model-value="group_id ?? ''"
          :options="groupOptions"
          @update:model-value="onGroupChange"
        />
      </label>

      <label class="w-48">
        <span class="input-label">{{ t('keyUsageDashboard.filters.model') }}</span>
        <Select
          :model-value="model ?? ''"
          :options="modelOptions"
          @update:model-value="onModelChange"
        />
      </label>

      <div class="ml-auto flex min-h-10 items-center gap-2">
        <p class="text-right text-xs text-gray-500 dark:text-gray-400">
          {{ summaryText }}
        </p>
        <button
          type="button"
          class="btn btn-secondary !p-2"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="emit('refresh')"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type {
  KeyUsageDashboardMetric,
  KeyUsageDashboardRange
} from '@/api/keyUsageDashboard'

const { t } = useI18n()

const props = defineProps<{
  range: KeyUsageDashboardRange
  metric: KeyUsageDashboardMetric
  group_id?: number
  model?: string
  groupOptions: Array<{ value: string | number | boolean | null; label: string }>
  modelOptions: Array<{ value: string | number | boolean | null; label: string }>
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:range': [value: KeyUsageDashboardRange]
  'update:metric': [value: KeyUsageDashboardMetric]
  'update:group': [value: number | undefined]
  'update:model': [value: string | undefined]
  refresh: []
}>()

const rangeOptions = computed<Array<{ value: KeyUsageDashboardRange; label: string }>>(() => [
  { value: 'today', label: t('keyUsageDashboard.ranges.today') },
  { value: '7d', label: t('keyUsageDashboard.ranges.7d') },
  { value: '30d', label: t('keyUsageDashboard.ranges.30d') }
])

const metricOptions = computed<Array<{ value: KeyUsageDashboardMetric; label: string }>>(() => [
  { value: 'cost', label: t('keyUsageDashboard.metrics.cost') },
  { value: 'requests', label: t('keyUsageDashboard.metrics.requests') },
  { value: 'tokens', label: t('keyUsageDashboard.metrics.tokens') }
])

const summaryText = computed(() => t('keyUsageDashboard.filters.scopeSummary', {
  range: t(`keyUsageDashboard.ranges.${props.range}`),
  metric: t(`keyUsageDashboard.metrics.${props.metric}`)
}))

function onGroupChange(value: string | number | boolean | null): void {
  const groupId = typeof value === 'number' ? value : Number(value)
  emit('update:group', Number.isFinite(groupId) && groupId > 0 ? groupId : undefined)
}

function onModelChange(value: string | number | boolean | null): void {
  const modelValue = String(value ?? '').trim()
  emit('update:model', modelValue || undefined)
}
</script>
