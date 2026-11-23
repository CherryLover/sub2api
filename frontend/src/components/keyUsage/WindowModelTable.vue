<template>
  <div v-if="rows.length > 0" class="overflow-x-auto" data-testid="window-model-table">
    <table class="w-full min-w-[36rem]">
      <thead>
        <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-950">
          <th class="px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.model') }}
          </th>
          <th class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.requests') }}
          </th>
          <th class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.totalTokens') }}
          </th>
          <th class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.cost') }}
          </th>
          <th class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.windows.share') }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="(row, index) in enrichedRows"
          :key="`${row.model}-${index}`"
          data-testid="window-model-row"
          class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
        >
          <td class="px-4 py-2.5 text-sm font-medium text-gray-900 dark:text-white">
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'inline-flex shrink-0 items-center rounded-md px-2 py-0.5 text-[11px] font-medium',
                  row.familyMeta.badgeClass
                ]"
              >
                {{ row.familyMeta.key === 'other' ? t('keyUsage.windows.otherFamily') : row.familyMeta.label }}
              </span>
              <span class="block max-w-[16rem] truncate" :title="row.model">{{ row.model || '-' }}</span>
            </div>
          </td>
          <td class="px-4 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-dark-200" :title="formatCountExact(row.requests)">{{ formatCount(row.requests) }}</td>
          <td class="px-4 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-dark-200" :title="formatCountExact(row.tokens)">{{ formatCount(row.tokens) }}</td>
          <td class="px-4 py-2.5 text-right text-sm font-medium tabular-nums text-gray-900 dark:text-white">{{ formatUsd(row.cost_usd) }}</td>
          <td class="px-4 py-2.5 text-right text-xs tabular-nums text-gray-600 dark:text-dark-300">
            <div class="flex items-center justify-end gap-2">
              <div class="h-1.5 w-14 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                <div
                  :class="['h-full rounded-full', row.familyMeta.barClass]"
                  :style="{ width: `${Math.min(Math.max(row.sharePct, 0), 100).toFixed(1)}%` }"
                />
              </div>
              <span class="w-10 text-right">{{ row.sharePct.toFixed(1) }}%</span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <p v-else data-testid="window-model-empty" class="px-5 py-8 text-center text-sm text-gray-500 dark:text-dark-400 sm:px-8">
    {{ t('keyUsage.windows.noModels') }}
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KeyUsageModelStat } from '@/api/keyUsage'
import { formatCount, formatCountExact, formatUsd } from '@/utils/keyUsageFormat'
import { getModelFamilyMeta } from '@/utils/modelFamily'

const props = defineProps<{
  models: KeyUsageModelStat[] | null | undefined
}>()

const { t } = useI18n()

const rows = computed<KeyUsageModelStat[]>(() =>
  Array.isArray(props.models) ? props.models.filter(Boolean) : []
)

const enrichedRows = computed(() => {
  let totalCost = 0
  let totalTokens = 0
  let totalRequests = 0
  for (const r of rows.value) {
    totalCost += Number(r.cost_usd) || 0
    totalTokens += Number(r.tokens) || 0
    totalRequests += Number(r.requests) || 0
  }

  return rows.value.map((r) => {
    const cost = Number(r.cost_usd) || 0
    const tokens = Number(r.tokens) || 0
    const requests = Number(r.requests) || 0
    let sharePct = 0
    if (totalCost > 0) {
      sharePct = (cost / totalCost) * 100
    } else if (totalTokens > 0) {
      sharePct = (tokens / totalTokens) * 100
    } else if (totalRequests > 0) {
      sharePct = (requests / totalRequests) * 100
    }
    return {
      ...r,
      sharePct,
      familyMeta: getModelFamilyMeta(r.model),
    }
  })
})
</script>
