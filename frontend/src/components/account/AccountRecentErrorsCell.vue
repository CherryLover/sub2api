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
            <component
              :is="chip.chainId ? 'button' : 'span'"
              v-for="chip in chips"
              :key="chip.key"
              :type="chip.chainId ? 'button' : undefined"
              :class="[chipBaseClass, chip.className, chip.chainId ? chipClickableClass : '']"
              :title="chip.chainId ? t('admin.accounts.recentErrors.viewChain') : undefined"
              data-test="recent-errors-chip"
              @click.stop="chip.chainId && openChain(chip.chainId)"
            >{{ chip.label }}</component>
            <span v-if="hiddenChipCount > 0" class="text-[11px] text-gray-400 dark:text-dark-400">+{{ hiddenChipCount }}</span>
          </div>
          <span class="whitespace-nowrap text-[11px] leading-4 text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.recentErrors.lastAt', { time: formatRelativeTime(lastError.at) }) }}
          </span>
        </div>
      </template>
      <div class="space-y-1 text-left" data-test="recent-errors-tooltip">
        <div v-if="hasOutcomeSummary" class="flex flex-wrap items-center gap-x-1.5 text-gray-300" data-test="recent-errors-summary">
          <span>{{ t('admin.accounts.recentErrors.summaryTotal', { count: props.errors.total }) }}</span>
          <span class="text-gray-500">·</span>
          <span class="text-amber-300">{{ t('admin.accounts.recentErrors.summaryRecovered', { count: outcomeSummary.recovered }) }}</span>
          <span class="text-gray-500">·</span>
          <span class="text-red-300">{{ t('admin.accounts.recentErrors.summaryFailed', { count: outcomeSummary.failed }) }}</span>
        </div>
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
        <!-- 「已恢复」这个词不解释清楚，站长会以为是「我们把它修好了」；它其实是网关自己救回来的 -->
        <div v-if="hasOutcomeSummary" class="border-t border-white/10 pt-1 text-gray-400" data-test="recent-errors-recovered-hint">
          {{ t('admin.accounts.recentErrors.recoveredExplainer') }}
        </div>
        <div v-if="hasChainEntry" class="text-gray-400" data-test="recent-errors-chain-hint">
          {{ t('admin.accounts.recentErrors.chainHint') }}
        </div>
      </div>
    </HelpTooltip>

    <!-- Errors counted but no detail for the latest one -->
    <div v-else class="flex flex-wrap items-center gap-1" data-test="recent-errors">
      <component
        :is="chip.chainId ? 'button' : 'span'"
        v-for="chip in chips"
        :key="chip.key"
        :type="chip.chainId ? 'button' : undefined"
        :class="[chipBaseClass, chip.className, chip.chainId ? chipClickableClass : '']"
        :title="chip.chainId ? t('admin.accounts.recentErrors.viewChain') : undefined"
        data-test="recent-errors-chip"
        @click.stop="chip.chainId && openChain(chip.chainId)"
      >{{ chip.label }}</component>
      <span v-if="hiddenChipCount > 0" class="text-[11px] text-gray-400 dark:text-dark-400">+{{ hiddenChipCount }}</span>
    </div>

    <!-- 首次点开才挂载，之后保持挂载以便走滑入动画 -->
    <RequestChainPanel
      v-if="chainMounted"
      :show="chainOpen"
      :client-request-id="chainRequestId"
      @close="closeChain"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import RequestChainPanel from '@/components/admin/account/RequestChainPanel.vue'
import type { AccountRecentErrorStatusCount, AccountRecentErrors } from '@/types'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const MAX_CHIPS = 3

type ChipTone = 'failed' | 'recovered' | 'unknown'

interface ErrorChip {
  key: string
  label: string
  className: string
  tone: ChipTone
  count: number
  statusCode: number
  chainId?: string
}

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
const chipClickableClass = 'cursor-pointer underline-offset-2 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40'

// 最终失败：真事故，红色
const FAILED_CLASS = 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
// 已恢复：网关自己救回来了，用柔和的琥珀色（比原来的 amber-100 更淡），不要看着像事故
const RECOVERED_CLASS = 'bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-500/30'

// 老后端不下发 recovered / failed 时的兜底配色：仍按状态码分档（旧口径）
const legacyClassFor = (statusCode: number): string => {
  if (statusCode === 429) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  if (statusCode >= 500) return FAILED_CLASS
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-300'
}

const TONE_RANK: Record<ChipTone, number> = { failed: 0, unknown: 1, recovered: 2 }

const lastError = computed(() => props.errors?.last ?? null)

// `last` 落在哪一档：上游状态码优先，和后端 by_status 的口径一致
const lastStatusCode = computed(() => {
  const last = lastError.value
  if (!last) return null
  if (typeof last.upstream_status_code === 'number' && last.upstream_status_code > 0) {
    return last.upstream_status_code
  }
  return last.status_code
})

const sortedStatuses = computed(() => {
  const list = props.errors?.by_status ?? []
  return list
    .filter(item => item && item.count > 0)
    .slice()
    .sort((a, b) => b.count - a.count || a.status_code - b.status_code)
})

/**
 * 后端把 recovered / failed 无条件序列化，拿不到拆分能力时两边都是 0（而不是字段缺失）。
 * 所以判据是「两边加起来大于 0」而不是「字段存在」——否则那种退化数据会被当成
 * 「0 恢复 0 失败」，整格 chip 直接消失，比显示旧口径还糟。
 */
