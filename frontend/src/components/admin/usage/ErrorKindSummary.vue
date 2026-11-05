<template>
  <section
    v-if="hasContent"
    data-testid="error-kind-summary"
    class="space-y-4 border-b border-gray-100 p-4 dark:border-dark-700/50"
  >
    <div v-if="props.loading" class="space-y-4" data-testid="error-kind-summary-loading">
      <div class="h-6 w-48 animate-pulse rounded bg-gray-100 dark:bg-dark-700" />
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="index in 8" :key="index" class="h-28 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" />
      </div>
    </div>

    <div v-else-if="props.error" class="flex flex-wrap items-center justify-between gap-3" data-testid="error-kind-summary-error">
      <p class="text-sm text-red-600 dark:text-red-400">{{ t('admin.usage.errorSummary.loadFailed') }}</p>
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('retry')">
        {{ t('admin.usage.errorSummary.retry') }}
      </button>
    </div>

    <template v-else>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">
          {{ t('admin.usage.errorSummary.title') }}
          <span class="ml-2 font-normal text-gray-500 dark:text-dark-300" data-testid="error-kind-total">{{ t('admin.usage.errorSummary.total', { n: formatNumber(total) }) }}</span>
        </h3>
        <button
          type="button"
          data-testid="error-kind-all"
          class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
          :class="props.selectedKind == null
            ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-950/40 dark:text-primary-200'
            : 'border-gray-200 text-gray-600 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'"
          :aria-pressed="props.selectedKind == null"
          @click="emit('select', null)"
        >
          {{ t('admin.usage.errorSummary.all') }}
        </button>
      </div>

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <button
          v-for="item in sortedItems"
          :key="item.kind"
          type="button"
          data-testid="error-kind-card"
          :data-kind="item.kind"
          class="min-w-0 rounded-lg border p-3 text-left transition-colors"
          :class="[
            cardTone(item),
            props.selectedKind === item.kind
              ? 'ring-2 ring-primary-500 ring-offset-1 dark:ring-primary-400 dark:ring-offset-dark-900'
              : '',
            item.kind === 'other'
              ? 'cursor-not-allowed opacity-70'
              : 'hover:border-gray-300 dark:hover:border-dark-500'
          ]"
          :disabled="item.kind === 'other'"
          :title="item.kind === 'other' ? t('admin.usage.errorSummary.otherUnavailable') : item.sample_message || displayType(item)"
          :aria-pressed="props.selectedKind === item.kind"
          @click="selectItem(item)"
        >
          <div class="flex items-start justify-between gap-3">
            <span class="min-w-0 break-words text-sm font-semibold text-gray-900 dark:text-white">{{ displayType(item) }}</span>
            <span class="shrink-0 text-lg font-bold tabular-nums text-gray-900 dark:text-white">{{ formatNumber(item.count) }}</span>
          </div>
          <div class="mt-3 h-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full" :class="progressTone(item)" :style="{ width: shareWidth(item) }" />
          </div>
          <p class="mt-1 text-[11px] tabular-nums text-gray-400 dark:text-dark-400">
            {{ t('admin.usage.errorSummary.share', { percent: sharePercent(item) }) }}
          </p>
        </button>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsErrorKindSummary, OpsErrorLogSummaryResponse } from '@/api/admin/ops'

const props = defineProps<{
  summary: OpsErrorLogSummaryResponse | null
  loading: boolean
  error: boolean
  selectedKind: string | null
}>()

const emit = defineEmits<{
  select: [kind: string | null]
  retry: []
}>()

const { t } = useI18n()

const sortedItems = computed(() =>
  [...(props.summary?.items ?? [])].sort((a, b) => b.count - a.count)
)
const total = computed(() => props.summary?.total ?? 0)
const hasContent = computed(() =>
  props.loading || props.error || total.value > 0
)

const reasonKeys: Record<string, string> = {
  model_not_supported: 'admin.usage.errorSummary.reasons.modelNotSupported',
  model_rate_limited: 'admin.usage.errorSummary.reasons.modelRateLimited',
  excluded: 'admin.usage.errorSummary.reasons.excluded',
  not_schedulable: 'admin.usage.errorSummary.reasons.notSchedulable',
  empty_pool: 'admin.usage.errorSummary.reasons.emptyPool',
}

const formatNumber = (value: number) => value.toLocaleString()

function displayType(item: OpsErrorKindSummary): string {
  if (item.kind === 'other') return t('admin.usage.errorSummary.other')
  if (item.reason) {
    const key = reasonKeys[item.reason]
    return key ? t(key) : item.reason
  }
  const message = item.sample_message.replace(/\s+/g, ' ').trim()
  const excerpt = message.length > 40 ? `${message.slice(0, 40)}…` : message
  return [item.status_code || '', excerpt].filter(Boolean).join(' ')
}

function sharePercent(item: OpsErrorKindSummary): string {
  if (!total.value) return '0'
  return ((item.count / total.value) * 100).toFixed(1)
}

function shareWidth(item: OpsErrorKindSummary): string {
  if (!total.value) return '0%'
  return `${Math.min(100, (item.count / total.value) * 100)}%`
}

function cardTone(item: OpsErrorKindSummary): string {
  if (item.status_code >= 500) {
    return 'border-red-200 bg-red-50/60 dark:border-red-900/60 dark:bg-red-950/20'
  }
  if (item.status_code >= 400) {
    return 'border-amber-200 bg-amber-50/60 dark:border-amber-900/60 dark:bg-amber-950/20'
  }
  return 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800'
}

function progressTone(item: OpsErrorKindSummary): string {
  if (item.status_code >= 500) return 'bg-red-500 dark:bg-red-400'
  if (item.status_code >= 400) return 'bg-amber-500 dark:bg-amber-400'
  return 'bg-primary-500 dark:bg-primary-400'
}

function selectItem(item: OpsErrorKindSummary) {
  if (item.kind === 'other') return
  emit('select', props.selectedKind === item.kind ? null : item.kind)
}
</script>
