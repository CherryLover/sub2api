<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TopologyKey } from '@/api/admin/topology'
import { keyLoadKind } from './topologyModel'
import { tagText, type Fx } from './topologyAnimation'

const props = defineProps<{
  apiKey: TopologyKey
  fx: Fx | null
  tipOpen: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle-tip', id: number): void
  (e: 'close-tip'): void
}>()

const { t } = useI18n()
const tr = (k: string, p?: Record<string, unknown>) => (p ? t(k, p) : t(k))

const load = computed(() => keyLoadKind(props.apiKey))
const groupLabel = computed(() => props.apiKey.group_name || t('admin.topology.key.noGroup'))

const tipTitle = computed(() => {
  const k = props.apiKey
  if (load.value === 'active') return t('admin.topology.key.tipTitleActive', { name: k.name, n: k.in_flight })
  if (load.value === 'queued') return t('admin.topology.key.tipTitleQueued', { name: k.name, n: k.queued })
  return t('admin.topology.key.tipTitleIdle', { name: k.name })
})
const tipBody = computed(() => {
  const k = props.apiKey
  if (load.value === 'active') return t('admin.topology.key.tipBodyActive', { n: k.in_flight })
  if (load.value === 'queued') return t('admin.topology.key.tipBodyQueued', { n: k.queued })
  return t('admin.topology.key.tipBodyIdle')
})

const ring = computed(() => {
  const kind = props.fx?.kind
  if (kind === 'error') return 'topo-ring-bad'
  if (kind === 'active') return 'topo-ring-ok'
  return ''
})
const tag = computed(() => (props.fx?.tag ? tagText(props.fx.tag, tr) : ''))
</script>

<template>
  <div
    class="relative flex h-14 items-center justify-between gap-2 rounded-xl border border-gray-200 bg-white pl-3.5 pr-2 text-[13px] dark:border-dark-600 dark:bg-dark-800"
    :data-topo-key="apiKey.id"
    data-testid="topology-key"
  >
    <span class="flex min-w-0 flex-col gap-px">
      <b class="truncate font-semibold text-gray-900 dark:text-white" :title="apiKey.name">{{ apiKey.name }}</b>
      <span class="truncate text-[11px] text-gray-400 dark:text-dark-400">{{ groupLabel }}</span>
    </span>
    <span class="flex flex-none items-center gap-1 whitespace-nowrap text-xs">
      <span v-if="apiKey.in_flight > 0" class="text-primary-700 dark:text-primary-300">
        {{ t('admin.topology.key.inFlight') }} <b>{{ apiKey.in_flight }}</b>
      </span>
      <span v-if="apiKey.queued > 0" class="text-amber-700 dark:text-amber-400">
        {{ t('admin.topology.key.queued') }} <b>{{ apiKey.queued }}</b>
      </span>
      <span v-if="load === 'idle'" class="text-gray-400 dark:text-dark-400">{{ t('admin.topology.key.idle') }}</span>
      <button
        type="button"
        class="flex h-[22px] w-[22px] items-center justify-center rounded-full text-gray-400 hover:bg-primary-50 hover:text-primary-700 dark:hover:bg-primary-900/30 dark:hover:text-primary-300"
        :aria-label="t('admin.topology.key.infoLabel')"
        :aria-expanded="tipOpen"
        data-topo-tip-trigger
        @click.stop="emit('toggle-tip', apiKey.id)"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round">
          <circle cx="7" cy="7" r="6" />
          <path d="M7 6.2 V10 M7 4 V4.1" />
        </svg>
      </button>
    </span>

    <div v-if="ring" class="pointer-events-none absolute inset-0 rounded-xl" :class="ring" />
    <span
      v-if="tag"
      class="pointer-events-none absolute -top-3 left-12 z-10 whitespace-nowrap rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
      :class="fx?.kind === 'error' ? 'bg-red-500 text-white' : 'bg-primary-600 text-white'"
      data-testid="topology-key-tag"
    >{{ tag }}</span>

    <div
      v-if="tipOpen"
      role="dialog"
      :aria-label="tipTitle"
      class="absolute left-[calc(100%+8px)] top-0 z-40 flex w-[300px] flex-col gap-2 rounded-xl border border-gray-200 bg-white px-4 py-3.5 text-xs leading-[18px] text-gray-700 shadow-xl dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
      data-topo-tip
      @click.stop
    >
      <div class="flex items-center justify-between gap-2">
        <b class="text-[13px] text-gray-900 dark:text-white">{{ tipTitle }}</b>
        <button
          type="button"
          class="flex h-6 w-6 items-center justify-center rounded-md bg-gray-100 dark:bg-dark-700"
          :aria-label="t('admin.topology.key.closeTip')"
          @click="emit('close-tip')"
        >
          <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M1 1 L9 9 M9 1 L1 9" /></svg>
        </button>
      </div>
      <span>{{ tipBody }}</span>
      <span class="text-gray-500 dark:text-dark-400">
        <span class="block">{{ t('admin.topology.key.tipInFlight') }}</span>
        <span class="block">{{ t('admin.topology.key.tipQueued') }}</span>
        <span class="block">{{ t('admin.topology.key.tipIdle') }}</span>
      </span>
    </div>
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
.dark .topo-ring-ok {
  box-shadow: 0 0 0 4px rgba(20, 184, 166, 0.2);
}
.dark .topo-ring-bad {
  box-shadow: 0 0 0 4px rgba(239, 68, 68, 0.2);
}
</style>