const hasBreakdown = (item: AccountRecentErrorStatusCount): boolean =>
  (item.recovered ?? 0) + (item.failed ?? 0) > 0

const outcomeSummary = computed(() => {
  const errors = props.errors
  if (!errors) return { recovered: 0, failed: 0 }
  const top = { recovered: errors.recovered ?? 0, failed: errors.failed ?? 0 }
  if (top.recovered + top.failed > 0) return top
  return sortedStatuses.value.reduce(
    (acc, item) => ({ recovered: acc.recovered + (item.recovered ?? 0), failed: acc.failed + (item.failed ?? 0) }),
    { recovered: 0, failed: 0 }
  )
})

const hasOutcomeSummary = computed(() => outcomeSummary.value.recovered + outcomeSummary.value.failed > 0)

// 一条错误能点开链路的前提是后端给了 client_request_id；给不出来就退化成纯展示，不报错
const chainIdFor = (item: AccountRecentErrorStatusCount): string | undefined => {
  if (item.last_client_request_id) return item.last_client_request_id
  const fallback = lastError.value?.client_request_id
  if (fallback && lastStatusCode.value === item.status_code) return fallback
  return undefined
}

const allChips = computed<ErrorChip[]>(() => {
  const statuses = sortedStatuses.value

  if (statuses.length === 0 && props.errors && props.errors.total > 0) {
    // by_status 缺失但确实有错误：至少把总数和结论画出来
    const { recovered, failed } = outcomeSummary.value
    if (hasOutcomeSummary.value) {
      const chips: ErrorChip[] = []
      if (failed > 0) {
        chips.push({
          key: 'total-failed',
          label: t('admin.accounts.recentErrors.chipTotalFailed', { count: failed }),
          className: FAILED_CLASS,
          tone: 'failed',
          count: failed,
          statusCode: 0,
          chainId: lastError.value?.client_request_id
        })
      }
      if (recovered > 0) {
        chips.push({
          key: 'total-recovered',
          label: t('admin.accounts.recentErrors.chipTotalRecovered', { count: recovered }),
          className: RECOVERED_CLASS,
          tone: 'recovered',
          count: recovered,
          statusCode: 0,
          chainId: lastError.value?.client_request_id
        })
      }
      return chips
    }
    return [{
      key: 'total',
      label: `×${props.errors.total}`,
      className: legacyClassFor(0),
      tone: 'unknown',
      count: props.errors.total,
      statusCode: 0,
      chainId: lastError.value?.client_request_id
    }]
  }

  const chips: ErrorChip[] = []
  for (const item of statuses) {
    const chainId = chainIdFor(item)

    if (!hasBreakdown(item)) {
      chips.push({
        key: String(item.status_code),
        label: `${item.status_code} ×${item.count}`,
        className: legacyClassFor(item.status_code),
        tone: 'unknown',
        count: item.count,
        statusCode: item.status_code,
        chainId
      })
      continue
    }

    const failed = item.failed ?? 0
    const recovered = item.recovered ?? 0
    // 后端只给了一边时，另一边按总数补齐，免得数字对不上总数
    const resolvedRecovered = typeof item.recovered === 'number' ? recovered : Math.max(0, item.count - failed)
    const resolvedFailed = typeof item.failed === 'number' ? failed : Math.max(0, item.count - recovered)

    if (resolvedFailed > 0) {
      chips.push({
        key: `${item.status_code}-failed`,
        label: t('admin.accounts.recentErrors.chipFailed', { code: item.status_code, count: resolvedFailed }),
        className: FAILED_CLASS,
        tone: 'failed',
        count: resolvedFailed,
        statusCode: item.status_code,
        chainId
      })
    }
    if (resolvedRecovered > 0) {
      chips.push({
        key: `${item.status_code}-recovered`,
        label: t('admin.accounts.recentErrors.chipRecovered', { code: item.status_code, count: resolvedRecovered }),
        className: RECOVERED_CLASS,
        tone: 'recovered',
        count: resolvedRecovered,
        statusCode: item.status_code,
        chainId
      })
    }
  }

  // 最终失败排最前：那才是需要站长看一眼的东西
  return chips.sort(
    (a, b) => TONE_RANK[a.tone] - TONE_RANK[b.tone] || b.count - a.count || a.statusCode - b.statusCode
  )
})

const chips = computed(() => allChips.value.slice(0, MAX_CHIPS))

const hiddenChipCount = computed(() => Math.max(0, allChips.value.length - MAX_CHIPS))

const hasChainEntry = computed(() => chips.value.some(chip => !!chip.chainId))

const lastStatusText = computed(() => {
  const last = lastError.value
  if (!last) return ''
  if (typeof last.upstream_status_code === 'number' && last.upstream_status_code > 0) {
    return t('admin.accounts.recentErrors.upstreamStatus', { code: last.upstream_status_code })
  }
  return String(last.status_code)
})

const chainMounted = ref(false)
const chainOpen = ref(false)
const chainRequestId = ref<string | null>(null)

const openChain = (clientRequestId: string) => {
  chainRequestId.value = clientRequestId
  if (!chainMounted.value) {
    chainMounted.value = true
    // 先挂载再置 show，抽屉才有滑入动画
    void nextTick(() => {
      chainOpen.value = true
    })
    return
  }
  chainOpen.value = true
}

const closeChain = () => {
  chainOpen.value = false
}
</script>
