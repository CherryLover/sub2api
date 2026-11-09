<template>
  <div
    data-test="routing-info"
    class="sticky left-0 m-3 flex max-w-6xl flex-col gap-3 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900 sm:mx-4"
  >
    <div v-if="!info" class="text-sm text-gray-500 dark:text-gray-400" data-test="routing-info-empty">
      <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.routing.noRecord') }}</span>
      <span class="ml-1">{{ t('admin.routing.noRecordHint') }}</span>
    </div>

    <template v-else>
      <!-- 选号链路：API 密钥 → 分组 → 候选账号 → 上游结果 -->
      <div class="flex flex-wrap items-center gap-2" data-test="routing-chain">
        <div class="flex min-w-[132px] flex-col gap-0.5 rounded-lg border border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
          <span>{{ t('admin.routing.usage.nodeApiKey') }}</span>
          <b class="text-sm text-gray-900 dark:text-white">{{ row.api_key?.name || '#' + row.api_key_id }}</b>
        </div>
        <span class="text-gray-400" aria-hidden="true">→</span>
        <div class="flex min-w-[132px] flex-col gap-0.5 rounded-lg border border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
          <span>{{ t('admin.routing.usage.nodeGroup') }}</span>
          <b class="text-sm text-gray-900 dark:text-white">{{ row.group?.name || '-' }}</b>
          <span>{{ methodLabel }}</span>
        </div>
        <template v-if="candidates.length > 0">
          <span class="text-gray-400" aria-hidden="true">→</span>
          <div class="flex min-w-[220px] flex-col gap-1 rounded-lg border border-gray-200 px-3 py-2 text-xs dark:border-dark-700" data-test="routing-candidates-list">
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.routing.usage.nodeCandidates') }}</span>
            <div
              v-for="c in candidates"
              :key="c.account_id"
              data-test="routing-candidate"
              :data-chosen="c.chosen ? 'true' : 'false'"
              class="flex items-center justify-between gap-3 rounded-md px-2 py-1"
              :class="c.chosen
                ? 'bg-primary-50 text-primary-800 dark:bg-primary-900/30 dark:text-primary-200'
                : c.reason ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-300'"
            >
              <span :class="c.chosen ? 'font-semibold' : ''">{{ c.chosen ? '✓ ' : '' }}{{ c.account_name || '#' + c.account_id }}</span>
              <span class="whitespace-nowrap">{{ candidateNote(c) }}</span>
            </div>
          </div>
        </template>
        <span class="text-gray-400" aria-hidden="true">→</span>
        <div class="flex min-w-[132px] flex-col gap-0.5 rounded-lg border border-emerald-200 px-3 py-2 text-xs text-gray-500 dark:border-emerald-800 dark:text-gray-400">
          <span>{{ t('admin.routing.usage.nodeResult') }}</span>
          <b class="text-sm text-emerald-600 dark:text-emerald-400">{{ t('admin.routing.usage.resultSuccess') }}</b>
          <span>{{ t('admin.routing.usage.resultLatency', { first: formatMs(row.first_token_ms), total: formatMs(row.duration_ms) }) }}</span>
        </div>
      </div>

      <!-- 尝试经过：多于一次时才显示 -->
      <div v-if="attempts.length > 1" class="flex flex-wrap items-center gap-2 text-xs" data-test="routing-info-attempts">
        <span class="font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.routing.usage.attemptsTitle') }}</span>
        <template v-for="(a, i) in attempts" :key="a.attempt">
          <span v-if="i > 0" class="text-gray-400" aria-hidden="true">→</span>
          <span
            data-test="routing-info-attempt"
            class="rounded-md px-2 py-1"
            :class="isSuccess(a.status_code)
              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
              : 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'"
          >{{ attemptText(a) }}</span>
        </template>
        <span v-if="failedMs > 0" class="text-gray-400 dark:text-gray-500">{{ t('admin.routing.usage.slowBecause', { seconds: formatSeconds(failedMs) }) }}</span>
      </div>

      <p class="text-xs text-gray-600 dark:text-gray-300" data-test="routing-why">
        <b>{{ t('admin.routing.usage.why') }}</b>{{ whyText }}
      </p>
      <p v-if="info.method === 'sticky' && attempts.length <= 1" class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.routing.usage.stickyHint') }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageLog, RoutingAttempt, RoutingCandidate, RoutingInfo } from '@/types'
import {
  candidateReasonLabel,
  diagnosisAttempts,
  formatLoad,
  formatSeconds,
  routingWhy,
  sortCandidatesByLoad
} from '@/utils/routingLabels'

interface Props {
  info: RoutingInfo | null | undefined
  row: AdminUsageLog
}

const props = defineProps<Props>()
const { t } = useI18n()

const candidates = computed(() => sortCandidatesByLoad(props.info?.candidates))
const attempts = computed(() => diagnosisAttempts(props.info))
const failedMs = computed(() =>
  attempts.value.filter((a) => !isSuccess(a.status_code)).reduce((sum, a) => sum + (a.duration_ms || 0), 0)
)

const methodLabel = computed(() => {
  const m = props.info?.method
  if (m === 'load_balance' || m === 'sticky' || m === 'single') return t(`admin.routing.methodShort.${m}`)
  return m || '-'
})

const whyText = computed(() => routingWhy(t, props.info, props.row.account?.name || ''))

function isSuccess(code: number): boolean {
  return code >= 200 && code < 300
}

function formatMs(ms: number | null | undefined): string {
  if (ms == null) return '-'
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(2)}s`
}

function candidateNote(c: RoutingCandidate): string {
  if (c.reason) {
    return t('admin.routing.usage.skipped', { reason: candidateReasonLabel(t, c.reason, props.row.model) })
  }
  const load = t('admin.routing.usage.load', { load: formatLoad(c) })
  if (c.chosen) return `${load} · ${t('admin.routing.usage.chosen')}`
  return `${load} · ${t('admin.routing.usage.busier')}`
}

function attemptText(a: RoutingAttempt): string {
  const account = a.account_name || `#${a.account_id}`
  if (isSuccess(a.status_code)) return t('admin.routing.usage.attemptOk', { n: a.attempt, account })
  const result = a.status_code
    ? t('admin.routing.diagnosis.upstreamStatus', { code: a.status_code })
    : t('admin.routing.diagnosis.networkError')
  return t('admin.routing.usage.attemptFailed', { n: a.attempt, account, result, seconds: formatSeconds(a.duration_ms) })
}
</script>
