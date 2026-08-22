<template>
  <div v-if="rows.length > 0" class="overflow-x-auto" data-testid="window-model-table">
    <table class="w-full min-w-[32rem]">
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
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="(row, index) in rows"
          :key="`${row.model}-${index}`"
          data-testid="window-model-row"
          class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
        >
          <td class="px-4 py-2.5 text-sm font-medium text-gray-900 dark:text-white">
            <span class="block max-w-[16rem] truncate" :title="row.model">{{ row.model || '-' }}</span>
          </td>
          <td class="px-4 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-dark-200">{{ formatCount(row.requests) }}</td>
          <td class="px-4 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-dark-200">{{ formatCount(row.tokens) }}</td>
          <td class="px-4 py-2.5 text-right text-sm font-medium tabular-nums text-gray-900 dark:text-white">{{ formatUsd(row.cost_usd) }}</td>
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
import { formatCount, formatUsd } from '@/utils/keyUsageFormat'

const props = defineProps<{
  models: KeyUsageModelStat[] | null | undefined
}>()

const { t } = useI18n()

const rows = computed<KeyUsageModelStat[]>(() =>
  Array.isArray(props.models) ? props.models.filter(Boolean) : []
)
</script>
