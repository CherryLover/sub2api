<template>
  <div v-if="families.length > 0" data-testid="model-family-summary" class="space-y-3">
    <div v-if="title || basisNote" class="flex flex-wrap items-center justify-between gap-2">
      <span v-if="title" class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
        {{ title }}
      </span>
      <span v-if="basisNote" class="text-[11px] text-gray-400 dark:text-dark-400">
        {{ basisNote }}
      </span>
    </div>

    <!-- Segmented proportion bar -->
    <div
      class="flex h-2.5 w-full overflow-hidden rounded-full bg-gray-200/80 dark:bg-dark-700/80"
      role="img"
      :aria-label="barAriaLabel"
    >
      <div
        v-for="item in families"
        :key="item.family"
        :class="['h-full transition-all duration-300', item.barClass]"
        :style="{ width: segmentWidth(item.sharePct) }"
        :title="`${displayLabel(item)}: ${formatShare(item.sharePct)} · ${formatUsd(item.cost)} · ${formatCount(item.requests)}`"
      />
    </div>

    <!-- Family breakdown cards -->
    <div class="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      <div
        v-for="item in families"
        :key="item.family"
        data-testid="model-family-card"
        :data-family="item.family"
        class="rounded-xl border border-gray-200/80 bg-white px-3.5 py-2.5 dark:border-dark-700/80 dark:bg-dark-900/70"
      >
        <div class="flex items-center justify-between gap-2">
          <div class="flex min-w-0 items-center gap-2">
            <span :class="['h-2.5 w-2.5 shrink-0 rounded-full', item.dotClass]" />
            <span class="truncate text-sm font-semibold text-gray-900 dark:text-white">
              {{ displayLabel(item) }}
            </span>
          </div>
          <span class="shrink-0 text-xs font-semibold tabular-nums text-gray-700 dark:text-dark-200">
            {{ formatShare(item.sharePct) }}
          </span>
        </div>

        <div class="mt-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-xs">
          <span class="font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatUsd(item.cost) }}
          </span>
          <span class="tabular-nums text-gray-500 dark:text-dark-400">
            {{ formatCount(item.requests) }} {{ requestsUnit }} · {{ formatCount(item.tokens) }} tok
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  aggregateModelFamilies,
  type ModelFamilyAggregate,
  type ModelUsageInput,
} from '@/utils/modelFamily'
import { formatCount, formatUsd } from '@/utils/keyUsageFormat'

const props = withDefaults(
  defineProps<{
    models: ModelUsageInput[] | null | undefined
    title?: string
    basisNote?: string
    otherLabel?: string
    requestsUnit?: string
  }>(),
  {
    title: '',
    basisNote: '',
    otherLabel: '',
    requestsUnit: '',
  }
)

const { t } = useI18n()

const families = computed<ModelFamilyAggregate[]>(() =>
  aggregateModelFamilies(Array.isArray(props.models) ? props.models : [])
)

const displayLabel = (item: ModelFamilyAggregate): string => {
  if (item.family === 'other') {
    return props.otherLabel || item.label
  }
  return item.label
}

const requestsUnit = computed(() => props.requestsUnit || t('keyUsage.requests'))

const formatShare = (pct: number): string => {
  if (!Number.isFinite(pct) || pct <= 0) return '0%'
  if (pct < 0.1) return '<0.1%'
  if (pct >= 10) return `${pct.toFixed(1)}%`
  return `${pct.toFixed(1)}%`
}

const segmentWidth = (pct: number): string => {
  if (!Number.isFinite(pct) || pct <= 0) return '0%'
  return `${Math.max(pct, 1.5).toFixed(2)}%`
}

const barAriaLabel = computed(() =>
  families.value.map((f) => `${displayLabel(f)} ${formatShare(f.sharePct)}`).join(', ')
)
</script>
