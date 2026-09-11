<template>
  <SideDrawer
    :show="show"
    :title="t('admin.accounts.requestChain.title')"
    :width="DRAWER_WIDTH"
    @close="emit('close')"
  >
    <template #subtitle>
      <span
        class="block truncate font-mono"
        :title="clientRequestId || ''"
        data-testid="chain-request-id"
      >{{ clientRequestId || '-' }}</span>
    </template>

    <template #actions>
      <button
        type="button"
        class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-dark-300"
        :title="t('admin.accounts.requestChain.refresh')"
        :aria-label="t('admin.accounts.requestChain.refresh')"
        :disabled="loading"
        data-testid="chain-refresh"
        @click="reload"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </template>

    <div class="space-y-4">
      <!-- 首次加载：给个明确的加载态，别白屏 -->
      <div
        v-if="loading && !chain"
        class="flex items-center gap-2 py-8 text-sm text-gray-500 dark:text-gray-400"
        data-testid="chain-loading"
      >
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('common.loading') }}
      </div>

      <!-- 拉不到就说拉不到，并给重试；不要留一个空面板让人以为「没有链路」 -->
      <div
        v-else-if="loadError"
        class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-900/20 dark:text-red-300"
        data-testid="chain-error"
      >
        <span>{{ t('admin.accounts.requestChain.loadFailed') }}</span>
        <button type="button" class="font-medium underline underline-offset-2" data-testid="chain-retry" @click="reload">
          {{ t('admin.accounts.requestChain.retry') }}
        </button>
      </div>

      <template v-else-if="chain">
        <!-- 整体结论放最上面：站长第一眼要看到的是「救回来了没有」，不是撞了几次 -->
        <div
          :class="['flex items-start gap-2.5 rounded-lg border px-3 py-2.5', outcomeBoxClass]"
          data-testid="chain-outcome"
        >
          <Icon :name="isRecovered ? 'checkCircle' : 'xCircle'" size="md" class="mt-px flex-shrink-0" />
          <div class="min-w-0">
            <p class="text-sm font-semibold" data-testid="chain-outcome-title">{{ outcomeTitle }}</p>
            <p class="mt-0.5 text-xs opacity-90" data-testid="chain-outcome-hint">{{ outcomeHint }}</p>
          </div>
        </div>

        <!-- 模型 · 时间 · 流式 -->
        <div
          class="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-gray-500 dark:text-gray-400"
          data-testid="chain-meta"
        >
          <span class="max-w-full truncate font-mono font-medium text-gray-700 dark:text-gray-200" :title="chain.model">{{ chain.model || '-' }}</span>
          <span class="text-gray-300 dark:text-dark-500">·</span>
          <span class="whitespace-nowrap font-mono">{{ formatTime(chain.created_at) }}</span>
          <span class="text-gray-300 dark:text-dark-500">·</span>
          <span class="whitespace-nowrap">{{ chain.stream ? t('admin.accounts.requestChain.stream') : t('admin.accounts.requestChain.nonStream') }}</span>
          <span
            v-if="chain.requested_model && chain.requested_model !== chain.model"
            class="max-w-full truncate font-mono text-gray-400 dark:text-dark-500"
            :title="chain.requested_model"
            data-testid="chain-requested-model"
          >↳ {{ t('admin.accounts.requestChain.requestedModel', { model: chain.requested_model }) }}</span>
        </div>

        <!-- 时间线 -->
        <section class="space-y-1.5" data-testid="chain-timeline">
          <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.requestChain.attempts', { count: attempts.length }) }}
          </h4>

          <ul class="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
            <li
              v-for="(attempt, index) in attempts"
              :key="`${attempt.seq}-${attempt.at}`"
              :class="[ROW_GRID_CLASS, 'px-2 py-1.5 text-xs']"
              data-testid="chain-attempt"
            >
              <span class="font-mono text-gray-400 dark:text-dark-500">{{ seqMark(attempt.seq ?? index + 1) }}</span>
              <span class="whitespace-nowrap font-mono text-gray-500 dark:text-gray-400">{{ formatTime(attempt.at) }}</span>
              <span
                class="min-w-0 truncate text-gray-700 dark:text-gray-200"
                :title="attemptAccountLabel(attempt)"
                data-testid="chain-attempt-account"
              >{{ attemptAccountLabel(attempt) }}</span>
              <!-- 没有上游状态码 ≠ 状态码 0：这次尝试压根没拿到上游响应（连接/凭证阶段就挂了） -->
              <span
                :class="['justify-self-start rounded px-1 font-mono text-[11px] font-medium', isFatalAttempt(index) ? FATAL_STATUS_CLASS : TRANSIENT_STATUS_CLASS]"
                :title="attempt.upstream_status_code == null ? t('admin.accounts.requestChain.noUpstreamStatus') : undefined"
                data-testid="chain-attempt-status"
              >{{ attempt.upstream_status_code ?? '-' }}</span>
              <span
                class="col-span-4 min-w-0 truncate pl-7 text-gray-500 dark:text-gray-400 sm:col-span-1 sm:pl-0"
                :title="attemptMessage(attempt)"
                data-testid="chain-attempt-message"
              >{{ attemptMessage(attempt) }}</span>
            </li>

            <li
              v-if="!attempts.length"
              class="px-2 py-3 text-center text-xs text-gray-400 dark:text-dark-500"
              data-testid="chain-no-attempts"
            >{{ t('admin.accounts.requestChain.noAttempts') }}</li>

            <!-- 收尾：成功绿 / 失败红 / 关联不到成功记录时明说 -->
            <li
              :class="[ROW_GRID_CLASS, 'px-2 py-1.5 text-xs font-medium', finalRowClass]"
              data-testid="chain-final"
            >
              <span class="font-mono">{{ finalMark }}</span>
              <span class="whitespace-nowrap">{{ t('admin.accounts.requestChain.finalLabel') }}</span>
              <span
                v-if="chain.final"
                class="min-w-0 truncate"
                :title="finalAccountLabel"
                data-testid="chain-final-account"
              >{{ finalAccountLabel }}</span>
              <span v-else class="min-w-0 truncate" data-testid="chain-final-missing">{{ t('admin.accounts.requestChain.finalMissing') }}</span>
              <span class="justify-self-start whitespace-nowrap" data-testid="chain-final-status">
                {{ chain.final ? (chain.final.succeeded ? t('admin.accounts.requestChain.finalSuccess') : t('admin.accounts.requestChain.finalFailed')) : '-' }}
              </span>
              <span
                class="col-span-4 min-w-0 truncate pl-7 font-normal opacity-90 sm:col-span-1 sm:pl-0"
                :title="finalMetrics"
                data-testid="chain-final-metrics"
              >{{ finalMetrics }}</span>
            </li>
          </ul>
        </section>
      </template>
    </div>
  </SideDrawer>
