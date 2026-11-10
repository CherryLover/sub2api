<template>
  <div class="flex flex-col gap-5 rounded-xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700/60 dark:bg-dark-900/40 lg:flex-row">
    <div class="min-w-0 flex-1">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
          {{ t('admin.usage.rankingDetail.models', { count: models.length }) }}
        </p>
        <p class="text-[10px] text-gray-400 dark:text-gray-500">{{ t('admin.usage.rankingDetail.costBasis') }}</p>
      </div>

      <div v-if="models.length === 0" class="py-6 text-center text-xs text-gray-400 dark:text-gray-500">
        {{ t('admin.usage.rankingDetail.noModels') }}
      </div>
      <div v-else class="mt-3 space-y-3">
        <div
          class="hidden grid-cols-[minmax(0,1.2fr)_70px_78px_84px_132px_132px] gap-2 text-[10px] text-gray-400 dark:text-gray-500 sm:grid"
        >
          <span>{{ t('admin.usage.rankingDetail.model') }}</span>
          <span class="text-right">{{ t('admin.usage.rankingDetail.requests') }}</span>
          <span class="text-right">{{ t('admin.usage.rankingDetail.tokens') }}</span>
          <span class="text-right">{{ t('admin.usage.rankingDetail.cost') }}</span>
          <span class="text-right">{{ t('admin.usage.rankingDetail.selfShare') }}</span>
          <span class="text-right">{{ t('admin.usage.rankingDetail.overallShare') }}</span>
        </div>

        <div
          v-for="model in modelRows"
          :key="model.model"
          class="grid grid-cols-1 gap-1 border-t border-gray-100 pt-2 text-xs dark:border-dark-700/50 sm:grid-cols-[minmax(0,1.2fr)_70px_78px_84px_132px_132px] sm:items-center sm:gap-2 sm:border-0 sm:pt-0"
        >
          <span class="truncate font-medium text-gray-700 dark:text-gray-200" :title="model.model">{{ model.model }}</span>
          <span class="text-right tabular-nums text-gray-500 dark:text-gray-400">{{ model.requests.toLocaleString() }}</span>
          <span class="text-right tabular-nums text-gray-500 dark:text-gray-400">{{ formatCompactNumber(model.tokens) }}</span>
          <span class="text-right font-medium tabular-nums text-gray-700 dark:text-gray-200">${{ formatCostFixed(model.actual_cost, 2) }}</span>
          <div class="flex items-center justify-end gap-1.5">
            <div class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
              <div class="h-full rounded-full bg-primary-500" :style="{ width: model.selfWidth }" />
            </div>
            <span class="w-10 text-right tabular-nums text-gray-500 dark:text-gray-400">{{ model.selfPct }}%</span>
          </div>
          <div class="flex items-center justify-end gap-1.5">
            <div class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
              <div class="h-full rounded-full bg-gray-400 dark:bg-dark-500" :style="{ width: model.overallWidth }" />
            </div>
            <span class="w-10 text-right tabular-nums text-gray-400 dark:text-gray-500">{{ model.overallPct }}%</span>
          </div>
        </div>
      </div>

      <div class="mt-3 flex flex-wrap items-center gap-4 text-[10px] text-gray-400 dark:text-gray-500">
        <span class="inline-flex items-center gap-1.5">
          <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />{{ t('admin.usage.rankingDetail.legendSelf') }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-1.5 w-1.5 rounded-full bg-gray-400 dark:bg-dark-500" />{{ t('admin.usage.rankingDetail.legendOverall') }}
        </span>
      </div>
    </div>

    <div class="w-full shrink-0 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700/60 dark:bg-dark-800/60 lg:w-[188px]">
      <p class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
        {{ t('admin.usage.rankingDetail.summary') }}
      </p>
      <dl class="mt-2 space-y-2 text-xs">
        <div class="flex justify-between gap-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.rankingDetail.requests') }}</dt>
          <dd class="tabular-nums text-gray-700 dark:text-gray-200">{{ requests.toLocaleString() }}</dd>
        </div>
        <div class="flex justify-between gap-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.rankingDetail.tokens') }}</dt>
          <dd class="tabular-nums text-gray-700 dark:text-gray-200">{{ formatCompactNumber(totalTokens) }}</dd>
        </div>
        <div class="flex justify-between gap-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.rankingDetail.cost') }}</dt>
          <dd class="tabular-nums text-gray-700 dark:text-gray-200">${{ formatCostFixed(rowCost, 2) }}</dd>
        </div>
        <div class="flex justify-between gap-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.rankingDetail.overallShare') }}</dt>
          <dd class="font-semibold tabular-nums text-primary-600 dark:text-primary-400">{{ overallPct }}%</dd>
        </div>
        <div class="flex justify-between gap-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.rankingDetail.avgPerRequest') }}</dt>
          <dd class="tabular-nums text-gray-700 dark:text-gray-200">${{ avgPerRequest }}</dd>
        </div>
      </dl>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatCompactNumber, formatCostFixed } from '@/utils/format'
import type { BreakdownModelShare } from '@/types'

const props = defineProps<{
  models: BreakdownModelShare[]
  rowCost: number
  overallCost: number
  requests: number
  totalTokens: number
}>()

const { t } = useI18n()

const clampPct = (value: number) => Math.min(Math.max(value, 0), 100)

const modelRows = computed(() =>
  (props.models ?? []).map((model) => {
    const selfPct = props.rowCost > 0 ? clampPct((model.actual_cost / props.rowCost) * 100) : 0
    const overallPct = props.overallCost > 0 ? clampPct((model.actual_cost / props.overallCost) * 100) : 0
    return {
      ...model,
      selfPct: selfPct.toFixed(1),
      overallPct: overallPct.toFixed(1),
      selfWidth: `${Math.max(selfPct, 1)}%`,
      overallWidth: `${Math.max(overallPct, 1)}%`
    }
  })
)

const overallPct = computed(() => {
  if (props.overallCost <= 0) return '0.0'
  return (clampPct((props.rowCost / props.overallCost) * 100)).toFixed(1)
})

const avgPerRequest = computed(() => {
  if (props.requests <= 0) return '0.00'
  return formatCostFixed(props.rowCost / props.requests, 2)
})
</script>
