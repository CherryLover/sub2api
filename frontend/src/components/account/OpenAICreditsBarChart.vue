<template>
  <div
    v-if="creditRows.length > 0"
    data-testid="openai-credits-bar-chart"
    class="mb-4 rounded-xl border border-gray-200/90 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900"
  >
    <!-- Header -->
    <div
      class="flex flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-5"
      :class="{ 'border-b border-gray-100 dark:border-dark-800': !collapsed }"
    >
      <div class="flex flex-wrap items-center gap-2.5">
        <span class="inline-flex h-6 w-6 items-center justify-center rounded-lg bg-teal-500/10 text-teal-600 dark:text-teal-400">
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3 13h4v8H3v-8zm7-8h4v16h-4V5zm7 4h4v12h-4V9z" />
          </svg>
        </span>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.creditsChart.title') }}
        </h3>
        <span class="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
          {{ t('admin.accounts.creditsChart.accountCount', { count: creditRows.length }) }}
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-4 text-xs">
        <div class="flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-full bg-teal-500" />
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.creditsChart.totalRemaining') }}:</span>
          <strong class="font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatCreditsAmount(summary.totalRemaining) }}
          </strong>
        </div>
        <div class="flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-full bg-amber-400" />
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.creditsChart.totalSpent') }}:</span>
          <strong class="font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatCreditsAmount(summary.totalSpent) }}
          </strong>
        </div>
        <div v-if="summary.unlimitedCount > 0" class="flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-full bg-indigo-500" />
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.creditsChart.unlimitedCount', { count: summary.unlimitedCount }) }}</span>
        </div>

        <button
          type="button"
          data-testid="credits-chart-toggle"
          class="rounded-md px-2 py-1 text-xs font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
          @click="collapsed = !collapsed"
        >
          {{ collapsed ? t('admin.accounts.creditsChart.expand') : t('admin.accounts.creditsChart.collapse') }}
        </button>
      </div>
    </div>

    <!-- Horizontal Bar Chart Body -->
    <div v-if="!collapsed" class="space-y-2.5 px-4 py-3.5 sm:px-5">
      <div
        v-for="row in creditRows"
        :key="row.account.id"
        data-testid="credits-chart-row"
        :data-account-id="row.account.id"
        class="grid grid-cols-1 items-center gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-gray-50 dark:hover:bg-dark-800/50 sm:grid-cols-[minmax(140px,220px)_1fr_auto]"
      >
        <!-- Account Name + Mode Badge -->
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate text-xs font-medium text-gray-900 dark:text-white" :title="row.account.name">
            {{ row.account.name }}
          </span>
          <span
            :class="[
              'inline-flex shrink-0 items-center rounded px-1.5 py-0.5 text-[10px] font-medium',
              getCodexCreditsModeBadgeClass(row.info.mode)
            ]"
            :title="t(`admin.accounts.creditsMode.${row.info.mode}Hint`)"
          >
            {{ t(`admin.accounts.creditsMode.${row.info.mode}`) }}
          </span>
        </div>

        <!-- Horizontal Stacked Bar (Remaining + Spent scaled to maxScale) -->
        <div class="min-w-0">
          <div
            v-if="row.info.unlimited"
            class="h-4 w-full overflow-hidden rounded-md bg-gradient-to-r from-indigo-500 via-teal-500 to-emerald-400 px-2 flex items-center"
          >
            <span class="text-[10px] font-semibold text-white">
              {{ t('admin.accounts.creditsChart.unlimited') }}
            </span>
          </div>
          <div
            v-else
            class="flex h-4 w-full overflow-hidden rounded-md bg-gray-100 dark:bg-dark-800"
            :title="`${t('admin.accounts.creditsChart.remaining')}: ${formatCreditsAmount(row.info.balance)} · ${t('admin.accounts.creditsChart.spent')}: ${formatCreditsAmount(row.info.spent)}`"
          >
            <div
              v-if="row.remainingBarPct > 0"
              data-testid="credits-bar-remaining"
              class="h-full bg-teal-500 transition-all duration-300"
              :style="{ width: `${row.remainingBarPct.toFixed(2)}%` }"
            />
            <div
              v-if="row.spentBarPct > 0"
              data-testid="credits-bar-spent"
              class="h-full bg-amber-400/90 transition-all duration-300"
              :style="{ width: `${row.spentBarPct.toFixed(2)}%` }"
            />
          </div>
        </div>

        <!-- Numeric Values -->
        <div class="flex shrink-0 items-center justify-end gap-2 text-xs tabular-nums">
          <template v-if="row.info.unlimited">
            <span class="font-semibold text-indigo-600 dark:text-indigo-400">∞</span>
          </template>
          <template v-else>
            <span class="font-semibold text-gray-900 dark:text-white">
              {{ formatCreditsAmount(row.info.balance) }}
            </span>
            <span v-if="row.info.spent > 0" class="text-[11px] text-amber-600 dark:text-amber-400">
              (-{{ formatCreditsAmount(row.info.spent) }})
            </span>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  formatCreditsAmount,
  getCodexCreditsModeBadgeClass,
  getOpenAICreditsInfo,
} from '@/utils/openaiCredits'

const props = defineProps<{
  accounts: Account[]
}>()

const { t } = useI18n()
const collapsed = ref(false)

const rawRows = computed(() => {
  const list = Array.isArray(props.accounts) ? props.accounts : []
  return list
    .filter((acc) => acc.platform === 'openai' && acc.type === 'oauth')
    .map((account) => ({
      account,
      info: getOpenAICreditsInfo(account),
    }))
    .filter((item) => item.info.hasData && (item.info.unlimited || item.info.peakBalance > 0 || item.info.hasCredits))
    .sort((a, b) => {
      if (a.info.unlimited !== b.info.unlimited) return a.info.unlimited ? -1 : 1
      if (b.info.balance !== a.info.balance) return b.info.balance - a.info.balance
      return b.info.peakBalance - a.info.peakBalance
    })
})

const maxScale = computed(() => {
  let max = 0
  for (const r of rawRows.value) {
    if (!r.info.unlimited && r.info.peakBalance > max) {
      max = r.info.peakBalance
    }
  }
  return max
})

const creditRows = computed(() => {
  const scale = maxScale.value
  return rawRows.value.map((r) => {
    let remainingBarPct = 0
    let spentBarPct = 0
    if (!r.info.unlimited && scale > 0) {
      remainingBarPct = (r.info.balance / scale) * 100
      spentBarPct = (r.info.spent / scale) * 100
      if (r.info.balance > 0 && remainingBarPct < 1.5) remainingBarPct = 1.5
      if (r.info.spent > 0 && spentBarPct < 1.5) spentBarPct = 1.5
      const sum = remainingBarPct + spentBarPct
      if (sum > 100) {
        remainingBarPct = (remainingBarPct / sum) * 100
        spentBarPct = (spentBarPct / sum) * 100
      }
    }
    return {
      ...r,
      remainingBarPct,
      spentBarPct,
    }
  })
})

const summary = computed(() => {
  let totalRemaining = 0
  let totalSpent = 0
  let unlimitedCount = 0
  for (const r of rawRows.value) {
    if (r.info.unlimited) {
      unlimitedCount += 1
    } else {
      totalRemaining += r.info.balance
      totalSpent += r.info.spent
    }
  }
  return { totalRemaining, totalSpent, unlimitedCount }
})
</script>