</template>

<script setup lang="ts">
/**
 * 请求链路面板：把一个 client_request_id 展开成「撞了哪些账号、每次什么状态码、最后落在谁身上」。
 *
 * 存在的理由：账号列的「上游错误」只会说 `502 ×5`，站长看着像五次事故；实际上网关会重试、
 * 必要时换账号，多数最后是成功的。这里用一条时间线把「五次 502 其实是一次自愈」讲清楚。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getRequestChain } from '@/api/admin/ops'
import type { RequestChain, RequestChainAttempt } from '@/types'
import SideDrawer from '@/components/common/SideDrawer.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatNumberLocaleString, formatTime } from '@/utils/format'

/**
 * 面板宽度（与 AccountLoadDrawer 同一套口径）：
 * - 手机 / 窄屏：96vw，留一点遮罩好点外面关掉，绝不溢出视口
 * - 大屏：至少 760px（时间线 5 列刚好铺开），再宽就跟到 44vw
 * 时间线比负载抽屉窄，因为只有 5 列且没有横向滚动的表格。
 */
const DRAWER_WIDTH = 'min(96vw, max(44vw, 760px))'

/**
 * 时间线列宽：不钉死每一列。
 * - 序号 / 状态码：内容就那么宽，钉最小值收紧
 * - 时间：固定到刚好放下 HH:MM:SS，且 whitespace-nowrap，绝不换行
 * - 账号：给 minmax(7rem, 7fr)，优先保证账号名完整；放不下才 truncate + title
 * - 错误信息：次要信息，拿剩下的 9fr；窄屏干脆掉到第二行整行显示
 */
const ROW_GRID_CLASS = [
  'grid grid-cols-[1.25rem_4.5rem_minmax(0,1fr)_auto] items-baseline gap-x-2 gap-y-0.5',
  'sm:grid-cols-[1.25rem_5rem_minmax(7rem,7fr)_3.25rem_minmax(0,9fr)]'
].join(' ')

// 重试过程里的错误：不刺眼（它们最后被救回来了）
const TRANSIENT_STATUS_CLASS = 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
// 压垮这次请求的那一下：红
const FATAL_STATUS_CLASS = 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'

const CIRCLED_DIGITS = '①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳'

