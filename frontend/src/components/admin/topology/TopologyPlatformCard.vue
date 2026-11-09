<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TopologyTotals } from '@/api/admin/topology'
import { GATE_IDS, gateIndexForReason, reasonLabel, summarizeGateBlocks } from './topologyModel'
import type { GateVisual } from './topologyAnimation'

const props = defineProps<{
  totals: TopologyTotals
  gateBlocks: Record<string, number>
  strategy: string
  gates: GateVisual[]
  blocked: { reason: string; gateIndex: number } | null
}>()

const { t } = useI18n()
const tr = (k: string, p?: Record<string, unknown>) => (p ? t(k, p) : t(k))

const gateRows = computed(() =>
  GATE_IDS.map((id, i) => ({
    id,
    title: t(`admin.topology.gates.${id}.title`),
    desc: t(`admin.topology.gates.${id}.desc`),
    visual: props.gates[i] ?? 'idle',
    blockedText:
      props.blocked && props.blocked.gateIndex === i
        ? t('admin.topology.platform.blockedChip', { reason: reasonLabel(props.blocked.reason, tr) })
        : ''
  }))
)

const blocks = computed(() => summarizeGateBlocks(props.gateBlocks))
const passed = computed(() => Math.max(0, props.totals.requests_5m - blocks.value.total))
const topBlock = computed(() => {
  const top = blocks.value.top
  if (!top) return null
  const gate = GATE_IDS[gateIndexForReason(top.reason)]
  return {
    gate: t(`admin.topology.gates.${gate}.title`),
    reason: reasonLabel(top.reason, tr),
    count: top.count
  }
})

const rowClass = (v: GateVisual) =>
  v === 'lit'
    ? 'bg-primary-50 dark:bg-primary-900/20'
    : v === 'blocked'
      ? 'bg-red-50 dark:bg-red-900/20'
      : v === 'retry'
        ? 'bg-amber-100 dark:bg-amber-900/30'
        : ''
const dotClass = (v: GateVisual) =>
  v === 'lit'
    ? 'border-primary-600 bg-primary-500'
    : v === 'blocked'
      ? 'border-red-600 bg-red-500'
      : v === 'retry'
        ? 'border-amber-600 bg-amber-500'
        : 'border-gray-300 bg-white dark:border-dark-500 dark:bg-dark-800'
</script>

<template>
  <div
    class="flex flex-col gap-3.5 rounded-2xl border border-primary-200 bg-white px-5 pb-4 pt-5 shadow-[0_10px_30px_rgba(13,148,136,0.10)] dark:border-primary-800/60 dark:bg-dark-800"
    data-topo-platform
    data-testid="topology-platform"
  >
    <div class="flex items-center gap-3">
      <span class="flex h-10 w-10 flex-none items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600">
        <svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true" fill="none" stroke="#ffffff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 6 H8 L14 16 H19 M3 16 H8 L10 12.6 M12 9.4 L14 6 H19 M16.5 3.5 L19 6 L16.5 8.5 M16.5 13.5 L19 16 L16.5 18.5" />
        </svg>
      </span>
      <span class="flex min-w-0 flex-1 flex-col gap-0.5">
        <b class="text-base text-gray-900 dark:text-white">{{ t('admin.topology.platform.title') }}</b>
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.topology.platform.subtitle') }}</span>
      </span>
      <span v-if="strategy" class="flex-none rounded-full bg-primary-100 px-3 py-1 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">{{ strategy }}</span>
    </div>
    <div class="h-px bg-slate-100 dark:bg-dark-700" />

    <ol class="relative flex flex-col gap-0.5" data-testid="topology-gates">
      <span class="absolute bottom-5 left-[15px] top-5 w-0.5 bg-slate-100 dark:bg-dark-700" aria-hidden="true" />
      <li
        v-for="row in gateRows"
        :key="row.id"
        class="relative flex items-center gap-3.5 rounded-[10px] px-2.5 py-[7px] transition-colors duration-150"
        :class="rowClass(row.visual)"
        :data-gate="row.id"
        :data-gate-state="row.visual"
      >
        <span class="relative z-[1] box-border h-3 w-3 flex-none rounded-full border-2 transition-colors duration-150" :class="dotClass(row.visual)" />
        <span class="flex min-w-0 flex-1 flex-col gap-px">
          <span class="truncate text-[13px] font-semibold text-gray-900 dark:text-white">{{ row.title }}</span>
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ row.desc }}</span>
        </span>
        <span
          v-if="row.blockedText"
          class="flex-none whitespace-nowrap rounded-full bg-red-500 px-2.5 py-0.5 text-[11px] font-semibold text-white"
          data-testid="topology-blocked-chip"
        >{{ row.blockedText }}</span>
      </li>
    </ol>

    <div class="mt-auto grid grid-cols-3 gap-2">
      <div class="flex min-w-0 flex-col gap-0.5 rounded-xl bg-gray-50 px-3 py-2.5 dark:bg-dark-900/50">
        <span class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('admin.topology.platform.window5m') }}</span>
        <span class="text-[13px] text-gray-900 dark:text-white" data-testid="topology-passed">
          {{ t('admin.topology.platform.passed') }} <b>{{ passed }}</b> · {{ t('admin.topology.platform.blocked') }} <b>{{ blocks.total }}</b>
        </span>
      </div>
      <div class="flex min-w-0 flex-col gap-0.5 rounded-xl bg-amber-50 px-3 py-2.5 dark:bg-amber-900/20">
        <span class="text-[11px] text-amber-800 dark:text-amber-300">{{ t('admin.topology.platform.retries') }}</span>
        <span class="text-[13px] text-gray-900 dark:text-white">
          {{ t('admin.topology.platform.retrySame') }} <b>{{ totals.retries_same_5m }}</b> · {{ t('admin.topology.platform.retrySwitch') }} <b>{{ totals.retries_switch_5m }}</b>
        </span>
      </div>
      <div class="flex min-w-0 flex-col gap-0.5 rounded-xl bg-red-50 px-3 py-2.5 dark:bg-red-900/20">
        <span class="text-[11px] text-red-800 dark:text-red-300">{{ t('admin.topology.platform.failedAfterRetry') }}</span>
        <span class="text-[13px] text-gray-900 dark:text-white"><b>{{ totals.failed_after_retry_5m }}</b> {{ t('admin.topology.platform.timesSuffix') }}</span>
      </div>
    </div>
    <div class="text-xs text-gray-500 dark:text-dark-400" data-testid="topology-top-block">
      <template v-if="topBlock">
        {{ t('admin.topology.platform.topBlock') }}<b class="text-gray-800 dark:text-dark-200">{{ topBlock.gate }}</b> · {{ topBlock.reason }} · <b class="text-red-600 dark:text-red-400">{{ topBlock.count }}</b>
      </template>
      <template v-else>{{ t('admin.topology.platform.topBlockNone') }}</template>
    </div>
  </div>
</template>
