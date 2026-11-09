<template>
  <div
    data-test="routing-diagnosis"
    class="sticky left-0 m-3 max-w-6xl overflow-hidden rounded-xl border border-primary-100 bg-white dark:border-primary-900/40 dark:bg-dark-900 sm:mx-4"
  >
    <!-- 旧数据 / 非选号阶段错误：没有诊断，只给原始信息 -->
    <template v-if="!diagnosis">
      <div class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300" data-test="routing-diagnosis-empty">
        <span class="font-medium text-gray-900 dark:text-white">{{ t('admin.routing.noRecord') }}</span>
        <span class="ml-1">{{ t('admin.routing.noRecordHint') }}</span>
      </div>
      <details v-if="fallbackRaw" class="border-t border-gray-100 px-4 py-2 text-xs dark:border-dark-700">
        <summary class="cursor-pointer text-gray-500 dark:text-gray-400">{{ t('admin.routing.diagnosis.raw') }}</summary>
        <pre class="mt-2 whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-3 font-mono text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ fallbackRaw }}</pre>
      </details>
    </template>

    <template v-else>
      <div class="border-b border-primary-100 bg-primary-50 px-4 py-2.5 text-xs text-primary-800 dark:border-primary-900/40 dark:bg-primary-900/20 dark:text-primary-200">
        <b>{{ t('admin.routing.diagnosis.title') }}</b>
        <span class="mx-1">·</span>
        <span data-test="routing-diagnosis-header">{{ headerText }}</span>
      </div>

      <div class="space-y-4 p-4">
        <!-- 漏斗：每一关剩下多少账号 -->
        <div v-if="candidates.length > 0" class="flex flex-wrap items-center gap-1.5 text-xs" data-test="routing-funnel">
          <span class="rounded-lg bg-gray-100 px-2.5 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            {{ t('admin.routing.diagnosis.funnelPool', { count: poolSize }) }}
          </span>
          <template v-for="step in funnel" :key="step.gate">
            <span class="text-gray-400">→</span>
            <span
              class="rounded-lg px-2.5 py-1"
              :class="step.count === 0
                ? 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'"
            >
              {{ t(`admin.routing.checks.${step.gate}`) }} <b>{{ step.count }}</b>
            </span>
          </template>
          <span class="text-gray-400">→</span>
          <span class="rounded-lg bg-red-50 px-2.5 py-1 font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300">
            {{ conclusionText }}
          </span>
        </div>

        <!-- 检查矩阵 -->
        <div v-if="candidates.length > 0" class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-xs" data-test="routing-matrix">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="whitespace-nowrap px-3 py-2 text-left font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.routing.diagnosis.account') }}</th>
                <th
                  v-for="gate in gates"
                  :key="gate"
                  class="whitespace-nowrap px-3 py-2 text-center font-semibold text-gray-600 dark:text-gray-300"
                >{{ t(`admin.routing.checks.${gate}`) }}</th>
                <th class="whitespace-nowrap px-3 py-2 text-left font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.routing.diagnosis.whyNot') }}</th>
                <th class="whitespace-nowrap px-3 py-2 text-left font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.routing.diagnosis.recover') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="c in candidates" :key="c.account_id" data-test="routing-matrix-row">
                <td class="whitespace-nowrap px-3 py-2 text-gray-900 dark:text-white">
                  <b>{{ c.account_name || '#' + c.account_id }}</b>
                  <span v-if="c.load_max > 0" class="ml-1.5 text-gray-400 dark:text-gray-500">{{ formatLoad(c) }}</span>
                </td>
                <td
                  v-for="gate in gates"
                  :key="gate"
                  class="px-3 py-2 text-center"
                  :data-gate="gate"
                  :data-state="gateState(c, gate)"
                >
                  <span v-if="gateState(c, gate) === 'pass'" class="font-bold text-emerald-600 dark:text-emerald-400">✓</span>
                  <span v-else-if="gateState(c, gate) === 'fail'" class="font-bold text-red-600 dark:text-red-400">✕</span>
                  <span v-else class="text-gray-300 dark:text-gray-600">—</span>
                </td>
                <td class="px-3 py-2 text-gray-700 dark:text-gray-300">
                  <span v-if="c.chosen" class="font-medium text-emerald-700 dark:text-emerald-400">{{ t('admin.routing.diagnosis.chosen') }}</span>
                  <span v-else>{{ candidateReasonLabel(t, c.reason, diagnosis.model) || '—' }}</span>
                </td>
                <td class="whitespace-nowrap px-3 py-2 text-gray-500 dark:text-gray-400">{{ recoverText(c) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 尝试经过 -->
        <div v-if="attempts.length > 0" class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <div class="border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
            {{ t('admin.routing.diagnosis.attemptsTitle') }}
          </div>
          <table class="min-w-full text-xs" data-test="routing-attempts">
            <thead>
              <tr class="text-gray-500 dark:text-gray-400">
                <th class="px-3 py-2 text-left font-semibold">{{ t('admin.routing.diagnosis.attemptOrder') }}</th>
                <th class="px-3 py-2 text-left font-semibold">{{ t('admin.routing.diagnosis.account') }}</th>
                <th class="px-3 py-2 text-left font-semibold">{{ t('admin.routing.diagnosis.attemptResult') }}</th>
                <th class="px-3 py-2 text-left font-semibold">{{ t('admin.routing.diagnosis.attemptDuration') }}</th>
                <th class="px-3 py-2 text-left font-semibold">{{ t('admin.routing.diagnosis.attemptNext') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="a in attempts" :key="a.attempt" data-test="routing-attempt-row" class="text-gray-700 dark:text-gray-300">
                <td class="whitespace-nowrap px-3 py-2">{{ t('admin.routing.diagnosis.nth', { n: a.attempt }) }}</td>
                <td class="whitespace-nowrap px-3 py-2">{{ a.account_name || '#' + a.account_id }}</td>
                <td class="whitespace-nowrap px-3 py-2">
                  <b :class="isSuccess(a.status_code) ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">{{ attemptResult(a.status_code) }}</b>
                </td>
                <td class="whitespace-nowrap px-3 py-2 tabular-nums">{{ formatAttemptDuration(a.duration_ms) }}</td>
                <td class="whitespace-nowrap px-3 py-2">{{ nextLabel(a.next) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 怎么修 -->
        <div
          v-if="fixHint"
          class="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:bg-amber-900/20 dark:text-amber-200"
          data-test="routing-fix"
        >
          <span><b>{{ t('admin.routing.diagnosis.fixTitle') }}</b>{{ fixHint.text }}</span>
          <RouterLink
            :to="fixHint.action === 'groups' ? '/admin/groups' : '/admin/accounts'"
            class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          >{{ fixHint.action === 'groups' ? t('admin.routing.diagnosis.goGroups') : t('admin.routing.diagnosis.goAccounts') }}</RouterLink>
        </div>

        <details v-if="rawText" class="text-xs" data-test="routing-raw">
          <summary class="cursor-pointer text-gray-500 dark:text-gray-400">{{ t('admin.routing.diagnosis.raw') }}</summary>
          <pre class="mt-2 whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-3 font-mono text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ rawText }}</pre>
        </details>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RoutingCandidate, RoutingDiagnosis } from '@/types'
import {
  ROUTING_GATES,
  candidateReasonLabel,
  diagnosisAttempts,
  diagnosisCandidates,
  diagnosisFixHint,
  diagnosisFunnel,
  formatAttemptDuration,
  formatLoad,
  formatRecoverTime,
  gateState,
  isConfigReason
} from '@/utils/routingLabels'

interface Props {
  diagnosis: RoutingDiagnosis | null | undefined
  statusCode?: number
  requestId?: string
  /** 诊断缺失时作为原始信息展示 */
  message?: string
}

const props = defineProps<Props>()
const { t } = useI18n()

const gates = ROUTING_GATES
const candidates = computed(() => diagnosisCandidates(props.diagnosis))
const attempts = computed(() => diagnosisAttempts(props.diagnosis))
const funnel = computed(() => diagnosisFunnel(props.diagnosis))
const poolSize = computed(() => props.diagnosis?.pool_size || candidates.value.length)
const status = computed(() => String(props.statusCode || 503))
const groupLabel = computed(() => props.diagnosis?.group_name || (props.diagnosis?.group_id ? `#${props.diagnosis.group_id}` : ''))
const methodLabel = computed(() => t('admin.routing.methodShort.load_balance'))

const headerText = computed(() => {
  if (candidates.value.length === 0) {
    return t('admin.routing.diagnosis.emptyPool', { group: groupLabel.value || '-', status: status.value })
  }
  if (groupLabel.value) {
    return t('admin.routing.diagnosis.header', {
      group: groupLabel.value,
      method: methodLabel.value,
      count: poolSize.value
    })
  }
  return t('admin.routing.diagnosis.headerNoGroup', { method: methodLabel.value, count: poolSize.value })
})

const conclusionText = computed(() =>
  attempts.value.length > 0
    ? t('admin.routing.diagnosis.conclusionTried', { status: status.value })
    : t('admin.routing.diagnosis.conclusionNone', { status: status.value })
)

const fixHint = computed(() => diagnosisFixHint(t, props.diagnosis, formatRecoverTime))

const rawText = computed(() => {
  const parts: string[] = []
  if (props.requestId) parts.push(t('admin.routing.diagnosis.requestId', { id: props.requestId }))
  const raw = props.diagnosis?.raw || props.message
  if (raw) parts.push(raw)
  return parts.join(' · ')
})

const fallbackRaw = computed(() => {
  const parts: string[] = []
  if (props.requestId) parts.push(t('admin.routing.diagnosis.requestId', { id: props.requestId }))
  if (props.message) parts.push(props.message)
  return parts.join(' · ')
})

function recoverText(c: RoutingCandidate): string {
  if (c.chosen || !c.reason) return '—'
  const at = formatRecoverTime(c.recover_at)
  if (at) return t('admin.routing.diagnosis.recoverAt', { time: at })
  if (isConfigReason(c.reason)) return t('admin.routing.diagnosis.needsConfig')
  return '—'
}

function isSuccess(code: number): boolean {
  return code >= 200 && code < 300
}

function attemptResult(code: number): string {
  if (!code) return t('admin.routing.diagnosis.networkError')
  if (isSuccess(code)) return t('admin.routing.diagnosis.success', { code })
  return t('admin.routing.diagnosis.upstreamStatus', { code })
}

function nextLabel(next: string | undefined): string {
  if (next === 'same_account' || next === 'switch' || next === 'give_up') {
    return t(`admin.routing.next.${next}`)
  }
  return '—'
}
</script>
