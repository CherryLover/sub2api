<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TopologyAccount } from '@/api/admin/topology'
import { accountQuotaView, accountStatusView, type QuotaTone } from './topologyModel'
import { tagText, type Fx } from './topologyAnimation'

const props = defineProps<{
  account: TopologyAccount
  /** 用于额度 / 恢复时间的相对时间（快照刷新时更新） */
  clock: number
  fx: Fx | null
  cooldownLeft: number
}>()

const { t } = useI18n()
const tr = (k: string, p?: Record<string, unknown>) => (p ? t(k, p) : t(k))

const status = computed(() => accountStatusView(props.account, props.clock, tr))
const quota = computed(() => accountQuotaView(props.account, props.clock, tr))

const statusClass = computed(() => {
  switch (status.value.tone) {
    case 'warn':
      return 'text-amber-700 dark:text-amber-400'
    case 'error':
      return 'text-red-700 dark:text-red-400'
    default:
      return 'text-gray-500 dark:text-dark-400'
  }
})

const dotClass = computed(() => {
  switch (status.value.tone) {
    case 'ok':
      return 'border-primary-500'
    case 'warn':
      return 'border-amber-400'
    case 'error':
      return 'border-red-400'
    default:
      return 'border-gray-300 dark:border-dark-500'
  }
})

const barColor = (tone: QuotaTone) =>
  tone === 'full' ? 'bg-orange-500' : tone === 'warn' ? 'bg-amber-500' : 'bg-primary-500'
const barText = (tone: QuotaTone) =>
  tone === 'full' ? 'text-orange-700 dark:text-orange-400' : tone === 'warn' ? 'text-amber-700 dark:text-amber-400' : 'text-gray-600 dark:text-dark-300'

const ring = computed(() => {
  switch (props.fx?.kind) {
    case 'active':
    case 'ok':
      return 'topo-ring-ok'
    case 'failed':
      return 'topo-ring-bad'
    case 'retry':
      return 'topo-ring-wait'
    default:
      return ''
  }
})

const tag = computed(() => {
  if (props.fx?.tag) {
    return { text: tagText(props.fx.tag, tr), kind: props.fx.kind }
  }
  if (props.cooldownLeft > 0) {
    return { text: t('admin.topology.tag.cooldown', { n: props.cooldownLeft }), kind: 'retry' as const }
  }
  return null
})

const tagClass = computed(() => {
  switch (tag.value?.kind) {
    case 'ok':
      return 'bg-primary-600 text-white'
    case 'failed':
      return 'bg-red-500 text-white'
    default:
      return 'border border-amber-300 bg-amber-100 text-amber-800 dark:border-amber-700 dark:bg-amber-900/60 dark:text-amber-200'
  }
})
</script>

<template>
  <div
    class="relative flex min-h-[100px] flex-col justify-center gap-[7px] rounded-xl border px-3.5 py-3"
    :class="status.available
      ? 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800'
      : 'border-dashed border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-900/40'"
    :data-topo-account="account.id"
    data-testid="topology-account"
    :title="status.title || undefined"
  >
    <div class="flex min-w-0 items-center gap-2.5 text-[13px]" :class="status.available ? '' : 'text-gray-500 dark:text-dark-400'">
      <span class="h-2.5 w-2.5 flex-none rounded-full border-[1.5px]" :class="dotClass" />
      <b class="truncate font-semibold" :class="status.available ? 'text-gray-900 dark:text-white' : 'text-gray-600 dark:text-dark-300'">{{ account.name }}</b>
      <span v-if="account.platform" class="flex-none text-gray-400 dark:text-dark-400">{{ account.platform }}</span>
      <span
        v-if="status.available"
        class="ml-auto flex-none text-xs text-gray-600 dark:text-dark-300"
        :title="t('admin.topology.account.concurrency')"
        data-testid="topology-account-concurrency"
      >{{ account.concurrency_current }}/{{ account.concurrency_max }}</span>
      <span v-else class="ml-auto flex-none whitespace-nowrap text-xs" :class="statusClass" data-testid="topology-account-status">
        {{ status.label }}<template v-if="status.recoverText"> · {{ status.recoverText }}</template>
      </span>
    </div>

    <div v-if="quota.mode === 'payg'" class="grid grid-cols-[40px_minmax(0,1fr)] items-center gap-2">
      <span class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('admin.topology.account.quotaLabel') }}</span>
      <span class="text-[11px] text-gray-500 dark:text-dark-400" data-testid="topology-payg">{{ t('admin.topology.account.payg') }}</span>
    </div>
    <template v-else>
      <div
        v-for="bar in quota.bars"
        :key="bar.window"
        class="grid grid-cols-[40px_minmax(0,1fr)_auto] items-center gap-2"
        :data-testid="`topology-quota-${bar.window}`"
      >
        <span class="text-[11px] text-gray-500 dark:text-dark-400">{{ bar.window === '5h' ? t('admin.topology.quota.w5h') : t('admin.topology.quota.w7d') }}</span>
        <div class="h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-dark-700">
          <span class="block h-1.5 rounded-full" :class="barColor(bar.tone)" :style="{ width: bar.percent + '%' }" />
        </div>
        <span class="whitespace-nowrap text-right text-[11px]" :class="barText(bar.tone)">{{ bar.text }}</span>
      </div>
      <div v-if="quota.bars.length === 0" class="grid grid-cols-[40px_minmax(0,1fr)_auto] items-center gap-2">
        <span class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('admin.topology.account.quotaLabel') }}</span>
        <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ t('admin.topology.account.quotaNone') }}</span>
        <span />
      </div>
    </template>
    <div v-if="status.reauth" class="text-right">
      <RouterLink to="/admin/accounts" class="text-[11px] font-medium text-primary-700 hover:text-primary-800 dark:text-primary-300" data-testid="topology-reauth">
        {{ t('admin.topology.account.reauth') }}
      </RouterLink>
    </div>

    <div v-if="cooldownLeft > 0" class="topo-cool pointer-events-none absolute inset-0 rounded-xl" data-testid="topology-cooldown" />
    <div v-if="ring" class="pointer-events-none absolute inset-0 rounded-xl" :class="ring" />
    <span
      v-if="tag"
      class="pointer-events-none absolute -top-3 right-3 z-10 whitespace-nowrap rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
      :class="tagClass"
      data-testid="topology-account-tag"
    >{{ tag.text }}</span>
  </div>
</template>

<style scoped>
.topo-ring-ok {
  border: 2px solid #14b8a6;
  box-shadow: 0 0 0 4px rgba(204, 251, 241, 0.9);
}
.topo-ring-bad {
  border: 2px solid #ef4444;
  box-shadow: 0 0 0 4px rgba(254, 226, 226, 0.9);
}
.topo-ring-wait {
  border: 2px dashed #f59e0b;
  box-shadow: 0 0 0 4px rgba(254, 243, 199, 0.9);
}
.topo-cool {
  border: 2px dashed #f59e0b;
  background: rgba(255, 251, 235, 0.72);
}
.dark .topo-ring-ok {
  box-shadow: 0 0 0 4px rgba(20, 184, 166, 0.2);
}
.dark .topo-ring-bad {
  box-shadow: 0 0 0 4px rgba(239, 68, 68, 0.2);
}
.dark .topo-ring-wait {
  box-shadow: 0 0 0 4px rgba(245, 158, 11, 0.2);
}
.dark .topo-cool {
  background: rgba(120, 53, 15, 0.35);
}
</style>
