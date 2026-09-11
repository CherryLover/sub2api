<template>
  <SideDrawer
    :show="show"
    :title="t('admin.accounts.capacity.load.title')"
    :width="DRAWER_WIDTH"
    @close="emit('close')"
  >
    <template #subtitle>
      <span v-if="account" class="inline-flex min-w-0 items-center gap-1.5" data-testid="load-drawer-account">
        <PlatformIcon :platform="account.platform" size="xs" />
        <span class="truncate font-medium text-gray-700 dark:text-gray-200">{{ account.name }}</span>
        <span class="flex-shrink-0 text-gray-400 dark:text-gray-500">#{{ account.id }} · {{ account.platform }}</span>
      </span>
    </template>

    <template #actions>
      <button
        type="button"
        class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-dark-300"
        :title="t('admin.accounts.capacity.load.refresh')"
        :aria-label="t('admin.accounts.capacity.load.refresh')"
        :disabled="loading"
        data-testid="load-drawer-refresh"
        @click="reload"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </template>

    <div class="space-y-4">
      <!-- 延迟提示 -->
      <p class="text-[11px] text-gray-400 dark:text-gray-500" data-testid="load-drawer-delay-hint">
        {{ t('admin.accounts.capacity.load.delayHint') }}
      </p>

      <!-- 头部：并发 / 等待 / 窗口内总请求 + 窗口切换 -->
      <div class="flex flex-wrap items-center gap-2">
        <span
          :class="['inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium', concurrencyClass]"
          data-testid="load-drawer-concurrency"
        >
          {{ t('admin.accounts.capacity.load.concurrency') }}
          <span class="font-mono">{{ currentConcurrency }}</span>
          <span class="opacity-60">/</span>
          <span class="font-mono">{{ maxConcurrency }}</span>
        </span>
        <span
          class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
          data-testid="load-drawer-waiting"
        >
          {{ t('admin.accounts.capacity.load.waiting') }}
          <span class="font-mono">{{ data?.waiting_count ?? 0 }}</span>
        </span>
        <span class="text-xs text-gray-500 dark:text-gray-400" data-testid="load-drawer-total">
          {{ t('admin.accounts.capacity.load.totalRequests', { count: data?.total_requests ?? 0 }) }}
        </span>

        <div class="ml-auto inline-flex overflow-hidden rounded-lg border border-gray-200 text-xs dark:border-dark-600" role="group">
          <button
            v-for="minutes in WINDOW_OPTIONS"
            :key="minutes"
            type="button"
            :class="[
              'px-2.5 py-1 transition-colors',
              windowMinutes === minutes
                ? 'bg-primary-500 font-medium text-white'
                : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'
            ]"
            :aria-pressed="windowMinutes === minutes"
            :data-testid="`load-window-${minutes}`"
            @click="setWindow(minutes)"
          >
            {{ t('admin.accounts.capacity.load.windowMinutes', { minutes }) }}
          </button>
        </div>
      </div>

      <!-- 主数据读取失败：给重试，不影响下方错误段 -->
      <div
        v-if="loadError"
        class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-900/20 dark:text-red-300"
        data-testid="load-drawer-error"
      >
        <span>{{ t('admin.accounts.capacity.load.loadFailed') }}</span>
        <button type="button" class="font-medium underline underline-offset-2" @click="reload">
          {{ t('admin.accounts.capacity.load.retry') }}
        </button>
      </div>

      <div v-else-if="loading && !data" class="flex items-center gap-2 py-6 text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t('common.loading') }}
      </div>

      <template v-else-if="data">
        <!-- 按密钥 / 按模型 chips -->
        <section class="space-y-1.5" data-testid="load-drawer-by-key">
          <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.capacity.load.byApiKey') }}
          </h4>
          <div v-if="byApiKey.length" class="flex flex-wrap gap-1.5">
            <span
              v-for="item in byApiKey"
              :key="item.api_key_id"
              class="inline-flex items-center gap-1 rounded-full bg-sky-50 px-2.5 py-0.5 text-xs text-sky-700 ring-1 ring-inset ring-sky-200 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-500/30"
              data-testid="load-chip-key"
            >
              <span class="font-medium">{{ item.name || `#${item.api_key_id}` }}</span>
              <span class="opacity-70">{{ t('admin.accounts.capacity.load.count', { count: item.count }) }}</span>
              <span class="font-mono opacity-70">{{ formatCost(item.cost) }}</span>
            </span>
          </div>
          <p v-else class="text-xs text-gray-400 dark:text-gray-500">-</p>
        </section>

        <section class="space-y-1.5" data-testid="load-drawer-by-model">
          <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.capacity.load.byModel') }}
          </h4>
          <div v-if="byModel.length" class="flex flex-wrap gap-1.5">
            <span
              v-for="item in byModel"
              :key="item.model"
              class="inline-flex items-center gap-1 rounded-full bg-violet-50 px-2.5 py-0.5 text-xs text-violet-700 ring-1 ring-inset ring-violet-200 dark:bg-violet-900/20 dark:text-violet-300 dark:ring-violet-500/30"
              data-testid="load-chip-model"
            >
              <span class="font-medium">{{ item.model }}</span>
              <span class="opacity-70">{{ t('admin.accounts.capacity.load.count', { count: item.count }) }}</span>
              <span class="font-mono opacity-70">{{ formatCost(item.cost) }}</span>
            </span>
          </div>
          <p v-else class="text-xs text-gray-400 dark:text-gray-500">-</p>
        </section>

        <!-- 请求明细表 -->
        <section class="space-y-1.5" data-testid="load-drawer-requests">
          <div class="flex items-baseline justify-between">
            <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.capacity.load.requests') }}
            </h4>
            <span v-if="data.total_requests > requestItems.length" class="text-[11px] text-gray-400 dark:text-gray-500">
              {{ t('admin.accounts.capacity.load.requestsLimitHint', { limit: requestItems.length }) }}
            </span>
          </div>
          <!--
            列宽：table-fixed + 主次分明的分配，不再每列钉死像素。
            - 真正定长的列给固定 px：时间 / 耗时 / 首字 / tokens / 费用（内容宽度可预估，多给也是浪费）
            - 次要文本列给小百分比：用户 / 密钥（长邮箱、长密钥名直接截断，悬停 title 看全名）
            - 模型列不写宽度 => 吃掉全部剩余空间，屏幕越宽它越宽，只有极端超长才截断
            - min-w：窄屏下不让模型列被压没，宁可让表格在卡片内横向滚（费用列 sticky 不会被裁）；
              ≥sm 会多出用户 / 首字两列，所以下限跟着抬到 800px
          -->
          <div v-if="requestItems.length" class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
            <table class="w-full min-w-[640px] table-fixed text-xs sm:min-w-[800px]">
              <thead class="bg-gray-50 text-left text-[11px] uppercase tracking-wide text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                <tr>
                  <th class="w-[72px] whitespace-nowrap px-1.5 py-1.5 font-medium">{{ t('admin.accounts.capacity.load.columns.time') }}</th>
                  <th class="hidden w-[12%] px-1.5 py-1.5 font-medium sm:table-cell">{{ t('admin.accounts.capacity.load.columns.user') }}</th>
                  <th class="w-[10%] px-1.5 py-1.5 font-medium">{{ t('admin.accounts.capacity.load.columns.apiKey') }}</th>
                  <th class="whitespace-nowrap px-1.5 py-1.5 font-medium">{{ t('admin.accounts.capacity.load.columns.model') }}</th>
                  <th class="w-[74px] px-1.5 py-1.5 text-right font-medium">{{ t('admin.accounts.capacity.load.columns.duration') }}</th>
                  <th class="hidden w-[74px] px-1.5 py-1.5 text-right font-medium sm:table-cell">{{ t('admin.accounts.capacity.load.columns.firstToken') }}</th>
                  <th class="w-[140px] whitespace-nowrap px-1.5 py-1.5 text-right font-medium">{{ t('admin.accounts.capacity.load.columns.tokens') }}</th>
                  <th class="sticky right-0 w-[88px] whitespace-nowrap bg-gray-50 py-1.5 pl-1.5 pr-3 text-right font-medium dark:bg-dark-700">{{ t('admin.accounts.capacity.load.columns.cost') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in requestItems" :key="item.id" class="text-gray-700 dark:text-gray-200" data-testid="load-request-row">
                  <td class="whitespace-nowrap px-1.5 py-1.5 font-mono text-gray-500 dark:text-gray-400">{{ formatTime(item.created_at) }}</td>
                  <td class="hidden px-1.5 py-1.5 sm:table-cell">
                    <div class="truncate" :title="item.user?.email || ''">{{ item.user?.email || '-' }}</div>
                  </td>
                  <td class="px-1.5 py-1.5">
                    <div class="truncate" :title="item.api_key?.name || ''">{{ item.api_key?.name || '-' }}</div>
                  </td>
                  <td class="px-1.5 py-1.5" data-testid="load-request-model">
                    <div class="flex items-center gap-1">
                      <span class="truncate" :title="item.model">{{ item.model }}</span>
                      <span
                        v-if="item.stream"
                        class="flex-shrink-0 rounded bg-gray-100 px-1 text-[9px] uppercase text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                      >stream</span>
                    </div>
                    <div
                      v-if="item.upstream_model && item.upstream_model !== item.model"
                      class="truncate text-[11px] text-gray-400 dark:text-gray-500"
                      :title="item.upstream_model"
                    >
                      ↳ {{ item.upstream_model }}
                    </div>
                  </td>
                  <td class="whitespace-nowrap px-1.5 py-1.5 text-right font-mono">{{ formatMs(item.duration_ms) }}</td>
                  <td class="hidden whitespace-nowrap px-1.5 py-1.5 text-right font-mono sm:table-cell">{{ formatMs(item.first_token_ms) }}</td>
                  <!-- 输入/输出各自不换行；缓存命中放不下时才折到第二行，不为了极端值把整列钉宽（两段中间的空格是换行点，别删） -->
                  <td class="px-1.5 py-1.5 text-right font-mono">
                    <span class="whitespace-nowrap">{{ formatNumber(item.input_tokens) }} / {{ formatNumber(item.output_tokens) }}</span> <span v-if="item.cache_read_tokens" class="whitespace-nowrap text-gray-400 dark:text-gray-500">(+{{ formatNumber(item.cache_read_tokens) }})</span>
                  </td>
                  <td
                    class="sticky right-0 whitespace-nowrap bg-white py-1.5 pl-1.5 pr-3 text-right font-mono dark:bg-dark-800"
                    data-testid="load-request-cost"
                  >{{ formatCost(item.actual_cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-center text-xs text-gray-400 dark:border-dark-700 dark:text-gray-500" data-testid="load-drawer-requests-empty">
            {{ t('admin.accounts.capacity.load.emptyRequests') }}
          </p>
        </section>
      </template>

      <!-- 同期错误：独立读取，失败只提示不阻断 -->
      <section class="space-y-1.5" data-testid="load-drawer-errors">
        <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.capacity.load.errors') }}
          <span v-if="errorLogs.length" class="ml-1 rounded-full bg-red-100 px-1.5 py-px text-[10px] font-medium text-red-600 dark:bg-red-900/30 dark:text-red-400">{{ errorLogs.length }}</span>
        </h4>
        <p v-if="errorLogsFailed" class="text-xs text-amber-600 dark:text-amber-400" data-testid="load-drawer-errors-failed">
          {{ t('admin.accounts.capacity.load.errorsFailed') }}
        </p>
        <p v-else-if="errorLogsLoading && !errorLogs.length" class="text-xs text-gray-400 dark:text-gray-500">
          {{ t('common.loading') }}
        </p>
        <ul v-else-if="errorLogs.length" class="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
          <li v-for="log in errorLogs" :key="log.id" class="flex items-start gap-2 px-2 py-1.5 text-xs" data-testid="load-error-row">
            <span class="whitespace-nowrap font-mono text-gray-500 dark:text-gray-400">{{ formatTime(log.created_at) }}</span>
            <span class="flex-shrink-0 rounded bg-red-100 px-1.5 font-mono text-[11px] text-red-700 dark:bg-red-900/30 dark:text-red-300">{{ log.status_code || '-' }}</span>
            <span v-if="log.phase || log.type" class="flex-shrink-0 text-gray-400 dark:text-gray-500">{{ log.phase }}<template v-if="log.type">/{{ log.type }}</template></span>
            <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-gray-200" :title="log.message">{{ log.message || '-' }}</span>
          </li>
        </ul>
        <p v-else class="text-xs text-gray-400 dark:text-gray-500" data-testid="load-drawer-errors-empty">
          {{ t('admin.accounts.capacity.load.errorsEmpty') }}
        </p>
      </section>
    </div>
  </SideDrawer>
</template>

<script setup lang="ts">
/**
 * 账号容量负载抽屉（甲方案）：并发数只是一个数字，这里把它展开成
 * 「最近 N 分钟这个账号在跑什么」——按密钥 / 按模型的聚合、逐条请求、以及同期错误。
 * 数据来自使用记录（usage_logs），只记成功请求，故有秒级延迟并且要单独拉错误日志。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { listErrorLogs } from '@/api/admin/ops'
import type { OpsErrorLog } from '@/api/admin/ops'
import type { AccountRecentRequestsResponse } from '@/api/admin/accounts'
import type { Account } from '@/types'
import SideDrawer from '@/components/common/SideDrawer.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatNumber, formatTime } from '@/utils/format'

/**
 * 抽屉宽度：请求明细是 8 列表格，780px 下模型列会被挤成竖排、费用列被裁掉，所以整体放宽。
 * - 手机 / 窄屏：96vw（留一点遮罩好点外面关掉），不会超出视口
 * - 中等屏：仍是 96vw 到 960px 之间，随屏幕加宽
 * - 大屏：至少 960px（实测刚好放下 8 列不横向滚动）；视口再宽就跟到 50vw，≥1920 时正好半屏
 */
const DRAWER_WIDTH = 'min(96vw, max(50vw, 960px))'

const WINDOW_OPTIONS = [5, 15, 60] as const
const DEFAULT_WINDOW_MINUTES = 15
const REQUEST_LIMIT = 50
const ERROR_LIMIT = 20

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()

const windowMinutes = ref<number>(DEFAULT_WINDOW_MINUTES)
const loading = ref(false)
const loadError = ref(false)
const data = ref<AccountRecentRequestsResponse | null>(null)
const errorLogs = ref<OpsErrorLog[]>([])
const errorLogsLoading = ref(false)
const errorLogsFailed = ref(false)
// 每次拉数递增；关闭抽屉 / 切换账号 / 切换窗口后，旧请求回来一律丢弃
let requestSeq = 0

// Go 侧空切片可能序列化成 null，这里统一兜底成空数组
const byApiKey = computed(() => data.value?.by_api_key ?? [])
const byModel = computed(() => data.value?.by_model ?? [])
const requestItems = computed(() => data.value?.items ?? [])

const currentConcurrency = computed(() => data.value?.current_concurrency ?? props.account?.current_concurrency ?? 0)
const maxConcurrency = computed(() => data.value?.max_concurrency ?? props.account?.concurrency ?? 0)

const concurrencyClass = computed(() => {
  const current = currentConcurrency.value
  const max = maxConcurrency.value
  if (max > 0 && current >= max) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (current > 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})

const formatMs = (ms: number | null | undefined): string => {
  if (ms === null || ms === undefined) return '-'
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`
  return `${Math.round(ms)}ms`
}

const formatCost = (value: number | null | undefined): string => {
  if (value === null || value === undefined) return '-'
  if (value === 0) return '$0'
  return `$${value.toFixed(value >= 0.01 ? 4 : 6)}`
}

const loadRecentRequests = async (accountId: number, minutes: number, seq: number) => {
  loading.value = true
  loadError.value = false
  try {
    const res = await adminAPI.accounts.getRecentRequests(accountId, { minutes, limit: REQUEST_LIMIT })
    if (seq !== requestSeq) return
    data.value = res
  } catch {
    if (seq !== requestSeq) return
    loadError.value = true
    data.value = null
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

// 同期错误：/admin/ops/errors 的 time_range 只认 5m/30m/1h/…（没有 15m，传了会静默退回 1h），
// 所以这里按窗口显式给 start_time / end_time，和上面的请求窗口严格同期。
const loadWindowErrors = async (accountId: number, minutes: number, seq: number) => {
  errorLogsLoading.value = true
  errorLogsFailed.value = false
  const end = new Date()
  const start = new Date(end.getTime() - minutes * 60 * 1000)
  try {
    const res = await listErrorLogs({
      account_id: accountId,
      view: 'all',
      page: 1,
      page_size: ERROR_LIMIT,
      start_time: start.toISOString(),
      end_time: end.toISOString(),
      sort_by: 'created_at',
      sort_order: 'desc'
    })
    if (seq !== requestSeq) return
    errorLogs.value = (res.items ?? []).slice(0, ERROR_LIMIT)
  } catch {
    if (seq !== requestSeq) return
    errorLogsFailed.value = true
    errorLogs.value = []
  } finally {
    if (seq === requestSeq) errorLogsLoading.value = false
  }
}

const load = async () => {
  const account = props.account
  if (!props.show || !account) return
  const seq = ++requestSeq
  await Promise.all([
    loadRecentRequests(account.id, windowMinutes.value, seq),
    loadWindowErrors(account.id, windowMinutes.value, seq)
  ])
}

const reload = () => {
  void load()
}

const setWindow = (minutes: number) => {
  if (windowMinutes.value === minutes) return
  windowMinutes.value = minutes
  void load()
}

watch(
  () => [props.show, props.account?.id] as const,
  ([show]) => {
    if (show && props.account) {
      windowMinutes.value = DEFAULT_WINDOW_MINUTES
      data.value = null
      errorLogs.value = []
      loadError.value = false
      errorLogsFailed.value = false
      void load()
    } else {
      // 关闭后让在途请求作废
      requestSeq += 1
      loading.value = false
      errorLogsLoading.value = false
    }
  },
  { immediate: true }
)

defineExpose({ windowMinutes, reload })
</script>