const props = defineProps<{
  show: boolean
  clientRequestId: string | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()

const chain = ref<RequestChain | null>(null)
const loading = ref(false)
const loadError = ref(false)
// 每次拉数递增；关闭面板 / 换请求 ID 后，旧请求回来一律丢弃
let requestSeq = 0

// Go 侧空切片会序列化成 null，统一兜底
const attempts = computed<RequestChainAttempt[]>(() => chain.value?.attempts ?? [])

const isRecovered = computed(() => chain.value?.outcome === 'recovered')

const outcomeBoxClass = computed(() =>
  isRecovered.value
    ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-500/30 dark:bg-emerald-900/20 dark:text-emerald-300'
    : 'border-red-200 bg-red-50 text-red-800 dark:border-red-500/30 dark:bg-red-900/20 dark:text-red-300'
)

const outcomeTitle = computed(() =>
  isRecovered.value
    ? t('admin.accounts.requestChain.outcomeRecovered')
    : t('admin.accounts.requestChain.outcomeFailed')
)

const outcomeHint = computed(() => {
  const current = chain.value
  if (!current) return ''
  const count = attempts.value.length
  if (!isRecovered.value) {
    return t('admin.accounts.requestChain.failedHint', { count, code: current.client_status_code })
  }
  if (current.final) {
    const account = (current.final.account_name ?? '').trim()
      || accountLabel(current.final.account_id, current.final.account_name)
    return t('admin.accounts.requestChain.recoveredHint', { count, account })
  }
  return t('admin.accounts.requestChain.recoveredHintNoFinal', { count })
})

// 最后一条尝试只有在整条链路最终失败时才算「致命」；恢复了的链路整条都是过程噪音
const isFatalAttempt = (index: number): boolean => !isRecovered.value && index === attempts.value.length - 1

const finalRowClass = computed(() => {
  const current = chain.value
  if (current?.final?.succeeded) {
    return 'bg-emerald-50/70 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  if (!current?.final && isRecovered.value) {
    // 说好恢复了却找不到成功记录：数据对不上，用警示色而不是红色，避免误报成事故
    return 'bg-amber-50/70 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  }
  return 'bg-red-50/70 text-red-700 dark:bg-red-900/20 dark:text-red-300'
})

const finalMark = computed(() => {
  const current = chain.value
  if (!current?.final) return '!'
  return current.final.succeeded ? '✓' : '✗'
})

// account_id / account_name 在后端都是 omitempty，关联不到账号时会整个缺席，不能直接拼串
const accountLabel = (accountId?: number | null, accountName?: string | null): string => {
  const name = (accountName ?? '').trim()
  if (!accountId) return name || '-'
  if (!name) return t('admin.accounts.requestChain.accountNoName', { id: accountId })
  return t('admin.accounts.requestChain.account', { id: accountId, name })
}

const finalAccountLabel = computed(() => {
  const final = chain.value?.final
  if (!final) return ''
  return accountLabel(final.account_id, final.account_name)
})

const finalMetrics = computed(() => {
  const final = chain.value?.final
  if (!final) return ''
  const parts: string[] = []
  if (typeof final.time_to_first_token_ms === 'number') {
    parts.push(t('admin.accounts.requestChain.firstToken', { value: formatMs(final.time_to_first_token_ms) }))
  }
  if (typeof final.total_tokens === 'number') {
    parts.push(t('admin.accounts.requestChain.tokens', { value: formatNumberLocaleString(final.total_tokens) }))
  }
  if (typeof final.cost === 'number') {
    parts.push(formatCost(final.cost))
  }
  return parts.join(' · ')
})

const attemptAccountLabel = (attempt: RequestChainAttempt): string =>
  accountLabel(attempt.account_id, attempt.account_name)

// kind 是网关给的归类（overloaded / rate_limit …），message 才是上游原话；优先原话
const attemptMessage = (attempt: RequestChainAttempt): string => attempt.message || attempt.kind || '-'

const seqMark = (seq: number): string => {
  if (Number.isInteger(seq) && seq >= 1 && seq <= CIRCLED_DIGITS.length) {
    return CIRCLED_DIGITS[seq - 1]
  }
  return String(seq)
}

const formatMs = (ms: number | null | undefined): string => {
  if (ms === null || ms === undefined) return '-'
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.round(ms)}ms`
}

const formatCost = (value: number | null | undefined): string => {
  if (value === null || value === undefined) return '-'
  if (value === 0) return '$0'
  return `$${value.toFixed(value >= 0.01 ? 4 : 6)}`
}

const load = async () => {
  const id = props.clientRequestId
  if (!props.show || !id) return

  const seq = ++requestSeq
  loading.value = true
  loadError.value = false
  try {
    const res = await getRequestChain(id)
    if (seq !== requestSeq) return
    chain.value = res
  } catch {
    if (seq !== requestSeq) return
    loadError.value = true
    chain.value = null
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

const reload = () => {
  void load()
}

watch(
  () => [props.show, props.clientRequestId] as const,
  ([isOpen, id], previous) => {
    const [wasOpen, previousId] = previous ?? [false, null]
    if (!isOpen) {
      // 关掉就作废在途请求，免得回来覆盖下一次打开的内容
      requestSeq++
      loading.value = false
      return
    }
    if (isOpen === wasOpen && id === previousId) return
    chain.value = null
    loadError.value = false
    void load()
  },
  { immediate: true }
)
</script>
