<template>
  <div
    v-if="slots.length > 0"
    data-testid="podium"
    class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-center sm:gap-4"
  >
    <!--
      DOM order is intentionally silver -> gold -> bronze so the desktop podium reads
      2nd (left) / 1st (center) / 3rd (right), the Olympic layout.
      On narrow screens the container becomes a column and the `order-*` classes put
      it back into 1 / 2 / 3 reading order while keeping the medal colours and labels.

      One slot == one medal, not one entry: the backend uses standard competition
      ranking (1224), so a medal rank can be shared by several Keys. Every tied Key is
      stacked inside its own medal slot — entries are never silently dropped, and the
      silver-left / gold-center / bronze-right layout is preserved.
    -->
    <div
      v-for="slot in slots"
      :key="slot.medal"
      data-testid="podium-slot"
      :data-rank="slot.rank"
      :data-medal="slot.medal"
      :data-tied="slot.entries.length > 1 ? 'true' : 'false'"
      class="flex w-full min-w-0 flex-col sm:w-36 sm:shrink-0"
      :class="[slot.orderClass, 'sm:order-none']"
    >
      <div class="flex min-w-0 flex-col gap-2">
        <div
          v-for="(entry, index) in slot.entries"
          :key="`${slot.medal}-${index}-${entry.key_name}`"
          data-testid="podium-entry"
          class="flex min-w-0 items-center gap-3 rounded-xl border p-3 transition-shadow sm:flex-col sm:gap-2 sm:text-center"
          :class="[
            slot.cardClass,
            index === slot.entries.length - 1 ? 'sm:rounded-b-none sm:pb-4' : '',
            entry.is_self ? 'ring-2 ring-primary-500 ring-offset-1 ring-offset-white dark:ring-offset-dark-900' : '',
          ]"
        >
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold shadow-sm"
            :class="slot.badgeClass"
          >{{ entry.rank }}</span>
          <div class="min-w-0 flex-1 text-left sm:w-full sm:flex-none sm:text-center">
            <p class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="entry.key_name">
              {{ entry.key_name }}
            </p>
            <p class="truncate text-xs text-gray-500 dark:text-dark-400">
              {{ slot.entries.length > 1 ? t('keyUsage.rankings.podiumTied', { medal: slot.medalLabel }) : slot.medalLabel }}
            </p>
          </div>
          <div class="shrink-0 text-right sm:w-full sm:text-center">
            <p class="text-sm font-bold tabular-nums text-gray-900 dark:text-white">
              {{ metricValueOf(entry, metric) }}
            </p>
            <span
              v-if="entry.is_self"
              data-testid="podium-self-badge"
              class="mt-1 inline-block rounded-full bg-primary-500/10 px-2 py-0.5 text-[10px] font-semibold text-primary-600 dark:text-primary-300"
            >{{ t('keyUsage.rankings.you') }}</span>
          </div>
        </div>
      </div>
      <!-- Pedestal: desktop only, height encodes the medal -->
      <div class="hidden rounded-b-xl sm:block" :class="[slot.barClass, slot.heightClass]"></div>
    </div>
  </div>
  <p v-else data-testid="podium-empty" class="py-6 text-center text-sm text-gray-500 dark:text-dark-400">
    {{ t('keyUsage.rankings.empty') }}
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KeyUsageMetric, KeyUsageRankingEntry } from '@/api/keyUsage'
import { metricValueOf } from '@/utils/keyUsageFormat'

const props = defineProps<{
  entries: KeyUsageRankingEntry[]
  metric: KeyUsageMetric
}>()

const { t } = useI18n()

type MedalRank = 1 | 2 | 3

interface MedalMeta {
  medal: 'gold' | 'silver' | 'bronze'
  labelKey: string
  orderClass: string
  heightClass: string
  cardClass: string
  badgeClass: string
  barClass: string
}

const MEDALS: Record<MedalRank, MedalMeta> = {
  1: {
    medal: 'gold',
    labelKey: 'keyUsage.rankings.podiumGold',
    orderClass: 'order-1',
    heightClass: 'sm:h-24',
    cardClass: 'border-amber-300 bg-amber-50/80 dark:border-amber-500/40 dark:bg-amber-500/10',
    badgeClass: 'bg-gradient-to-br from-amber-300 to-amber-500 text-amber-950',
    barClass: 'bg-gradient-to-b from-amber-400 to-amber-300/40 dark:from-amber-500/60 dark:to-amber-500/10',
  },
  2: {
    medal: 'silver',
    labelKey: 'keyUsage.rankings.podiumSilver',
    orderClass: 'order-2',
    heightClass: 'sm:h-16',
    cardClass: 'border-slate-300 bg-slate-50/80 dark:border-slate-500/40 dark:bg-slate-500/10',
    badgeClass: 'bg-gradient-to-br from-slate-200 to-slate-400 text-slate-800',
    barClass: 'bg-gradient-to-b from-slate-400 to-slate-300/40 dark:from-slate-500/60 dark:to-slate-500/10',
  },
  3: {
    medal: 'bronze',
    labelKey: 'keyUsage.rankings.podiumBronze',
    orderClass: 'order-3',
    heightClass: 'sm:h-10',
    cardClass: 'border-orange-300 bg-orange-50/80 dark:border-orange-500/40 dark:bg-orange-500/10',
    badgeClass: 'bg-gradient-to-br from-orange-300 to-orange-500 text-orange-950',
    barClass: 'bg-gradient-to-b from-orange-400 to-orange-300/40 dark:from-orange-500/60 dark:to-orange-500/10',
  },
}

interface PodiumSlot extends MedalMeta {
  rank: MedalRank
  entries: KeyUsageRankingEntry[]
  medalLabel: string
}

function medalRankOf(entry: KeyUsageRankingEntry, index: number): MedalRank | null {
  // `rank` is authoritative; fall back to array position when the payload omits it.
  const raw = Math.trunc(Number(entry.rank)) || index + 1
  if (raw === 1 || raw === 2 || raw === 3) return raw
  return null
}

const slots = computed<PodiumSlot[]>(() => {
  const byRank = new Map<MedalRank, KeyUsageRankingEntry[]>()
  const entries = Array.isArray(props.entries) ? props.entries : []

  entries.forEach((entry, index) => {
    if (!entry) return
    const rank = medalRankOf(entry, index)
    if (rank === null) return
    const bucket = byRank.get(rank)
    if (bucket) {
      // Tied Keys share the medal: keep every one of them instead of dropping
      // everything after the first (the backend's 1224 ranking skips numbers,
      // so de-duplicating by rank would blank out a whole medal slot).
      bucket.push({ ...entry, rank })
    } else {
      byRank.set(rank, [{ ...entry, rank }])
    }
  })

  // Silver (left) -> Gold (center) -> Bronze (right)
  const layout: MedalRank[] = [2, 1, 3]
  return layout
    .filter(rank => (byRank.get(rank)?.length ?? 0) > 0)
    .map(rank => ({
      ...MEDALS[rank],
      rank,
      entries: byRank.get(rank) as KeyUsageRankingEntry[],
      medalLabel: t(MEDALS[rank].labelKey),
    }))
})
</script>
