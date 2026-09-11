<template>
  <div>
    <!-- Loading state (first load only; refreshes keep showing the last data) -->
    <div v-if="props.loading && !props.errors" class="space-y-1">
      <div class="h-3 w-14 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <!-- 数据取不到（运维监控关闭 / 查询失败）：后端返回 null，与"确实零错误"不是一回事。
         两者都画成 `-` 会让监控关掉时整列看起来像"全站零错误"。 -->
    <HelpTooltip
      v-else-if="!props.errors"
      width-class="w-max max-w-[260px]"
      class="!ml-0"
    >
      <template #trigger>
        <span
          class="cursor-help text-sm text-gray-400 dark:text-dark-500"
          data-test="recent-errors-unavailable"
        >?</span>
      </template>
      <div class="text-left">{{ t('admin.accounts.recentErrors.unavailable') }}</div>
    </HelpTooltip>

    <!-- 真的没有错误 -->
    <span
      v-else-if="props.errors.total <= 0"
      class="text-sm text-gray-400 dark:text-dark-500"
      data-test="recent-errors-empty"
    >-</span>

    <HelpTooltip
      v-else-if="lastError"
      width-class="w-max max-w-[320px]"
      class="!ml-0"
    >
      <template #trigger>
        <div class="flex cursor-help flex-col gap-1" data-test="recent-errors">
          <div class="flex flex-wrap items-center gap-1">
            <span
              v-for="chip in chips"
              :key="chip.key"
              :class="chipBaseClass + ' ' + chip.className"
              data-test="recent-errors-chip"
            >{{ chip.label }}</span>
            <span v-if="hiddenStatusCount > 0" class="text-[11px] text-gray-400 dark:text-dark-400">+{{ hiddenStatusCount }}</span>
          </div>
          <span class="whitespace-nowrap text-[11px] leading-4 text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.recentErrors.lastAt', { time: formatRelativeTime(lastError.at) }) }}
          </span>
        </div>
      </template>
      <div class="space-y-1 text-left" data-test="recent-errors-tooltip">
        <div class="flex flex-wrap items-center gap-x-1.5 text-gray-300">
          <span>{{ formatDateTime(lastError.at) }}</span>
          <span class="text-gray-500">·</span>
          <span class="font-medium text-white">{{ lastStatusText }}</span>
          <template v-if="lastError.model">
            <span class="text-gray-500">·</span>
            <span class="font-mono">{{ lastError.model }}</span>
          </template>
        </div>
        <div v-if="lastError.message" class="whitespace-pre-wrap break-words text-gray-100">{{ lastError.message }}</div>
      </div>
    </HelpTooltip>

    <!-- Errors counted but no detail for the latest one -->
    <div v-else class="flex flex-wrap items-center gap-1" data-test="recent-errors">
      <span
        v-for="chip in chips"
        :key="chip.key"
        :class="chipBaseClass + ' ' + chip.className"
        data-test="recent-errors-chip"
      >{{ chip.label }}</span>
      <span v-if="hiddenStatusCount > 0" class="text-[11px] text-gray-400 dark:text-dark-400">+{{ hiddenStatusCount }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { AccountRecentErrors } from '@/types'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const MAX_CHIPS = 3

const props = withDefaults(
  defineProps<{
    errors?: AccountRecentErrors | null
    loading?: boolean
  }>(),
  {
    errors: null,
    loading: false
  }
)

const { t } = useI18n()

const chipBaseClass = 'inline-flex items-center whitespace-nowrap rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-4'

const chipClassFor = (statusCode: number): string => {
  if (statusCode === 429) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  if (statusCode >= 500) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-300'
}

const sortedStatuses = computed(() => {
  const list = props.errors?.by_status ?? []
  return list
    .filter(item => item && item.count > 0)
    .slice()
    .sort((a, b) => b.count - a.count || a.status_code - b.status_code)
})

const chips = computed(() => {
  const top = sortedStatuses.value.slice(0, MAX_CHIPS)
  if (top.length === 0 && props.errors && props.errors.total > 0) {
    return [{ key: 'total', label: `×${props.errors.total}`, className: chipClassFor(0) }]
  }
  return top.map(item => ({
    key: String(item.status_code),
    label: `${item.status_code} ×${item.count}`,
    className: chipClassFor(item.status_code)
  }))
})

const hiddenStatusCount = computed(() => Math.max(0, sortedStatuses.value.length - MAX_CHIPS))

const lastError = computed(() => props.errors?.last ?? null)

const lastStatusText = computed(() => {
  const last = lastError.value
  if (!last) return ''
  if (typeof last.upstream_status_code === 'number' && last.upstream_status_code > 0) {
    return t('admin.accounts.recentErrors.upstreamStatus', { code: last.upstream_status_code })
  }
  return String(last.status_code)
})
</script>
