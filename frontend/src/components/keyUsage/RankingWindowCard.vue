<template>
  <section
    data-testid="ranking-window"
    :data-window="windowKey"
    class="overflow-hidden rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/90"
  >
    <header class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:px-8">
      <h4 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
        {{ title }}
      </h4>
      <p v-if="!rankingUnavailable && hasData" class="text-xs text-gray-500 dark:text-dark-400">
        <span v-if="selfRank > 0" data-testid="ranking-self-summary" class="font-semibold text-primary-600 dark:text-primary-300">
          {{ t('keyUsage.rankings.selfRank', { rank: selfRank, total: totalKeys }) }}
        </span>
        <span v-else data-testid="ranking-self-unranked">{{ t('keyUsage.rankings.selfUnranked') }}</span>
      </p>
    </header>

    <!--
      Three distinct states, on purpose:
        unavailable - the backend could not compute rankings at all (self_rank = 0 AND
                      total_keys = 0). Must never be rendered as "#1 of 1", which is
                      visually indistinguishable from real data.
        has data    - a podium/table, or at least a real rank for this Key.
        empty       - rankings worked, this window simply has no usage.
    -->
    <div v-if="rankingUnavailable" data-testid="ranking-window-unavailable" class="px-5 py-10 text-center sm:px-8">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('keyUsage.rankings.unavailable') }}</p>
    </div>

    <div v-else-if="hasData">
      <div class="px-5 py-6 sm:px-8">
        <RankingPodium :entries="topEntries" :metric="metric" />
      </div>
      <RankingTable :data="data" :metric="metric" />
    </div>

    <div v-else data-testid="ranking-window-empty" class="px-5 py-10 text-center sm:px-8">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('keyUsage.rankings.empty') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KeyUsageMetric, KeyUsageRankingWindow, KeyUsageWindowKey } from '@/api/keyUsage'
import RankingPodium from './RankingPodium.vue'
import RankingTable from './RankingTable.vue'

const props = defineProps<{
  title: string
  windowKey: KeyUsageWindowKey
  data: KeyUsageRankingWindow | null
  metric: KeyUsageMetric
}>()

const { t } = useI18n()

const topEntries = computed(() => {
  const top = props.data?.top
  return Array.isArray(top) ? top.filter(Boolean).slice(0, 3) : []
})

const totalKeys = computed(() => Number(props.data?.total_keys) || 0)
const selfRank = computed(() => Number(props.data?.self_rank) || 0)

/**
 * The backend encodes "ranking could not be computed" as total_keys = 0 AND self_rank = 0
 * with an empty `top` (see emptyKeyUsageRanking). Anything else is real data — a Key with
 * no usage on an otherwise empty site legitimately reports rank 1 of 1.
 */
const rankingUnavailable = computed(
  () => !props.data || (selfRank.value <= 0 && totalKeys.value <= 0 && topEntries.value.length === 0)
)

const hasData = computed(() => topEntries.value.length > 0 || Boolean(props.data?.self && selfRank.value > 0))
</script>
