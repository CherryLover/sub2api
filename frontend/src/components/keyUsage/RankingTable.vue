<template>
  <div v-if="rows.length > 0" class="overflow-x-auto" data-testid="ranking-table">
    <table class="w-full min-w-[36rem]">
      <thead>
        <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-950">
          <th class="px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.rankings.rank') }}
          </th>
          <th class="px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
            {{ t('keyUsage.rankings.keyName') }}
          </th>
          <th
            class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider"
            :class="metric === 'requests' ? 'text-primary-600 dark:text-primary-300' : 'text-gray-500 dark:text-dark-400'"
          >{{ t('keyUsage.requests') }}</th>
          <th
            class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider"
            :class="metric === 'tokens' ? 'text-primary-600 dark:text-primary-300' : 'text-gray-500 dark:text-dark-400'"
          >{{ t('keyUsage.totalTokens') }}</th>
          <th
            class="px-4 py-2.5 text-right text-xs font-semibold uppercase tracking-wider"
            :class="metric === 'cost' ? 'text-primary-600 dark:text-primary-300' : 'text-gray-500 dark:text-dark-400'"
          >{{ t('keyUsage.cost') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in rows"
          :key="`${row.pinned ? 'self' : 'top'}-${row.entry.rank}-${row.entry.key_name}`"
          data-testid="ranking-row"
          :data-rank="row.entry.rank"
          :data-self="row.entry.is_self ? 'true' : 'false'"
          class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
          :class="row.entry.is_self
            ? 'bg-primary-500/10 dark:bg-primary-500/15'
            : 'hover:bg-gray-50 dark:hover:bg-dark-800/60'"
        >
          <td class="px-4 py-2.5 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">
            #{{ row.entry.rank }}
          </td>
          <td class="max-w-[14rem] px-4 py-2.5 text-sm text-gray-700 dark:text-dark-200">
            <div class="flex items-center gap-2">
              <span class="truncate" :title="row.entry.key_name">{{ row.entry.key_name }}</span>
              <span
                v-if="row.entry.is_self"
                data-testid="ranking-self-badge"
                class="shrink-0 rounded-full bg-primary-500/15 px-2 py-0.5 text-[10px] font-semibold text-primary-600 dark:text-primary-300"
              >{{ t('keyUsage.rankings.you') }}</span>
            </div>
          </td>
          <td
            class="px-4 py-2.5 text-right text-sm tabular-nums"
            :class="metric === 'requests' ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-700 dark:text-dark-200'"
          >{{ formatCount(row.entry.requests) }}</td>
          <td
            class="px-4 py-2.5 text-right text-sm tabular-nums"
            :class="metric === 'tokens' ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-700 dark:text-dark-200'"
          >{{ formatCount(row.entry.tokens) }}</td>
          <td
            class="px-4 py-2.5 text-right text-sm tabular-nums"
            :class="metric === 'cost' ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-700 dark:text-dark-200'"
          >{{ formatUsd(row.entry.cost_usd) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
  <p v-else data-testid="ranking-table-empty" class="px-6 py-6 text-center text-sm text-gray-500 dark:text-dark-400">
    {{ t('keyUsage.rankings.empty') }}
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KeyUsageMetric, KeyUsageRankingEntry, KeyUsageRankingWindow } from '@/api/keyUsage'
import { formatCount, formatUsd } from '@/utils/keyUsageFormat'

const props = defineProps<{
  data: KeyUsageRankingWindow | null
  metric: KeyUsageMetric
}>()

const { t } = useI18n()

interface Row {
  entry: KeyUsageRankingEntry
  /** Pinned rows are the querier's own key when it falls outside the top list. */
  pinned: boolean
}

const rows = computed<Row[]>(() => {
  const data = props.data
  if (!data) return []

  const top = Array.isArray(data.top) ? data.top.filter(Boolean) : []
  const result: Row[] = top.map((entry, index) => ({
    entry: { ...entry, rank: Number(entry.rank) || index + 1 },
    pinned: false,
  }))

  const self = data.self
  if (self && self.rank > 0 && !result.some(row => row.entry.is_self)) {
    result.push({ entry: { ...self, is_self: true }, pinned: true })
  }
  return result
})
</script>
